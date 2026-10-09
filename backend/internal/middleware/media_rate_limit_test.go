package middleware

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func mediaRequest(method, path, remoteAddr string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = remoteAddr
	return req
}

// TestMediaRateLimitThrottlesPastBudget pins the per-IP budget on /media:
// requests past the burst get 429 with Retry-After, the rejection is never
// cached, and another client IP keeps its own bucket.
func TestMediaRateLimitThrottlesPastBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	limit := MediaRateLimit(60, 3)
	r.GET("/media/*key", limit, func(c *gin.Context) { c.Status(http.StatusOK) })
	r.HEAD("/media/*key", limit, func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, mediaRequest(http.MethodGet, "/media/businesses/1/a.png", "203.0.113.7:1000"))
		if w.Code != http.StatusOK {
			t.Fatalf("request #%d within burst got %d, want 200", i, w.Code)
		}
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, mediaRequest(http.MethodHead, "/media/businesses/1/a.png", "203.0.113.7:1001"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("HEAD past the shared GET/HEAD budget got %d, want 429", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After = %q, want %q", got, "1")
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store (a cached 429 would break the image for everyone behind a cache)", got)
	}

	other := httptest.NewRecorder()
	r.ServeHTTP(other, mediaRequest(http.MethodGet, "/media/businesses/1/a.png", "198.51.100.9:1000"))
	if other.Code != http.StatusOK {
		t.Fatalf("a different client IP got %d, want 200 (buckets are per IP)", other.Code)
	}
}

// TestMediaRateLimitDisabled: 0 turns the limiter off (operators with a CDN in
// front of /media).
func TestMediaRateLimitDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/media/*key", MediaRateLimit(0, 0), func(c *gin.Context) { c.Status(http.StatusOK) })
	for i := 0; i < 50; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, mediaRequest(http.MethodGet, "/media/a.png", "203.0.113.7:1000"))
		if w.Code != http.StatusOK {
			t.Fatalf("request #%d got %d with the limiter disabled", i, w.Code)
		}
	}
}

// TestMediaRateLimitKeepsAPIBudgetSeparate replays the production chain: the
// global API limiter in front of every route, plus the media limiter on
// /media. Media traffic must not spend the API budget, and must still be
// throttled by its own.
func TestMediaRateLimitKeepsAPIBudgetSeparate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	global := NewSimpleRateLimiter(context.Background(), 1) // API budget: 1/min
	defer global.Stop()

	r := gin.New()
	r.Use(global.RateLimit())
	r.GET("/media/*key", MediaRateLimit(60, 5), func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/api/v1/orders", func(c *gin.Context) { c.Status(http.StatusOK) })

	const ip = "203.0.113.7:1000"
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, mediaRequest(http.MethodGet, "/media/a.png", ip))
		if w.Code != http.StatusOK {
			t.Fatalf("media request #%d got %d, want 200", i, w.Code)
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, mediaRequest(http.MethodGet, "/media/a.png", ip))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("media past its own budget got %d, want 429", w.Code)
	}

	api := httptest.NewRecorder()
	r.ServeHTTP(api, mediaRequest(http.MethodGet, "/api/v1/orders", ip))
	if api.Code != http.StatusOK {
		t.Fatalf("first API request after media traffic got %d, want 200 (media must not spend the API budget)", api.Code)
	}
}

func mediaLimitRouter(rpm, burst int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New() // trusts every proxy, so X-Forwarded-For names the client
	r.GET("/media/*key", MediaRateLimit(rpm, burst), func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

// allowedBeforeThrottle sends requests until the first 429 and returns how
// many were allowed (capped at limit+1).
func allowedBeforeThrottle(t *testing.T, r *gin.Engine, limit int, build func() *http.Request) int {
	t.Helper()
	for i := 0; i <= limit; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, build())
		if w.Code == http.StatusTooManyRequests {
			return i
		}
		if w.Code != http.StatusOK {
			t.Fatalf("request #%d got %d, want 200 or 429", i, w.Code)
		}
	}
	return limit + 1
}

// TestMediaRateLimitGivesUnforwardedInternalPeerItsOwnBudget pins R2-4: the
// next/image optimizer and server renders in the frontend container reach
// /media unstamped (no X-Forwarded-For), so all of them share the frontend's
// address. That peer gets MediaRateLimitInternalMultiplier times the budget in
// a bucket of its own, so a cold optimizer cache does not 429 every guest, and
// public clients keep their own per-IP budget alongside it.
func TestMediaRateLimitGivesUnforwardedInternalPeerItsOwnBudget(t *testing.T) {
	const burst = 3
	want := burst * MediaRateLimitInternalMultiplier
	for _, peer := range []string{"172.18.0.5:41000", "10.0.0.7:41000", "127.0.0.1:41000", "[::1]:41000", "[fd00::5]:41000", "[::ffff:10.0.0.7]:41000"} {
		t.Run(peer, func(t *testing.T) {
			r := mediaLimitRouter(60, burst)
			got := allowedBeforeThrottle(t, r, want, func() *http.Request {
				return mediaRequest(http.MethodGet, "/media/businesses/1/a.png", peer)
			})
			if got != want {
				t.Fatalf("unforwarded private peer %s allowed %d before 429, want %d (burst %d x %d)",
					peer, got, want, burst, MediaRateLimitInternalMultiplier)
			}
			public := allowedBeforeThrottle(t, r, burst, func() *http.Request {
				return mediaRequest(http.MethodGet, "/media/businesses/1/a.png", "203.0.113.7:1000")
			})
			if public != burst {
				t.Fatalf("public client allowed %d after the internal bucket drained, want its own %d", public, burst)
			}
		})
	}
}

// TestMediaRateLimitForwardedRequestsKeepTheClientBudget: the larger internal
// bucket is only for a server fetching on its own behalf. A relayed request
// (X-Forwarded-For or X-Real-IP) and a public peer both get the normal
// per-client budget.
func TestMediaRateLimitForwardedRequestsKeepTheClientBudget(t *testing.T) {
	const burst = 3
	cases := map[string]func() *http.Request{
		"private peer relaying X-Forwarded-For": func() *http.Request {
			req := mediaRequest(http.MethodGet, "/media/a.png", "172.18.0.5:41000")
			req.Header.Set("X-Forwarded-For", "203.0.113.7")
			return req
		},
		"private peer relaying a private guest": func() *http.Request {
			req := mediaRequest(http.MethodGet, "/media/a.png", "172.18.0.5:41000")
			req.Header.Set("X-Forwarded-For", "192.168.1.20")
			return req
		},
		"private peer with X-Real-IP only": func() *http.Request {
			req := mediaRequest(http.MethodGet, "/media/a.png", "172.18.0.5:41000")
			req.Header.Set("X-Real-IP", "203.0.113.7")
			return req
		},
		"public peer without forwarding headers": func() *http.Request {
			return mediaRequest(http.MethodGet, "/media/a.png", "198.51.100.9:1000")
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			r := mediaLimitRouter(60, burst)
			if got := allowedBeforeThrottle(t, r, burst*MediaRateLimitInternalMultiplier, build); got != burst {
				t.Fatalf("allowed %d before 429, want the per-client burst %d", got, burst)
			}
		})
	}
}

// TestMediaRateLimitTrustedPlatformClientIsNotInternal: behind Cloudflare
// (TRUSTED_PLATFORM=cloudflare) the client comes from CF-Connecting-IP, so a
// private tunnel peer relaying it must not get the internal budget.
func TestMediaRateLimitTrustedPlatformClientIsNotInternal(t *testing.T) {
	const burst = 3
	r := mediaLimitRouter(60, burst)
	r.TrustedPlatform = gin.PlatformCloudflare
	got := allowedBeforeThrottle(t, r, burst*MediaRateLimitInternalMultiplier, func() *http.Request {
		req := mediaRequest(http.MethodGet, "/media/a.png", "172.18.0.9:41000")
		req.Header.Set("CF-Connecting-IP", "203.0.113.7")
		return req
	})
	if got != burst {
		t.Fatalf("allowed %d before 429, want the per-client burst %d", got, burst)
	}
}

// TestMediaRateLimitHugeBudgetDoesNotOverflow: the internal multiplier
// saturates instead of wrapping to a negative rate or burst.
func TestMediaRateLimitHugeBudgetDoesNotOverflow(t *testing.T) {
	if got := saturatingMul(math.MaxInt/2, MediaRateLimitInternalMultiplier); got != math.MaxInt {
		t.Fatalf("saturatingMul overflowed to %d", got)
	}
	r := mediaLimitRouter(math.MaxInt, math.MaxInt)
	for _, peer := range []string{"172.18.0.5:41000", "203.0.113.7:1000"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, mediaRequest(http.MethodGet, "/media/a.png", peer))
		if w.Code != http.StatusOK {
			t.Fatalf("peer %s got %d with a max budget", peer, w.Code)
		}
	}
}

// BenchmarkMediaRateLimit measures the /media route chain without a limiter
// (the route before F5) and with the media limiter, from an unforwarded
// private peer (the frontend container: internal bucket) and from a public
// client (per-IP bucket) (docs/performance/oss-storage.md).
func BenchmarkMediaRateLimit(b *testing.B) {
	gin.SetMode(gin.TestMode)
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	cases := []struct {
		name   string
		remote string
		chain  []gin.HandlerFunc
	}{
		{"none", "192.168.1.1:1234", []gin.HandlerFunc{ok}},
		{"limited", "192.168.1.1:1234", []gin.HandlerFunc{MediaRateLimit(1<<30, 1<<30), ok}},
		{"limited_public", "203.0.113.7:1234", []gin.HandlerFunc{MediaRateLimit(1<<30, 1<<30), ok}},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			router := gin.New()
			router.GET("/media/*key", tc.chain...)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest(http.MethodGet, "/media/businesses/1/0123456789abcdef_20261003_120000.png", nil)
				req.RemoteAddr = tc.remote
				router.ServeHTTP(httptest.NewRecorder(), req)
			}
		})
	}
}
