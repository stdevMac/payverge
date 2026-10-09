package server

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

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/database/dbtest"
)

func setupPaymentHistoryPerfDB(t testing.TB, gormLogger logger.Interface) (*database.Business, time.Time, time.Time) {
	t.Helper()

	gormDB := setupStaffHandlerTestDBForTB(t)
	if gormLogger != nil {
		gormDB = gormDB.Session(&gorm.Session{Logger: gormLogger})
		database.SetTestDB(gormDB)
	}
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
	))
	require.NoError(t, dbtest.EnsurePaymentEventsView(gormDB))

	business := &database.Business{
		BusinessId:     fmt.Sprintf("payment-history-%d", time.Now().UnixNano()),
		Name:           "Payment History Perf",
		OwnerAddress:   "0xPaymentHistoryOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, gormDB.Create(business).Error)

	start := time.Now().UTC().AddDate(0, 0, -7).Truncate(24 * time.Hour)
	end := start.Add(7 * 24 * time.Hour)
	for i := 0; i < 500; i++ {
		table := &database.Table{
			BusinessID: business.ID,
			TableCode:  fmt.Sprintf("PAY-HIST-%03d", i),
			Name:       fmt.Sprintf("Payment History Table %03d", i),
			QRCode:     strings.Repeat("qr-payload", 128),
			IsActive:   true,
		}
		require.NoError(t, gormDB.Create(table).Error)

		paymentTime := start.Add(time.Duration(i%500) * time.Minute)
		billItems := "[" + strings.TrimSuffix(strings.Repeat(`{"id":"item","name":"Bench"},`, 256), ",") + "]"
		bill := &database.Bill{
			BusinessID:     business.ID,
			TableID:        table.ID,
			BillNumber:     fmt.Sprintf("PAY-HIST-BILL-%03d", i),
			Items:          billItems,
			TotalAmount:    5000,
			TipAmount:      500,
			Status:         database.BillStatusPaid,
			SettlementAddr: "0x1111111111111111111111111111111111111111",
			TippingAddr:    "0x2222222222222222222222222222222222222222",
			CreatedAt:      paymentTime.Add(-30 * time.Minute),
			UpdatedAt:      paymentTime,
		}
		require.NoError(t, gormDB.Create(bill).Error)

		require.NoError(t, gormDB.Create(&database.Payment{
			BillID:        bill.ID,
			PayerAddr:     fmt.Sprintf("0xpayer%03d", i),
			Amount:        5000,
			TipAmount:     500,
			TxHash:        fmt.Sprintf("pay_hist_%03d", i),
			Status:        database.PaymentStatusConfirmed,
			PaymentMethod: "crypto",
			CreatedAt:     paymentTime,
			UpdatedAt:     paymentTime,
		}).Error)
	}

	return business, start, end
}

func setupStaffHandlerTestDBForTB(t testing.TB) *gorm.DB {
	t.Helper()

	if tb, ok := t.(*testing.T); ok {
		return setupStaffHandlerTestDB(tb)
	}
	gormDB := setupBenchmarkStaffHandlerDB(t)
	return gormDB
}

func setupBenchmarkStaffHandlerDB(t testing.TB) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()), time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	database.SetTestDB(gormDB)
	InitializeRBAC(database.GetDBWrapper())
	return gormDB
}

func TestLoadPaymentHistoryItemsUsesProjectedJoin(t *testing.T) {
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	business, start, end := setupPaymentHistoryPerfDB(t, recorder)

	items, _, err := loadPaymentHistoryItemsFiltered(business.ID, start, end, PaymentHistoryFilter{})
	require.NoError(t, err)
	require.Len(t, items, 500)
	assert.NotEmpty(t, items[0].BillNumber)
	assert.NotEmpty(t, items[0].TableName)
	assert.Zero(t, recorder.selectStarCount("bills"), "payment history should not hydrate full bill rows")
	assert.Zero(t, recorder.selectStarCount("tables"), "payment history should not hydrate full table rows")
	assert.LessOrEqual(t, recorder.selectCount("bills"), 1, "payment history should not query bills once per payment")
	assert.LessOrEqual(t, recorder.selectCount("tables"), 1, "payment history should not query tables once per payment")

	// Newest first — without an explicit ORDER BY the list surfaced in
	// insertion order, which read as randomly sorted on the dashboard.
	for i := 1; i < len(items); i++ {
		require.False(t, items[i].CreatedAt.After(items[i-1].CreatedAt),
			"payment history must be sorted newest-first (row %d after row %d)", i, i-1)
	}
}

func BenchmarkLoadPaymentHistoryItemsSQLite(b *testing.B) {
	business, start, end := setupPaymentHistoryPerfDB(b, logger.Default.LogMode(logger.Silent))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		items, _, err := loadPaymentHistoryItemsFiltered(business.ID, start, end, PaymentHistoryFilter{})
		if err != nil {
			b.Fatal(err)
		}
		if len(items) != 500 {
			b.Fatalf("expected 500 payment history items, got %d", len(items))
		}
	}
}

// BenchmarkLoadPaymentHistoryItemsPaginatedSQLite measures the new server-paged
// path (page of 20) against the full-window benchmark above — the payload the
// client actually renders per page shrinks from 500 rows to 20 even though the
// COUNT still scans the window.
func BenchmarkLoadPaymentHistoryItemsPaginatedSQLite(b *testing.B) {
	business, start, end := setupPaymentHistoryPerfDB(b, logger.Default.LogMode(logger.Silent))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		items, total, err := loadPaymentHistoryItemsFiltered(business.ID, start, end, PaymentHistoryFilter{Page: 1, PageSize: 20})
		if err != nil {
			b.Fatal(err)
		}
		if len(items) != 20 || total != 500 {
			b.Fatalf("expected 20 items of 500 total, got %d of %d", len(items), total)
		}
	}
}
