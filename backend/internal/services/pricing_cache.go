package services

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/stdevmac/payverge/backend/internal/boundedcache"
	"github.com/stdevmac/payverge/backend/internal/database"
)

// pricingSnapshot holds everything PriceOrderInputsByBusinessID needs to
// price an order. Reads on the snapshot are safe; callers must NOT mutate
// the slices (they are shared across requests).
type pricingSnapshot struct {
	business   *database.Business
	menu       *database.Menu
	categories []database.MenuCategory
	offers     []database.Offer
	bundles    []database.Bundle
	cachedAt   time.Time
	// gen is the business's pricing generation when the load started. A
	// snapshot whose gen is behind the current one was loaded before an
	// invalidation and must neither be cached nor served.
	gen uint64
}

// PricingCacheTTL bounds how long an order-pricing snapshot (and the guest
// read models that share it) can be reused before it's refetched. Freshness
// does not rest on the TTL: menu, offer, bundle, business and table writes
// call InvalidatePricingCache / InvalidatePublicGuestBusiness, so operators
// see their edits immediately. Twenty seconds only bounds changes no hook
// observes (schedule boundaries, inventory 86s, hours flips) while letting a
// dinner-rush of guest polls reuse one DB fan-out.
const PricingCacheTTL = 20 * time.Second

// pricingSF coalesces concurrent cache misses so a thundering herd at TTL
// expiry runs exactly one DB fan-out per key, not one per goroutine. The
// group is package-level so it spans the lifetime of the process.
var pricingSF singleflight.Group

var (
	pricingCacheMu       sync.RWMutex
	pricingCache         = make(map[uint]*pricingSnapshot)
	pricingCacheDisabled atomic.Bool

	// businessByCustomURLCache is a bounded LRU + TTL cache for custom-URL → Business
	// lookups. The 5000-entry cap prevents an attacker from churning unbounded
	// slug variants into heap. Backed by boundedcache so both the LRU eviction
	// and the TTL expiry are handled automatically.
	businessByCustomURLCache = boundedcache.New[string, *database.Business](5000, PricingCacheTTL)

	// guestTableContextByCodeCache is a bounded LRU + TTL cache for QR table
	// codes. The 5000-entry cap matches the custom-URL cache for the same
	// attacker-churn reason.
	guestTableContextByCodeCache = boundedcache.New[string, *cachedGuestTableContext](5000, PricingCacheTTL)

	publicGuestBusinessExtrasMu sync.RWMutex
	publicGuestBusinessExtras   = make(map[uint]*cachedPublicGuestBusinessExtras)

	// guestValidatorMemo remembers the ETag a guest menu/table response was
	// last served with, so a conditional poll whose If-None-Match still
	// matches can answer 304 before loading the menu snapshot, promotions,
	// inventory orderability or translations. Entries share PricingCacheTTL
	// and are dropped by the same write hooks as the snapshots they summarize.
	guestValidatorMemo = boundedcache.New[string, *guestValidatorEntry](20000, PricingCacheTTL)
)

type guestValidatorEntry struct {
	etag              string
	businessID        uint
	businessUpdatedAt time.Time
	gen               uint64
}

// pricingGen counts invalidations per business (guarded by pricingCacheMu).
// A load or a remembered validator stamped with an older generation raced an
// invalidation: the write it missed is not reflected in what it holds.
var pricingGen = make(map[uint]uint64)

// PricingGeneration returns the business's current pricing generation.
// Capture it before loading guest/pricing data; RememberGuestValidator
// refuses to remember a validator computed under an older generation.
func PricingGeneration(businessID uint) uint64 {
	pricingCacheMu.RLock()
	defer pricingCacheMu.RUnlock()
	return pricingGen[businessID]
}

// pricingSnapshotLoadedHook, when set (tests only), runs after a snapshot's
// DB reads and before it is stored, so a test can land an invalidation in
// the load-to-store window.
var pricingSnapshotLoadedHook func(businessID uint)

type cachedGuestTableContext struct {
	table    *database.Table
	business *database.Business
	cachedAt time.Time
}

// PublicGuestBusinessExtras is the read model behind
// GET /guest/table/:code/business metadata. It intentionally groups
// low-churn language/plugin fields so repeated guest page loads can reuse one
// short-lived snapshot.
type PublicGuestBusinessExtras struct {
	BusinessLanguages   []database.BusinessLanguage
	SupportedLanguages  []database.SupportedLanguage
	TrustpilotEnabled   bool
	TrustpilotReviewURL string
}

type cachedPublicGuestBusinessExtras struct {
	extras   PublicGuestBusinessExtras
	cachedAt time.Time
}

func init() {
	// Tests reset the DB between cases via database.SetTestDB. The cached
	// snapshot is invalid against a fresh database, so wipe it on swap.
	database.RegisterOnDBChange(ResetPricingCache)
}

func getPricingSnapshot(businessID uint) (*pricingSnapshot, error) {
	return getPricingSnapshotFrom(businessID, nil)
}

// getPricingSnapshotFrom returns the cached menu and promotion snapshot.
// known, when it is already the businessID row, is copied into the snapshot
// instead of issuing another SELECT against businesses.
func getPricingSnapshotFrom(businessID uint, known *database.Business) (*pricingSnapshot, error) {
	// A coalesced flight may have started before an invalidation; its result
	// is stale for this caller. Retry so the returned snapshot reflects every
	// invalidation that happened before the call returned (bounded: a
	// business under continuous writes gets the newest load we saw).
	var snap *pricingSnapshot
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		snap, err = loadPricingSnapshot(businessID, known)
		if err != nil || snap.gen == PricingGeneration(businessID) {
			return snap, err
		}
	}
	return snap, err
}

func loadPricingSnapshot(businessID uint, known *database.Business) (*pricingSnapshot, error) {
	if !pricingCacheDisabled.Load() {
		pricingCacheMu.RLock()
		snap, ok := pricingCache[businessID]
		pricingCacheMu.RUnlock()
		if ok && time.Since(snap.cachedAt) < PricingCacheTTL {
			return snap, nil
		}
	}

	// Single-flight: coalesce concurrent misses for the same businessID so
	// only one DB fan-out runs regardless of burst size.
	key := "snap:" + uint2str(businessID)
	v, err, _ := pricingSF.Do(key, func() (interface{}, error) {
		// Double-check inside the flight: a prior flight for this key may have
		// just populated the cache and released the key before this (late)
		// caller entered Do, so re-read before fetching to keep the burst to one
		// DB fan-out.
		if !pricingCacheDisabled.Load() {
			pricingCacheMu.RLock()
			snap, ok := pricingCache[businessID]
			pricingCacheMu.RUnlock()
			if ok && time.Since(snap.cachedAt) < PricingCacheTTL {
				return snap, nil
			}
		}
		gen := PricingGeneration(businessID)
		var business *database.Business
		if known != nil && known.ID != 0 && known.ID == businessID {
			copied := *known
			business = &copied
		} else {
			loaded, err := database.GetBusinessByID(businessID)
			if err != nil {
				return nil, err
			}
			business = loaded
		}
		menu, categories, err := database.GetMenuByBusinessID(businessID)
		if err != nil {
			// Treat menu-not-found as an empty menu so the caller can still
			// serve a guest with no items; it is a real product state.
			menu = &database.Menu{BusinessID: businessID, Categories: "[]"}
			categories = []database.MenuCategory{}
		}
		offers, bundles := LoadActivePromotionsForBusiness(business)

		snap := &pricingSnapshot{
			business:   business,
			menu:       menu,
			categories: categories,
			offers:     offers,
			bundles:    bundles,
			cachedAt:   time.Now(),
			gen:        gen,
		}
		if pricingSnapshotLoadedHook != nil {
			pricingSnapshotLoadedHook(businessID)
		}

		if !pricingCacheDisabled.Load() {
			pricingCacheMu.Lock()
			reapStalePricingSnapshots()
			// An invalidation during the load means these rows may predate
			// the write; do not let them repopulate the cache.
			if pricingGen[businessID] == gen {
				pricingCache[businessID] = snap
			}
			pricingCacheMu.Unlock()
		}

		return snap, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*pricingSnapshot), nil
}

// reapStalePricingSnapshots removes expired entries from pricingCache.
// Must be called with pricingCacheMu held for writing.
func reapStalePricingSnapshots() {
	now := time.Now()
	for id, s := range pricingCache {
		if now.Sub(s.cachedAt) >= PricingCacheTTL {
			delete(pricingCache, id)
		}
	}
}

// reapStalePublicGuestBusinessExtras removes expired entries.
// Must be called with publicGuestBusinessExtrasMu held for writing.
func reapStalePublicGuestBusinessExtras() {
	now := time.Now()
	for id, e := range publicGuestBusinessExtras {
		if now.Sub(e.cachedAt) >= PricingCacheTTL {
			delete(publicGuestBusinessExtras, id)
		}
	}
}

// uint2str converts a uint to its decimal string representation without
// importing strconv (avoids a dependency in the hot path — fmt.Sprintf is
// acceptable but adds an alloc; this avoids it).
func uint2str(n uint) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

// MenuDataForBusiness returns cached menu/promotion data for a business.
// Read-heavy guest endpoints (e.g. GET /business/:customUrl/menu) can use
// this to avoid hitting Postgres for menu + offers + bundles on every
// request. The cache TTL is PricingCacheTTL.
func MenuDataForBusiness(businessID uint) (*database.Menu, []database.MenuCategory, []database.Offer, []database.Bundle, error) {
	snap, err := getPricingSnapshot(businessID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return snap.menu, snap.categories, snap.offers, snap.bundles, nil
}

// InvalidatePricingCache evicts the cached snapshot for a single business.
// Call this after any write that affects pricing inputs: menu edits, offer
// or bundle CRUD, business profile changes (timezone, currency).
func InvalidatePricingCache(businessID uint) {
	pricingCacheMu.Lock()
	delete(pricingCache, businessID)
	pricingCacheMu.Unlock()
	invalidateGuestValidators(businessID)
}

// invalidateGuestValidators bumps the business's generation (so in-flight
// loads cannot re-remember a pre-write validator) and drops its memo entries.
func invalidateGuestValidators(businessID uint) {
	pricingCacheMu.Lock()
	pricingGen[businessID]++
	pricingCacheMu.Unlock()
	guestValidatorMemo.InvalidateFunc(func(entry *guestValidatorEntry) bool {
		return entry != nil && entry.businessID == businessID
	})
}

// RememberGuestValidator records the ETag a complete guest menu/table body
// was served with under key (endpoint + business/table + language). Callers
// must not remember validators for responses they mark no-store. gen is the
// PricingGeneration captured before the body's data was loaded; a validator
// computed across an invalidation is not remembered.
func RememberGuestValidator(key string, businessID uint, businessUpdatedAt time.Time, gen uint64, etag string) {
	if pricingCacheDisabled.Load() || key == "" || etag == "" {
		return
	}
	if gen != PricingGeneration(businessID) {
		return
	}
	guestValidatorMemo.EvictExpired()
	guestValidatorMemo.Set(key, &guestValidatorEntry{etag: etag, businessID: businessID, businessUpdatedAt: businessUpdatedAt, gen: gen})
}

// GuestValidator returns the remembered ETag for key while it is fresh and the
// business row it was computed from has not changed since.
func GuestValidator(key string, businessID uint, businessUpdatedAt time.Time) (string, bool) {
	if pricingCacheDisabled.Load() || key == "" {
		return "", false
	}
	entry, ok := guestValidatorMemo.Get(key)
	if !ok || entry == nil || entry.businessID != businessID || !entry.businessUpdatedAt.Equal(businessUpdatedAt) {
		return "", false
	}
	if entry.gen != PricingGeneration(businessID) {
		return "", false
	}
	return entry.etag, true
}

// ResetPricingCache clears every cached entry. Used by SetTestDB and
// available to test helpers that need a known starting state.
func ResetPricingCache() {
	pricingCacheMu.Lock()
	pricingCache = make(map[uint]*pricingSnapshot)
	pricingGen = make(map[uint]uint64)
	pricingCacheMu.Unlock()

	businessByCustomURLCache = boundedcache.New[string, *database.Business](5000, PricingCacheTTL)
	guestTableContextByCodeCache = boundedcache.New[string, *cachedGuestTableContext](5000, PricingCacheTTL)
	guestValidatorMemo = boundedcache.New[string, *guestValidatorEntry](20000, PricingCacheTTL)

	// Reset the single-flight group too: a DB swap (or a -count>1 test run)
	// must not let an in-flight key from the prior state coalesce with a fresh
	// load, which otherwise produces a flaky double-fetch.
	pricingSF = singleflight.Group{}

	publicGuestBusinessExtrasMu.Lock()
	publicGuestBusinessExtras = make(map[uint]*cachedPublicGuestBusinessExtras)
	publicGuestBusinessExtrasMu.Unlock()
}

// BusinessByCustomURL returns a (possibly cached) Business looked up by its
// public customURL slug. The menu/storefront read paths hit this on every
// request, so caching the lookup for PricingCacheTTL trims the last DB
// query off the menu-browse hot path.
//
// The fetcher argument is the DB-backed loader to call on cache miss;
// callers pass database.GetBusinessByCustomURL (kept as an argument to
// avoid an import cycle inside the database package).
func BusinessByCustomURL(customURL string, fetcher func(string) (*database.Business, error)) (*database.Business, error) {
	if !pricingCacheDisabled.Load() {
		if biz, ok := businessByCustomURLCache.Get(customURL); ok {
			return biz, nil
		}
	}

	// Single-flight: coalesce concurrent misses for the same customURL so
	// only one DB lookup runs regardless of burst size.
	v, err, _ := pricingSF.Do("cu:"+customURL, func() (interface{}, error) {
		// Double-check inside the flight: a prior flight may have populated the
		// cache and released the key before this late caller entered Do.
		if !pricingCacheDisabled.Load() {
			if biz, ok := businessByCustomURLCache.Get(customURL); ok {
				return biz, nil
			}
		}
		business, err := fetcher(customURL)
		if err != nil {
			return nil, err
		}
		if !pricingCacheDisabled.Load() {
			// EvictExpired removes stale entries before storing the new one,
			// bounding the cache to live keys (reap-on-write).
			businessByCustomURLCache.EvictExpired()
			businessByCustomURLCache.Set(customURL, business)
		}
		return business, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*database.Business), nil
}

// InvalidateBusinessCustomURL evicts the cached lookup for a customURL.
// Call when the business profile changes — particularly when the slug
// itself is updated or when BusinessPageEnabled / IsActive flips.
func InvalidateBusinessCustomURL(customURL string) {
	businessByCustomURLCache.Invalidate(customURL)
}

// PublicGuestTableContextByCode caches the table+business context behind QR
// table loads for the same short TTL as pricing data. This avoids repeated
// table/business reads during guest bursts while keeping operator changes
// visible quickly.
func PublicGuestTableContextByCode(code string, fetcher func(string) (*database.Table, *database.Business, error)) (*database.Table, *database.Business, error) {
	if !pricingCacheDisabled.Load() {
		if entry, ok := guestTableContextByCodeCache.Get(code); ok {
			return entry.table, entry.business, nil
		}
	}

	// Single-flight: coalesce concurrent misses for the same QR code.
	v, err, _ := pricingSF.Do("tc:"+code, func() (interface{}, error) {
		// Double-check inside the flight: a prior flight may have populated the
		// cache and released the key before this late caller entered Do.
		if !pricingCacheDisabled.Load() {
			if entry, ok := guestTableContextByCodeCache.Get(code); ok {
				return entry, nil
			}
		}
		table, business, err := fetcher(code)
		if err != nil {
			return nil, err
		}
		ctx := &cachedGuestTableContext{table: table, business: business, cachedAt: time.Now()}
		if !pricingCacheDisabled.Load() {
			// EvictExpired removes stale entries before storing the new one,
			// bounding the cache to live keys (reap-on-write).
			guestTableContextByCodeCache.EvictExpired()
			guestTableContextByCodeCache.Set(code, ctx)
		}
		return ctx, nil
	})
	if err != nil {
		return nil, nil, err
	}
	ctx := v.(*cachedGuestTableContext)
	return ctx.table, ctx.business, nil
}

// InvalidatePublicGuestBusiness evicts every public guest read model for a
// business: the QR table contexts (table + business rows) of all its tables
// and its guest business extras (languages, Trustpilot). Call it after a write
// to the business row, its tables, its languages or its plugins, so guests do
// not see the old state for up to PricingCacheTTL.
func InvalidatePublicGuestBusiness(businessID uint) {
	invalidateGuestValidators(businessID)
	guestTableContextByCodeCache.InvalidateFunc(func(ctx *cachedGuestTableContext) bool {
		return ctx != nil && ctx.table != nil && ctx.table.BusinessID == businessID
	})
	publicGuestBusinessExtrasMu.Lock()
	delete(publicGuestBusinessExtras, businessID)
	publicGuestBusinessExtrasMu.Unlock()
}

// CachedPublicGuestBusinessExtras caches low-churn public guest business
// metadata for the same short TTL as menu/table snapshots.
func CachedPublicGuestBusinessExtras(businessID uint, fetcher func(uint) (PublicGuestBusinessExtras, error)) (PublicGuestBusinessExtras, error) {
	if !pricingCacheDisabled.Load() {
		publicGuestBusinessExtrasMu.RLock()
		entry, ok := publicGuestBusinessExtras[businessID]
		publicGuestBusinessExtrasMu.RUnlock()
		if ok && time.Since(entry.cachedAt) < PricingCacheTTL {
			return entry.extras, nil
		}
	}

	// Single-flight: coalesce concurrent misses for the same businessID.
	key := "extras:" + uint2str(businessID)
	v, err, _ := pricingSF.Do(key, func() (interface{}, error) {
		// Double-check inside the flight: a prior flight may have populated the
		// cache and released the key before this late caller entered Do.
		if !pricingCacheDisabled.Load() {
			publicGuestBusinessExtrasMu.RLock()
			entry, ok := publicGuestBusinessExtras[businessID]
			publicGuestBusinessExtrasMu.RUnlock()
			if ok && time.Since(entry.cachedAt) < PricingCacheTTL {
				return entry.extras, nil
			}
		}
		extras, err := fetcher(businessID)
		if err != nil {
			return nil, err
		}
		if !pricingCacheDisabled.Load() {
			publicGuestBusinessExtrasMu.Lock()
			reapStalePublicGuestBusinessExtras()
			publicGuestBusinessExtras[businessID] = &cachedPublicGuestBusinessExtras{
				extras:   extras,
				cachedAt: time.Now(),
			}
			publicGuestBusinessExtrasMu.Unlock()
		}
		return extras, nil
	})
	if err != nil {
		return PublicGuestBusinessExtras{}, err
	}
	return v.(PublicGuestBusinessExtras), nil
}

// DisablePricingCacheForTest turns the order-pricing caches off for one test,
// so every read hits the DB, and restores the previous state on cleanup.
func DisablePricingCacheForTest(tb testing.TB) {
	tb.Helper()
	prev := pricingCacheDisabled.Swap(true)
	tb.Cleanup(func() { pricingCacheDisabled.Store(prev) })
}
