package main

// SQLite-backed smoke test that exercises seedAll twice and verifies row
// counts are stable on the second run.
//
// We use sqlite (not Postgres testcontainers) on purpose: this test runs
// inside `go test ./...` without Docker, and we only need to validate the
// dedupe keys behave as advertised. The Postgres-specific bits — `jsonb`
// columns on Business.OnboardingState and a handful of others — are not
// exercised by the seeder itself (it writes the raw `{}` blob, which sqlite
// happily stores as text).

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestSeedAllIsIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping seed CLI smoke test in short mode")
	}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	// AutoMigrate only the models the seeder writes. We intentionally avoid
	// migrating the full domain (some models use Postgres-specific column
	// types that sqlite cannot translate).
	if err := db.AutoMigrate(
		&database.Business{},
		&database.Menu{},
		&database.Staff{},
		&database.StaffIdentity{},
		&database.StaffMembership{},
		&database.Table{},
		&database.Bill{},
		&database.BillItem{},
		&database.Payment{},
		&database.Order{},
		&database.StaffLoginCode{},
	); err != nil {
		t.Skipf("sqlite cannot AutoMigrate the payverge schema (postgres-specific types): %v", err)
	}

	ctx := context.Background()
	opts := seedOpts{
		Businesses:       3,
		MenuItemsPerBiz:  5,
		StaffPerBiz:      2,
		HistoricalOrders: 10,
	}

	if err := seedAll(ctx, db, opts); err != nil {
		t.Fatalf("first seedAll run: %v", err)
	}

	type counts struct {
		biz, menus, staff, tables, bills, payments, orders int64
	}
	snapshot := func() counts {
		var c counts
		db.Model(&database.Business{}).Where("business_id LIKE ?", "perf-seed-%").Count(&c.biz)
		db.Model(&database.Menu{}).Count(&c.menus)
		db.Model(&database.Staff{}).Where("email LIKE ?", "perf-seed-staff-%").Count(&c.staff)
		db.Model(&database.Table{}).Where("table_code LIKE ?", "perf-seed-%").Count(&c.tables)
		db.Model(&database.Bill{}).Where("bill_number LIKE ?", "PERF-%").Count(&c.bills)
		db.Model(&database.Payment{}).Where("tx_hash LIKE ?", "0xperfseed%").Count(&c.payments)
		db.Model(&database.Order{}).Where("order_number LIKE ?", "PERFORD-%").Count(&c.orders)
		return c
	}

	first := snapshot()

	wantBiz := int64(opts.Businesses)
	wantMenus := int64(opts.Businesses)
	wantStaff := int64(opts.Businesses * opts.StaffPerBiz)
	wantTables := int64(opts.Businesses)
	wantBills := int64(opts.Businesses * (opts.HistoricalOrders + 1))
	wantHistoricalBills := int64(opts.Businesses * opts.HistoricalOrders)

	if first.biz != wantBiz {
		t.Errorf("businesses: want %d, got %d", wantBiz, first.biz)
	}
	if first.menus != wantMenus {
		t.Errorf("menus: want %d, got %d", wantMenus, first.menus)
	}
	if first.staff != wantStaff {
		t.Errorf("staff: want %d, got %d", wantStaff, first.staff)
	}
	if first.tables != wantTables {
		t.Errorf("tables: want %d, got %d", wantTables, first.tables)
	}
	if first.bills != wantBills {
		t.Errorf("bills: want %d, got %d", wantBills, first.bills)
	}
	if first.payments != wantHistoricalBills {
		t.Errorf("payments: want %d, got %d", wantHistoricalBills, first.payments)
	}
	if first.orders != wantHistoricalBills {
		t.Errorf("orders: want %d, got %d", wantHistoricalBills, first.orders)
	}

	var activeBills, activeCodes int64
	db.Model(&database.Bill{}).Where("bill_number LIKE ? AND status = ?", "PERF-ACTIVE-%", database.BillStatusOpen).Count(&activeBills)
	db.Model(&database.StaffLoginCode{}).Where("code LIKE ? AND used = ?", "86%", false).Count(&activeCodes)
	if activeBills != wantBiz || activeCodes != wantBiz {
		t.Errorf("active fixtures: want %d bills/codes, got bills=%d codes=%d", wantBiz, activeBills, activeCodes)
	}

	// Simulate one capacity iteration mutating the active aggregate. A reseed
	// must restore a clean bill and remove only D5 request identities.
	var activeBill database.Bill
	if err := db.Where("bill_number = ?", activeBillNumber(1)).First(&activeBill).Error; err != nil {
		t.Fatalf("load active bill: %v", err)
	}
	requestID := "d5-test-1"
	if err := db.Create(&database.Order{
		BillID: activeBill.ID, BusinessID: activeBill.BusinessID,
		OrderNumber: "D5-TEST", Status: database.OrderStatusPending,
		CreatedBy: "guest", ClientRequestID: &requestID, Items: "[]",
	}).Error; err != nil {
		t.Fatalf("create D5 order: %v", err)
	}
	if err := db.Create(&database.BillItem{
		ID: "00000000-0000-0000-0000-000000000001", BillID: activeBill.ID,
		Name: "D5 Item", Price: 5, Quantity: 1, Subtotal: 5,
	}).Error; err != nil {
		t.Fatalf("create D5 bill item: %v", err)
	}
	if err := db.Model(&activeBill).Updates(map[string]any{"subtotal": 500, "total_amount": 500}).Error; err != nil {
		t.Fatalf("mutate active bill: %v", err)
	}

	if err := seedAll(ctx, db, opts); err != nil {
		t.Fatalf("second seedAll run: %v", err)
	}

	second := snapshot()
	if first != second {
		t.Errorf("counts changed across runs:\n  first:  %+v\n  second: %+v", first, second)
	}
	var d5Orders, billItems int64
	db.Model(&database.Order{}).Where("bill_id = ? AND client_request_id LIKE ?", activeBill.ID, "d5-%").Count(&d5Orders)
	db.Model(&database.BillItem{}).Where("bill_id = ?", activeBill.ID).Count(&billItems)
	if err := db.First(&activeBill, activeBill.ID).Error; err != nil {
		t.Fatalf("reload reset active bill: %v", err)
	}
	if d5Orders != 0 || billItems != 0 || activeBill.Subtotal != 0 || activeBill.TotalAmount != 0 || activeBill.Status != database.BillStatusOpen {
		t.Errorf("active fixture was not reset: orders=%d items=%d bill=%+v", d5Orders, billItems, activeBill)
	}
}
