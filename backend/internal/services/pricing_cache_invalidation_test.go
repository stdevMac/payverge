package services

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestInvalidatePricingCache_DropsCachedSnapshot asserts that
// InvalidatePricingCache evicts a primed entry from the live (enabled) cache.
// The cache must remain ENABLED — disabling it would mask a missing call.
func TestInvalidatePricingCache_DropsCachedSnapshot(t *testing.T) {
	pricingCacheDisabled.Store(false)
	t.Cleanup(ResetPricingCache)
	ResetPricingCache()

	// Prime a snapshot directly (avoids needing a DB) so we can observe eviction.
	pricingCacheMu.Lock()
	pricingCache[7] = &pricingSnapshot{cachedAt: time.Now()}
	pricingCacheMu.Unlock()

	InvalidatePricingCache(7)

	pricingCacheMu.RLock()
	_, ok := pricingCache[7]
	pricingCacheMu.RUnlock()
	if ok {
		t.Fatal("InvalidatePricingCache did not evict the cached snapshot")
	}
}

// BenchmarkInvalidatePricingCache measures the cost of the hot-path O(1) map
// delete under a write lock. Baseline before wiring into menu write handlers.
func BenchmarkInvalidatePricingCache(b *testing.B) {
	pricingCacheDisabled.Store(false)
	b.Cleanup(ResetPricingCache)
	// Keep one entry alive so we always have something to delete.
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pricingCacheMu.Lock()
		pricingCache[99] = &pricingSnapshot{cachedAt: time.Now()}
		pricingCacheMu.Unlock()
		InvalidatePricingCache(99)
	}
}

// TestInvalidateBusinessCustomURL_DropsCustomURLEntry asserts that
// InvalidateBusinessCustomURL evicts a primed custom-URL entry.
// businessByCustomURLCache is now a boundedcache.Cache; this test uses its
// public Set/Get API rather than direct map access.
func TestInvalidateBusinessCustomURL_DropsCustomURLEntry(t *testing.T) {
	pricingCacheDisabled.Store(false)
	t.Cleanup(ResetPricingCache)
	ResetPricingCache()

	// Prime via the cache's public API.
	businessByCustomURLCache.Set("my-biz", &database.Business{})

	InvalidateBusinessCustomURL("my-biz")

	if _, ok := businessByCustomURLCache.Get("my-biz"); ok {
		t.Fatal("InvalidateBusinessCustomURL did not evict the cached entry")
	}
}

// TestInvalidatePublicGuestBusiness_DropsTableContextsAndExtras asserts that
// a business-level invalidation evicts that business's QR table contexts and
// guest extras, and leaves another business's entries alone.
func TestInvalidatePublicGuestBusiness_DropsTableContextsAndExtras(t *testing.T) {
	pricingCacheDisabled.Store(false)
	t.Cleanup(ResetPricingCache)
	ResetPricingCache()

	ctxFor := func(businessID uint) *cachedGuestTableContext {
		return &cachedGuestTableContext{
			table:    &database.Table{BusinessID: businessID},
			business: &database.Business{},
			cachedAt: time.Now(),
		}
	}
	guestTableContextByCodeCache.Set("T-A1", ctxFor(1))
	guestTableContextByCodeCache.Set("T-A2", ctxFor(1))
	guestTableContextByCodeCache.Set("T-B1", ctxFor(2))
	publicGuestBusinessExtrasMu.Lock()
	publicGuestBusinessExtras[1] = &cachedPublicGuestBusinessExtras{cachedAt: time.Now()}
	publicGuestBusinessExtras[2] = &cachedPublicGuestBusinessExtras{cachedAt: time.Now()}
	publicGuestBusinessExtrasMu.Unlock()

	InvalidatePublicGuestBusiness(1)

	for _, code := range []string{"T-A1", "T-A2"} {
		if _, ok := guestTableContextByCodeCache.Get(code); ok {
			t.Fatalf("table context %s survived its business invalidation", code)
		}
	}
	if _, ok := guestTableContextByCodeCache.Get("T-B1"); !ok {
		t.Fatal("another business's table context was evicted")
	}
	publicGuestBusinessExtrasMu.RLock()
	_, extrasA := publicGuestBusinessExtras[1]
	_, extrasB := publicGuestBusinessExtras[2]
	publicGuestBusinessExtrasMu.RUnlock()
	if extrasA {
		t.Fatal("guest extras survived their business invalidation")
	}
	if !extrasB {
		t.Fatal("another business's guest extras were evicted")
	}
}
