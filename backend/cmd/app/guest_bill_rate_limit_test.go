package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/middleware"
)

// newGuestLimiterMirrorRouter mirrors the production wiring in main.go for the
// public guest surface: the global per-IP limiter wraps everything, the shared
// guestTableReadLimiter guards every public read, and guestOrderRateLimiter
// guards the guest create/pay writes. Route paths and limiter placement are
// copied from main.go's publicRoutes block; the budgets come from the SAME
// middleware constants main.go uses, so a rebalance there is exercised here.
func newGuestLimiterMirrorRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// main.go production default: GLOBAL_RATE_LIMIT_REQUESTS_PER_MINUTE=300.
	global := middleware.NewSimpleRateLimiter(ctx, 300)
	guestTableReadLimiter := middleware.NewSimpleRateLimiter(ctx, middleware.GuestTableReadRequestsPerMinute)
	guestOrderRateLimiter := middleware.NewSimpleRateLimiter(ctx, middleware.GuestOrderWriteRequestsPerMinute)

	ok := func(c *gin.Context) { c.Status(http.StatusOK) }

	r := gin.New()
	r.Use(global.RateLimit())
	pub := r.Group("/api/v1/")
	{
		pub.GET("/guest/table/:code", guestTableReadLimiter.RateLimit(), ok)
		pub.GET("/guest/table/:code/bill", guestTableReadLimiter.RateLimit(), ok)
		pub.GET("/guest/table/:code/business", guestTableReadLimiter.RateLimit(), ok)
		pub.GET("/guest/table/:code/menu", guestTableReadLimiter.RateLimit(), ok)
		pub.GET("/guest/table/:code/events", guestTableReadLimiter.RateLimit(), ok)
		pub.GET("/guest/table/:code/service-call", guestTableReadLimiter.RateLimit(), ok)
		pub.GET("/guest/table/:code/loyalty-rate", guestTableReadLimiter.RateLimit(), ok)
		pub.GET("/currencies", guestTableReadLimiter.RateLimit(), ok)
		pub.GET("/languages", guestTableReadLimiter.RateLimit(), ok)
		pub.GET("/business/:customUrl", guestTableReadLimiter.RateLimit(), ok)

		pub.POST("/guest/table/:code/bill", guestOrderRateLimiter.RateLimit(), ok)
		pub.POST("/guest/table/:code/order", guestOrderRateLimiter.RateLimit(), ok)
		pub.POST("/guest/table/:code/order/quote", guestOrderRateLimiter.RateLimit(), ok)
		pub.POST("/guest/table/:code/service-call", guestOrderRateLimiter.RateLimit(), ok)
	}
	return r
}

func guestSweepRequest(t *testing.T, r *gin.Engine, method, path, ip string) int {
	t.Helper()
	var body *strings.Reader
	if method == http.MethodPost {
		body = strings.NewReader("{}")
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, body)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	req.RemoteAddr = ip + ":51000"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

// TestGuestRateLimit_RealisticDiningSweepReachesBill replays issue #814: a
// normal guest click sweep (table home, menu cards, status/event refreshes)
// followed by viewing and creating the bill must NOT trip the guest read
// limiter. The sweep models the worst single minute of a party of four at one
// table — venue Wi-Fi NATs every phone at the venue behind ONE public IP, so
// the per-IP budget is shared by the whole party:
//
//	per guest, worst minute:
//	  4 page navigations x 7 reads (table, business, menu, status,
//	                                currencies, languages, loyalty-rate) = 28
//	  12 menu/status refreshes while browsing cards                     = 12
//	  4 SSE /events reconnects (proxy idle timeouts)                    =  4
//	  4 bill recovery polls (useGuestBillSync RECOVERY_POLL_MS=15s)     =  4
//	  4 service-call status checks                                      =  4
//	  -> 52 reads; x4 guests on the shared NAT IP = 208 reads/minute
//
// then the paying guest loads the bill page (3 reads) and creates the bill
// (1 write). Every request must clear both the route limiter and the global
// production limiter without a single 429.
//
// Scope of the guarantee: this covers the party-of-four sweep above (~212
// requests/minute). The untouched global production limiter (300/min per IP)
// still bounds heavier multi-diner NAT traffic, so larger parties can 429 at
// the global layer — that ceiling is a deliberate, security-scoped decision
// outside this rebalance (see middleware.GuestTableReadRequestsPerMinute docs).
func TestGuestRateLimit_RealisticDiningSweepReachesBill(t *testing.T) {
	r := newGuestLimiterMirrorRouter(t)

	const venueNATIP = "203.0.113.7" // one public IP for the whole party
	code := "M03Y18GB3P"

	perGuestReads := func() []string {
		reads := make([]string, 0, 52)
		for nav := 0; nav < 4; nav++ {
			reads = append(reads,
				"/api/v1/guest/table/"+code,
				"/api/v1/guest/table/"+code+"/business",
				"/api/v1/guest/table/"+code+"/menu",
				"/api/v1/guest/table/"+code+"/bill",
				"/api/v1/currencies",
				"/api/v1/languages",
				"/api/v1/guest/table/"+code+"/loyalty-rate",
			)
		}
		for i := 0; i < 12; i++ {
			if i%2 == 0 {
				reads = append(reads, "/api/v1/guest/table/"+code+"/menu")
			} else {
				reads = append(reads, "/api/v1/guest/table/"+code+"/bill")
			}
		}
		for i := 0; i < 4; i++ {
			reads = append(reads, "/api/v1/guest/table/"+code+"/events")
		}
		for i := 0; i < 4; i++ {
			reads = append(reads, "/api/v1/guest/table/"+code+"/bill")
		}
		for i := 0; i < 4; i++ {
			reads = append(reads, "/api/v1/guest/table/"+code+"/service-call")
		}
		return reads
	}

	limited := []string{}
	for guest := 0; guest < 4; guest++ {
		for _, path := range perGuestReads() {
			if status := guestSweepRequest(t, r, http.MethodGet, path, venueNATIP); status == http.StatusTooManyRequests {
				limited = append(limited, fmt.Sprintf("guest %d GET %s", guest, path))
			}
		}
	}
	require.Emptyf(t, limited,
		"a realistic dining sweep (4 guests behind one venue NAT IP, %d reads) must not be rate limited; got %d x 429 (first: %v). Guest read budget is middleware.GuestTableReadRequestsPerMinute=%d",
		4*52, len(limited), limited, middleware.GuestTableReadRequestsPerMinute)

	// The paying guest now opens the bill page and creates/pays the bill —
	// the exact flow issue #814 saw blocked by 429 "we're a bit busy".
	for _, path := range []string{
		"/api/v1/guest/table/" + code,
		"/api/v1/guest/table/" + code + "/bill",
		"/api/v1/guest/table/" + code + "/business",
	} {
		status := guestSweepRequest(t, r, http.MethodGet, path, venueNATIP)
		require.Equalf(t, http.StatusOK, status,
			"GET %s must succeed for a diner paying an open check after normal browsing", path)
	}
	status := guestSweepRequest(t, r, http.MethodPost, "/api/v1/guest/table/"+code+"/bill", venueNATIP)
	require.Equal(t, http.StatusOK, status,
		"POST /guest/table/:code/bill must succeed for a diner paying an open check after normal browsing")
}

// TestGuestRateLimit_AbusiveReadFloodStillLimited keeps the security property:
// a single IP hammering the guest read surface far beyond any realistic dining
// pattern must still be throttled by the shared per-IP read limiter.
func TestGuestRateLimit_AbusiveReadFloodStillLimited(t *testing.T) {
	r := newGuestLimiterMirrorRouter(t)

	const attackerIP = "198.51.100.99"
	okCount := 0
	sawLimited := false
	const flood = 400
	for i := 0; i < flood; i++ {
		// Rotate table codes: enumeration probes vary the code, but the
		// limiter keys on IP, so rotation must not reset the budget.
		path := fmt.Sprintf("/api/v1/guest/table/PROBE%05d/menu", i)
		switch guestSweepRequest(t, r, http.MethodGet, path, attackerIP) {
		case http.StatusOK:
			okCount++
		case http.StatusTooManyRequests:
			sawLimited = true
		}
	}
	assert.True(t, sawLimited, "an abusive %d-request read flood from one IP must see 429s", flood)
	assert.LessOrEqualf(t, okCount, middleware.GuestTableReadRequestsPerMinute,
		"per-IP read budget must cap successful abuse requests at GuestTableReadRequestsPerMinute=%d",
		middleware.GuestTableReadRequestsPerMinute)
}

// TestGuestRateLimit_AbusiveWriteFloodStillLimited pins the guest write budget.
func TestGuestRateLimit_AbusiveWriteFloodStillLimited(t *testing.T) {
	r := newGuestLimiterMirrorRouter(t)

	const attackerIP = "198.51.100.100"
	okCount := 0
	sawLimited := false
	const flood = 200
	for i := 0; i < flood; i++ {
		switch guestSweepRequest(t, r, http.MethodPost, "/api/v1/guest/table/M03Y18GB3P/order", attackerIP) {
		case http.StatusOK:
			okCount++
		case http.StatusTooManyRequests:
			sawLimited = true
		}
	}
	assert.True(t, sawLimited, "an abusive %d-request write flood from one IP must see 429s", flood)
	assert.LessOrEqualf(t, okCount, middleware.GuestOrderWriteRequestsPerMinute,
		"per-IP write budget must cap successful abuse requests at GuestOrderWriteRequestsPerMinute=%d",
		middleware.GuestOrderWriteRequestsPerMinute)
}

// TestGuestRateLimit_MainGoUsesSharedConstants is the structural guard: main.go
// must construct the two shared guest limiters from the middleware constants
// (with env overrides), so the behavioral tests above exercise the real budget.
func TestGuestRateLimit_MainGoUsesSharedConstants(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	src, err := os.ReadFile(filepath.Join(wd, "main.go"))
	require.NoError(t, err, "expected to read main.go next to this test")
	body := string(src)

	require.Contains(t, body,
		`intEnv("GUEST_TABLE_READ_RATE_LIMIT_REQUESTS_PER_MINUTE", middleware.GuestTableReadRequestsPerMinute)`,
		"guestTableReadLimiter must be built from middleware.GuestTableReadRequestsPerMinute")
	require.Contains(t, body,
		`intEnv("GUEST_ORDER_RATE_LIMIT_REQUESTS_PER_MINUTE", middleware.GuestOrderWriteRequestsPerMinute)`,
		"guestOrderRateLimiter must be built from middleware.GuestOrderWriteRequestsPerMinute")
	// The two guest limiter assignments must not regress to inline literals
	// (the pattern that let #814's 60/min budget hide from review). Other
	// limiters (analytics, space-scan) are out of scope here.
	inlineGuestRead := regexp.MustCompile(`guestTableReadLimiter\s*:=\s*middleware\.NewSimpleRateLimiter\(context\.Background\(\),\s*\d`)
	assert.False(t, inlineGuestRead.MatchString(body),
		"guestTableReadLimiter must not be built from an inline literal (use middleware.GuestTableReadRequestsPerMinute)")
	inlineGuestWrite := regexp.MustCompile(`guestOrderRateLimiter\s*:=\s*middleware\.NewSimpleRateLimiter\(context\.Background\(\),\s*\d`)
	assert.False(t, inlineGuestWrite.MatchString(body),
		"guestOrderRateLimiter must not be built from an inline literal (use middleware.GuestOrderWriteRequestsPerMinute)")
}

// TestGuestRateLimit_PublicReadRoutesThrottled pins the five public GETs that
// previously had no sibling limiter. Each path gets its own guestTableReadLimiter
// so a flood of one route cannot exhaust the bucket of another. The next request
// from the same IP is 429; a different IP still receives 200.
func TestGuestRateLimit_PublicReadRoutesThrottled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	r := gin.New()
	pub := r.Group("/api/v1/")

	paths := []struct {
		pattern string
		example string
	}{
		{"/business/:customUrl/menu", "/api/v1/business/cafe/menu"},
		{"/guest/bill/:bill_token/orders", "/api/v1/guest/bill/tok123/orders"},
		{"/exchange-rate", "/api/v1/exchange-rate"},
		{"/convert", "/api/v1/convert"},
		{"/business/:customUrl/reservations/availability", "/api/v1/business/cafe/reservations/availability"},
	}
	for _, path := range paths {
		limiter := middleware.NewSimpleRateLimiter(ctx, middleware.GuestTableReadRequestsPerMinute)
		pub.GET(path.pattern, limiter.RateLimit(), ok)
	}

	for i, path := range paths {
		attacker := fmt.Sprintf("198.51.100.%d", i+1)
		var last int
		for n := 0; n < middleware.GuestTableReadRequestsPerMinute+1; n++ {
			last = guestSweepRequest(t, r, http.MethodGet, path.example, attacker)
		}
		assert.Equalf(t, http.StatusTooManyRequests, last,
			"GET %s must 429 once one IP exceeds GuestTableReadRequestsPerMinute", path.example)

		other := guestSweepRequest(t, r, http.MethodGet, path.example, fmt.Sprintf("203.0.113.%d", i+10))
		assert.Equalf(t, http.StatusOK, other,
			"GET %s from a different IP must still succeed", path.example)
	}
}
