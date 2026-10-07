package print

import (
	"fmt"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// setupTestDB creates an isolated in-memory SQLite database for a single test.
// It auto-migrates Business and Printer so the router tests have a real schema.
// Reused by Task 3.5.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Use t.Name() in the DSN so each test gets its own in-memory database.
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&database.Business{},
		&database.Printer{},
		&database.Table{},
		&database.Payment{},
		&database.Bill{},
		&database.PrintJob{},
		&database.Order{},
	); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}
	return db
}

// mustCreateBusiness inserts a minimal valid Business row.
//
// NOT NULL fields that must be set (discovered from database.Business struct tags):
//   - BusinessId  (uniqueIndex;not null)  — use t.Name() for uniqueness
//   - OwnerAddress (index;not null)
//   - Name         (not null)
//   - SettlementAddr (not null)
//   - TippingAddr    (not null)
//
// Reused by Task 3.5.
func mustCreateBusiness(t *testing.T, db *gorm.DB) *database.Business {
	t.Helper()
	b := &database.Business{
		BusinessId:     fmt.Sprintf("biz-%s", t.Name()),
		OwnerAddress:   "0x0000000000000000000000000000000000000001",
		Name:           "Test Bistro",
		SettlementAddr: "0x0000000000000000000000000000000000000002",
		TippingAddr:    "0x0000000000000000000000000000000000000003",
	}
	if err := db.Create(b).Error; err != nil {
		t.Fatalf("create business: %v", err)
	}
	return b
}

func TestRouter_PicksEnabledPrinterByRole(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)

	locID := uint(7)
	disabled := database.Printer{
		BusinessID: business.ID, LocationID: &locID, Name: "off",
		Role: "bill", Transport: "browser", Enabled: false,
	}
	enabled := database.Printer{
		BusinessID: business.ID, LocationID: &locID, Name: "on",
		Role: "bill", Transport: "browser", Enabled: true,
	}
	// Create the enabled printer first (Enabled:true is the non-zero value, safe to Create).
	if err := db.Create(&enabled).Error; err != nil {
		t.Fatal(err)
	}
	// Create the disabled printer: Enabled:false is the Go zero value, so GORM skips it
	// and the column default (true) takes effect.  Create it as enabled=true first, then
	// explicitly update to false so the DB reflects the intended value.
	disabled.Enabled = true
	if err := db.Create(&disabled).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&disabled).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	disabled.Enabled = false // keep local struct in sync

	r := NewRouter(db)
	got, err := r.Route(business.ID, &locID, "bill")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.ID != enabled.ID {
		t.Fatalf("expected enabled printer (id=%d), got id=%d", enabled.ID, got.ID)
	}
}

func TestRouter_ReturnsErrNoPrinterWhenNoneRegistered(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)

	r := NewRouter(db)
	_, err := r.Route(business.ID, nil, "bill")
	if err == nil || err != ErrNoPrinterForRole {
		t.Fatalf("expected ErrNoPrinterForRole, got %v", err)
	}
}
