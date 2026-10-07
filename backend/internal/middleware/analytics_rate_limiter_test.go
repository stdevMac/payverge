package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// analyticsRouter wires the three public analytics ingest endpoints behind a
// single shared SimpleRateLimiter, mirroring main.go's public-route setup.
func analyticsRouter(cap int) (*gin.Engine, *SimpleRateLimiter) {
	rl := NewSimpleRateLimiter(context.Background(), cap)
	r := gin.New()
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	r.POST("/analytics/page-view", rl.RateLimit(), ok)
	r.POST("/analytics/interaction", rl.RateLimit(), ok)
	r.POST("/analytics/conversion", rl.RateLimit(), ok)
	return r, rl
}

func hitAnalytics(r *gin.Engine, ip, path string) int {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.Header.Set("X-Forwarded-For", ip)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

// TestAnalyticsSharedLimiter_BudgetIsCombinedAcrossEndpoints pins the B2 fix:
// the three public analytics ingest endpoints MUST share one per-IP budget so a
// flooder can't triple it by fanning bogus events across page-view /
// interaction / conversion. A regression to a per-endpoint limiter would let the
// 4th mixed request through and fail this test.
func TestAnalyticsSharedLimiter_BudgetIsCombinedAcrossEndpoints(t *testing.T) {
	r, rl := analyticsRouter(3)
	defer rl.Stop()
	ip := "203.0.113.7"

	// Spend the budget by spreading across all three endpoints.
	if c := hitAnalytics(r, ip, "/analytics/page-view"); c != http.StatusOK {
		t.Fatalf("page-view 1: expected 200, got %d", c)
	}
	if c := hitAnalytics(r, ip, "/analytics/interaction"); c != http.StatusOK {
		t.Fatalf("interaction 1: expected 200, got %d", c)
	}
	if c := hitAnalytics(r, ip, "/analytics/conversion"); c != http.StatusOK {
		t.Fatalf("conversion 1: expected 200, got %d", c)
	}

	// 4th request on ANY of the three must be throttled — the budget is shared.
	if c := hitAnalytics(r, ip, "/analytics/interaction"); c != http.StatusTooManyRequests {
		t.Fatalf("4th cross-endpoint request: expected 429, got %d (per-IP analytics budget is not shared across routes)", c)
	}
}

// TestAnalyticsSharedLimiter_PerIPIndependent confirms one abusive IP does not
// throttle a different legitimate client.
func TestAnalyticsSharedLimiter_PerIPIndependent(t *testing.T) {
	r, rl := analyticsRouter(1)
	defer rl.Stop()

	if c := hitAnalytics(r, "198.51.100.1", "/analytics/page-view"); c != http.StatusOK {
		t.Fatalf("attacker first request: expected 200, got %d", c)
	}
	if c := hitAnalytics(r, "198.51.100.1", "/analytics/page-view"); c != http.StatusTooManyRequests {
		t.Fatalf("attacker second request: expected 429, got %d", c)
	}
	// A different IP still has a fresh budget.
	if c := hitAnalytics(r, "198.51.100.2", "/analytics/conversion"); c != http.StatusOK {
		t.Fatalf("other client: expected 200, got %d", c)
	}
}
