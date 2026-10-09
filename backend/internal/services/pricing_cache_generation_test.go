package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupPricingGenerationTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	// No menu/offer/bundle tables: the snapshot loader treats those read
	// errors as an empty menu, which is all this race test needs.
	database.SetTestDB(db)
	pricingCacheDisabled.Store(false)
	ResetPricingCache()
	t.Cleanup(func() {
		pricingSnapshotLoadedHook = nil
		ResetPricingCache()
	})
}

// A menu write that lands while a snapshot is loading must not be undone by
// that load: the pre-write rows are neither cached nor returned.
func TestPricingSnapshot_InvalidationDuringLoadIsNotCached(t *testing.T) {
	setupPricingGenerationTestDB(t)
	const businessID = 41
	known := &database.Business{ID: businessID}

	loads := 0
	pricingSnapshotLoadedHook = func(id uint) {
		loads++
		if loads == 1 {
			InvalidatePricingCache(id) // operator saves a price mid-load
		}
	}

	snap, err := getPricingSnapshotFrom(businessID, known)
	require.NoError(t, err)
	require.Equal(t, 2, loads, "the stale load must be retried")
	require.Equal(t, PricingGeneration(businessID), snap.gen, "returned snapshot must postdate the invalidation")

	pricingCacheMu.RLock()
	cached, ok := pricingCache[businessID]
	pricingCacheMu.RUnlock()
	require.True(t, ok)
	require.Same(t, snap, cached, "only the post-invalidation load may be cached")
}

// A guest validator computed from data loaded before an invalidation must not
// be remembered, and one remembered before an invalidation must not answer.
func TestGuestValidator_RejectsValidatorAcrossInvalidation(t *testing.T) {
	pricingCacheDisabled.Store(false)
	ResetPricingCache()
	t.Cleanup(ResetPricingCache)

	const businessID = 42
	updatedAt := time.Unix(1_700_000_000, 0)

	// Load started, then the operator wrote, then the handler remembers.
	gen := PricingGeneration(businessID)
	InvalidatePricingCache(businessID)
	RememberGuestValidator("menu|x|en", businessID, updatedAt, gen, `W/"old"`)
	_, ok := GuestValidator("menu|x|en", businessID, updatedAt)
	require.False(t, ok, "a validator computed across an invalidation must not be memoized")

	// Fresh generation: remembered and served.
	gen = PricingGeneration(businessID)
	RememberGuestValidator("menu|x|en", businessID, updatedAt, gen, `W/"new"`)
	etag, ok := GuestValidator("menu|x|en", businessID, updatedAt)
	require.True(t, ok)
	require.Equal(t, `W/"new"`, etag)

	// A guest-business invalidation also retires it.
	InvalidatePublicGuestBusiness(businessID)
	_, ok = GuestValidator("menu|x|en", businessID, updatedAt)
	require.False(t, ok)
}
