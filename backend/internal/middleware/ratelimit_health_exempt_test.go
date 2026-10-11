package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestHealthExemptFromRateLimit asserts /api/v1/health* is never 429'd by the
// global limiter, so orchestrator probes survive a traffic spike (EXT-6).
func TestHealthExemptFromRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rl := NewSimpleRateLimiter(context.Background(), 1) // budget of 1/min
	defer rl.Stop()

	r := gin.New()
	r.Use(rl.RateLimit())
	r.GET("/api/v1/health/live", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/api/v1/orders", func(c *gin.Context) { c.Status(http.StatusOK) })

	// Health probes far past the budget must all be 200.
	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("health probe #%d got %d, want 200 (must be exempt)", i, w.Code)
		}
	}

	// A non-health route still gets throttled past the budget.
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil))
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil))
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 2nd /orders to be 429, got %d", w2.Code)
	}
}

// TestMediaExemptFromRateLimit asserts public /media/* objects are never 429'd:
// one guest menu page fans out to dozens of images, which must not burn the
// per-IP API budget. Only the exact "/media/" prefix is exempt.
func TestMediaExemptFromRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rl := NewSimpleRateLimiter(context.Background(), 1) // budget of 1/min
	defer rl.Stop()

	r := gin.New()
	r.Use(rl.RateLimit())
	r.GET("/media/*key", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/mediaX/*key", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/media/businesses/1/a.png", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("media request #%d got %d, want 200 (must be exempt)", i, w.Code)
		}
	}

	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/mediaX/a.png", nil))
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/mediaX/a.png", nil))
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("a look-alike prefix must stay limited; got %d", w2.Code)
	}
}

// BenchmarkRateLimiterMediaPath measures the exempt /media/* fast path
// (docs/performance/oss-storage.md).
func BenchmarkRateLimiterMediaPath(b *testing.B) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	rl := NewSimpleRateLimiter(context.Background(), 1000)
	b.Cleanup(rl.Stop)
	router.Use(rl.RateLimit())
	router.GET("/media/*key", func(c *gin.Context) { c.Status(http.StatusOK) })
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/media/businesses/1/0123456789abcdef_20261003_120000.png", nil)
		req.Header.Set("X-Forwarded-For", "192.168.1.1")
		router.ServeHTTP(httptest.NewRecorder(), req)
	}
}
