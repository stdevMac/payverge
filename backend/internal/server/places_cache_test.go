package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// countingPlaces is an upstream stub that counts calls and records the
// language each reviews call was made with.
type countingPlaces struct {
	reviews, details atomic.Int64
	mu               sync.Mutex
	langs            []string
	err              atomic.Pointer[error]
	gate             chan struct{} // when set, upstream calls block on it
}

func (p *countingPlaces) fail(err error) { p.err.Store(&err) }
func (p *countingPlaces) heal()          { p.err.Store(nil) }

func (p *countingPlaces) GetPlaceReviews(placeID, language string) ([]any, error) {
	p.reviews.Add(1)
	p.mu.Lock()
	p.langs = append(p.langs, language)
	p.mu.Unlock()
	if p.gate != nil {
		<-p.gate
	}
	if e := p.err.Load(); e != nil {
		return nil, *e
	}
	return []any{services.PlaceReview{AuthorName: "Ada", Rating: 5, Text: "lovely (" + language + ")"}}, nil
}

func (p *countingPlaces) GetPlaceDetails(placeID string) (any, error) {
	p.details.Add(1)
	if p.gate != nil {
		<-p.gate
	}
	if e := p.err.Load(); e != nil {
		return nil, *e
	}
	return &services.PlaceDetails{PlaceID: placeID, Name: "Venue", Rating: 4.6, UserRatingsTotal: 120}, nil
}

func TestCachedPlaces_RepeatReadsHitTheCache(t *testing.T) {
	up := &countingPlaces{}
	svc := newCachedPlacesService(up)
	for i := 0; i < 50; i++ {
		d, err := svc.GetPlaceDetails("place-a")
		require.NoError(t, err)
		require.Equal(t, "place-a", d.(*services.PlaceDetails).PlaceID)
		r, err := svc.GetPlaceReviews("place-a", "en")
		require.NoError(t, err)
		require.Len(t, r, 1)
	}
	require.EqualValues(t, 1, up.details.Load(), "details must be fetched once per place per TTL")
	require.EqualValues(t, 1, up.reviews.Load(), "reviews must be fetched once per place+language per TTL")

	// Another Place ID is its own key.
	_, err := svc.GetPlaceDetails("place-b")
	require.NoError(t, err)
	require.EqualValues(t, 2, up.details.Load())

	// A caller mutating the returned slice cannot poison the cache.
	r, _ := svc.GetPlaceReviews("place-a", "en")
	r[0] = "poisoned"
	r2, _ := svc.GetPlaceReviews("place-a", "en")
	require.IsType(t, services.PlaceReview{}, r2[0])
}

// The public ?language= parameter must not mint unbounded cache keys or
// upstream calls.
func TestCachedPlaces_LanguageIsNormalizedBeforeKeying(t *testing.T) {
	up := &countingPlaces{}
	svc := newCachedPlacesService(up)
	for _, l := range []string{"es", "ES", "es-AR", "es_ar", " es "} {
		_, err := svc.GetPlaceReviews("place-a", l)
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, up.reviews.Load(), "es variants share one key")

	for i := 0; i < 200; i++ {
		_, err := svc.GetPlaceReviews("place-a", fmt.Sprintf("x%d-attack", i))
		require.NoError(t, err)
	}
	require.EqualValues(t, 2, up.reviews.Load(), "unknown languages collapse to Google's default")
	require.Equal(t, []string{"es", ""}, up.langs, "upstream only ever sees normalized codes")
}

func TestNormalizePlacesLanguage(t *testing.T) {
	for in, want := range map[string]string{
		"":              "",
		"en":            "en",
		"EN-gb":         "en-GB",
		"pt_br":         "pt-BR",
		"pt-AO":         "pt",
		"es-AR":         "es",
		"es-419":        "es-419",
		"zh-tw":         "zh-TW",
		"zh-Hant-TW":    "zh",
		"he":            "iw",
		"nb-NO":         "no",
		"fil":           "fil",
		"klingon":       "",
		"en;DROP TABLE": "",
		"a-very-long-tag-xxxxxxxxxxxxxxxxxxxxxxxxxxxx": "",
	} {
		require.Equal(t, want, normalizePlacesLanguage(in), "input %q", in)
	}
}

func TestCachedPlaces_FailuresAreCachedBriefly(t *testing.T) {
	up := &countingPlaces{}
	up.fail(errors.New("upstream 503"))
	svc := newCachedPlacesServiceWithTTL(up, time.Hour, 40*time.Millisecond, 64)

	for i := 0; i < 10; i++ {
		_, err := svc.GetPlaceDetails("place-a")
		require.Error(t, err)
	}
	require.EqualValues(t, 1, up.details.Load(), "an outage must not be hammered")

	up.heal()
	time.Sleep(60 * time.Millisecond)
	d, err := svc.GetPlaceDetails("place-a")
	require.NoError(t, err, "a failure must not be cached for the success TTL")
	require.NotNil(t, d)
	require.EqualValues(t, 2, up.details.Load())

	_, err = svc.GetPlaceDetails("place-a")
	require.NoError(t, err)
	require.EqualValues(t, 2, up.details.Load(), "the recovered value is cached")
}

func TestCachedPlaces_ConcurrentMissesShareOneUpstreamCall(t *testing.T) {
	up := &countingPlaces{gate: make(chan struct{})}
	svc := newCachedPlacesService(up)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.GetPlaceDetails("place-hot")
			require.NoError(t, err)
		}()
	}
	time.Sleep(30 * time.Millisecond)
	close(up.gate)
	wg.Wait()
	require.EqualValues(t, 1, up.details.Load())
}

func TestCachedPlaces_MemoryIsBounded(t *testing.T) {
	up := &countingPlaces{}
	svc := newCachedPlacesServiceWithTTL(up, time.Hour, time.Minute, 8)
	for i := 0; i < 100; i++ {
		_, _ = svc.GetPlaceDetails(fmt.Sprintf("place-%d", i))
	}
	require.LessOrEqual(t, svc.hits.Len(), 8)
}

// Public storefront route: repeated renders cost one upstream call.
func TestGetPublicBusinessGoogleRoutes_ServedFromCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	business := createPublicBusinessRouteTestBusiness(t, "places-cache-venue", true, true)

	up := &countingPlaces{}
	prev := googlePlacesService
	googlePlacesService = newCachedPlacesService(up)
	t.Cleanup(func() { googlePlacesService = prev })

	for i := 0; i < 20; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
		c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?language=x%d", i), nil)
		GetPublicBusinessGoogleReviews(c)
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), `"author_name":"Ada"`)

		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		GetPublicBusinessGoogleDetails(c)
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), `"user_ratings_total":120`)
	}
	require.EqualValues(t, 1, up.reviews.Load())
	require.EqualValues(t, 1, up.details.Load())

}

// SetGooglePlacesService installs the cached reader for every reviews/details
// call site.
func TestSetGooglePlacesService_InstallsCache(t *testing.T) {
	prevConcrete, prev := googlePlacesServiceConcrete, googlePlacesService
	t.Cleanup(func() { googlePlacesServiceConcrete, googlePlacesService = prevConcrete, prev })

	SetGooglePlacesService(services.NewGooglePlacesService("test-key"))
	_, ok := googlePlacesService.(*cachedPlacesService)
	require.True(t, ok, "got %T", googlePlacesService)

	SetGooglePlacesService(nil)
	require.Nil(t, googlePlacesService)
}

// BenchmarkPlacesDetailsRead compares the pre-change read path (every call
// goes upstream) with the cached decorator now installed by
// SetGooglePlacesService. upstream-calls/op is the billed Google request
// rate per storefront render.
func BenchmarkPlacesDetailsRead(b *testing.B) {
	for _, tc := range []struct {
		name string
		wrap func(GooglePlacesService) GooglePlacesService
	}{
		{"uncached", func(s GooglePlacesService) GooglePlacesService { return s }},
		{"cached", func(s GooglePlacesService) GooglePlacesService { return newCachedPlacesService(s) }},
	} {
		b.Run(tc.name, func(b *testing.B) {
			up := &countingPlaces{}
			svc := tc.wrap(up)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := svc.GetPlaceDetails("place-bench"); err != nil {
					b.Fatal(err)
				}
				if _, err := svc.GetPlaceReviews("place-bench", "es-AR"); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(up.details.Load()+up.reviews.Load())/float64(b.N), "upstream-calls/op")
		})
	}
}
