package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

// ErrExchangeRateCooldown is returned when FetchLatestRates refuses to call
// Coinbase again because the previous upstream attempt failed recently.
// A zero failureCooldown disables this and retries on every call.
var ErrExchangeRateCooldown = errors.New("exchange rate fetch cooling down")

// CoinbaseExchangeRateResponse represents the response from Coinbase API
type CoinbaseExchangeRateResponse struct {
	Data struct {
		Currency string            `json:"currency"`
		Rates    map[string]string `json:"rates"`
	} `json:"data"`
}

// ExchangeRateService handles fetching and caching exchange rates
type ExchangeRateService struct {
	db              *database.DB
	coinbaseURL     string
	httpClient      *http.Client
	lastFetchedNano atomic.Int64 // UnixNano of last successful fetch; 0 = never
	cacheDuration   time.Duration
	// fetchGroup collapses concurrent Coinbase GETs into one in-flight call.
	fetchGroup singleflight.Group
	// lastFailedNano is the UnixNano of the last failed upstream fetch; 0 = none.
	lastFailedNano atomic.Int64
	// failureCooldown suppresses retries after an upstream failure.
	// Zero means no cooldown (test literals that leave it unset).
	failureCooldown time.Duration
	// fetchLatestRatesFn is the function called when a staleness-triggered
	// refresh is needed. Defaults to s.FetchLatestRates. Injected by tests to
	// substitute a blocking or no-op fetch without making real HTTP calls.
	fetchLatestRatesFn func() error
}

// NewExchangeRateService creates a new exchange rate service
func NewExchangeRateService(db *database.DB) *ExchangeRateService {
	s := &ExchangeRateService{
		db:              db,
		coinbaseURL:     "https://api.coinbase.com/v2/exchange-rates?currency=USDC",
		httpClient:      &http.Client{Timeout: 10 * time.Second},
		cacheDuration:   20 * time.Minute, // Cache rates for 20 minutes
		failureCooldown: 45 * time.Second,
	}
	s.fetchLatestRatesFn = s.FetchLatestRates
	return s
}

// recentlyFetched reports whether a fetch happened within cacheDuration.
func (s *ExchangeRateService) recentlyFetched() bool {
	n := s.lastFetchedNano.Load()
	if n == 0 {
		return false
	}
	return time.Since(time.Unix(0, n)) < s.cacheDuration
}

// markFetched records a successful fetch timestamp.
func (s *ExchangeRateService) markFetched(t time.Time) {
	s.lastFetchedNano.Store(t.UnixNano())
}

// exchangeRateCooldownError reports a recent upstream failure when the
// cooldown is enabled. A zero failureCooldown always returns nil.
func (s *ExchangeRateService) exchangeRateCooldownError() error {
	if s.failureCooldown <= 0 {
		return nil
	}
	failedAt := s.lastFailedNano.Load()
	if failedAt == 0 {
		return nil
	}
	ago := time.Since(time.Unix(0, failedAt))
	if ago >= s.failureCooldown {
		return nil
	}
	return fmt.Errorf("%w: last upstream fetch failed %s ago", ErrExchangeRateCooldown, ago)
}

// FetchLatestRates fetches the latest exchange rates from Coinbase.
// Concurrent callers share one upstream GET. After a failure, further calls
// fail fast until failureCooldown elapses.
func (s *ExchangeRateService) FetchLatestRates() error {
	if s.recentlyFetched() {
		log.Println("Exchange rates recently fetched, skipping...")
		return nil
	}
	if err := s.exchangeRateCooldownError(); err != nil {
		return err
	}
	_, err, _ := s.fetchGroup.Do("coinbase", func() (any, error) {
		if s.recentlyFetched() {
			return nil, nil
		}
		if err := s.exchangeRateCooldownError(); err != nil {
			return nil, err
		}
		err := s.fetchLatestRatesOnce()
		if err != nil {
			s.lastFailedNano.Store(time.Now().UnixNano())
		} else {
			s.lastFailedNano.Store(0)
		}
		return nil, err
	})
	return err
}

// fetchLatestRatesOnce performs one Coinbase GET and writes observations.
func (s *ExchangeRateService) fetchLatestRatesOnce() error {
	// Check if we need to fetch (rate limiting)
	if s.recentlyFetched() {
		log.Println("Exchange rates recently fetched, skipping...")
		return nil
	}

	log.Println("Fetching latest exchange rates from Coinbase...")

	resp, err := s.httpClient.Get(s.coinbaseURL)
	if err != nil {
		return fmt.Errorf("failed to fetch exchange rates: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("coinbase API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var coinbaseResp CoinbaseExchangeRateResponse
	if err := json.Unmarshal(body, &coinbaseResp); err != nil {
		return fmt.Errorf("failed to parse coinbase response: %w", err)
	}

	// Update rates in database
	fetchedAt := time.Now()
	for currency, rateStr := range coinbaseResp.Data.Rates {
		// Parse rate as float
		var rate float64
		if _, err := fmt.Sscanf(rateStr, "%f", &rate); err != nil {
			log.Printf("Failed to parse rate for %s: %v", currency, err)
			continue
		}

		// Skip if rate is 0 or invalid
		if rate <= 0 {
			continue
		}

		// Store-on-change: only writes a new row when the rate actually moved;
		// otherwise advances last_seen_at on the current row in place.
		if err := s.db.CurrencyService.RecordRateObservation("USDC", currency, rate, "coinbase", fetchedAt); err != nil {
			log.Printf("Failed to record exchange rate for %s: %v", currency, err)
		}
	}

	s.markFetched(fetchedAt)
	log.Printf("Successfully updated %d exchange rates", len(coinbaseResp.Data.Rates))
	return nil
}

// resolveExchangeRate returns the rate for a pair plus the effective "seen at"
// timestamp of the freshest data backing it (the older leg for a cross rate).
// It performs a synchronous fetch only when a needed USDC leg is missing
// entirely; it never refreshes merely-stale data — that policy belongs to the
// caller (lenient GetExchangeRate vs. fail-closed GetFreshExchangeRate).
func (s *ExchangeRateService) resolveExchangeRate(fromCurrency, toCurrency string) (float64, time.Time, error) {
	if fromCurrency == toCurrency {
		return 1.0, time.Now().UTC(), nil
	}

	// Try direct lookup first.
	if rate, err := s.db.CurrencyService.GetExchangeRate(fromCurrency, toCurrency); err == nil {
		return rate.Rate, rate.EffectiveSeenAt(), nil
	}

	// Direct lookup failed, try cross-currency conversion via USDC.
	log.Printf("Direct rate not found for %s to %s, trying cross-currency conversion via USDC", fromCurrency, toCurrency)

	fromRate, err := s.db.CurrencyService.GetExchangeRate("USDC", fromCurrency)
	if err != nil {
		log.Printf("USDC to %s rate not found, fetching latest rates...", fromCurrency)
		if fetchErr := s.FetchLatestRates(); fetchErr != nil {
			return 0, time.Time{}, fmt.Errorf("exchange rate not found and failed to fetch: %w", fetchErr)
		}
		fromRate, err = s.db.CurrencyService.GetExchangeRate("USDC", fromCurrency)
		if err != nil {
			return 0, time.Time{}, fmt.Errorf("USDC to %s rate not found", fromCurrency)
		}
	}

	toRate, err := s.db.CurrencyService.GetExchangeRate("USDC", toCurrency)
	if err != nil {
		log.Printf("USDC to %s rate not found", toCurrency)
		return 0, time.Time{}, fmt.Errorf("USDC to %s rate not found", toCurrency)
	}

	// Cross rate: fromCurrency -> toCurrency = (USDC/toCurrency) / (USDC/fromCurrency).
	if fromRate.Rate == 0 {
		return 0, time.Time{}, fmt.Errorf("invalid USDC to %s rate: zero", fromCurrency)
	}
	crossRate := toRate.Rate / fromRate.Rate
	log.Printf("Cross-currency conversion: %s to %s = %.8f (via USDC: %f / %f)",
		fromCurrency, toCurrency, crossRate, toRate.Rate, fromRate.Rate)

	seenAt := fromRate.EffectiveSeenAt()
	if toRate.EffectiveSeenAt().Before(seenAt) {
		seenAt = toRate.EffectiveSeenAt()
	}
	return crossRate, seenAt, nil
}

// GetExchangeRate gets the exchange rate for a currency pair. It tolerates
// staleness: if the backing data is older than an hour it triggers a
// non-blocking background refresh and still returns the last-good value.
// Staleness keys off LastSeenAt (when last confirmed), not FetchedAt — a
// long-stable rate is fresh as long as we keep confirming it.
func (s *ExchangeRateService) GetExchangeRate(fromCurrency, toCurrency string) (float64, error) {
	rate, seenAt, err := s.resolveExchangeRate(fromCurrency, toCurrency)
	if err != nil {
		return 0, err
	}
	if fromCurrency != toCurrency && time.Since(seenAt) > time.Hour {
		log.Printf("Exchange rate for %s to %s is stale, triggering async refresh...", fromCurrency, toCurrency)
		// Refresh in the background so this request is not blocked on the
		// Coinbase round-trip; the periodic fetcher bounds how stale it can get.
		fetchFn := s.fetchLatestRatesFn
		logger.SafeGo(func() {
			if err := fetchFn(); err != nil {
				log.Printf("Async exchange rate refresh failed: %v", err)
			}
		})
	}
	return rate, nil
}

// GetFreshExchangeRate is GetExchangeRate with a hard staleness ceiling: it
// returns an error (rather than serving a stale value) when the backing rate is
// older than maxAge. Used to mint locked crypto quotes, where serving a stale
// or corrupt rate would lock the guest into a wrong settlement amount.
func (s *ExchangeRateService) GetFreshExchangeRate(fromCurrency, toCurrency string, maxAge time.Duration) (float64, error) {
	rate, seenAt, err := s.resolveExchangeRate(fromCurrency, toCurrency)
	if err != nil {
		return 0, err
	}
	if fromCurrency != toCurrency && maxAge > 0 && time.Since(seenAt) > maxAge {
		return 0, fmt.Errorf("exchange rate %s->%s is too stale to quote (last seen %s ago)",
			fromCurrency, toCurrency, time.Since(seenAt).Round(time.Minute))
	}
	return rate, nil
}

// ConvertAmount converts an amount from one currency to another
func (s *ExchangeRateService) ConvertAmount(amount float64, fromCurrency, toCurrency string) (float64, error) {
	rate, err := s.GetExchangeRate(fromCurrency, toCurrency)
	if err != nil {
		return 0, err
	}

	return amount * rate, nil
}

// quoteRateMaxAge bounds how stale a rate may be when minting a locked crypto
// quote. The 5-minute periodic fetcher plus the 1h async refresh keep rates far
// under this; the ceiling only trips when the rate pipeline has been dead long
// enough that locking a value becomes risky. Tunable via QUOTE_RATE_MAX_AGE_MINUTES.
func quoteRateMaxAge() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("QUOTE_RATE_MAX_AGE_MINUTES")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return time.Duration(n) * time.Minute
		}
	}
	return 6 * time.Hour
}

// ConvertAmountForQuote converts an amount using a rate bounded by the quote
// staleness ceiling. Unlike ConvertAmount it fails closed rather than locking a
// stale rate into a guest's crypto settlement quote.
func (s *ExchangeRateService) ConvertAmountForQuote(amount float64, fromCurrency, toCurrency string) (float64, error) {
	rate, err := s.GetFreshExchangeRate(fromCurrency, toCurrency, quoteRateMaxAge())
	if err != nil {
		return 0, err
	}
	return amount * rate, nil
}

// runRateFetchTick performs one rate-refresh pass with panic isolation. The
// periodic loop runs in its own goroutine, so a panic deep inside FetchLatestRates
// (e.g. a driver/GORM panic, or a malformed-upstream-body parse) would otherwise
// unwind the loop and permanently freeze rate refresh at stale values until the
// next process restart. logger.SafeTick contains the panic (logging it with a
// stack at Error level) and lets the loop continue to the next tick — the same
// per-tick isolation every other scheduler in this package already uses.
func runRateFetchTick(fetch func() error) {
	logger.SafeTick("exchange-rate-fetch", func() {
		if err := fetch(); err != nil {
			log.Printf("Periodic exchange rate fetch failed: %v", err)
		}
	})
}

// RunPeriodicFetch blocks, fetching rates once immediately and then every
// 5 minutes, until ctx is cancelled. Run it on a dbWorkerGroup so shutdown
// can stop the loop before the database pool closes.
func (s *ExchangeRateService) RunPeriodicFetch(ctx context.Context) {
	runPeriodic(ctx, 5*time.Minute, s.FetchLatestRates)
}

// runPeriodic calls fetch immediately and on every tick until ctx is done.
// A tick that arrives after cancellation does not start another fetch.
func runPeriodic(ctx context.Context, interval time.Duration, fetch func() error) {
	runRateFetchTick(fetch)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ctx.Err() != nil {
				return
			}
			runRateFetchTick(fetch)
		}
	}
}

// InitializeDefaultCurrencies adds default supported currencies to the database
func (s *ExchangeRateService) InitializeDefaultCurrencies() error {
	defaultCurrencies := []database.SupportedCurrency{
		{Code: "USD", Name: "US Dollar", Symbol: "$", IsActive: true},
		{Code: "EUR", Name: "Euro", Symbol: "€", IsActive: true},
		{Code: "GBP", Name: "British Pound", Symbol: "£", IsActive: true},
		{Code: "JPY", Name: "Japanese Yen", Symbol: "¥", IsActive: true},
		{Code: "AUD", Name: "Australian Dollar", Symbol: "A$", IsActive: true},
		{Code: "CAD", Name: "Canadian Dollar", Symbol: "C$", IsActive: true},
		{Code: "CHF", Name: "Swiss Franc", Symbol: "CHF", IsActive: true},
		{Code: "CNY", Name: "Chinese Yuan", Symbol: "¥", IsActive: true},
		{Code: "ARS", Name: "Argentine Peso", Symbol: "$", IsActive: true},
		{Code: "AED", Name: "UAE Dirham", Symbol: "د.إ", IsActive: true},
		{Code: "BRL", Name: "Brazilian Real", Symbol: "R$", IsActive: true},
		{Code: "MXN", Name: "Mexican Peso", Symbol: "$", IsActive: true},
		{Code: "INR", Name: "Indian Rupee", Symbol: "₹", IsActive: true},
		{Code: "KRW", Name: "South Korean Won", Symbol: "₩", IsActive: true},
		{Code: "SGD", Name: "Singapore Dollar", Symbol: "S$", IsActive: true},
		{Code: "HKD", Name: "Hong Kong Dollar", Symbol: "HK$", IsActive: true},
		{Code: "NOK", Name: "Norwegian Krone", Symbol: "kr", IsActive: true},
		{Code: "SEK", Name: "Swedish Krona", Symbol: "kr", IsActive: true},
		{Code: "DKK", Name: "Danish Krone", Symbol: "kr", IsActive: true},
		{Code: "PLN", Name: "Polish Zloty", Symbol: "zł", IsActive: true},
		{Code: "CLP", Name: "Chilean Peso", Symbol: "$", IsActive: true},
		{Code: "COP", Name: "Colombian Peso", Symbol: "$", IsActive: true},
		{Code: "PEN", Name: "Peruvian Sol", Symbol: "S/", IsActive: true},
		{Code: "UYU", Name: "Uruguayan Peso", Symbol: "$U", IsActive: true},
		{Code: "PYG", Name: "Paraguayan Guarani", Symbol: "₲", IsActive: true},
		{Code: "BOB", Name: "Bolivian Boliviano", Symbol: "Bs", IsActive: true},
		{Code: "CRC", Name: "Costa Rican Colon", Symbol: "₡", IsActive: true},
		{Code: "DOP", Name: "Dominican Peso", Symbol: "RD$", IsActive: true},
	}

	for _, currency := range defaultCurrencies {
		// Check if currency already exists
		var existing database.SupportedCurrency
		err := s.db.GetGorm().Where("code = ?", currency.Code).First(&existing).Error

		if err != nil {
			// Currency doesn't exist, create it
			if err := s.db.GetGorm().Create(&currency).Error; err != nil {
				log.Printf("Failed to create currency %s: %v", currency.Code, err)
			} else {
				log.Printf("Created currency: %s", currency.Code)
			}
		}
	}

	return nil
}
