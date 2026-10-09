package server

import (
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupBillStampTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.Bill{},
	))
	return gormDB
}

func newBillStampTestContext(staffID uint, tokenType string) *gin.Context {
	c, _ := gin.CreateTestContext(nil)
	if tokenType != "" {
		c.Set("token_type", tokenType)
	}
	if staffID != 0 {
		c.Set("staff_id", staffID)
	}
	return c
}

func createBillStampTestBill(t *testing.T, status database.BillStatus) *database.Bill {
	t.Helper()
	bill := &database.Bill{
		BusinessID:  1,
		BillNumber:  fmt.Sprintf("BS-%d", time.Now().UnixNano()),
		TotalAmount: 50,
		PaidAmount:  50,
		Status:      status,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	return bill
}

// TestStampBillClosedByStaff_DoesNotStampPaidBill verifies the critical
// misattribution guard: the helper MUST NOT stamp paid bills. Paid-path
// attribution is now owned by the service layer (applyBillPaymentAmounts),
// where the staff ID is written atomically with the status transition.
//
// Regression guard: prior to this fix, a bill auto-closed to "paid" by a
// Web3 webhook (no staff) could be silently claimed by any staff member who
// later hit any endpoint that called this helper. Restricting the stamp to
// status=closed eliminates that vector.
func TestStampBillClosedByStaff_DoesNotStampPaidBill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupBillStampTestDB(t)

	bill := createBillStampTestBill(t, database.BillStatusPaid)
	c := newBillStampTestContext(7, "staff")

	StampBillClosedByStaff(c, bill.ID)

	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	assert.Nil(t, reloaded.ClosedByStaffID, "paid bills must not be stamped by the helper — paid-path attribution is service-layer only")
}

// TestStampBillClosedByStaff_DoesNotStampAfterPaidWithoutStaff is the
// explicit regression test for the misattribution scenario: a paid bill with
// no staff attribution (auto-closed by a webhook) must stay that way even
// when a staff member later invokes an endpoint that calls the helper.
func TestStampBillClosedByStaff_DoesNotStampAfterPaidWithoutStaff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupBillStampTestDB(t)

	// Simulate the exact scenario: Web3 webhook auto-closed the bill to
	// "paid" without staff context. ClosedByStaffID is nil.
	bill := createBillStampTestBill(t, database.BillStatusPaid)

	// An unrelated staff member later hits an endpoint that invokes the
	// helper. Under the old logic this would have stamped them as the
	// closer — under the new logic the helper is a no-op for paid bills.
	c := newBillStampTestContext(123, "staff")
	StampBillClosedByStaff(c, bill.ID)

	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	assert.Nil(t, reloaded.ClosedByStaffID, "staff who did not drive the payment must NOT be attributed as closer")
}

// TestStampBillClosedByStaff_NoOpForOpenBill ensures partial payments that
// leave the bill "open" do NOT prematurely attribute closure. Only a
// transition to paid/closed should trigger the stamp.
func TestStampBillClosedByStaff_NoOpForOpenBill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupBillStampTestDB(t)

	bill := createBillStampTestBill(t, database.BillStatusOpen)
	c := newBillStampTestContext(7, "staff")

	StampBillClosedByStaff(c, bill.ID)

	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	assert.Nil(t, reloaded.ClosedByStaffID, "open bills must not be stamped")
	assert.Nil(t, reloaded.ClosedAt)
}

// TestStampBillClosedByStaff_OwnerLeavesFieldNil encodes the intended
// semantic: owners closing their own bills do NOT get staff attribution.
// The analytics today.by_current_staff aggregate is explicitly a staff
// metric.
func TestStampBillClosedByStaff_OwnerLeavesFieldNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupBillStampTestDB(t)

	bill := createBillStampTestBill(t, database.BillStatusClosed)

	// token_type = "user" represents OAuth owners; "web3" represents wallet
	// owners. Both should leave the field nil.
	for _, tokenType := range []string{"user", "web3", ""} {
		c := newBillStampTestContext(42, tokenType)
		StampBillClosedByStaff(c, bill.ID)
	}

	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	assert.Nil(t, reloaded.ClosedByStaffID, "owner-initiated closures must leave attribution nil")
}

// TestStampBillClosedByStaff_FirstStampWinsRace simulates concurrent
// promotions: two staff members each try to stamp the same bill. The
// conditional WHERE (closed_by_staff_id IS NULL) must ensure the first wins
// and subsequent calls are no-ops.
func TestStampBillClosedByStaff_FirstStampWinsRace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupBillStampTestDB(t)

	bill := createBillStampTestBill(t, database.BillStatusClosed)

	firstCtx := newBillStampTestContext(11, "staff")
	StampBillClosedByStaff(firstCtx, bill.ID)

	secondCtx := newBillStampTestContext(22, "staff")
	StampBillClosedByStaff(secondCtx, bill.ID)

	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	require.NotNil(t, reloaded.ClosedByStaffID)
	assert.Equal(t, uint(11), *reloaded.ClosedByStaffID, "first staff stamp wins; racing stamps are no-ops")
}

// TestStampBillClosedByStaff_StampsClosedBillsToo covers the CloseBill path
// where the bill status transitions directly to "closed" (unpaid closure)
// rather than "paid". Both terminal states must be attributable.
func TestStampBillClosedByStaff_StampsClosedBillsToo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupBillStampTestDB(t)

	bill := createBillStampTestBill(t, database.BillStatusClosed)
	c := newBillStampTestContext(99, "staff")

	StampBillClosedByStaff(c, bill.ID)

	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	require.NotNil(t, reloaded.ClosedByStaffID)
	assert.Equal(t, uint(99), *reloaded.ClosedByStaffID)
}

// TestStampBillClosedByStaff_PreservesExistingClosedAt verifies that if a
// bill was already closed (closed_at set) and then a subsequent stamp call
// runs, we never clobber the original closure timestamp.
func TestStampBillClosedByStaff_PreservesExistingClosedAt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupBillStampTestDB(t)

	bill := createBillStampTestBill(t, database.BillStatusClosed)
	originalClosedAt := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, database.GetDB().Model(&database.Bill{}).Where("id = ?", bill.ID).
		Update("closed_at", &originalClosedAt).Error)

	c := newBillStampTestContext(7, "staff")
	StampBillClosedByStaff(c, bill.ID)

	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	require.NotNil(t, reloaded.ClosedByStaffID)
	assert.Equal(t, uint(7), *reloaded.ClosedByStaffID)
	require.NotNil(t, reloaded.ClosedAt)
	assert.Equal(t, originalClosedAt.Unix(), reloaded.ClosedAt.Unix(), "existing closed_at must not be overwritten")
}
