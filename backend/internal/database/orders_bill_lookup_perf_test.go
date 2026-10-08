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

// setupOrdersByBillIDPerfDB seeds one business, one bill, and N orders on that
// bill, returning the bill id. It wires the supplied logger so callers can
// capture the emitted SQL.
func setupOrdersByBillIDPerfDB(t testing.TB, gormLogger logger.Interface) (businessID uint, billID uint) {
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
	t.Cleanup(func() { _ = sqlDB.Close() })

	SetTestDB(gormDB)
	require.NoError(t, db.AutoMigrate(&Business{}, &Table{}, &Bill{}, &Order{}))

	business := &Business{
		BusinessId:      fmt.Sprintf("orders-by-bill-%d", time.Now().UnixNano()),
		Name:            "Orders By Bill Perf",
		OwnerAddress:    "0xOrdersByBillOwner",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "AED",
	}
	require.NoError(t, db.Create(business).Error)

	billItems := "[" + strings.TrimSuffix(strings.Repeat(`{"id":"item","name":"Bench","quantity":1},`, 256), ",") + "]"
	bill := &Bill{
		BusinessID:     business.ID,
		BillNumber:     "ORDERS-BY-BILL-001",
		Items:          billItems,
		Subtotal:       1250,
		TotalAmount:    1250,
		Status:         BillStatusOpen,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, db.Create(bill).Error)

	orderItems := "[" + strings.TrimSuffix(strings.Repeat(`{"id":"item","menu_item_name":"Bench","quantity":1,"price":12.5,"subtotal":12.5},`, 8), ",") + "]"
	for i := 0; i < 25; i++ {
		require.NoError(t, db.Create(&Order{
			BillID:      bill.ID,
			BusinessID:  business.ID,
			OrderNumber: fmt.Sprintf("ORDERS-BY-BILL-%03d", i),
			Status:      OrderStatusApproved,
			CreatedBy:   "guest",
			Items:       orderItems,
		}).Error)
	}

	return business.ID, bill.ID
}

func TestGetOrdersByBillIDDoesNotHydrateBill(t *testing.T) {
	recorder := &orderListSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	_, billID := setupOrdersByBillIDPerfDB(t, recorder)

	orders, err := GetOrdersByBillID(billID)
	require.NoError(t, err)
	require.Len(t, orders, 25)
	// Business preload survives because Order.MarshalJSON resolves currency from it.
	assert.NotZero(t, orders[0].Business.ID, "Business association must stay preloaded for currency resolution")
	// Bill preload must be gone — no SELECT against bills, no bill items snapshot read.
	assert.Zero(t, recorder.selectStarCount("bills"), "orders-by-bill lookup must not SELECT the parent bill")
	assert.False(t, recorder.statementSelectsFrom("bills", strings.Join(recorder.statements, " | ")), "orders-by-bill lookup must not read the bills table at all")
	assert.Zero(t, orders[0].Bill.ID, "Bill association must not be hydrated")
}

// TestGetOrdersByBillIDUsesNarrowBusinessProjection is the X-1 access-shape
// guard for the public guest poll: no `SELECT *` against businesses, no
// sensitive columns hydrated, currency projection intact.
func TestGetOrdersByBillIDUsesNarrowBusinessProjection(t *testing.T) {
	recorder := &orderListSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	businessID, billID := setupOrdersByBillIDPerfDB(t, recorder)
	require.NoError(t, db.Model(&Business{}).Where("id = ?", businessID).
		Update("owner_address", "0xLEAKTEST").Error)

	orders, err := GetOrdersByBillID(billID)
	require.NoError(t, err)
	require.Len(t, orders, 25)

	assert.NotZero(t, orders[0].Business.ID, "currency projection must stay preloaded")
	assert.Equal(t, "AED", orders[0].Business.DefaultCurrency)
	assert.Empty(t, orders[0].Business.OwnerAddress, "guest poll must not hydrate sensitive Business columns")
	assert.Zero(t, recorder.selectStarCount("businesses"), "guest poll must not SELECT * the businesses table")
}

func BenchmarkGetOrdersByBillIDSQLite(b *testing.B) {
	_, billID := setupOrdersByBillIDPerfDB(b, logger.Default.LogMode(logger.Silent))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		orders, err := GetOrdersByBillID(billID)
		if err != nil {
			b.Fatal(err)
		}
		if len(orders) != 25 {
			b.Fatalf("expected 25 orders, got %d", len(orders))
		}
	}
}

// TestGetOrdersByBillIDProjectsGuestColumnsAndCapsRows is the I1 guard for the
// public guest-orders poll: the orders read must name its columns (no
// SELECT *, never the quote_snapshot jsonb or staff actor fields) and carry a
// row cap, so one long-lived bill cannot make every poll unbounded.
func TestGetOrdersByBillIDProjectsGuestColumnsAndCapsRows(t *testing.T) {
	recorder := &orderListSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	_, billID := setupOrdersByBillIDPerfDB(t, recorder)
	recorder.statements = nil

	orders, err := GetOrdersByBillID(billID)
	require.NoError(t, err)
	require.Len(t, orders, 25)
	assert.NotEmpty(t, orders[0].Items, "guest view still needs the item snapshot")

	assert.Zero(t, recorder.selectStarCount("orders"), "guest poll must not SELECT * the orders table")
	for _, col := range []string{"quote_snapshot", "approved_by", "cancelled_by", "client_request_id"} {
		assert.Falsef(t, recorder.selectMentionsColumn("orders", col), "guest poll must not read orders.%s", col)
	}
	var ordersStmt string
	for _, s := range recorder.statements {
		if recorder.statementSelectsFrom("orders", s) {
			ordersStmt = strings.ToLower(s)
		}
	}
	require.NotEmpty(t, ordersStmt)
	assert.Contains(t, ordersStmt, fmt.Sprintf("limit %d", GuestOrdersByBillLimit), "guest poll must cap rows")
}

func TestGetOrdersByBillIDReturnsNewestWithinCap(t *testing.T) {
	_, billID := setupOrdersByBillIDPerfDB(t, logger.Default.LogMode(logger.Silent))
	var bizID uint
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", billID).Pluck("business_id", &bizID).Error)
	base := time.Now().Add(-time.Hour)
	for i := 0; i < GuestOrdersByBillLimit+5; i++ {
		require.NoError(t, db.Create(&Order{
			BillID: billID, BusinessID: bizID, OrderNumber: fmt.Sprintf("CAP-%03d", i),
			Status: OrderStatusApproved, CreatedBy: "guest", Items: "[]",
			CreatedAt: base.Add(time.Duration(i) * time.Second),
		}).Error)
	}
	orders, err := GetOrdersByBillID(billID)
	require.NoError(t, err)
	require.Len(t, orders, GuestOrdersByBillLimit)
	assert.False(t, orders[0].CreatedAt.Before(orders[len(orders)-1].CreatedAt), "newest first")
}
