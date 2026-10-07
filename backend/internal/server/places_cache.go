package server

import (
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/boundedcache"

	"golang.org/x/sync/singleflight"
)

// Google Places response cache (SEC H-places). The public storefront and
// diner pages call /business/:customUrl/google/{reviews,details} on every
// render, and each call used to be a billed Place Details request
// (Atmosphere SKU) to maps.googleapis.com. An anonymous client could replay
// the public route, or vary ?language=, to run up the venue owner's Google
// bill. Reviews and ratings change slowly, so responses are cached per Place
// ID (and normalized language) for placesCacheTTL, failures are cached
// briefly so an upstream outage is not hammered, and concurrent misses for
// the same key share one upstream call.
//
// The cache is in-memory and per replica: N replicas make at most N upstream
// calls per key per TTL.
const (
	placesCacheTTL         = 12 * time.Hour
	placesNegativeCacheTTL = 5 * time.Minute
	placesCacheMaxEntries  = 4096
)

// placesLanguages is the set of language codes the Places API documents.
// Anything else collapses to its primary subtag or to "" (Google's default),
// so a caller cannot mint unbounded cache keys or upstream calls through the
// public ?language= parameter.
var placesLanguages = func() map[string]string {
	codes := []string{
		"af", "am", "ar", "az", "be", "bg", "bn", "bs", "ca", "cs", "da", "de",
		"el", "en", "en-AU", "en-GB", "es", "es-419", "et", "eu", "fa", "fi",
		"fil", "fr", "fr-CA", "gl", "gu", "hi", "hr", "hu", "hy", "id", "is",
		"it", "iw", "ja", "ka", "kk", "km", "kn", "ko", "ky", "lo", "lt", "lv",
		"mk", "ml", "mn", "mr", "ms", "my", "ne", "nl", "no", "pa", "pl", "pt",
		"pt-BR", "pt-PT", "ro", "ru", "si", "sk", "sl", "sq", "sr", "sv", "sw",
		"ta", "te", "th", "tr", "uk", "ur", "uz", "vi", "zh", "zh-CN", "zh-HK",
		"zh-TW", "zu",
	}
	m := make(map[string]string, len(codes)+2)
	for _, c := range codes {
		m[strings.ToLower(c)] = c
	}
	// Common aliases Google accepts under another code.
	m["he"] = "iw"
	m["nb"] = "no"
	return m
}()

// normalizePlacesLanguage maps a client-supplied language tag onto the
// bounded set above. "es-AR" becomes "es", "PT_br" becomes "pt-BR", and
// anything unknown or oversized becomes "".
func normalizePlacesLanguage(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 35 {
		return ""
	}
	tag := strings.ToLower(strings.ReplaceAll(raw, "_", "-"))
	if canon, ok := placesLanguages[tag]; ok {
		return canon
	}
	if primary, _, found := strings.Cut(tag, "-"); found {
		if canon, ok := placesLanguages[primary]; ok {
			return canon
		}
	}
	return ""
}

type placesCacheEntry struct {
	reviews []any
	details any
	err     error
}

// cachedPlacesService decorates a GooglePlacesService with two bounded
// caches: successes for the long TTL, failures for the short one.
type cachedPlacesService struct {
	inner    GooglePlacesService
	hits     *boundedcache.Cache[string, placesCacheEntry]
	failures *boundedcache.Cache[string, placesCacheEntry]
	group    singleflight.Group
}

func newCachedPlacesService(inner GooglePlacesService) *cachedPlacesService {
	return newCachedPlacesServiceWithTTL(inner, placesCacheTTL, placesNegativeCacheTTL, placesCacheMaxEntries)
}

func newCachedPlacesServiceWithTTL(inner GooglePlacesService, ttl, negativeTTL time.Duration, maxEntries int) *cachedPlacesService {
	return &cachedPlacesService{
		inner:    inner,
		hits:     boundedcache.New[string, placesCacheEntry](maxEntries, ttl),
		failures: boundedcache.New[string, placesCacheEntry](maxEntries, negativeTTL),
	}
}

func (s *cachedPlacesService) GetPlaceReviews(placeID, language string) ([]any, error) {
	lang := normalizePlacesLanguage(language)
	e := s.load("reviews|"+placeID+"|"+lang, func() placesCacheEntry {
		reviews, err := s.inner.GetPlaceReviews(placeID, lang)
		return placesCacheEntry{reviews: reviews, err: err}
	})
	if e.err != nil {
		return nil, e.err
	}
	// Handlers only serialize the slice; hand out a copy so no caller can
	// mutate the cached backing array.
	return append([]any(nil), e.reviews...), nil
}

func (s *cachedPlacesService) GetPlaceDetails(placeID string) (any, error) {
	e := s.load("details|"+placeID, func() placesCacheEntry {
		details, err := s.inner.GetPlaceDetails(placeID)
		return placesCacheEntry{details: details, err: err}
	})
	if e.err != nil {
		return nil, e.err
	}
	return e.details, nil
}

func (s *cachedPlacesService) cached(key string) (placesCacheEntry, bool) {
	if e, ok := s.hits.Get(key); ok {
		return e, true
	}
	return s.failures.Get(key)
}

func (s *cachedPlacesService) load(key string, fetch func() placesCacheEntry) placesCacheEntry {
	if e, ok := s.cached(key); ok {
		return e
	}
	v, _, _ := s.group.Do(key, func() (any, error) {
		// A caller that arrived just after the previous flight finished finds
		// the fresh entry here instead of making a second upstream call.
		if e, ok := s.cached(key); ok {
			return e, nil
		}
		e := fetch()
		if e.err != nil {
			s.failures.Set(key, e)
		} else {
			s.failures.Invalidate(key)
			s.hits.Set(key, e)
		}
		return e, nil
	})
	return v.(placesCacheEntry)
}
