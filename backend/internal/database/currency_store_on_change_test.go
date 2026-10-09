package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupExchangeRateTestService(t *testing.T) *CurrencyService {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file:exchange_rate_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	_ = gormDB.Migrator().DropTable(&ExchangeRate{})
	require.NoError(t, gormDB.AutoMigrate(&ExchangeRate{}))

	return NewCurrencyService(gormDB)
}

func countExchangeRateRows(t *testing.T, s *CurrencyService, from, to string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, s.db.Model(&ExchangeRate{}).
		Where("from_currency = ? AND to_currency = ?", from, to).
		Count(&n).Error)
	return n
}

// TestRecordRateObservation_UnchangedRateDoesNotInsert is the core regression
// guard against the old insert-only behavior: confirming the same rate across
// many cycles must produce exactly one row, advancing last_seen_at in place.
func TestRecordRateObservation_UnchangedRateDoesNotInsert(t *testing.T) {
	s := setupExchangeRateTestService(t)
	base := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 10; i++ {
		at := base.Add(time.Duration(i) * 20 * time.Minute)
		require.NoError(t, s.RecordRateObservation("USDC", "EUR", 0.92, "coinbase", at))
	}

	require.Equal(t, int64(1), countExchangeRateRows(t, s, "USDC", "EUR"),
		"unchanged rate over 10 cycles must remain a single row")

	latest, err := s.GetExchangeRate("USDC", "EUR")
	require.NoError(t, err)
	require.Equal(t, 0.92, latest.Rate)
	require.Equal(t, base.Add(9*20*time.Minute), latest.LastSeenAt.UTC(),
		"last_seen_at must advance to the most recent confirmation")
	require.Equal(t, base, latest.FetchedAt.UTC(),
		"fetched_at must remain when the value began")
}

// TestRecordRateObservation_ChangedRateInsertsNewStep verifies a changed rate
// appends a new step row while preserving the prior one.
func TestRecordRateObservation_ChangedRateInsertsNewStep(t *testing.T) {
	s := setupExchangeRateTestService(t)
	base := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

	require.NoError(t, s.RecordRateObservation("USDC", "EUR", 0.92, "coinbase", base))
	require.NoError(t, s.RecordRateObservation("USDC", "EUR", 0.93, "coinbase", base.Add(time.Hour)))

	require.Equal(t, int64(2), countExchangeRateRows(t, s, "USDC", "EUR"))

	latest, err := s.GetExchangeRate("USDC", "EUR")
	require.NoError(t, err)
	require.Equal(t, 0.93, latest.Rate)
	require.Equal(t, base.Add(time.Hour), latest.FetchedAt.UTC())
}

// TestGetExchangeRateAtOrBefore_StepFunction verifies as-of lookups remain exact
// across a sequence of changes — the property that lets accounting convert at a
// historical instant.
func TestGetExchangeRateAtOrBefore_StepFunction(t *testing.T) {
	s := setupExchangeRateTestService(t)
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(24 * time.Hour)
	t2 := t0.Add(48 * time.Hour)

	require.NoError(t, s.RecordRateObservation("USDC", "ARS", 1000, "coinbase", t0))
	require.NoError(t, s.RecordRateObservation("USDC", "ARS", 1100, "coinbase", t1))
	require.NoError(t, s.RecordRateObservation("USDC", "ARS", 1200, "coinbase", t2))

	cases := []struct {
		at   time.Time
		want float64
	}{
		{t0.Add(time.Hour), 1000},      // within first step
		{t1.Add(-time.Second), 1000},   // just before second change
		{t1.Add(12 * time.Hour), 1100}, // within second step
		{t2.Add(time.Hour), 1200},      // within latest step
	}
	for _, c := range cases {
		got, err := s.GetExchangeRateAtOrBefore("USDC", "ARS", c.at)
		require.NoError(t, err)
		require.Equalf(t, c.want, got.Rate, "as-of %s", c.at)
	}
}

// TestRecordRateObservation_OutOfOrderDoesNotRegressLastSeen guards the
// last_seen_at monotonicity invariant against a late/out-of-order observation.
func TestRecordRateObservation_OutOfOrderDoesNotRegressLastSeen(t *testing.T) {
	s := setupExchangeRateTestService(t)
	base := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

	require.NoError(t, s.RecordRateObservation("USDC", "EUR", 0.92, "coinbase", base.Add(time.Hour)))
	// An older observation of the same rate must not pull last_seen_at backward.
	require.NoError(t, s.RecordRateObservation("USDC", "EUR", 0.92, "coinbase", base))

	latest, err := s.GetExchangeRate("USDC", "EUR")
	require.NoError(t, err)
	require.Equal(t, base.Add(time.Hour), latest.LastSeenAt.UTC())
	require.Equal(t, int64(1), countExchangeRateRows(t, s, "USDC", "EUR"))
}

// TestExchangeRate_EffectiveSeenAtFallback verifies legacy rows (no last_seen_at)
// fall back to fetched_at for staleness.
func TestExchangeRate_EffectiveSeenAtFallback(t *testing.T) {
	fetched := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	legacy := ExchangeRate{FetchedAt: fetched}
	require.Equal(t, fetched, legacy.EffectiveSeenAt())

	seen := fetched.Add(time.Hour)
	current := ExchangeRate{FetchedAt: fetched, LastSeenAt: seen}
	require.Equal(t, seen, current.EffectiveSeenAt())
}
