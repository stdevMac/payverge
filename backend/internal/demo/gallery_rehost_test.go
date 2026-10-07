package demo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/require"
)

// fakeRehoster records what it was asked to re-host and serves a predictable
// hosted URL, so the seeder's use of the cache can be asserted without S3.
type fakeRehoster struct {
	mu     sync.Mutex
	calls  []string
	hosted map[string]string
	fail   bool
}

func newFakeRehoster() *fakeRehoster {
	return &fakeRehoster{hosted: map[string]string{}}
}

func (f *fakeRehoster) Rehost(_ context.Context, sourceURL string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, sourceURL)
	if f.fail {
		return "", errors.New("rehost unavailable")
	}
	hosted := "https://images.payverge.test/demo-arg/assets/" + fmt.Sprint(len(f.hosted)) + ".jpg"
	f.hosted[sourceURL] = hosted
	return hosted, nil
}

func (f *fakeRehoster) Hosted(sourceURL string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	url, ok := f.hosted[sourceURL]
	return url, ok
}

// L4-20 / decision #5 → superseded by the AR own-host seed: demo gallery rows
// now point straight at our public store (s3.PublicURL("demo-arg/assets/...")),
// so the SSRF allowlist can always fetch them and no stock host can 503 a
// sales demo. The rehoster stays wired for any future non-own-host source, but
// own-hosted assets must never round-trip through it (copying the bucket into
// itself wastes startup time and can 403 on itself).
func TestGalleryRowsPreferTheRehostedCopy(t *testing.T) {
	db := newDemoServiceTestDB(t)
	business := database.Business{Name: "Rehosted gallery demo"}
	require.NoError(t, db.Create(&business).Error)

	rehoster := newFakeRehoster()
	svc := NewService(db, Options{AssetRehoster: rehoster})
	svc.warmDemoAssets(context.Background())
	require.NoError(t, svc.ensureBusinessPageData(context.Background(), db, business.ID, profiles()[0]))

	var rows []database.BusinessGalleryImage
	require.NoError(t, db.Where("business_id = ?", business.ID).Order("display_order").Find(&rows).Error)
	require.Len(t, rows, 3)
	for _, row := range rows {
		require.NotContains(t, row.ImageURL, "unsplash.com",
			"gallery row %q still points at the stock host", row.Caption)
		require.True(t, strings.HasPrefix(row.ImageURL, demoAssetURL("")), "gallery row %q = %q", row.Caption, row.ImageURL)
	}
	require.Empty(t, rehoster.calls, "own-hosted demo assets must never be re-hosted into their own bucket")
}

// Without a re-hoster — the default, and every existing test — seeding must
// still produce the same own-hosted URLs. A demo with photos beats one with none.
func TestGalleryFallsBackToTheSourceURLWithoutARehoster(t *testing.T) {
	db := newDemoServiceTestDB(t)
	business := database.Business{Name: "No rehoster demo"}
	require.NoError(t, db.Create(&business).Error)

	svc := NewService(db, Options{})
	svc.warmDemoAssets(context.Background())
	require.NoError(t, svc.ensureBusinessPageData(context.Background(), db, business.ID, profiles()[0]))

	var rows []database.BusinessGalleryImage
	require.NoError(t, db.Where("business_id = ?", business.ID).Order("display_order").Find(&rows).Error)
	require.Len(t, rows, 3)
	for _, row := range rows {
		require.True(t, strings.HasPrefix(row.ImageURL, demoAssetURL("")), "gallery row %q = %q", row.Caption, row.ImageURL)
	}
}

// A failing re-hoster must never degrade the seed: own-hosted sources bypass
// it entirely, so the rows stay identical and the seed still succeeds.
func TestGalleryFallsBackWhenRehostingFails(t *testing.T) {
	db := newDemoServiceTestDB(t)
	business := database.Business{Name: "Failing rehoster demo"}
	require.NoError(t, db.Create(&business).Error)

	rehoster := newFakeRehoster()
	rehoster.fail = true
	svc := NewService(db, Options{AssetRehoster: rehoster})
	svc.warmDemoAssets(context.Background())
	require.NoError(t, svc.ensureBusinessPageData(context.Background(), db, business.ID, profiles()[0]))

	var rows []database.BusinessGalleryImage
	require.NoError(t, db.Where("business_id = ?", business.ID).Order("display_order").Find(&rows).Error)
	require.Len(t, rows, 3)
	for _, row := range rows {
		require.True(t, strings.HasPrefix(row.ImageURL, demoAssetURL("")), "gallery row %q = %q", row.Caption, row.ImageURL)
	}
}

// Warming happens once, before the seed transaction opens — and because every
// distinct gallery source across both demo profiles is already own-hosted, it
// must make ZERO network calls.
func TestWarmDemoAssetsCoversEveryDistinctGallerySource(t *testing.T) {
	db := newDemoServiceTestDB(t)
	rehoster := newFakeRehoster()
	svc := NewService(db, Options{AssetRehoster: rehoster})

	svc.warmDemoAssets(context.Background())

	for _, p := range profiles() {
		for _, src := range galleryImageSources(p) {
			require.True(t, isOwnHostedDemoAsset(src),
				"gallery source %q must live on the demo asset host", src)
		}
	}
	require.Empty(t, rehoster.calls, "own-hosted sources must not be warmed through the rehoster")
	_ = db
}

func TestS3AssetRehosterFetchesOnceAndUploadsUnderADeterministicKey(t *testing.T) {
	fetches := 0
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetches++
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8\xff\xe0 not really a jpeg"))
	}))
	defer source.Close()

	var keys []string
	rehoster := NewS3AssetRehoster(func(data []byte, name, folder, contentType string) (string, error) {
		keys = append(keys, folder+"/"+name)
		require.Equal(t, "image/jpeg", contentType)
		require.NotEmpty(t, data)
		return "https://images.payverge.test/" + folder + "/" + name, nil
	})

	first, err := rehoster.Rehost(context.Background(), source.URL+"/photo.jpg")
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(first, ".jpg"), "hosted URL should keep an image extension: %s", first)

	second, err := rehoster.Rehost(context.Background(), source.URL+"/photo.jpg")
	require.NoError(t, err)

	require.Equal(t, first, second, "the same source must resolve to the same hosted URL")
	require.Equal(t, 1, fetches, "a cached source must not be fetched again")
	require.Len(t, keys, 1)

	hosted, ok := rehoster.Hosted(source.URL + "/photo.jpg")
	require.True(t, ok)
	require.Equal(t, first, hosted)
}

func TestS3AssetRehosterReportsAFailedFetchAndCachesNothing(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer source.Close()

	uploads := 0
	rehoster := NewS3AssetRehoster(func([]byte, string, string, string) (string, error) {
		uploads++
		return "", nil
	})

	_, err := rehoster.Rehost(context.Background(), source.URL+"/photo.jpg")
	require.Error(t, err)
	require.Equal(t, 0, uploads, "nothing should be uploaded when the source could not be read")

	_, ok := rehoster.Hosted(source.URL + "/photo.jpg")
	require.False(t, ok, "a failed re-host must not poison the cache")
}

func TestS3AssetRehosterRejectsANonImageSource(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>not an image</html>"))
	}))
	defer source.Close()

	uploads := 0
	rehoster := NewS3AssetRehoster(func([]byte, string, string, string) (string, error) {
		uploads++
		return "", nil
	})

	_, err := rehoster.Rehost(context.Background(), source.URL+"/photo.jpg")
	require.Error(t, err)
	require.Equal(t, 0, uploads)
}
