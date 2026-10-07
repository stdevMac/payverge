package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRateLimitKeyForIP(t *testing.T) {
	cases := map[string]string{
		"":                              "unknown",
		"  ":                            "unknown",
		"203.0.113.7":                   "203.0.113.7",
		"::ffff:203.0.113.7":            "203.0.113.7",
		"2001:db8:1:2:aaaa::1":          "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff:ffff:ffff:1": "2001:db8:1:2::/64",
		"2001:db8:1:3::1":               "2001:db8:1:3::/64",
		"fe80::1%eth0":                  "fe80::/64",
		"not-an-ip":                     "not-an-ip",
	}
	for in, want := range cases {
		if got := RateLimitKeyForIP(in); got != want {
			t.Errorf("RateLimitKeyForIP(%q) = %q, want %q", in, got, want)
		}
	}
}

// Low: an IPv6 client rotating source addresses inside its own /64 must share
// one bucket on every IP-keyed limiter.
func TestIPKeyedLimiters_BucketIPv6PerSlash64(t *testing.T) {
	simple := NewSimpleRateLimiter(context.Background(), 1)
	t.Cleanup(simple.Stop)

	for name, mw := range map[string]gin.HandlerFunc{
		"RateLimit":       RateLimit(1, 1),
		"AuthRateLimiter": AuthRateLimiter(1, 1),
		"BusinessNoParam": BusinessRateLimit(1),
		"SimpleRateLimit": simple.RateLimit(),
	} {
		t.Run(name, func(t *testing.T) {
			r := newTestRouterWithMiddleware(mw)
			if w := performRequest(r, "2001:db8:aa:bb::1"); w.Code != http.StatusOK {
				t.Fatalf("first address: expected 200, got %d", w.Code)
			}
			if w := performRequest(r, "2001:db8:aa:bb:dead:beef:0:2"); w.Code != http.StatusTooManyRequests {
				t.Fatalf("second address in the same /64: expected 429, got %d", w.Code)
			}
			if w := performRequest(r, "2001:db8:aa:cc::1"); w.Code != http.StatusOK {
				t.Fatalf("different /64: expected 200, got %d", w.Code)
			}
		})
	}
}

// M-track: a business limiter on a route with no business param (the public
// /delivery/:delivery_number/track lookup) must still throttle per client
// instead of passing every request through.
func TestBusinessRateLimit_NoBusinessParamFallsBackToClientBucket(t *testing.T) {
	r := gin.New()
	r.GET("/delivery/:delivery_number/track", BusinessRateLimitWithBurst(30, 5), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	track := func(ip, number string) int {
		req := httptest.NewRequest(http.MethodGet, "/delivery/"+number+"/track", nil)
		req.Header.Set("X-Forwarded-For", ip)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	// Enumerating delivery numbers from one client exhausts one shared bucket.
	for i := 0; i < 5; i++ {
		if code := track("198.51.100.20", "DEL-"+string(rune('A'+i))); code != http.StatusOK {
			t.Fatalf("request %d within burst: expected 200, got %d", i, code)
		}
	}
	if code := track("198.51.100.20", "DEL-Z"); code != http.StatusTooManyRequests {
		t.Fatalf("request beyond burst: expected 429, got %d", code)
	}
	// Another client is unaffected.
	if code := track("198.51.100.21", "DEL-A"); code != http.StatusOK {
		t.Fatalf("other client: expected 200, got %d", code)
	}
}

// BenchmarkIPKeyedRateLimit measures the per-request cost of the IP-keyed
// token-bucket limiters (RateLimit and AuthRateLimiter run on many routes).
// The bucket never empties, so every iteration takes the allow path.
func BenchmarkIPKeyedRateLimit(b *testing.B) {
	for _, tc := range []struct {
		name string
		mw   gin.HandlerFunc
		ip   string
	}{
		{"RateLimit/ipv4", RateLimit(1<<30, 1<<30), "203.0.113.7"},
		{"RateLimit/ipv6", RateLimit(1<<30, 1<<30), "2001:db8:1:2::7"},
		{"AuthRateLimiter/ipv4", AuthRateLimiter(1<<30, 1<<30), "203.0.113.7"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			r := newTestRouterWithMiddleware(tc.mw)
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("X-Forwarded-For", tc.ip)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				if w.Code != http.StatusOK {
					b.Fatalf("got %d", w.Code)
				}
			}
		})
	}
}
