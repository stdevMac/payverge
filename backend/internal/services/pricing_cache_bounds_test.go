package services

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestBusinessByCustomURLSingleFlight(t *testing.T) {
	ResetPricingCache()
	var calls int64
	start := make(chan struct{})
	fetch := func(u string) (*database.Business, error) {
		atomic.AddInt64(&calls, 1)
		<-start // hold all concurrent misses open
		return &database.Business{}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = BusinessByCustomURL("acme", fetch) }()
	}
	close(start)
	wg.Wait()
	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Fatalf("expected 1 fetch under concurrent miss (single-flight), got %d", got)
	}
}

func TestGuestTableContextCacheEvictsExpiredOnWrite(t *testing.T) {
	ResetPricingCache()
	// plant an entry with an expiresAt in the past so it is already stale
	seedStaleGuestTableContext("old-code")
	// trigger a fresh store for a different code; reap-on-write must clear stale entries
	_, _, _ = PublicGuestTableContextByCode("new-code", func(string) (*database.Table, *database.Business, error) {
		return &database.Table{}, &database.Business{}, nil
	})
	if guestTableContextCacheLen() > 1 {
		t.Fatalf("stale entry not reaped, len=%d", guestTableContextCacheLen())
	}
}

// BenchmarkBusinessByCustomURLConcurrentMiss fires 32 concurrent misses per
// iteration and records total DB fetches. With single-flight, a burst of
// concurrent misses for the same key produces far fewer than 32 fetches per
// iteration — the metric tracks coalescing efficiency, not correctness.
//
// Before single-flight: fetch-count/op ≈ 32.
// After single-flight:  fetch-count/op ≈ 1 (bounded by singleflight window).
func BenchmarkBusinessByCustomURLConcurrentMiss(b *testing.B) {
	var totalCalls int64
	fetcher := func(u string) (*database.Business, error) {
		atomic.AddInt64(&totalCalls, 1)
		return &database.Business{}, nil
	}

	b.ResetTimer()
	b.ReportAllocs()
	var iterFetches int64
	for i := 0; i < b.N; i++ {
		// Cold miss: evict the entry so every iteration exercises the miss path.
		ResetPricingCache()
		before := atomic.LoadInt64(&totalCalls)

		const concurrency = 32
		var wg sync.WaitGroup
		start := make(chan struct{})
		for j := 0; j < concurrency; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, _ = BusinessByCustomURL("bench-biz", fetcher)
			}()
		}
		close(start)
		wg.Wait()

		iterFetches += atomic.LoadInt64(&totalCalls) - before
	}
	// Report average fetches per iteration. With single-flight this is close to 1.
	b.ReportMetric(float64(iterFetches)/float64(b.N), "db-fetches/op")
}

// seedStaleGuestTableContext plants an expired entry into the guest-table-context
// cache. It uses SetAt with an expiresAt already in the past, so the entry is
// considered stale by EvictExpired. Test-only; touches package-level state.
func seedStaleGuestTableContext(code string) {
	past := time.Now().Add(-2 * PricingCacheTTL)
	guestTableContextByCodeCache.SetAt(
		code,
		&cachedGuestTableContext{
			table:    &database.Table{},
			business: &database.Business{},
			cachedAt: past,
		},
		past, // expiresAt is already elapsed
	)
}

// guestTableContextCacheLen returns the live entry count for assertion in tests.
func guestTableContextCacheLen() int {
	return guestTableContextByCodeCache.Len()
}
