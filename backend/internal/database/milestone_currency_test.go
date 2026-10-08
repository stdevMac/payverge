package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupMilestoneCurrencyDB(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gdb.AutoMigrate(&Business{}, &Bill{}, &Payment{}, &AlternativePayment{},
		&BusinessMilestoneEvent{}, &BusinessRevenueAggregate{}, &ExchangeRate{}))
	SetTestDB(gdb)
	return gdb
}

func seedUSDCRate(t *testing.T, db *gorm.DB, to string, rate float64) {
	t.Helper()
	require.NoError(t, db.Create(&ExchangeRate{
		FromCurrency: "USDC", ToCurrency: to, Rate: rate, Source: "test", FetchedAt: time.Now(),
	}).Error)
}

// seedZeroRevenueAggregate ensures the aggregate-update path is used.
// RecordPaymentMilestonesTx's first-touch init assumes the payment is already
// in the ledger (seed totals include the delta); direct unit tests pre-create
// a zero aggregate so the delta is applied as previous→total.
func seedZeroRevenueAggregate(t *testing.T, db *gorm.DB, businessID uint) {
	t.Helper()
	require.NoError(t, db.Create(&BusinessRevenueAggregate{BusinessID: businessID}).Error)
}

// P2-9: an ARS business must NOT emit the "$1,000" milestone at 100,000
// ARS-cents (≈US$0.69 at 1450 ARS/USD); it must emit it when its ARS revenue
// crosses the converted equivalent, with the local amount in the payload.
func TestRevenueMilestones_CurrencyAwareThresholds(t *testing.T) {
	db := setupMilestoneCurrencyDB(t)
	seedUSDCRate(t, db, "USD", 1.0)
	seedUSDCRate(t, db, "ARS", 1450.0)

	biz := Business{ID: 1, BusinessId: "ars-biz", Name: "ARS Biz", DefaultCurrency: "ARS"}
	require.NoError(t, db.Create(&biz).Error)
	seedZeroRevenueAggregate(t, db, 1)

	// US$689-equivalent (100,000,000 ARS cents / 1450) — below the $1,000 ladder rung.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return RecordPaymentMilestonesTx(tx, 1, 0, 100_000_000, 0, 1, BillStatusOpen)
	}))
	var count int64
	require.NoError(t, db.Model(&BusinessMilestoneEvent{}).
		Where("business_id = ? AND milestone_type = ?", 1, BusinessMilestoneTypeRevenue).
		Count(&count).Error)
	require.Zero(t, count, "US$689-equivalent must not trigger the $1,000 milestone in ARS")

	// +US$345-equivalent → total ≈US$1,034 → crosses the $1,000 rung once.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return RecordPaymentMilestonesTx(tx, 1, 0, 50_000_000, 0, 1, BillStatusOpen)
	}))
	var events []BusinessMilestoneEvent
	require.NoError(t, db.Where("business_id = ? AND milestone_type = ?", 1, BusinessMilestoneTypeRevenue).
		Find(&events).Error)
	require.Len(t, events, 1, "crossing the converted $1,000 equivalent emits exactly one event")
	require.Equal(t, int64(100_000), events[0].ThresholdCents,
		"threshold_cents stores the USD ladder value as the stable dedupe key")
	display, _ := events[0].Payload["display_text"].(string)
	require.Contains(t, display, "ARS", "display renders the business-currency amount")
	require.Contains(t, display, "$1,000", "display keeps the US$ equivalent for context")
}

// USD businesses are byte-identical to today's behavior.
func TestRevenueMilestones_USDUnchanged(t *testing.T) {
	db := setupMilestoneCurrencyDB(t)
	seedUSDCRate(t, db, "USD", 1.0)

	biz := Business{ID: 2, BusinessId: "usd-biz", Name: "USD Biz", DefaultCurrency: "USD"}
	require.NoError(t, db.Create(&biz).Error)
	seedZeroRevenueAggregate(t, db, 2)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return RecordPaymentMilestonesTx(tx, 2, 0, 150_000, 0, 1, BillStatusOpen)
	}))
	var events []BusinessMilestoneEvent
	require.NoError(t, db.Where("business_id = ?", 2).Find(&events).Error)
	require.Len(t, events, 1)
	display, _ := events[0].Payload["display_text"].(string)
	require.Equal(t, "$1,000", display, "USD display text is unchanged")
}

// Missing rate → 1:1 fallback (today's behavior), never a dead milestone machine.
func TestRevenueMilestones_MissingRateFallsBackToParity(t *testing.T) {
	db := setupMilestoneCurrencyDB(t)

	biz := Business{ID: 3, BusinessId: "xxx-biz", Name: "No Rate", DefaultCurrency: "XXX"}
	require.NoError(t, db.Create(&biz).Error)
	seedZeroRevenueAggregate(t, db, 3)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return RecordPaymentMilestonesTx(tx, 3, 0, 150_000, 0, 1, BillStatusOpen)
	}))
	var count int64
	require.NoError(t, db.Model(&BusinessMilestoneEvent{}).Where("business_id = ?", 3).Count(&count).Error)
	require.Equal(t, int64(1), count, "no resolvable rate → 1:1 fallback keeps milestones alive")
}
