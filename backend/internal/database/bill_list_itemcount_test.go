package database

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupBillListBenchDB mirrors setupOrderTestDB for testing.B (which the
// *testing.T-typed helper can't serve), including the manual bill_items DDL.
func setupBillListBenchDB(b *testing.B) {
	b.Helper()
	dsn := fmt.Sprintf("file:bill-list-bench-%d?mode=memory&cache=shared", time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(b, err)
	sqlDB, err := gormDB.DB()
	require.NoError(b, err)
	sqlDB.SetMaxOpenConns(1)
	b.Cleanup(func() { _ = sqlDB.Close() })
	db = gormDB
	require.NoError(b, db.AutoMigrate(&Business{}, &Table{}, &Bill{}))
	db.Exec("DROP TABLE IF EXISTS bill_items")
	require.NoError(b, db.Exec(`
		CREATE TABLE bill_items (
			id TEXT PRIMARY KEY,
			bill_id INTEGER NOT NULL,
			menu_item_id TEXT DEFAULT '',
			name TEXT NOT NULL,
			price REAL NOT NULL,
			quantity INTEGER NOT NULL,
			options TEXT,
			item_type TEXT DEFAULT 'menu_item',
			bundle_id INTEGER,
			parent_bundle_id INTEGER,
			source_offer_id INTEGER,
			order_id INTEGER,
			subtotal REAL NOT NULL,
			created_at DATETIME
		)`).Error)
	require.NoError(b, db.Exec("CREATE INDEX idx_bill_items_bill_id ON bill_items(bill_id)").Error)
}

// Audit G-03: the operator bill LIST payload undercounted items ("0 items")
// because the legacy bill.items JSON snapshot is empty for bills whose items
// live in bill_items. The list projection must expose an item_count computed
// from bill_items — in the SAME query (no per-row reloads).

func seedItemCountBills(t *testing.T) (bizID uint, modernNumber, legacyNumber string) {
	t.Helper()
	biz := helperBusiness(t, 0, 0)

	// Modern bill: items live in bill_items; the JSON snapshot is empty —
	// exactly the live-verified "0 items vs panel 2" shape.
	modern := &Bill{
		BusinessID: biz.ID,
		BillNumber: "PV-itemcount-modern",
		Status:     BillStatusOpen,
		Items:      "[]",
		CreatedAt:  time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(modern).Error)
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&BillItem{
			ID:       fmt.Sprintf("bi-modern-%d", i),
			BillID:   modern.ID,
			Name:     "Burger",
			Price:    10,
			Quantity: 1,
			Subtotal: 10,
		}).Error)
	}

	// Legacy bill: JSON snapshot only, no bill_items rows.
	legacy := &Bill{
		BusinessID: biz.ID,
		BillNumber: "PV-itemcount-legacy",
		Status:     BillStatusOpen,
		Items:      `[{"name":"Old Snapshot","quantity":1}]`,
		CreatedAt:  time.Date(2026, 6, 9, 11, 0, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(legacy).Error)

	// A different bill's items must not bleed into the counts.
	other := &Bill{
		BusinessID: biz.ID,
		BillNumber: "PV-itemcount-other",
		Status:     BillStatusOpen,
		Items:      "[]",
		CreatedAt:  time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(other).Error)
	require.NoError(t, db.Create(&BillItem{
		ID:       "bi-other-0",
		BillID:   other.ID,
		Name:     "Fries",
		Price:    5,
		Quantity: 1,
		Subtotal: 5,
	}).Error)

	return biz.ID, modern.BillNumber, legacy.BillNumber
}

func TestGetBillListRows_ItemCountFromBillItems(t *testing.T) {
	setupOrderTestDB(t)
	bizID, modernNumber, legacyNumber := seedItemCountBills(t)

	result, err := GetBillListRowsByBusinessIDFilteredPaginated(
		bizID, BillListFilters{}, PaginationParams{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)
	require.Len(t, result.Data, 3)

	counts := map[string]int64{}
	for _, row := range result.Data {
		counts[row.BillNumber] = row.ItemCount
	}
	assert.Equal(t, int64(2), counts[modernNumber], "modern bill must count its bill_items rows")
	assert.Equal(t, int64(0), counts[legacyNumber], "legacy snapshot-only bill has no bill_items rows (frontend falls back to the JSON snapshot)")
	assert.Equal(t, int64(1), counts["PV-itemcount-other"], "counts must be scoped per bill")
}

func TestGetBillListRows_PhysicalItemQuantityExcludesFinancialRows(t *testing.T) {
	setupOrderTestDB(t)
	bizID, modernNumber, _ := seedItemCountBills(t)
	var bill Bill
	require.NoError(t, db.Where("bill_number = ?", modernNumber).First(&bill).Error)
	require.NoError(t, db.Model(&BillItem{}).
		Where("id = ?", "bi-modern-0").
		Update("quantity", 2).Error)
	require.NoError(t, db.Create(&BillItem{
		ID: "bi-modern-bundle-parent", BillID: bill.ID, Name: "Dinner bundle",
		ItemType: "bundle", Quantity: 1, Price: 20, Subtotal: 20,
	}).Error)
	require.NoError(t, db.Create(&BillItem{
		ID: "bi-modern-bundle-child", BillID: bill.ID, Name: "Included fries",
		ItemType: "bundle_item", Quantity: 2, Price: 0, Subtotal: 0,
	}).Error)
	require.NoError(t, db.Create(&BillItem{
		ID: "bi-modern-discount", BillID: bill.ID, Name: "Promotion",
		ItemType: "discount", Quantity: 1, Price: -5, Subtotal: -5,
	}).Error)

	result, err := GetBillListRowsByBusinessIDFilteredPaginated(
		bizID, BillListFilters{}, PaginationParams{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)
	for _, row := range result.Data {
		if row.BillNumber == modernNumber {
			assert.Equal(t, int64(5), row.ItemCount)
			// Sellable lines only: menu qty 2 + menu qty 1 + bundle parent 1 = 4.
			// Bundle children and discounts must not inflate the operator count.
			assert.Equal(t, int64(4), row.PhysicalItemQuantity)
			return
		}
	}
	t.Fatal("modern bill row not found")
}

// The count must come from the page query itself — never a per-row reload.
func TestGetBillListRows_ItemCountIsSingleQuery(t *testing.T) {
	recorder := &orderListSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupOrderTestDB(t)
	bizID, _, _ := seedItemCountBills(t)

	// Record only the statements the list call emits.
	recorder.statements = nil
	db.Logger = recorder

	_, err := GetBillListRowsByBusinessIDFilteredPaginated(
		bizID, BillListFilters{}, PaginationParams{Page: 1, PageSize: 10},
	)
	require.NoError(t, err)

	selects := 0
	for _, stmt := range recorder.statements {
		upper := strings.ToUpper(stmt)
		if strings.HasPrefix(strings.TrimSpace(upper), "SELECT") {
			selects++
			// No standalone bill_items query: bill_items may appear only
			// inside a statement that selects FROM bills.
			if strings.Contains(upper, "BILL_ITEMS") && !strings.Contains(upper, "FROM `BILLS`") && !strings.Contains(upper, `FROM "BILLS"`) && !strings.Contains(upper, "FROM BILLS") {
				t.Fatalf("standalone bill_items query detected (N+1): %s", stmt)
			}
		}
	}
	// Exactly two SELECTs: the COUNT and the page projection.
	assert.Equal(t, 2, selects, "list must stay at COUNT + one page query, got: %v", recorder.statements)
}

// Baseline/after benchmark for the perf gate (run with -benchmem).
func BenchmarkGetBillListRowsSQLite(b *testing.B) {
	setupBillListBenchDB(b)

	biz := &Business{
		BusinessId:      fmt.Sprintf("bill-list-bench-%d", time.Now().UnixNano()),
		Name:            "Bill List Bench",
		OwnerAddress:    "0xBillListBench",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
	}
	require.NoError(b, db.Create(biz).Error)

	for i := 0; i < 60; i++ {
		bill := &Bill{
			BusinessID: biz.ID,
			BillNumber: fmt.Sprintf("PV-bench-%03d", i),
			Status:     BillStatusOpen,
			Items:      "[]",
			CreatedAt:  time.Now().Add(time.Duration(i) * time.Second),
		}
		require.NoError(b, db.Create(bill).Error)
		for j := 0; j < 4; j++ {
			require.NoError(b, db.Create(&BillItem{
				ID:       fmt.Sprintf("bi-%03d-%d", i, j),
				BillID:   bill.ID,
				Name:     "Bench Item",
				Price:    10,
				Quantity: 1,
				Subtotal: 10,
			}).Error)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := GetBillListRowsByBusinessIDFilteredPaginated(
			biz.ID, BillListFilters{}, PaginationParams{Page: 1, PageSize: 50},
		)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Data) != 50 {
			b.Fatalf("expected 50 rows, got %d", len(result.Data))
		}
	}
}
