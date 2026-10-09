package database

import (
	"context"
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

type orderListSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *orderListSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *orderListSQLRecorder) statementSelectsFrom(table string, statement string) bool {
	normalized := strings.ToLower(strings.TrimSpace(statement))
	return strings.Contains(normalized, "from `"+table+"`") ||
		strings.Contains(normalized, "from \""+table+"\"") ||
		strings.Contains(normalized, "from "+table)
}

func (r *orderListSQLRecorder) selectStarCount(table string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.TrimSpace(statement))
		if !strings.HasPrefix(normalized, "select *") {
			continue
		}
		if r.statementSelectsFrom(table, statement) {
			count++
		}
	}
	return count
}

func (r *orderListSQLRecorder) selectMentionsColumn(table string, column string) bool {
	needleQuotedBacktick := "`" + strings.ToLower(column) + "`"
	needleQuotedDouble := `"` + strings.ToLower(column) + `"`
	for _, statement := range r.statements {
		normalized := strings.ToLower(statement)
		if !r.statementSelectsFrom(table, statement) {
			continue
		}
		if strings.Contains(normalized, needleQuotedBacktick) ||
			strings.Contains(normalized, needleQuotedDouble) ||
			strings.Contains(normalized, "."+strings.ToLower(column)) {
			return true
		}
	}
	return false
}

func setupOrderListPerfDB(t testing.TB, gormLogger logger.Interface) (*Business, PaginationParams) {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(t, err, "open in-memory database")

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	SetTestDB(gormDB)
	// DeliveryOrder is required so the orders-list NOT EXISTS (dead-delivery
	// exclusion, audit M7) subquery can resolve the delivery_orders table.
	require.NoError(t, db.AutoMigrate(&Business{}, &Table{}, &Bill{}, &Order{}, &DeliveryOrder{}))

	business := &Business{
		BusinessId:     fmt.Sprintf("order-list-%d", time.Now().UnixNano()),
		Name:           "Order List Perf",
		OwnerAddress:   "0xOrderListOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, db.Create(business).Error)

	table := &Table{
		BusinessID: business.ID,
		TableCode:  "ORDER-LIST-1",
		Name:       "Order List Table",
		QRCode:     strings.Repeat("qr-payload", 128),
		IsActive:   true,
	}
	require.NoError(t, db.Create(table).Error)

	billItems := "[" + strings.TrimSuffix(strings.Repeat(`{"id":"item","name":"Bench","quantity":1},`, 256), ",") + "]"
	orderItems := "[" + strings.TrimSuffix(strings.Repeat(`{"id":"item","menu_item_name":"Bench","quantity":1,"price":12.5,"subtotal":12.5},`, 8), ",") + "]"
	// Realistic checkout quote snapshot: frozen per-line cent breakdown that the
	// list must never read back.
	quoteSnapshot := JSONRawMessage(`{"lines":[` + strings.TrimSuffix(strings.Repeat(`{"menu_item_id":1,"quantity":1,"unit_cents":1250,"subtotal_cents":1250,"tax_cents":0},`, 32), ",") + `],"subtotal_cents":40000,"total_cents":40000}`)
	now := time.Now().UTC()
	for i := 0; i < 500; i++ {
		status := BillStatusOpen
		if i%5 == 0 {
			status = BillStatusClosed
		}

		bill := &Bill{
			BusinessID:     business.ID,
			TableID:        table.ID,
			BillNumber:     fmt.Sprintf("ORDER-LIST-BILL-%03d", i),
			Items:          billItems,
			Subtotal:       1250,
			TotalAmount:    1250,
			Status:         status,
			SettlementAddr: "0x1111111111111111111111111111111111111111",
			TippingAddr:    "0x2222222222222222222222222222222222222222",
			CreatedAt:      now.Add(-time.Duration(i) * time.Minute),
			UpdatedAt:      now.Add(-time.Duration(i) * time.Minute),
		}
		require.NoError(t, db.Create(bill).Error)

		orderStatus := OrderStatusApproved
		if i%4 == 0 {
			orderStatus = OrderStatusPending
		}
		require.NoError(t, db.Create(&Order{
			BillID:        bill.ID,
			BusinessID:    business.ID,
			OrderNumber:   fmt.Sprintf("ORDER-LIST-%03d", i),
			Status:        orderStatus,
			CreatedBy:     "guest",
			Items:         orderItems,
			QuoteSnapshot: quoteSnapshot,
			CreatedAt:     now.Add(-time.Duration(i) * time.Minute),
			UpdatedAt:     now.Add(-time.Duration(i) * time.Minute),
		}).Error)
	}

	return business, PaginationParams{Page: 1, PageSize: 100}
}

func TestGetOrdersByBusinessIDPaginatedPreloadsBillSummaryOnly(t *testing.T) {
	recorder := &orderListSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	business, pagination := setupOrderListPerfDB(t, recorder)

	result, err := GetOrdersByBusinessIDPaginated(
		business.ID,
		"pending,approved",
		pagination,
		OrderListOptions{ActiveBillsOnly: true},
	)
	require.NoError(t, err)
	require.Len(t, result.Data, 100)
	assert.NotZero(t, result.Data[0].Bill.ID)
	assert.NotEmpty(t, result.Data[0].Bill.BillNumber)
	assert.NotZero(t, result.Data[0].Bill.TableID)
	assert.Empty(t, result.Data[0].Bill.Items, "order list should not hydrate bill item snapshots")
	assert.Empty(t, result.Data[0].Bill.SettlementAddr, "order list must not project settlement wallets (FIND-039)")
	assert.Empty(t, result.Data[0].Bill.TippingAddr, "order list must not project tipping wallets (FIND-039)")
	assert.Zero(t, recorder.selectStarCount("bills"), "order list should project bill summary fields instead of SELECT *")
	assert.False(t, recorder.selectMentionsColumn("bills", "items"), "order list should not select legacy bill items snapshots")
	assert.False(t, recorder.selectMentionsColumn("bills", "settlement_addr"), "order list must not SELECT settlement_addr")
	assert.False(t, recorder.selectMentionsColumn("bills", "tipping_addr"), "order list must not SELECT tipping_addr")
}

// The operator order list never emits quote_snapshot (json:"-"); hydrating the
// per-order jsonb checkout quote on every KDS/dashboard poll is pure over-fetch.
func TestGetOrdersByBusinessIDPaginatedSkipsQuoteSnapshot(t *testing.T) {
	recorder := &orderListSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	business, pagination := setupOrderListPerfDB(t, recorder)
	recorder.statements = nil // only the list read, not the seed writes

	result, err := GetOrdersByBusinessIDPaginated(
		business.ID,
		"pending,approved",
		pagination,
		OrderListOptions{ActiveBillsOnly: true},
	)
	require.NoError(t, err)
	require.Len(t, result.Data, 100)
	first := result.Data[0]
	assert.NotZero(t, first.ID)
	assert.NotEmpty(t, first.OrderNumber)
	assert.NotEmpty(t, first.Items)
	assert.Equal(t, "guest", first.CreatedBy)
	assert.Empty(t, first.QuoteSnapshot, "order list must not hydrate the checkout quote snapshot")
	assert.Zero(t, recorder.selectStarCount("orders"), "order list should project order columns instead of SELECT *")
	assert.False(t, recorder.selectMentionsColumn("orders", "quote_snapshot"), "order list must not SELECT quote_snapshot")
	assert.False(t, recorder.selectMentionsColumn("orders", "client_request_id"), "order list must not SELECT client_request_id")
}

func BenchmarkGetOrdersByBusinessIDPaginatedSQLite(b *testing.B) {
	business, pagination := setupOrderListPerfDB(b, logger.Default.LogMode(logger.Silent))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result, err := GetOrdersByBusinessIDPaginated(
			business.ID,
			"pending,approved",
			pagination,
			OrderListOptions{ActiveBillsOnly: true},
		)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Data) != 100 {
			b.Fatalf("expected 100 orders, got %d", len(result.Data))
		}
	}
}
