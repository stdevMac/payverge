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

func setupTableStatusPerfDB(t testing.TB, gormLogger logger.Interface) uint {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	SetTestDB(gormDB)
	require.NoError(t, db.AutoMigrate(&Business{}, &Table{}, &Bill{}, &TableReservation{}, &Order{}))
	require.NoError(t, db.Exec(`CREATE TABLE bill_items (
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

	business := &Business{
		BusinessId:     fmt.Sprintf("table-status-%d", time.Now().UnixNano()),
		Name:           "Table Status Perf",
		OwnerAddress:   "0xTableStatusOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	require.NoError(t, db.Create(business).Error)

	now := time.Now().UTC()
	billItems := "[" + strings.TrimSuffix(strings.Repeat(`{"id":"item","name":"Bench","quantity":1},`, 256), ",") + "]"
	for i := 0; i < 500; i++ {
		table := &Table{
			BusinessID: business.ID,
			TableCode:  fmt.Sprintf("STATUS-%03d", i),
			Name:       fmt.Sprintf("Status Table %03d", i),
			Capacity:   4,
			QRCode:     strings.Repeat("qr-payload", 128),
			IsActive:   true,
		}
		require.NoError(t, db.Create(table).Error)

		require.NoError(t, db.Create(&Bill{
			BusinessID:     business.ID,
			TableID:        table.ID,
			BillNumber:     fmt.Sprintf("STATUS-BILL-%03d", i),
			Items:          billItems,
			Subtotal:       5000,
			TotalAmount:    5000,
			PaidAmount:     1000,
			Status:         BillStatusOpen,
			SettlementAddr: "0x1111111111111111111111111111111111111111",
			TippingAddr:    "0x2222222222222222222222222222222222222222",
			CreatedAt:      now.Add(-time.Duration(i) * time.Minute),
			UpdatedAt:      now.Add(-time.Duration(i) * time.Minute),
		}).Error)

		if i%5 == 0 {
			tableID := table.ID
			require.NoError(t, db.Create(&TableReservation{
				BusinessID:       business.ID,
				TableID:          &tableID,
				CustomerName:     fmt.Sprintf("Guest %03d", i),
				CustomerEmail:    fmt.Sprintf("guest-%03d@example.com", i),
				PartySize:        2,
				ReservationTime:  now.Add(15 * time.Minute),
				Duration:         60,
				Status:           "confirmed",
				ConfirmationCode: fmt.Sprintf("STATUS-CONF-%03d", i),
				SpecialRequests:  strings.Repeat("window ", 32),
				Notes:            strings.Repeat("internal note ", 32),
			}).Error)
		}
	}

	return business.ID
}

func TestGetTablesWithStatusProjectsActiveBills(t *testing.T) {
	recorder := &orderListSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	businessID := setupTableStatusPerfDB(t, recorder)

	recorder.statements = nil
	rows, err := GetTablesWithStatus(businessID, false)
	require.NoError(t, err)
	require.Len(t, rows, 500)
	assert.Equal(t, "occupied", rows[0]["status"])
	assert.Equal(t, 1, rows[0]["active_bills_count"])

	activeBills, ok := rows[0]["active_bills"].([]Bill)
	require.True(t, ok)
	require.Len(t, activeBills, 1)
	assert.NotZero(t, activeBills[0].ID)
	assert.NotEmpty(t, activeBills[0].BillNumber)
	assert.Empty(t, activeBills[0].Items, "table status only needs active bill summary metadata")
	assert.Zero(t, recorder.selectStarCount("bills"), "table status should project active bill fields instead of SELECT *")
	assert.False(t, recorder.selectMentionsColumn("bills", "items"), "table status should not select legacy bill item snapshots")
	assert.False(t, recorder.selectMentionsColumn("tables", "qr_code"), "table status should not ship QR payload blobs on the poll path")
	assert.False(t, recorder.selectMentionsColumn("table_reservations", "customer_email"), "table status should project reservation list fields only")
}

func TestGetTablesWithStatusProjectsPhysicalItemQuantity(t *testing.T) {
	businessID := setupTableStatusPerfDB(t, logger.Default.LogMode(logger.Silent))
	var bill Bill
	require.NoError(t, db.Where("business_id = ?", businessID).Order("id ASC").First(&bill).Error)
	require.NoError(t, db.Create(&BillItem{
		ID: "table-status-menu", BillID: bill.ID, Name: "Entree",
		ItemType: "menu_item", Quantity: 2, Price: 10, Subtotal: 20,
	}).Error)
	require.NoError(t, db.Create(&BillItem{
		ID: "table-status-discount", BillID: bill.ID, Name: "Discount",
		ItemType: "discount", Quantity: 1, Price: -2, Subtotal: -2,
	}).Error)

	rows, err := GetTablesWithStatus(businessID, false)
	require.NoError(t, err)
	for _, row := range rows {
		bills, _ := row["active_bills"].([]Bill)
		if len(bills) > 0 && bills[0].ID == bill.ID {
			assert.Equal(t, int64(2), row["active_bill_physical_item_quantity"])
			return
		}
	}
	t.Fatal("active bill table row not found")
}

func TestGetTablesWithStatusCountsBundlesNotExpandedChildren(t *testing.T) {
	businessID := setupTableStatusPerfDB(t, logger.Default.LogMode(logger.Silent))
	var bill Bill
	require.NoError(t, db.Where("business_id = ?", businessID).Order("id ASC").First(&bill).Error)
	require.NoError(t, db.Create(&BillItem{
		ID: "table-status-bundle", BillID: bill.ID, Name: "Date Night",
		ItemType: "bundle", Quantity: 4, Price: 80, Subtotal: 320,
	}).Error)
	require.NoError(t, db.Create(&BillItem{
		ID: "table-status-bundle-child", BillID: bill.ID, Name: "Steak",
		ItemType: "bundle_item", Quantity: 4, Price: 0, Subtotal: 0,
	}).Error)

	rows, err := GetTablesWithStatus(businessID, false)
	require.NoError(t, err)
	for _, row := range rows {
		bills, _ := row["active_bills"].([]Bill)
		if len(bills) > 0 && bills[0].ID == bill.ID {
			assert.Equal(t, int64(4), row["active_bill_physical_item_quantity"],
				"4× Date Night must read as 4 covers, not 4+4 component plates")
			return
		}
	}
	t.Fatal("active bill table row not found")
}

func BenchmarkGetTablesWithStatusSQLite(b *testing.B) {
	businessID := setupTableStatusPerfDB(b, logger.Default.LogMode(logger.Silent))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := GetTablesWithStatus(businessID, false)
		if err != nil {
			b.Fatal(err)
		}
		if len(rows) != 500 {
			b.Fatalf("expected 500 table status rows, got %d", len(rows))
		}
	}
}
