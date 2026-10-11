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

// setupGetOrderByIDPerfDB seeds one business (with a synthetic Stripe ID and
// OnboardingState blob that must never appear in the Business preload when the
// projection is narrow), one bill, and one order, then returns the order ID.
func setupGetOrderByIDPerfDB(t testing.TB, gormLogger logger.Interface) (orderID uint) {
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
		BusinessId:      fmt.Sprintf("order-by-id-%d", time.Now().UnixNano()),
		Name:            "GetOrderByID Perf Business",
		OwnerAddress:    "0xGetOrderByIDOwner",
		SettlementAddr:  "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		TippingAddr:     "0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
		DefaultCurrency: "MXN",
		DisplayCurrency: "MXN",
	}
	require.NoError(t, db.Create(business).Error)

	bill := &Bill{
		BusinessID:  business.ID,
		BillNumber:  "ORDER-BY-ID-PERF-001",
		Subtotal:    500,
		TotalAmount: 500,
		Status:      BillStatusOpen,
	}
	require.NoError(t, db.Create(bill).Error)

	order := &Order{
		BillID:      bill.ID,
		BusinessID:  business.ID,
		OrderNumber: "OBYPERF-001",
		Status:      OrderStatusPending,
		CreatedBy:   "guest",
		Items:       `[{"id":"item-1","menu_item_name":"Test Item","quantity":1,"price":5.0,"subtotal":5.0}]`,
	}
	require.NoError(t, db.Create(order).Error)

	return order.ID
}

// TestGetOrderByIDUsesNarrowBusinessProjection is the PRELOAD-03 access-shape
// guard: GetOrderByID must load Business with the narrow currency projection
// only (id/default_currency/display_currency). Sensitive columns (StripeCustomerID,
// OnboardingState, etc.) must not be hydrated.
func TestGetOrderByIDUsesNarrowBusinessProjection(t *testing.T) {
	recorder := &orderListSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	orderID := setupGetOrderByIDPerfDB(t, recorder)

	order, _, err := GetOrderByID(orderID)
	require.NoError(t, err)
	require.NotNil(t, order)

	// Currency projection must be populated (the only thing MarshalJSON needs).
	assert.NotZero(t, order.Business.ID, "Business.ID must be set by the narrow projection")
	assert.Equal(t, "MXN", order.Business.DefaultCurrency, "DefaultCurrency must be projected")
	assert.Equal(t, "MXN", order.Business.DisplayCurrency, "DisplayCurrency must be projected")

	// Sensitive columns must not be hydrated by the narrow projection.
	assert.Empty(t, order.Business.OwnerAddress,
		"PRELOAD-03: GetOrderByID must not hydrate Business.OwnerAddress (narrow projection)")

	// No SELECT * against businesses allowed.
	assert.Zero(t, recorder.selectStarCount("businesses"),
		"PRELOAD-03: GetOrderByID must not SELECT * the businesses table")
}

// BenchmarkGetOrderByID_Before captures the baseline (full Business preload).
// Run this BEFORE the fix to record the before numbers.
func BenchmarkGetOrderByID_Before(b *testing.B) {
	orderID := setupGetOrderByIDPerfDB(b, logger.Default.LogMode(logger.Silent))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		order, _, err := GetOrderByID(orderID)
		if err != nil {
			b.Fatal(err)
		}
		if order == nil {
			b.Fatal("order must not be nil")
		}
	}
}

// BenchmarkGetOrderByID_After captures the after numbers (narrow Business
// projection). Run after implementing the fix to record the after numbers.
func BenchmarkGetOrderByID_After(b *testing.B) {
	orderID := setupGetOrderByIDPerfDB(b, logger.Default.LogMode(logger.Silent))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		order, _, err := GetOrderByID(orderID)
		if err != nil {
			b.Fatal(err)
		}
		if order == nil {
			b.Fatal("order must not be nil")
		}
	}
}
