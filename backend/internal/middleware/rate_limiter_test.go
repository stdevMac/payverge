package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestRouter returns a Gin engine with the given middleware applied to POST /.
func newTestRouterWithMiddleware(mw gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.POST("/", mw, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func performRequest(r *gin.Engine, ip string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Forwarded-For", ip)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestAuthRateLimiter_AllowsWithinLimit verifies that requests within the limit succeed.
func TestAuthRateLimiter_AllowsWithinLimit(t *testing.T) {
	// 5 requests per minute, burst of 5 — all 5 initial requests should pass.
	r := newTestRouterWithMiddleware(AuthRateLimiter(5, 5))
	ip := "10.0.0.1"

	for i := 0; i < 5; i++ {
		w := performRequest(r, ip)
		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}
}

// TestAuthRateLimiter_BlocksExcessRequests verifies that the 3rd request with a burst of 2 is rejected.
func TestAuthRateLimiter_BlocksExcessRequests(t *testing.T) {
	// 2 requests per minute, burst of 2 — 3rd request should be rate-limited.
	r := newTestRouterWithMiddleware(AuthRateLimiter(2, 2))
	ip := "10.0.0.2"

	// First two should succeed.
	for i := 0; i < 2; i++ {
		w := performRequest(r, ip)
		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// Third should be blocked with Retry-After / X-RateLimit-* (#295).
	w := performRequest(r, ip)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("3rd request: expected 429, got %d", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got == "" {
		t.Errorf("expected Retry-After header on auth 429")
	}
	if got := w.Header().Get("X-RateLimit-Limit"); got != "2" {
		t.Errorf("expected X-RateLimit-Limit=2, got %q", got)
	}
	if got := w.Header().Get("X-RateLimit-Remaining"); got != "0" {
		t.Errorf("expected X-RateLimit-Remaining=0, got %q", got)
	}
	if got := w.Header().Get("X-RateLimit-Reset"); got == "" {
		t.Errorf("expected X-RateLimit-Reset header on auth 429")
	}
}

// TestAuthRateLimiter_DifferentIPsIndependent verifies that different IPs have independent limits.
func TestAuthRateLimiter_DifferentIPsIndependent(t *testing.T) {
	// Burst of 1 — each IP gets only 1 request before being limited.
	r := newTestRouterWithMiddleware(AuthRateLimiter(1, 1))

	ip1 := "10.1.0.1"
	ip2 := "10.1.0.2"

	// First request from ip1 should succeed.
	w1 := performRequest(r, ip1)
	if w1.Code != http.StatusOK {
		t.Errorf("ip1 first request: expected 200, got %d", w1.Code)
	}

	// Second request from ip1 should be blocked.
	w2 := performRequest(r, ip1)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("ip1 second request: expected 429, got %d", w2.Code)
	}

	// First request from ip2 should still succeed (independent bucket).
	w3 := performRequest(r, ip2)
	if w3.Code != http.StatusOK {
		t.Errorf("ip2 first request: expected 200, got %d", w3.Code)
	}
}

// newTestRouterWithBusinessMiddleware creates a Gin engine with BusinessRateLimit applied.
// The route mirrors the production path with :business_id.
func newTestRouterWithBusinessMiddleware(requestsPerMinute int) *gin.Engine {
	r := gin.New()
	mw := BusinessRateLimit(requestsPerMinute)
	r.POST("/businesses/:business_id/delivery/quote", mw, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func performBusinessRequest(r *gin.Engine, ip, businessID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/businesses/"+businessID+"/delivery/quote", nil)
	req.Header.Set("X-Forwarded-For", ip)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestBusinessRateLimit_AllowsWithinBurst verifies that requests within the burst pass.
func TestBusinessRateLimit_AllowsWithinBurst(t *testing.T) {
	// burst=1 means 1 token in the bucket initially; the limiter returns true once.
	r := newTestRouterWithBusinessMiddleware(60) // 1/s effectively, burst=1

	w := performBusinessRequest(r, "10.2.0.1", "42")
	if w.Code != http.StatusOK {
		t.Errorf("first request: expected 200, got %d", w.Code)
	}
}

// TestBusinessRateLimit_BlocksBeyondBurst verifies that the second request is
// rejected with the default burst (1).
func TestBusinessRateLimit_BlocksBeyondBurst(t *testing.T) {
	r := newTestRouterWithBusinessMiddleware(1)
	ip := "10.3.0.1"
	businessID := "99"

	if w := performBusinessRequest(r, ip, businessID); w.Code != http.StatusOK {
		t.Errorf("first request: expected 200, got %d", w.Code)
	}
	if w := performBusinessRequest(r, ip, businessID); w.Code != http.StatusTooManyRequests {
		t.Errorf("second request: expected 429, got %d", w.Code)
	}
}

// TestBusinessRateLimit_DifferentBusinessesIndependent verifies that different
// business IDs have independent buckets for the same IP.
func TestBusinessRateLimit_DifferentBusinessesIndependent(t *testing.T) {
	r := newTestRouterWithBusinessMiddleware(1)
	ip := "10.4.0.1"

	if w := performBusinessRequest(r, ip, "1"); w.Code != http.StatusOK {
		t.Errorf("business 1 first: expected 200, got %d", w.Code)
	}
	if w := performBusinessRequest(r, ip, "1"); w.Code != http.StatusTooManyRequests {
		t.Errorf("business 1 second: expected 429, got %d", w.Code)
	}

	// Business 2 should have a fresh bucket.
	if w := performBusinessRequest(r, ip, "2"); w.Code != http.StatusOK {
		t.Errorf("business 2 first: expected 200, got %d", w.Code)
	}
}

// TestBusinessRateLimitWithBurst_AllowsMultipleWithinBurst pins the
// delivery-flow burst path: 30/min sustained, burst 5, six fast requests
// produces five 200s then one 429.
func TestBusinessRateLimitWithBurst_AllowsMultipleWithinBurst(t *testing.T) {
	r := gin.New()
	mw := BusinessRateLimitWithBurst(30, 5)
	r.POST("/businesses/:business_id/delivery/quote", mw, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	ip := "10.5.0.1"
	for i := 0; i < 5; i++ {
		if w := performBusinessRequest(r, ip, "42"); w.Code != http.StatusOK {
			t.Errorf("request %d (within burst): expected 200, got %d", i+1, w.Code)
		}
	}
	if w := performBusinessRequest(r, ip, "42"); w.Code != http.StatusTooManyRequests {
		t.Errorf("request 6 (beyond burst): expected 429, got %d", w.Code)
	}
}

// TestBusinessRateLimitWithBurst_AIWaiterBurst3 pins the AI-waiter chat-box
// burst contract: 20/min sustained, burst 3, so a guest can fire a quick
// follow-up (or double-tap send) — four fast requests produce three 200s then
// one 429. A regression to the default burst=1 would 429 the second rapid send.
func TestBusinessRateLimitWithBurst_AIWaiterBurst3(t *testing.T) {
	r := gin.New()
	mw := BusinessRateLimitWithBurst(20, 3)
	r.POST("/businesses/:business_id/ai-waiter", mw, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	perform := func(ip, businessID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/businesses/"+businessID+"/ai-waiter", nil)
		req.Header.Set("X-Forwarded-For", ip)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	ip := "10.6.0.1"
	for i := 0; i < 3; i++ {
		if w := perform(ip, "7"); w.Code != http.StatusOK {
			t.Errorf("request %d (within burst): expected 200, got %d", i+1, w.Code)
		}
	}
	if w := perform(ip, "7"); w.Code != http.StatusTooManyRequests {
		t.Errorf("request 4 (beyond burst): expected 429, got %d", w.Code)
	}
}

// TestRateLimiter_evictsStaleEntries verifies that entries older than the TTL
// are removed by evictStale.
func TestRateLimiter_evictsStaleEntries(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1), 1)
	rl.GetLimiter("test-key")

	rl.mu.RLock()
	if _, exists := rl.limiters["test-key"]; !exists {
		t.Fatal("expected test-key to exist after GetLimiter")
	}
	rl.mu.RUnlock()

	// Back-date the lastSeen entry so it looks stale.
	rl.mu.Lock()
	rl.lastSeen["test-key"] = time.Now().Add(-15 * time.Minute)
	rl.mu.Unlock()

	rl.evictStale(10 * time.Minute)

	rl.mu.RLock()
	defer rl.mu.RUnlock()
	if _, exists := rl.limiters["test-key"]; exists {
		t.Fatal("expected test-key to be evicted after staleness")
	}
}

// TestRateLimiter_doesNotEvictActiveEntries verifies that recently-seen
// entries survive an eviction pass.
func TestRateLimiter_doesNotEvictActiveEntries(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1), 1)
	rl.GetLimiter("active-key")
	rl.evictStale(10 * time.Minute)

	rl.mu.RLock()
	defer rl.mu.RUnlock()
	if _, exists := rl.limiters["active-key"]; !exists {
		t.Fatal("expected active-key to survive eviction")
	}
}

// TestPaymentRateLimit_ScopedPerBill_SameIP verifies that two different bills
// from the same venue IP are not mutually throttled, while the same bill IS
// throttled on a rapid second attempt.
func TestPaymentRateLimit_ScopedPerBill_SameIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/businesses/:business_id/bills/:bill_id/pay", PaymentRateLimit(), func(c *gin.Context) { c.Status(http.StatusOK) })

	call := func(bill string) int {
		req := httptest.NewRequest(http.MethodPost, "/businesses/7/bills/"+bill+"/pay", nil)
		req.RemoteAddr = "203.0.113.9:5555" // shared venue IP
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	if got := call("100"); got != http.StatusOK {
		t.Errorf("bill 100 first call: expected 200, got %d", got)
	}
	if got := call("200"); got != http.StatusOK {
		t.Errorf("bill 200 (different guest, same IP): expected 200, got %d", got)
	}
	if got := call("100"); got != http.StatusTooManyRequests {
		t.Errorf("bill 100 second call (<10s): expected 429, got %d", got)
	}
}

// TestPaymentRateLimit_IPFallback_NoBillParam verifies that the legacy IP-only
// throttle is preserved on routes without a bill_id param (e.g. PayPal/Tron return).
func TestPaymentRateLimit_IPFallback_NoBillParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/paypal/return", PaymentRateLimit(), func(c *gin.Context) { c.Status(http.StatusOK) })
	call := func() int {
		req := httptest.NewRequest(http.MethodGet, "/paypal/return", nil)
		req.RemoteAddr = "198.51.100.4:4444"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	if got := call(); got != http.StatusOK {
		t.Errorf("first call: expected 200, got %d", got)
	}
	if got := call(); got != http.StatusTooManyRequests {
		t.Errorf("second call (<10s): expected 429, got %d", got)
	}
}

// managerActionTestRouter mounts ManagerActionRateLimit on the shapes the
// PIN-gated operator routes actually use: /bills/:bill_id/void (no business
// param at all) and /businesses/:id/crypto-refunds. `actor` stands in for
// HybridAuthenticationMiddleware, which runs at the group level and has
// therefore already populated the context when the route-level limiter
// executes. The terminal handler answers 400 so a test can prove the handler's
// own validation — not the limiter — replied.
func managerActionTestRouter(actor gin.HandlerFunc) *gin.Engine {
	mw := ManagerActionRateLimit()
	r := gin.New()
	if actor != nil {
		r.Use(actor)
	}
	reached := func(c *gin.Context) {
		// Stand-in for VoidBill's own empty-body validation.
		c.JSON(http.StatusBadRequest, gin.H{"error": "Reason must be at least 3 characters"})
	}
	r.POST("/inside/bills/:bill_id/void", mw, reached)
	r.POST("/inside/bills/:bill_id/refund", mw, reached)
	r.POST("/inside/businesses/:id/crypto-refunds", mw, reached)
	return r
}

func managerActionCall(r *gin.Engine, path, ip string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.RemoteAddr = ip + ":51000"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestManagerActionRateLimit_ProbeThenVoidReachesValidation is the issue 909
// repro: one request against a bill id that does not exist (404 at
// RequireBillBusinessAccess, but the limiter has already spent the token),
// immediately followed by an empty void on a real bill. Under the payment
// limiter the second call came back 429 "Payment rate limit exceeded"; an expo
// correcting a check must instead reach the handler's own validation.
func TestManagerActionRateLimit_ProbeThenVoidReachesValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := managerActionTestRouter(nil)

	if got := managerActionCall(r, "/inside/bills/999999/void", "203.0.113.77").Code; got != http.StatusBadRequest {
		t.Fatalf("probe call: expected the handler to answer (400), got %d", got)
	}
	w := managerActionCall(r, "/inside/bills/1335/void", "203.0.113.77")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty void right after a probe: expected 400 validation, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestManagerActionRateLimit_SeparateBucketFromPayments proves void no longer
// draws on a payment-shaped bucket: a caller who has just exhausted the payment
// limiter from the same IP can still correct a check, and vice versa.
func TestManagerActionRateLimit_SeparateBucketFromPayments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const ip = "203.0.113.78"

	pay := gin.New()
	pay.POST("/guest/bill/:bill_token/plugin-payment", PaymentRateLimit(), func(c *gin.Context) { c.Status(http.StatusOK) })
	payCall := func() int {
		req := httptest.NewRequest(http.MethodPost, "/guest/bill/tok-1/plugin-payment", nil)
		req.RemoteAddr = ip + ":52000"
		w := httptest.NewRecorder()
		pay.ServeHTTP(w, req)
		return w.Code
	}
	if got := payCall(); got != http.StatusOK {
		t.Fatalf("payment first call: expected 200, got %d", got)
	}
	if got := payCall(); got != http.StatusTooManyRequests {
		t.Fatalf("payment second call: expected the payment bucket to be spent (429), got %d", got)
	}

	mgr := managerActionTestRouter(nil)
	if got := managerActionCall(mgr, "/inside/bills/1335/void", ip).Code; got != http.StatusBadRequest {
		t.Fatalf("void with the payment bucket exhausted: expected 400, got %d", got)
	}
}

// TestManagerActionRateLimit_ScopedPerBillAndRoute keeps the bucket from
// collapsing to one IP-wide token across every PIN-gated route: a void, a
// refund and a crypto-refund request from the same terminal are distinct
// operator actions and must not starve each other.
func TestManagerActionRateLimit_ScopedPerBillAndRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := managerActionTestRouter(nil)
	const ip = "203.0.113.79"

	for _, path := range []string{
		"/inside/bills/1335/void",
		"/inside/bills/1702/void",
		"/inside/bills/1335/refund",
		"/inside/businesses/142/crypto-refunds",
	} {
		if got := managerActionCall(r, path, ip).Code; got != http.StatusBadRequest {
			t.Errorf("%s: expected the handler to answer (400), got %d", path, got)
		}
	}
}

// TestManagerActionRateLimit_ScopedPerActor keeps two managers sharing one
// venue's NATed IP from throttling each other while working the same check.
func TestManagerActionRateLimit_ScopedPerActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const ip = "203.0.113.81"

	// One limiter instance, one venue IP, one bill — only the actor changes.
	staffID := uint(11)
	r := managerActionTestRouter(func(c *gin.Context) { c.Set("staff_id", staffID) })

	throttled := false
	for i := 0; i < 40; i++ {
		if managerActionCall(r, "/inside/bills/1335/void", ip).Code == http.StatusTooManyRequests {
			throttled = true
			break
		}
	}
	if !throttled {
		t.Fatalf("expected manager 11 to exhaust their own bucket")
	}

	staffID = 22
	if got := managerActionCall(r, "/inside/bills/1335/void", ip).Code; got != http.StatusBadRequest {
		t.Errorf("second manager on the same venue IP: expected 400, got %d", got)
	}
}

// TestManagerActionRateLimit_StillFailsClosed pins that this is a real limiter,
// not a bypass: sustained hammering of one target still 429s, with an operator
// envelope that says what was throttled and carries a machine code.
func TestManagerActionRateLimit_StillFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := managerActionTestRouter(nil)
	const ip = "203.0.113.80"

	var last *httptest.ResponseRecorder
	throttled := false
	for i := 0; i < 40; i++ {
		last = managerActionCall(r, "/inside/bills/4242/void", ip)
		if last.Code == http.StatusTooManyRequests {
			throttled = true
			break
		}
	}
	if !throttled {
		t.Fatalf("manager action limiter never engaged: last=%d", last.Code)
	}
	body := last.Body.String()
	if strings.Contains(strings.ToLower(body), "payment") {
		t.Errorf("throttle copy still talks about payments: %s", body)
	}
	if !strings.Contains(body, "MANAGER_ACTION_RATE_LIMITED") {
		t.Errorf("expected a machine-readable code in the throttle envelope: %s", body)
	}
}

// BenchmarkRateLimiterEvictStaleChurn exercises GetLimiter across many unique
// keys (per-IP / per-business buckets) and periodically evicts, proving the
// limiters + lastSeen maps stay bounded rather than growing one entry per key
// forever. Records bytes/op and allocs/op.
func BenchmarkRateLimiterEvictStaleChurn(b *testing.B) {
	rl := NewRateLimiter(rate.Limit(1), 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "business:" + strconv.Itoa(i&0xffff) + ":client:" + strconv.Itoa(i)
		rl.GetLimiter(key)
		if i%4096 == 0 {
			// Back-date lastSeen so the eviction can fully drain idle keys.
			rl.mu.Lock()
			past := time.Now().Add(-15 * time.Minute)
			for k := range rl.lastSeen {
				rl.lastSeen[k] = past
			}
			rl.mu.Unlock()
			rl.evictStale(10 * time.Minute)
		}
	}
	b.StopTimer()

	rl.mu.RLock()
	live := len(rl.limiters)
	rl.mu.RUnlock()
	if b.N > 8192 && live >= b.N {
		b.Fatalf("limiters grew unbounded: %d live keys for %d iterations", live, b.N)
	}
}
