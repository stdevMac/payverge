package services

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestGetExchangeRateAsync asserts that GetExchangeRate returns the stale
// cached rate immediately without blocking on a slow FetchLatestRates call.
//
// Correctness contract: the staleness-triggered background refresh must never
// block the caller. The last-good rate is served until the async fetch
// completes and atomically updates the DB row.
func TestGetExchangeRateAsync(t *testing.T) {
	// Build an in-memory SQLite DB with the exchange_rates table so
	// CurrencyService.GetExchangeRate works without a real Postgres connection.
	gormDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := gormDB.AutoMigrate(&database.ExchangeRate{}); err != nil {
		t.Fatalf("migrate exchange_rates: %v", err)
	}

	// Seed a stale rate (LastSeenAt > 1 hour ago) so the staleness branch fires.
	staleTime := time.Now().Add(-2 * time.Hour)
	seed := database.ExchangeRate{
		FromCurrency: "USD",
		ToCurrency:   "ARS",
		Rate:         1465.0,
		Source:       "coinbase",
		FetchedAt:    staleTime,
		LastSeenAt:   staleTime,
	}
	if err := gormDB.Create(&seed).Error; err != nil {
		t.Fatalf("seed stale rate: %v", err)
	}

	svc := &ExchangeRateService{
		db:            &database.DB{},
		cacheDuration: time.Minute,
		// httpClient is nil — fetchLatestRatesFn replaces the real call.
	}
	svc.db.CurrencyService = database.NewCurrencyService(gormDB)

	// Inject a blocking fetchLatestRatesFn so we can observe non-blocking behaviour.
	fetchUnblock := make(chan struct{})
	fetchStarted := make(chan struct{}, 1)
	svc.fetchLatestRatesFn = func() error {
		select {
		case fetchStarted <- struct{}{}:
		default:
		}
		<-fetchUnblock // block until the test releases it
		return nil
	}

	// GetExchangeRate must return before fetchUnblock is closed.
	done := make(chan float64, 1)
	go func() {
		rate, err := svc.GetExchangeRate("USD", "ARS")
		if err != nil {
			t.Errorf("GetExchangeRate returned error: %v", err)
		}
		done <- rate
	}()

	// The async fetch must start (goroutine spawned) but must not block the caller.
	select {
	case <-fetchStarted:
		// good — background refresh goroutine has begun
	case <-time.After(2 * time.Second):
		t.Fatal("fetchLatestRatesFn was never called (async goroutine not spawned)")
	}

	// The rate result must arrive before we release the blocking fetch.
	select {
	case got := <-done:
		if got != seed.Rate {
			t.Fatalf("GetExchangeRate returned %v; want stale rate %v", got, seed.Rate)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("GetExchangeRate blocked waiting for the async fetch (must be non-blocking)")
	}

	// Release the background goroutine so it doesn't leak under -race.
	close(fetchUnblock)
}
