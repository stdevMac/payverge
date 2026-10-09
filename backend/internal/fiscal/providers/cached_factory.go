package providers

import (
	"context"
	"fmt"
	"sync"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal"
)

// CachingFactory memoizes the underlying ProviderFactory.Build result per
// (settingsID, credentials fingerprint) so a single WSAAClient/token (held inside
// the constructed Provider) is reused across many fiscal jobs for the same
// business.
//
// AFIP/WSAA refuses to issue a fresh login ticket while a prior TA is still
// valid, so rebuilding per job both wastes a cert-CMS sign + SOAP round-trip
// and triggers "ya posee un TA valido" rejections.
//
// WSAA token refresh: the cached Provider's WSAAClient manages its own token
// lifecycle (caches until 5 minutes before expiry, coalesces concurrent
// refreshes via singleflight). The factory cache only gates Provider
// construction — once a Provider is cached, its internal WSAAClient handles
// all subsequent token refreshes transparently.
//
// Concurrency: Build is safe for concurrent use. A read-lock fast-path avoids
// lock contention on the hot cached-hit path; the write-lock is only acquired
// when a new entry is stored.
type CachingFactory struct {
	inner fiscal.ProviderFactory
	mu    sync.RWMutex
	cache map[uint]cachedEntry
}

type cachedEntry struct {
	fingerprint string
	provider    fiscal.Provider
}

// compile-time assertion: *CachingFactory must satisfy fiscal.ProviderFactory.
var _ fiscal.ProviderFactory = (*CachingFactory)(nil)

// NewCachingFactory wraps inner so that identical (settingsID, fingerprint)
// calls share one Provider instance. Rows with an empty fingerprint are never
// cached (fallthrough to inner every call) so legacy/unconfigured rows never
// serve stale credentials.
func NewCachingFactory(inner fiscal.ProviderFactory) *CachingFactory {
	return &CachingFactory{inner: inner, cache: make(map[uint]cachedEntry)}
}

// Build returns a cached Provider when settings.ID and
// settings.CredentialsFingerprint match a prior call; otherwise it delegates to
// the wrapped inner factory and memoizes the result for future calls.
func (f *CachingFactory) Build(ctx context.Context, settings *database.BusinessFiscalSettings) (fiscal.Provider, error) {
	if settings == nil {
		return nil, fmt.Errorf("fiscal: nil settings passed to CachingFactory")
	}

	key := settings.ID
	fp := settings.CredentialsFingerprint

	// Fast path: return the cached Provider without a write-lock when the
	// (settingsID, fingerprint) pair is already memoized. Empty fingerprint
	// skips the cache entirely — see NewCachingFactory doc.
	if fp != "" {
		f.mu.RLock()
		if ent, ok := f.cache[key]; ok && ent.fingerprint == fp {
			p := ent.provider
			f.mu.RUnlock()
			return p, nil
		}
		f.mu.RUnlock()
	}

	// Cache miss: delegate to the inner factory. This may happen concurrently
	// for the same key (two goroutines both pass the read check before either
	// stores). That is acceptable — the extra build is bounded and correct
	// because both calls produce equivalent Providers for the same fingerprint.
	provider, err := f.inner.Build(ctx, settings)
	if err != nil {
		return nil, err
	}

	// Only memoize when we have a stable fingerprint to invalidate on. An empty
	// fingerprint (legacy rows) always re-delegates to inner so credentials can
	// never go stale on those rows.
	if fp != "" {
		f.mu.Lock()
		f.cache[key] = cachedEntry{fingerprint: fp, provider: provider}
		f.mu.Unlock()
	}
	return provider, nil
}
