package splitting

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type splitServiceSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *splitServiceSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *splitServiceSQLRecorder) selectCount(table string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select ") {
			continue
		}
		if strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(normalized, "from "+table) {
			count++
		}
	}
	return count
}

func (r *splitServiceSQLRecorder) selectStarCount(table string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select *") {
			continue
		}
		if strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(normalized, "from "+table) {
			count++
		}
	}
	return count
}

func (r *splitServiceSQLRecorder) selectMentionsColumn(table, column string) bool {
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select ") {
			continue
		}
		if !(strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(normalized, "from "+table)) {
			continue
		}
		if strings.Contains(normalized, "`"+column+"`") ||
			strings.Contains(normalized, "\""+column+"\"") ||
			strings.Contains(normalized, "."+column) ||
			strings.Contains(normalized, " "+column+" ") {
			return true
		}
	}
	return false
}

func setupSplitServicePerfDB(t testing.TB, gormLogger logger.Interface) (*SplittingService, uint, map[string][]string) {
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

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.Table{}, &database.Bill{}, &database.Payment{}))
	require.NoError(t, createSplitServiceBillItemsTable(gormDB))

	business := &database.Business{
		BusinessId:     fmt.Sprintf("split-service-%d", time.Now().UnixNano()),
		Name:           "Split Service Perf",
		OwnerAddress:   "0xSplitServiceOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	require.NoError(t, gormDB.Create(business).Error)

	table := &database.Table{
		BusinessID: business.ID,
		TableCode:  "SPLIT-SVC-1",
		Name:       "Split Service Table",
		QRCode:     strings.Repeat("qr-payload", 128),
		IsActive:   true,
	}
	require.NoError(t, gormDB.Create(table).Error)

	normalizedItems := []database.BillItem{
		{ID: "svc-burger", Name: "Burger", Price: 50.00, Quantity: 1, Subtotal: 50.00},
		{ID: "svc-pasta", Name: "Pasta", Price: 45.00, Quantity: 1, Subtotal: 45.00},
		{ID: "svc-dessert", Name: "Dessert", Price: 30.00, Quantity: 1, Subtotal: 30.00},
	}
	legacyItems := append([]database.BillItem{}, normalizedItems...)
	for i := 0; i < 253; i++ {
		legacyItems = append(legacyItems, database.BillItem{
			ID:       fmt.Sprintf("legacy-%03d", i),
			Name:     fmt.Sprintf("Legacy Item %03d", i),
			Price:    12.50,
			Quantity: 1,
			Subtotal: 12.50,
		})
	}
	itemsJSON, err := json.Marshal(legacyItems)
	require.NoError(t, err)

	bill := &database.Bill{
		BusinessID:       business.ID,
		TableID:          table.ID,
		BillNumber:       "SPLIT-SERVICE-BILL-001",
		Items:            string(itemsJSON),
		Subtotal:         12_500,
		TaxAmount:        1_250,
		ServiceFeeAmount: 625,
		TotalAmount:      14_375,
		Status:           database.BillStatusOpen,
		SettlementAddr:   "0x1111111111111111111111111111111111111111",
		TippingAddr:      "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, gormDB.Create(bill).Error)

	for _, item := range normalizedItems {
		require.NoError(t, gormDB.Exec(
			"INSERT INTO bill_items (id, bill_id, name, price, quantity, subtotal, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			item.ID,
			bill.ID,
			item.Name,
			item.Price,
			item.Quantity,
			item.Subtotal,
			time.Now().UTC(),
		).Error)
	}

	for i := 0; i < 250; i++ {
		require.NoError(t, gormDB.Create(&database.Payment{
			BillID:    bill.ID,
			PayerAddr: fmt.Sprintf("0xpayer%03d", i),
			Amount:    1250,
			TxHash:    fmt.Sprintf("split_service_payment_%03d", i),
			Status:    database.PaymentStatusConfirmed,
		}).Error)
	}

	return NewSplittingService(database.GetDBWrapper()), bill.ID, map[string][]string{
		"alice": {"svc-burger", "svc-dessert"},
		"bob":   {"svc-pasta"},
	}
}

func createSplitServiceBillItemsTable(gormDB *gorm.DB) error {
	if err := gormDB.Exec("DROP TABLE IF EXISTS bill_items").Error; err != nil {
		return err
	}
	return gormDB.Exec(`CREATE TABLE bill_items (
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
	)`).Error
}

func TestCalculateEqualSplitUsesBillAmountProjection(t *testing.T) {
	recorder := &splitServiceSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	svc, billID, _ := setupSplitServicePerfDB(t, recorder)

	recorder.statements = nil
	result, err := svc.CalculateEqualSplit(billID, 5, map[string]string{
		"alice": "Alice",
		"bob":   "Bob",
		"carol": "Carol",
		"dave":  "Dave",
		"erin":  "Erin",
	})
	require.NoError(t, err)
	require.Len(t, result.Splits, 5)
	assert.Equal(t, 28.75, result.Splits[0].Amount)
	assert.Zero(t, recorder.selectStarCount("bills"), "equal split should project bill amount fields instead of SELECT *")
	assert.Zero(t, recorder.selectCount("businesses"), "equal split should not preload business")
	assert.Zero(t, recorder.selectCount("tables"), "equal split should not preload table")
	assert.Zero(t, recorder.selectCount("payments"), "equal split should not preload payments")
}

func TestCalculateItemSplitUsesProjectedBillAndRelationalItems(t *testing.T) {
	recorder := &splitServiceSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	svc, billID, itemSelections := setupSplitServicePerfDB(t, recorder)

	recorder.statements = nil
	result, err := svc.CalculateItemSplit(billID, itemSelections, map[string]string{
		"alice": "Alice",
		"bob":   "Bob",
	})
	require.NoError(t, err)
	require.Len(t, result.Splits, 2)
	assert.Zero(t, recorder.selectStarCount("bills"), "item split should project bill amount fields instead of SELECT *")
	assert.False(t, recorder.selectMentionsColumn("bills", "items"), "item split should read relational bill_items before legacy snapshots")
	assert.Zero(t, recorder.selectCount("businesses"), "item split should not preload business")
	assert.Zero(t, recorder.selectCount("tables"), "item split should not preload table")
	assert.Zero(t, recorder.selectCount("payments"), "item split should not preload payments")
}

func BenchmarkCalculateEqualSplitSQLite(b *testing.B) {
	svc, billID, _ := setupSplitServicePerfDB(b, logger.Default.LogMode(logger.Silent))
	people := map[string]string{
		"alice": "Alice",
		"bob":   "Bob",
		"carol": "Carol",
		"dave":  "Dave",
		"erin":  "Erin",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := svc.CalculateEqualSplit(billID, 5, people)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Splits) != 5 {
			b.Fatalf("expected 5 splits, got %d", len(result.Splits))
		}
	}
}

func BenchmarkCalculateItemSplitSQLite(b *testing.B) {
	svc, billID, itemSelections := setupSplitServicePerfDB(b, logger.Default.LogMode(logger.Silent))
	people := map[string]string{"alice": "Alice", "bob": "Bob"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := svc.CalculateItemSplit(billID, itemSelections, people)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Splits) != 2 {
			b.Fatalf("expected 2 splits, got %d", len(result.Splits))
		}
	}
}
