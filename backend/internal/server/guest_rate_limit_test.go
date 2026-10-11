package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

// guestOrderRateLimitBudget mirrors the per-IP budget wired in cmd/app/main.go
// for the public guest order/bill creation endpoints (B9). Keep this in sync
// with main.go: if the production budget changes, this test should be updated
// deliberately, not silently.
const guestOrderRateLimitBudget = 120

// guestCreateRouter wires the public guest write endpoints behind a
// single shared SimpleRateLimiter, mirroring main.go's public-route setup so an
// attacker can't flood order/bill creation from one IP.
func guestCreateRouter(cap int) (*gin.Engine, *middleware.SimpleRateLimiter) {
	rl := middleware.NewSimpleRateLimiter(context.Background(), cap)
	r := gin.New()
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	r.POST("/guest/table/:code/bill", rl.RateLimit(), ok)
	r.POST("/guest/table/:code/order", rl.RateLimit(), ok)
	r.POST("/guest/bill/:bill_token/request-alternative-payment", rl.RateLimit(), ok)
	return r, rl
}

func TestGuestAlternativePaymentRequestSharesWriteBudgetAndReturns429(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, rl := guestCreateRouter(1)
	defer rl.Stop()

	ip := "203.0.113.44"
	before := metrics.CurrentAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestRateLimited)
	if c := hitGuestCreate(r, ip, "/guest/table/ABC123/order"); c != http.StatusOK {
		t.Fatalf("first guest write: expected 200, got %d", c)
	}
	if c := hitGuestCreate(r, ip, "/guest/bill/public-token/request-alternative-payment"); c != http.StatusTooManyRequests {
		t.Fatalf("alternative payment request after budget exhaustion: expected 429, got %d", c)
	}
	if got := metrics.CurrentAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestRateLimited) - before; got != 1 {
		t.Fatalf("alternative payment rate-limit metric delta: want 1, got %v", got)
	}
}

func hitGuestCreate(r *gin.Engine, ip, path string) int {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.Header.Set("X-Forwarded-For", ip)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

// guestTableReadRateLimitBudget mirrors cmd/app/main.go's unauthenticated
// GET table-by-code / bill lookup limiter (#293 enumeration). Distinct from
// guestOrderRateLimitBudget (writes).
const guestTableReadRateLimitBudget = 60

func guestTableReadRouter(cap int) (*gin.Engine, *middleware.SimpleRateLimiter) {
	rl := middleware.NewSimpleRateLimiter(context.Background(), cap)
	r := gin.New()
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	r.GET("/guest/table/:code", rl.RateLimit(), ok)
	r.GET("/guest/table/:code/bill", rl.RateLimit(), ok)
	r.GET("/table/:code", rl.RateLimit(), ok)
	return r, rl
}

func hitGuestGet(r *gin.Engine, ip, path string) int {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-Forwarded-For", ip)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

// TestGuestTableLookupRateLimitSequentialProbesReturn429 pins #293: unauthenticated
// GET table-code / bill probes share a per-IP budget distinct from POST guest
// order creation. Sequential probes past the budget must 429.
func TestGuestTableLookupRateLimitSequentialProbesReturn429(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r, rl := guestTableReadRouter(3)
	defer rl.Stop()

	ip := "203.0.113.80"
	if c := hitGuestGet(r, ip, "/guest/table/demo-75-ai-pro-table-01"); c != http.StatusOK {
		t.Fatalf("probe 1 table: expected 200, got %d", c)
	}
	if c := hitGuestGet(r, ip, "/guest/table/demo-75-ai-pro-table-02/bill"); c != http.StatusOK {
		t.Fatalf("probe 2 bill: expected 200, got %d", c)
	}
	if c := hitGuestGet(r, ip, "/table/demo-74-core-table-09"); c != http.StatusOK {
		t.Fatalf("probe 3 legacy /table: expected 200, got %d", c)
	}
	if c := hitGuestGet(r, ip, "/guest/table/demo-75-ai-pro-table-03/bill"); c != http.StatusTooManyRequests {
		t.Fatalf("probe 4: expected 429 after sequential enumeration, got %d", c)
	}
}

func TestGuestTableLookupRateLimitProductionBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r, rl := guestTableReadRouter(guestTableReadRateLimitBudget)
	defer rl.Stop()

	ip := "203.0.113.81"
	for i := 0; i < guestTableReadRateLimitBudget; i++ {
		path := "/guest/table/PROBE01/bill"
		if i%2 == 0 {
			path = "/guest/table/PROBE01"
		}
		if c := hitGuestGet(r, ip, path); c != http.StatusOK {
			t.Fatalf("request %d/%d: expected 200, got %d", i+1, guestTableReadRateLimitBudget, c)
		}
	}
	if c := hitGuestGet(r, ip, "/guest/table/PROBE01/bill"); c != http.StatusTooManyRequests {
		t.Fatalf("request %d: expected 429, got %d", guestTableReadRateLimitBudget+1, c)
	}
}

func TestGuestTableLookupRateLimitIndependentFromOrderWrites(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reads, readRL := guestTableReadRouter(1)
	defer readRL.Stop()
	writes, writeRL := guestCreateRouter(1)
	defer writeRL.Stop()

	ip := "203.0.113.82"
	if c := hitGuestGet(reads, ip, "/guest/table/ABC123/bill"); c != http.StatusOK {
		t.Fatalf("first GET lookup: expected 200, got %d", c)
	}
	if c := hitGuestGet(reads, ip, "/guest/table/ABC124/bill"); c != http.StatusTooManyRequests {
		t.Fatalf("second GET lookup: expected 429, got %d", c)
	}
	if c := hitGuestCreate(writes, ip, "/guest/table/ABC123/order"); c != http.StatusOK {
		t.Fatalf("POST order after GET budget exhaust must still be allowed (distinct limiter): got %d", c)
	}
	if c := hitGuestCreate(writes, ip, "/guest/table/ABC123/order"); c != http.StatusTooManyRequests {
		t.Fatalf("second POST order: expected 429 on write limiter, got %d", c)
	}
}

// TestGuestOrderRateLimit pins the B9 fix: the public guest order/bill creation
// endpoints MUST be rate-limited per IP. With a 120/min budget, the 121st
// request from a single client IP must be throttled with 429.
func TestGuestOrderRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r, rl := guestCreateRouter(guestOrderRateLimitBudget)
	defer rl.Stop()

	ip := "203.0.113.42"

	// The first `budget` requests must all be allowed (legitimate diners on one
	// restaurant NAT/WiFi should not be throttled within the budget).
	for i := 0; i < guestOrderRateLimitBudget; i++ {
		if c := hitGuestCreate(r, ip, "/guest/table/ABC123/order"); c != http.StatusOK {
			t.Fatalf("request %d/%d: expected 200, got %d", i+1, guestOrderRateLimitBudget, c)
		}
	}

	// Request #121 (one past the budget) must be throttled.
	if c := hitGuestCreate(r, ip, "/guest/table/ABC123/order"); c != http.StatusTooManyRequests {
		t.Fatalf("request %d: expected 429 (budget exhausted), got %d", guestOrderRateLimitBudget+1, c)
	}
}

// TestGuestCreateSharedBudgetAcrossOrderAndBill confirms the order and bill
// creation endpoints share one per-IP budget, so a flooder can't double their
// allowance by alternating between the two routes.
func TestGuestCreateSharedBudgetAcrossOrderAndBill(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Small budget so the assertion is cheap and obvious.
	r, rl := guestCreateRouter(3)
	defer rl.Stop()

	ip := "203.0.113.43"

	if c := hitGuestCreate(r, ip, "/guest/table/ABC123/order"); c != http.StatusOK {
		t.Fatalf("order 1: expected 200, got %d", c)
	}
	if c := hitGuestCreate(r, ip, "/guest/table/ABC123/bill"); c != http.StatusOK {
		t.Fatalf("bill 1: expected 200, got %d", c)
	}
	if c := hitGuestCreate(r, ip, "/guest/table/ABC123/order"); c != http.StatusOK {
		t.Fatalf("order 2: expected 200, got %d", c)
	}
	// 4th request on either route must be throttled — budget is shared.
	if c := hitGuestCreate(r, ip, "/guest/table/ABC123/bill"); c != http.StatusTooManyRequests {
		t.Fatalf("4th cross-endpoint request: expected 429, got %d (per-IP guest-create budget is not shared across routes)", c)
	}
}

// TestGuestCreatePerIPIndependent confirms one abusive IP does not throttle a
// different legitimate client.
func TestGuestCreatePerIPIndependent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r, rl := guestCreateRouter(1)
	defer rl.Stop()

	if c := hitGuestCreate(r, "198.51.100.10", "/guest/table/ABC123/order"); c != http.StatusOK {
		t.Fatalf("attacker first request: expected 200, got %d", c)
	}
	if c := hitGuestCreate(r, "198.51.100.10", "/guest/table/ABC123/order"); c != http.StatusTooManyRequests {
		t.Fatalf("attacker second request: expected 429, got %d", c)
	}
	// A different IP still has a fresh budget.
	if c := hitGuestCreate(r, "198.51.100.11", "/guest/table/ABC123/bill"); c != http.StatusOK {
		t.Fatalf("other client: expected 200, got %d", c)
	}
}
