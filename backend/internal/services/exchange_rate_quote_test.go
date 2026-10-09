package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func newQuoteRateService(t *testing.T) (*ExchangeRateService, *gorm.DB) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	require.NoError(t, gormDB.AutoMigrate(&database.ExchangeRate{}))
	svc := &ExchangeRateService{db: &database.DB{}, cacheDuration: time.Minute}
	svc.db.CurrencyService = database.NewCurrencyService(gormDB)
	// Async refresh must never block or panic in these tests.
	svc.fetchLatestRatesFn = func() error { return nil }
	return svc, gormDB
}

func seedRate(t *testing.T, gormDB *gorm.DB, from, to string, rate float64, age time.Duration) {
	t.Helper()
	at := time.Now().Add(-age)
	require.NoError(t, gormDB.Create(&database.ExchangeRate{
		FromCurrency: from, ToCurrency: to, Rate: rate, Source: "coinbase",
		FetchedAt: at, LastSeenAt: at,
	}).Error)
}

// TestGetFreshExchangeRate_RejectsStale locks F-RATEBAND part (a): minting a
// locked crypto quote must fail closed when the backing rate is older than the
// quote ceiling, rather than locking in a possibly-wrong value. General reads
// (GetExchangeRate) still tolerate staleness.
func TestGetFreshExchangeRate_RejectsStale(t *testing.T) {
	svc, gormDB := newQuoteRateService(t)
	seedRate(t, gormDB, "USDC", "ARS", 1000, 3*time.Hour) // stale leg
	seedRate(t, gormDB, "USDC", "USD", 1.0, 3*time.Hour)

	if _, err := svc.GetFreshExchangeRate("ARS", "USD", time.Hour); err == nil {
		t.Fatalf("expected a stale rate to be rejected for quoting")
	}

	// Lenient read still serves the stale rate (behavior preserved).
	if _, err := svc.GetExchangeRate("ARS", "USD"); err != nil {
		t.Fatalf("GetExchangeRate must still serve a stale rate, got %v", err)
	}
}

func TestGetFreshExchangeRate_AcceptsFresh(t *testing.T) {
	svc, gormDB := newQuoteRateService(t)
	seedRate(t, gormDB, "USDC", "ARS", 1000, time.Minute)
	seedRate(t, gormDB, "USDC", "USD", 1.0, time.Minute)

	rate, err := svc.GetFreshExchangeRate("ARS", "USD", time.Hour)
	if err != nil {
		t.Fatalf("fresh rate should be accepted, got %v", err)
	}
	// ARS->USD = (USDC/USD) / (USDC/ARS) = 1.0/1000.
	if rate <= 0 || rate > 0.01 {
		t.Fatalf("unexpected cross rate %v", rate)
	}

	converted, err := svc.ConvertAmountForQuote(1000, "ARS", "USD")
	if err != nil {
		t.Fatalf("ConvertAmountForQuote fresh should succeed, got %v", err)
	}
	if converted <= 0 {
		t.Fatalf("expected positive converted amount, got %v", converted)
	}
}
