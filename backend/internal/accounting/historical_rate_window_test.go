package accounting

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// seedHourlyRates inserts one rate per hour for each pair, ending at last.
func seedHourlyRates(tb testing.TB, db *gorm.DB, pairs [][2]string, hours int, last time.Time) {
	tb.Helper()
	batch := make([]database.ExchangeRate, 0, hours)
	for _, pair := range pairs {
		batch = batch[:0]
		for h := hours - 1; h >= 0; h-- {
			at := last.Add(-time.Duration(h) * time.Hour)
			batch = append(batch, database.ExchangeRate{
				FromCurrency: pair[0], ToCurrency: pair[1],
				Rate: 1000 + float64(h%97), Source: "test", FetchedAt: at, LastSeenAt: at,
			})
		}
		if err := db.CreateInBatches(batch, 500).Error; err != nil {
			tb.Fatal(err)
		}
	}
}

// The resolver must read only the summary window plus the latest rate at or
// before the window start per pair — never the pair's whole history.
func TestHistoricalRateResolver_LoadsOnlyWindowPlusAnchor(t *testing.T) {
	db := setupAccountingTestDB(t)
	service := NewService(database.GetDBWrapper())

	windowStart := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, time.March, 8, 0, 0, 0, 0, time.UTC)

	// One ancient anchor rate long before the window, then nothing until
	// mid-window, plus ancient noise for another pair.
	anchorAt := windowStart.AddDate(-1, 0, 0)
	require.NoError(t, db.Create(&database.ExchangeRate{FromCurrency: "ARS", ToCurrency: "USD", Rate: 0.001, FetchedAt: anchorAt.AddDate(0, 0, -1), LastSeenAt: anchorAt}).Error)
	require.NoError(t, db.Create(&database.ExchangeRate{FromCurrency: "ARS", ToCurrency: "USD", Rate: 0.002, FetchedAt: anchorAt, LastSeenAt: anchorAt}).Error)
	midWindow := windowStart.AddDate(0, 0, 3)
	require.NoError(t, db.Create(&database.ExchangeRate{FromCurrency: "ARS", ToCurrency: "USD", Rate: 0.004, FetchedAt: midWindow, LastSeenAt: midWindow}).Error)
	afterWindow := windowEnd.Add(time.Hour)
	require.NoError(t, db.Create(&database.ExchangeRate{FromCurrency: "ARS", ToCurrency: "USD", Rate: 9, FetchedAt: afterWindow, LastSeenAt: afterWindow}).Error)
	seedHourlyRates(t, db, [][2]string{{"USDC", "ARS"}}, 24*60, windowStart.Add(-48*time.Hour))

	var mu sync.Mutex
	var queries []string
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:capture_fx", func(tx *gorm.DB) {
		mu.Lock()
		queries = append(queries, tx.Statement.SQL.String())
		mu.Unlock()
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove("test:capture_fx") })

	resolver, err := service.newHistoricalRateResolverForCurrencies([]string{"ARS"}, "USD", windowStart, windowEnd)
	require.NoError(t, err)

	// Pre-window anchor still resolves the first days of the window.
	rate, ok := resolver.rateAtOrBefore("ARS", "USD", windowStart)
	require.True(t, ok)
	require.InDelta(t, 0.002, rate.Rate, 1e-9)
	rate, ok = resolver.rateAtOrBefore("ARS", "USD", midWindow.Add(time.Hour))
	require.True(t, ok)
	require.InDelta(t, 0.004, rate.Rate, 1e-9)
	usdc, ok := resolver.rateAtOrBefore("USDC", "ARS", windowStart)
	require.True(t, ok, "USDC pair anchored by its latest pre-window row")
	require.Equal(t, windowStart.Add(-48*time.Hour), usdc.FetchedAt.UTC())

	total := 0
	for _, timeline := range resolver.timelines {
		total += len(timeline)
	}
	require.Equal(t, 3, total, "only the window rows plus one anchor per pair are loaded; timelines=%v", resolver.timelines)
	for _, timeline := range resolver.timelines {
		for _, r := range timeline {
			require.False(t, r.FetchedAt.After(windowEnd), "no post-cutoff rows")
		}
	}

	mu.Lock()
	defer mu.Unlock()
	for _, q := range queries {
		lower := strings.ToLower(q)
		if !strings.Contains(lower, "exchange_rates") {
			continue
		}
		require.True(t, strings.Contains(lower, "fetched_at >") || strings.Contains(lower, "max(fetched_at)"),
			"every exchange_rates read must be window-bounded or a per-pair MAX anchor: %s", q)
	}
}

// BenchmarkHistoricalRateResolver_30DayWindow: two pairs with two years of
// hourly history (~35k rows), a 30-day summary window.
// Run: go test ./internal/accounting/ -run '^$' -bench BenchmarkHistoricalRateResolver_30DayWindow -benchmem -count=3
func BenchmarkHistoricalRateResolver_30DayWindow(b *testing.B) {
	dsn := fmt.Sprintf("file:bench_fx_window_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(db)
	if err := db.AutoMigrate(&database.ExchangeRate{}); err != nil {
		b.Fatal(err)
	}
	end := time.Date(2026, time.March, 31, 0, 0, 0, 0, time.UTC)
	start := end.AddDate(0, 0, -30)
	seedHourlyRates(b, db, [][2]string{{"ARS", "USD"}, {"USDC", "ARS"}}, 24*365*2, end)
	service := NewService(database.GetDBWrapper())

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := service.newHistoricalRateResolverForCurrencies([]string{"ARS"}, "USD", start, end); err != nil {
			b.Fatal(err)
		}
	}
}
