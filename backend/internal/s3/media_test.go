package s3

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// installLocalStores points the package at fresh temp-dir local stores for
// the duration of the test.
func installLocalStores(t *testing.T) (pub, prot *LocalStore) {
	t.Helper()
	var err error
	pub, err = NewLocalStore(t.TempDir())
	require.NoError(t, err)
	prot, err = NewLocalStore(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(SetStores(pub, prot))
	return pub, prot
}

func mediaRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/media/*key", MediaHandler())
	r.HEAD("/media/*key", MediaHandler())
	return r
}

func doMedia(r http.Handler, method, target string, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for k, v := range header {
		req.Header[k] = v
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func putPublic(t *testing.T, s Store, key, contentType, body string) {
	t.Helper()
	require.NoError(t, s.Put(context.Background(), key, strings.NewReader(body), PutOptions{ContentType: contentType}))
}

func TestMediaServesPublicObjectWithCachingHeaders(t *testing.T) {
	pub, _ := installLocalStores(t)
	r := mediaRouter()
	key := "businesses/4/menu_items/9f86d081884c7d65_provoleta.png"
	putPublic(t, pub, key, "image/png", "png-bytes")

	w := doMedia(r, http.MethodGet, "/media/"+key, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "png-bytes", w.Body.String())
	h := w.Header()
	require.Equal(t, "image/png", h.Get("Content-Type"))
	require.Equal(t, "nosniff", h.Get("X-Content-Type-Options"))
	require.Equal(t, MediaCacheImmutable, h.Get("Cache-Control"))
	require.Equal(t, "cross-origin", h.Get("Cross-Origin-Resource-Policy"))
	require.Contains(t, h.Get("Content-Security-Policy"), "sandbox")
	require.NotEmpty(t, h.Get("Last-Modified"))
	etag := h.Get("ETag")
	require.True(t, strings.HasPrefix(etag, `"`), "strong etag: %s", etag)

	// Conditional GET → 304 without a body.
	w = doMedia(r, http.MethodGet, "/media/"+key, http.Header{"If-None-Match": {etag}})
	require.Equal(t, http.StatusNotModified, w.Code)
	require.Empty(t, w.Body.String())
	require.Equal(t, etag, w.Header().Get("ETag"))

	// Stale validator → full body.
	w = doMedia(r, http.MethodGet, "/media/"+key, http.Header{"If-None-Match": {`"deadbeef"`}})
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "png-bytes", w.Body.String())

	// HEAD → headers + length, no body.
	w = doMedia(r, http.MethodHead, "/media/"+key, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "9", w.Header().Get("Content-Length"))
	require.Equal(t, etag, w.Header().Get("ETag"))
	require.Empty(t, w.Body.String())

	// Range (local bodies are seekable).
	w = doMedia(r, http.MethodGet, "/media/"+key, http.Header{"Range": {"bytes=0-2"}})
	require.Equal(t, http.StatusPartialContent, w.Code)
	require.Equal(t, "png", w.Body.String())
}

func TestMediaCacheControlByKeyShape(t *testing.T) {
	cases := map[string]string{
		"businesses/1/menu_items/0123456789abcdef_dish.png":            MediaCacheImmutable,
		"businesses/1/logo/550e8400-e29b-41d4-a716-446655440000.png":   MediaCacheImmutable,
		"menu_items/ai_generated/1/20260101_0123456789abcdef0123.webp": MediaCacheImmutable,
		"demo-arg/assets/carta/provoleta.jpg":                          MediaCacheDefault,
		"businesses/1/logo/logo.png":                                   MediaCacheDefault,
	}
	for key, want := range cases {
		require.Equal(t, want, MediaCacheControl(key), key)
	}
}

func TestMediaRejectsTraversalAndMalformedKeys(t *testing.T) {
	pub, _ := installLocalStores(t)
	r := mediaRouter()
	putPublic(t, pub, "ok/file.png", "image/png", "x")

	for _, target := range []string{
		"/media/../etc/passwd",
		"/media/ok/../../etc/passwd",
		"/media/..%2f..%2fetc%2fpasswd",
		"/media/%2e%2e/%2e%2e/etc/passwd",
		"/media/ok/%2e%2e/file.png",
		"/media//etc/passwd",
		"/media/ok//file.png",
		"/media/ok/./file.png",
		"/media/.env",
		"/media/ok/.hidden",
		"/media/ok%5c..%5cfile.png",
		"/media/",
		"/media",
	} {
		w := doMedia(r, http.MethodGet, target, nil)
		require.Contains(t, []int{http.StatusNotFound, http.StatusMovedPermanently, http.StatusTemporaryRedirect}, w.Code, target)
		require.NotEqual(t, "x", w.Body.String(), target)
		if w.Code == http.StatusNotFound && strings.HasPrefix(target, "/media/") {
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"), target)
		}
	}
	// Direct calls (bypassing router path cleaning) are also guarded.
	for _, key := range []string{"../x", "ok/../../x", "/abs", "ok/..", "", "ok"} {
		w := httptest.NewRecorder()
		ServeMedia(w, httptest.NewRequest(http.MethodGet, "/media/x", nil), key)
		require.Equal(t, http.StatusNotFound, w.Code, key)
	}
}

func TestMediaNeverServesProtectedKeySpaces(t *testing.T) {
	pub, prot := installLocalStores(t)
	r := mediaRouter()
	// Even if a protected-looking key somehow lands in the public store (a
	// shared/misconfigured bucket), /media must answer exactly like a miss.
	for _, key := range []string{
		"protected/fiscal-receipts/1/a.pdf",
		"fiscal-receipts/1/2/a.pdf",
		"ledger/1/attachments/a.pdf",
		"ai/menu-extraction/1/2/page.png",
		"space-scans/1/scan.jpg",
		"storage-probe/abc.txt",
		"businesses/12/contracts/contract.pdf",
	} {
		putPublic(t, pub, key, "application/pdf", "secret")
		w := doMedia(r, http.MethodGet, "/media/"+key, nil)
		require.Equal(t, http.StatusNotFound, w.Code, key)
		require.NotContains(t, w.Body.String(), "secret", key)
		w = doMedia(r, http.MethodHead, "/media/"+key, nil)
		require.Equal(t, http.StatusNotFound, w.Code, key)
	}
	// Objects in the protected store are never reachable through /media.
	require.NoError(t, prot.Put(context.Background(), "businesses/1/receipt.pdf", strings.NewReader("prot"), PutOptions{ContentType: "application/pdf"}))
	w := doMedia(r, http.MethodGet, "/media/businesses/1/receipt.pdf", nil)
	require.Equal(t, http.StatusNotFound, w.Code)
	require.False(t, IsProtectedMediaKey("businesses/12/logo/contracts.png"))
}

func TestMediaContentTypeHardening(t *testing.T) {
	pub, _ := installLocalStores(t)
	r := mediaRouter()
	putPublic(t, pub, "u/evil.svg", "image/svg+xml", `<svg onload="alert(1)"/>`)
	putPublic(t, pub, "u/evil.html", "text/html", "<script>alert(1)</script>")
	putPublic(t, pub, "u/notype.png", "", "png")
	putPublic(t, pub, "u/menu.pdf", "application/pdf", "%PDF-1.4")
	putPublic(t, pub, "u/jpg-alias.jpg", "image/jpg", "jpg")
	require.NoError(t, pub.Put(context.Background(), "u/dispo.png", strings.NewReader("x"), PutOptions{
		ContentType:        "image/png",
		ContentDisposition: "inline; filename=\"../../evil\r\nSet-Cookie: a=b.png\"",
	}))

	for _, key := range []string{"u/evil.svg", "u/evil.html"} {
		w := doMedia(r, http.MethodGet, "/media/"+key, nil)
		require.Equal(t, http.StatusOK, w.Code, key)
		require.Equal(t, "application/octet-stream", w.Header().Get("Content-Type"), key)
		require.True(t, strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment"), key)
		require.Contains(t, w.Header().Get("Content-Security-Policy"), "sandbox", key)
	}

	w := doMedia(r, http.MethodGet, "/media/u/notype.png", nil)
	require.Equal(t, "image/png", w.Header().Get("Content-Type"), "extension fallback")

	w = doMedia(r, http.MethodGet, "/media/u/jpg-alias.jpg", nil)
	require.Equal(t, "image/jpeg", w.Header().Get("Content-Type"))

	w = doMedia(r, http.MethodGet, "/media/u/menu.pdf", nil)
	require.Equal(t, "application/pdf", w.Header().Get("Content-Type"))
	require.Equal(t, "SAMEORIGIN", w.Header().Get("X-Frame-Options"))
	require.Equal(t, "frame-ancestors 'self'", w.Header().Get("Content-Security-Policy"))

	w = doMedia(r, http.MethodGet, "/media/u/dispo.png", nil)
	require.Equal(t, http.StatusOK, w.Code)
	disposition := w.Header().Get("Content-Disposition")
	require.NotContains(t, disposition, "\n")
	require.NotContains(t, disposition, "..")
	require.Empty(t, w.Header().Get("Set-Cookie"))
}

func TestMediaMissingDirectoryAndMethods(t *testing.T) {
	pub, _ := installLocalStores(t)
	r := mediaRouter()
	putPublic(t, pub, "dir/sub/file.png", "image/png", "x")

	for _, target := range []string{"/media/dir", "/media/dir/sub", "/media/dir/missing.png"} {
		w := doMedia(r, http.MethodGet, target, nil)
		require.Equal(t, http.StatusNotFound, w.Code, target)
		require.NotContains(t, w.Body.String(), "file.png", "no directory listing")
	}

	w := httptest.NewRecorder()
	ServeMedia(w, httptest.NewRequest(http.MethodPost, "/media/dir/sub/file.png", nil), "dir/sub/file.png")
	require.Equal(t, http.StatusMethodNotAllowed, w.Code)

	restore := SetStores(nil, nil)
	defer restore()
	w = doMedia(r, http.MethodGet, "/media/dir/sub/file.png", nil)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestMediaServesFromS3DriverWithoutCDN(t *testing.T) {
	fake := newFakeS3(t)
	pub := newFakeS3Store(t, fake, "pub", "")
	t.Cleanup(SetStores(pub, nil))
	require.NoError(t, pub.Put(context.Background(), "menu/0123456789abcdef_x.webp", bytes.NewReader([]byte("webp")), PutOptions{ContentType: "image/webp"}))

	r := mediaRouter()
	w := doMedia(r, http.MethodGet, "/media/menu/0123456789abcdef_x.webp", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "webp", w.Body.String())
	require.Equal(t, "image/webp", w.Header().Get("Content-Type"))
	require.Equal(t, MediaCacheImmutable, w.Header().Get("Cache-Control"))
	etag := w.Header().Get("ETag")
	require.NotEmpty(t, etag)

	w = doMedia(r, http.MethodGet, "/media/menu/0123456789abcdef_x.webp", http.Header{"If-None-Match": {etag}})
	require.Equal(t, http.StatusNotModified, w.Code)

	w = doMedia(r, http.MethodGet, "/media/menu/missing.webp", nil)
	require.Equal(t, http.StatusNotFound, w.Code)
}

// BenchmarkServeMedia measures the /media hot path on the local driver: a
// full 48 KiB image GET and a revalidation (304).
func BenchmarkServeMedia(b *testing.B) {
	pub, err := NewLocalStore(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	restore := SetStores(pub, nil)
	defer restore()
	key := "businesses/1/menu_items/0123456789abcdef_dish.jpg"
	if err := pub.Put(context.Background(), key, bytes.NewReader(bytes.Repeat([]byte{0xAB}, 48<<10)), PutOptions{ContentType: "image/jpeg"}); err != nil {
		b.Fatal(err)
	}
	info, err := pub.Stat(context.Background(), key)
	if err != nil {
		b.Fatal(err)
	}
	r := mediaRouter()

	b.Run("get", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest(http.MethodGet, "/media/"+key, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				b.Fatalf("status %d", w.Code)
			}
			_, _ = io.Copy(io.Discard, w.Body)
		}
	})
	b.Run("revalidate_304", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest(http.MethodGet, "/media/"+key, nil)
			req.Header.Set("If-None-Match", info.ETag)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusNotModified {
				b.Fatalf("status %d", w.Code)
			}
		}
	})
}
