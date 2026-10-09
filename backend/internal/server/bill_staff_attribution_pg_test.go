//go:build integration
// +build integration

package server_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// TestStampBillClosedByStaff_PostgresIntegration exercises the bill-stamp
// path against a real Postgres instance. The default unit suite uses SQLite,
// which doesn't expose Postgres-specific behavior (identifier case handling,
// COALESCE typing, row-locking semantics). This test skips automatically
// when TEST_DATABASE_URL is unset so it stays a no-op in dev environments.
//
// Intended to run in CI via a Postgres service container; locally it can be
// exercised with:
//
//	TEST_DATABASE_URL="postgres://test:test@localhost:5432/test?sslmode=disable" \
//	  go test -tags integration ./internal/server/... -run PostgresIntegration -v
func TestStampBillClosedByStaff_PostgresIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	}

	gin.SetMode(gin.TestMode)

	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "open postgres")

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Ping(), "ping postgres")

	// Register the connection with the package-level handle so the helper
	// picks it up through database.GetDB().
	database.SetTestDB(gormDB)

	// Minimal AutoMigrate — only the tables the helper touches and their
	// foreign-key dependencies. We intentionally do NOT run the full
	// autoMigrate() so unrelated schema work doesn't leak into the test DB.
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.Table{},
		&database.Bill{},
	), "automigrate bills/business/staff/tables")

	// Seed a business so the bill's foreign key is satisfied. Use a unique
	// business_id so repeated runs against a warm DB don't collide.
	biz := &database.Business{
		BusinessId:     fmt.Sprintf("pg-stamp-%d", time.Now().UnixNano()),
		OwnerAddress:   "0x0000000000000000000000000000000000000001",
		Name:           "PG Stamp Test Biz",
		SettlementAddr: "0x0000000000000000000000000000000000000002",
		TippingAddr:    "0x0000000000000000000000000000000000000003",
	}
	require.NoError(t, gormDB.Create(biz).Error, "create business")
	table := &database.Table{
		BusinessID: biz.ID,
		TableCode:  fmt.Sprintf("pg-stamp-table-%d", time.Now().UnixNano()),
		Name:       "PG Stamp Table",
		Capacity:   2,
		IsActive:   true,
	}
	require.NoError(t, gormDB.Create(table).Error, "create table")
	staff := &database.Staff{
		BusinessID: biz.ID,
		Name:       "PG Stamp Staff",
		Email:      fmt.Sprintf("pg-stamp-staff-%d@payverge.test", time.Now().UnixNano()),
		Role:       database.StaffRoleManager,
		IsActive:   true,
	}
	require.NoError(t, gormDB.Create(staff).Error, "create staff")

	// Teardown: clean up the rows we inserted so the test is idempotent
	// across runs against a persistent database.
	var billID uint
	t.Cleanup(func() {
		if billID != 0 {
			gormDB.Exec("DELETE FROM bills WHERE id = ?", billID)
		}
		gormDB.Exec("DELETE FROM staff WHERE id = ?", staff.ID)
		gormDB.Exec("DELETE FROM tables WHERE id = ?", table.ID)
		gormDB.Exec("DELETE FROM businesses WHERE id = ?", biz.ID)
	})

	// Insert a closed bill with ClosedByStaffID nil — the exact shape that
	// the stamp helper is designed to update.
	bill := &database.Bill{
		BusinessID:  biz.ID,
		TableID:     table.ID,
		BillNumber:  fmt.Sprintf("PG-STAMP-%d", time.Now().UnixNano()),
		TotalAmount: 50,
		PaidAmount:  50,
		Status:      database.BillStatusClosed,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, gormDB.Create(bill).Error, "create bill")
	billID = bill.ID

	// Synthesize a gin context authenticated as staff.
	c, _ := gin.CreateTestContext(nil)
	c.Set("token_type", "staff")
	c.Set("staff_id", staff.ID)

	// Exercise: run the stamp against real Postgres. This is the path that
	// validates `UPDATE ... SET closed_by_staff_id = ?, closed_at = COALESCE(...)`
	// compiles and executes correctly under pgx/lib-pq semantics.
	server.StampBillClosedByStaff(c, bill.ID)

	// Assert: reload and confirm the staff ID and closed_at are stamped.
	var reloaded database.Bill
	require.NoError(t, gormDB.First(&reloaded, bill.ID).Error, "reload bill")
	require.NotNil(t, reloaded.ClosedByStaffID, "closed_by_staff_id must be stamped")
	assert.Equal(t, staff.ID, *reloaded.ClosedByStaffID)
	require.NotNil(t, reloaded.ClosedAt, "closed_at must be stamped when previously null")
}
