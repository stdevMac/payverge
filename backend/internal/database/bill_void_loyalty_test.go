package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupVoidLoyaltyTestDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	db = gormDB
	require.NoError(t, db.AutoMigrate(
		&Business{},
		&Bill{},
		&BillHistoryEvent{},
		&CustomerBusiness{},
		&Order{},
		&OperationalAlert{},
		&OperationalAlertEvent{},
	))
}

// TestVoidBillRestoresLoyaltyPoints proves that voiding an unpaid bill that
// carries a loyalty redemption returns the spent points to the customer. The
// bill never converted to a payment (VoidBill refuses paid bills), so the points
// the guest spent must come back — otherwise they lose points for nothing.
func TestVoidBillRestoresLoyaltyPoints(t *testing.T) {
	setupVoidLoyaltyTestDB(t)
	const businessID, customerID uint = 1, 1

	// Customer redeemed 500 points (balance already drawn down to 20).
	require.NoError(t, db.Create(&CustomerBusiness{
		CustomerID:    customerID,
		BusinessID:    businessID,
		LoyaltyPoints: 20,
		IsActive:      true,
	}).Error)

	redeemer := customerID
	bill := &Bill{
		BusinessID:                  businessID,
		BillNumber:                  "VOID-LOY-1",
		Status:                      BillStatusOpen,
		Items:                       "[]",
		PaidAmount:                  0,
		LoyaltyDiscountCents:        500,
		LoyaltyPointsRedeemed:       500,
		LoyaltyRedeemedByCustomerID: &redeemer,
	}
	require.NoError(t, db.Create(bill).Error)

	_, err := VoidBill(bill.ID, "staff", "guest left without paying")
	require.NoError(t, err)

	var cb CustomerBusiness
	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", customerID, businessID).First(&cb).Error)
	assert.Equal(t, 520, cb.LoyaltyPoints, "voiding a redeemed bill must return the 500 spent points (20 + 500)")
}

// TestVoidBillNoRedemptionLeavesPointsUntouched confirms the common path (a bill
// with no redemption) voids cleanly and does not touch any loyalty balance.
func TestVoidBillNoRedemptionLeavesPointsUntouched(t *testing.T) {
	setupVoidLoyaltyTestDB(t)
	const businessID, customerID uint = 1, 1

	require.NoError(t, db.Create(&CustomerBusiness{
		CustomerID:    customerID,
		BusinessID:    businessID,
		LoyaltyPoints: 100,
		IsActive:      true,
	}).Error)

	bill := &Bill{
		BusinessID: businessID,
		BillNumber: "VOID-NOLOY-1",
		Status:     BillStatusOpen,
		Items:      "[]",
		PaidAmount: 0,
	}
	require.NoError(t, db.Create(bill).Error)

	_, err := VoidBill(bill.ID, "staff", "duplicate bill")
	require.NoError(t, err)

	var cb CustomerBusiness
	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", customerID, businessID).First(&cb).Error)
	assert.Equal(t, 100, cb.LoyaltyPoints, "a bill with no redemption must not change loyalty points")
}

// TestVoidBillAlreadyUndoneRedemptionDoesNotDoubleRefund proves that if the
// redemption was already undone (points returned, fields cleared) before the
// void, voiding does not credit the points a second time.
func TestVoidBillAlreadyUndoneRedemptionDoesNotDoubleRefund(t *testing.T) {
	setupVoidLoyaltyTestDB(t)
	const businessID, customerID uint = 1, 1

	require.NoError(t, db.Create(&CustomerBusiness{
		CustomerID:    customerID,
		BusinessID:    businessID,
		LoyaltyPoints: 500, // already restored by a prior undo
		IsActive:      true,
	}).Error)

	bill := &Bill{
		BusinessID:            businessID,
		BillNumber:            "VOID-UNDONE-1",
		Status:                BillStatusOpen,
		Items:                 "[]",
		LoyaltyPointsRedeemed: 0, // cleared by undo
	}
	require.NoError(t, db.Create(bill).Error)

	_, err := VoidBill(bill.ID, "staff", "changed mind")
	require.NoError(t, err)

	var cb CustomerBusiness
	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", customerID, businessID).First(&cb).Error)
	assert.Equal(t, 500, cb.LoyaltyPoints, "must not double-refund points after an undo")
}
