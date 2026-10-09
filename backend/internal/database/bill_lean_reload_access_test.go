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
)

// setupLeanReloadDB seeds one business carrying heavy blobs/Stripe-style IDs, a
// paid bill on it (with several payments, an alternative payment, and a big
// bill.Items JSON snapshot), then wires the package-level db at it. This is the
// "fully-related bill" the settle-path reload (OF-03) used to hydrate through
// GetBillByID. It returns the bill id.
//
// The settle path's only need is the FRESH SCALAR columns of the bill after the
// settlement write — none of its three consumers
// (recordCRMSettlementVisitForPaidBill / sendPaymentCompletionEmails /
// enqueueFiscalJobForPaidBill) nor the post-reload code reads a preloaded
// relation. GetBillByIDLean must therefore issue exactly ONE query and leave
// Business/Table/Payments/AlternativePayments empty.
func setupLeanReloadDB(t testing.TB) (billID uint) {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "open in-memory database")

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	SetTestDB(gormDB)
	// BillItem is intentionally NOT auto-migrated: its gen_random_uuid() default
	// is unsupported under SQLite, and the legacy snapshot path falls back to the
	// bill.Items JSON snapshot when the table is absent — exactly the heavy shape
	// the _Before benchmark (GetBillByID) must exercise.
	require.NoError(t, db.AutoMigrate(
		&Business{}, &Table{}, &Bill{}, &Payment{}, &AlternativePayment{},
	))

	business := &Business{
		BusinessId:      fmt.Sprintf("lean-reload-%d", time.Now().UnixNano()),
		Name:            "Lean Reload Biz",
		OwnerAddress:    "0xLeanReloadOwner",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		IsActive:        true,
	}
	require.NoError(t, db.Create(business).Error)

	table := &Table{BusinessID: business.ID, TableCode: "T-1", Name: "Table 1"}
	require.NoError(t, db.Create(table).Error)

	billItems := "[" + strings.TrimSuffix(strings.Repeat(`{"id":"item","name":"Bench","quantity":1},`, 64), ",") + "]"
	bill := &Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     "B-LEAN-1",
		Items:          billItems,
		Subtotal:       2500,
		TotalAmount:    2500,
		PaidAmount:     2500,
		TipAmount:      300,
		Status:         BillStatusPaid,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, db.Create(bill).Error)

	for i := 0; i < 5; i++ {
		require.NoError(t, db.Create(&Payment{
			BillID:    bill.ID,
			PayerAddr: fmt.Sprintf("0xpayer-%d", i),
			Amount:    500,
			TxHash:    fmt.Sprintf("0xpayment-%d", i),
		}).Error)
	}
	require.NoError(t, db.Create(&AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "0xcash-participant",
		PaymentMethod:   PaymentMethodCash,
		Amount:          100,
	}).Error)

	return bill.ID
}

// newLeanReloadQueryCounter registers a post-query callback that increments a
// counter on every SELECT issued against the package-level db.
func newLeanReloadQueryCounter(t testing.TB) (count *int, cleanup func()) {
	t.Helper()
	n := 0
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("lean_reload_count", func(tx *gorm.DB) {
		n++
	}))
	return &n, func() {
		_ = db.Callback().Query().Remove("lean_reload_count")
	}
}

// TestGetBillByIDLeanIsSingleQueryNoRelations is the OF-03 access-shape guard for
// the settle-path bill reload. The lean reload must:
//   - issue exactly ONE query (no Preload of Business/Table/Payments, no
//     alt-payment load, no items-snapshot second SELECT),
//   - return the fresh SCALAR columns the settle consumers read, and
//   - leave the preloaded relations zero/empty.
func TestGetBillByIDLeanIsSingleQueryNoRelations(t *testing.T) {
	billID := setupLeanReloadDB(t)

	count, cleanup := newLeanReloadQueryCounter(t)
	defer cleanup()

	bill, items, err := GetBillByIDLean(billID)
	require.NoError(t, err)
	require.NotNil(t, bill)

	assert.Equal(t, 1, *count, "lean reload must issue exactly ONE query (no preloads, no items snapshot, no alt-payments load)")

	// Scalar columns the three settle consumers + post-reload code read.
	assert.Equal(t, billID, bill.ID)
	assert.NotZero(t, bill.BusinessID, "BusinessID feeds sendPaymentCompletionEmails / flushPendingMilestones / events")
	assert.Equal(t, "B-LEAN-1", bill.BillNumber, "BillNumber feeds the receipt + JSON response + split state")
	assert.Equal(t, BillStatusPaid, bill.Status, "Status gates CRM visit + fiscal enqueue")
	assert.Equal(t, int64(2500), bill.TotalAmount, "TotalAmount feeds the receipt + remaining-amount response")
	assert.Equal(t, int64(2500), bill.PaidAmount, "PaidAmount feeds the remaining-amount response")

	// NO consumer reads a preloaded relation — they must be zero/empty.
	assert.Empty(t, bill.Payments, "settle path must not hydrate Payments")
	assert.Empty(t, bill.AlternativePayments, "settle path must not hydrate AlternativePayments")
	assert.Zero(t, bill.Business.ID, "settle path must not hydrate Business (would leak Stripe IDs)")
	assert.Zero(t, bill.Table.ID, "settle path must not hydrate Table")
	assert.Empty(t, items, "lean reload must not run the items-snapshot second query")
}

// BenchmarkSettleBillReload_Before captures the legacy over-fetching settle-path
// reload (GetBillByID: Business + Table + Payments preloads + alt-payment load +
// items snapshot) as the OF-03 baseline.
func BenchmarkSettleBillReload_Before(b *testing.B) {
	billID := setupLeanReloadDB(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bill, _, err := GetBillByID(billID)
		if err != nil {
			b.Fatal(err)
		}
		if bill.ID == 0 {
			b.Fatal("expected bill id")
		}
	}
}

// BenchmarkSettleBillReload_After captures the lean single-query reload.
func BenchmarkSettleBillReload_After(b *testing.B) {
	billID := setupLeanReloadDB(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bill, _, err := GetBillByIDLean(billID)
		if err != nil {
			b.Fatal(err)
		}
		if bill.ID == 0 {
			b.Fatal("expected bill id")
		}
	}
}
