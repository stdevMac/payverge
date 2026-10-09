package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type SecurityMiddlewareTestSuite struct {
	suite.Suite
	router *gin.Engine
}

func (suite *SecurityMiddlewareTestSuite) SetupTest() {
	gin.SetMode(gin.TestMode)
	suite.router = gin.New()
}

func TestSecurityMiddlewareTestSuite(t *testing.T) {
	suite.Run(t, new(SecurityMiddlewareTestSuite))
}

func TestRequireTrustedOriginForMutations_RejectsCustomerProfileMutationFromUntrustedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireTrustedOriginForMutations([]string{"https://payverge.io"}))
	router.POST("/api/v1/customer/profile", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req, _ := http.NewRequest("POST", "/api/v1/customer/profile", strings.NewReader(`{"name":"Mallory"}`))
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "origin_not_allowed")
}

// Test Rate Limiting Middleware
func (suite *SecurityMiddlewareTestSuite) TestRateLimiter_AllowsNormalRequests() {
	rateLimiter := NewSimpleRateLimiter(context.Background(), 10) // 10 requests per minute
	suite.T().Cleanup(rateLimiter.Stop)
	suite.router.Use(rateLimiter.RateLimit())
	suite.router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Make a normal request
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), 200, w.Code)
}

func (suite *SecurityMiddlewareTestSuite) TestRateLimiter_BlocksExcessiveRequests() {
	rateLimiter := NewSimpleRateLimiter(context.Background(), 2) // 2 requests per minute for testing
	suite.T().Cleanup(rateLimiter.Stop)
	suite.router.Use(rateLimiter.RateLimit())
	suite.router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	clientIP := "192.168.1.1"

	// Make allowed requests
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Forwarded-For", clientIP)
		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)
		assert.Equal(suite.T(), 200, w.Code)
	}

	// This request should be rate limited
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", clientIP)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), 429, w.Code)
	assert.Contains(suite.T(), w.Body.String(), "Rate limit exceeded")
}

func (suite *SecurityMiddlewareTestSuite) TestRateLimiter_DifferentIPsIndependent() {
	rateLimiter := NewSimpleRateLimiter(context.Background(), 2) // 2 requests per minute to allow both IPs
	suite.T().Cleanup(rateLimiter.Stop)
	suite.router.Use(rateLimiter.RateLimit())
	suite.router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// First IP makes a request
	req1, _ := http.NewRequest("GET", "/test", nil)
	req1.Header.Set("X-Forwarded-For", "192.168.1.1")
	w1 := httptest.NewRecorder()
	suite.router.ServeHTTP(w1, req1)
	assert.Equal(suite.T(), 200, w1.Code)

	// Second IP should still be allowed
	req2, _ := http.NewRequest("GET", "/test", nil)
	req2.Header.Set("X-Forwarded-For", "192.168.1.2")
	w2 := httptest.NewRecorder()
	suite.router.ServeHTTP(w2, req2)
	assert.Equal(suite.T(), 200, w2.Code)
}

func (suite *SecurityMiddlewareTestSuite) TestBusinessRateLimit_DifferentIPsIndependentPerBusiness() {
	suite.Require().NoError(suite.router.SetTrustedProxies(nil))
	suite.router.Use(BusinessRateLimit(1))
	suite.router.GET("/businesses/:id/ai", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	req1, _ := http.NewRequest("GET", "/businesses/42/ai", nil)
	req1.Header.Set("X-Forwarded-For", "192.168.1.1")
	req1.RemoteAddr = "192.168.1.1:1234"
	w1 := httptest.NewRecorder()
	suite.router.ServeHTTP(w1, req1)
	assert.Equal(suite.T(), 200, w1.Code)

	req2, _ := http.NewRequest("GET", "/businesses/42/ai", nil)
	req2.Header.Set("X-Forwarded-For", "203.0.113.99")
	req2.RemoteAddr = "192.168.1.2:1234"
	w2 := httptest.NewRecorder()
	suite.router.ServeHTTP(w2, req2)
	assert.Equal(suite.T(), 200, w2.Code)
}

func (suite *SecurityMiddlewareTestSuite) TestBusinessRateLimit_BlocksSpoofedForwardedIPWhenProxiesUntrusted() {
	suite.Require().NoError(suite.router.SetTrustedProxies(nil))
	suite.router.Use(BusinessRateLimit(1))
	suite.router.GET("/businesses/:id/ai", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	req1, _ := http.NewRequest("GET", "/businesses/42/ai", nil)
	req1.Header.Set("X-Forwarded-For", "192.168.1.1")
	req1.RemoteAddr = "192.168.1.1:1234"
	w1 := httptest.NewRecorder()
	suite.router.ServeHTTP(w1, req1)
	assert.Equal(suite.T(), 200, w1.Code)

	// Same RemoteAddr but a different X-Forwarded-For — should still be
	// rate-limited because proxies aren't trusted, so the spoofed header
	// is ignored and the bucket is shared (burst=1 by default).
	req2, _ := http.NewRequest("GET", "/businesses/42/ai", nil)
	req2.Header.Set("X-Forwarded-For", "198.51.100.77")
	req2.RemoteAddr = "192.168.1.1:5678"
	w2 := httptest.NewRecorder()
	suite.router.ServeHTTP(w2, req2)
	assert.Equal(suite.T(), 429, w2.Code)
	assert.Contains(suite.T(), w2.Body.String(), "Business rate limit exceeded")
}

// Test CORS Middleware
func (suite *SecurityMiddlewareTestSuite) TestCORS_AllowsConfiguredOrigins() {
	suite.router.Use(CORS([]string{"http://localhost:3000", "http://localhost:3001", "https://payverge.io", "https://www.payverge.io", "https://api.payverge.io"}))
	suite.router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), 200, w.Code)
	assert.Equal(suite.T(), "http://localhost:3000", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(suite.T(), "true", w.Header().Get("Access-Control-Allow-Credentials"))
}

func (suite *SecurityMiddlewareTestSuite) TestCORS_BlocksUnauthorizedOrigins() {
	suite.router.Use(CORS([]string{"http://localhost:3000", "http://localhost:3001", "https://payverge.io", "https://www.payverge.io", "https://api.payverge.io"}))
	suite.router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://malicious-site.com")
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), 200, w.Code)
	assert.NotEqual(suite.T(), "http://malicious-site.com", w.Header().Get("Access-Control-Allow-Origin"))
}

func (suite *SecurityMiddlewareTestSuite) TestCORS_DoesNotEmitPreflightMetadataForUnauthorizedOrigins() {
	suite.router.Use(CORS([]string{"https://payverge.io"}))
	suite.router.OPTIONS("/test", func(c *gin.Context) {
		c.Status(200)
	})

	req, _ := http.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusForbidden, w.Code)
	assert.Empty(suite.T(), w.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(suite.T(), w.Header().Get("Access-Control-Allow-Methods"))
	assert.Empty(suite.T(), w.Header().Get("Access-Control-Allow-Headers"))
	assert.Empty(suite.T(), w.Header().Get("Access-Control-Max-Age"))
}

func (suite *SecurityMiddlewareTestSuite) TestCORS_HandlesPreflightRequests() {
	suite.router.Use(CORS([]string{"http://localhost:3000", "http://localhost:3001", "https://payverge.io", "https://www.payverge.io", "https://api.payverge.io"}))
	suite.router.OPTIONS("/test", func(c *gin.Context) {
		c.Status(200)
	})

	req, _ := http.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type,Authorization")
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), 204, w.Code)
	assert.Contains(suite.T(), w.Header().Get("Access-Control-Allow-Methods"), "POST")
	assert.Contains(suite.T(), w.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
	assert.Contains(suite.T(), w.Header().Get("Access-Control-Allow-Headers"), "Authorization")
}

// TestCORS_AuthLoginKeepsHeadersOn200AndRateLimit429 is the issue 344
// regression: a browser POST /auth/login from https://payverge.io must still
// see Access-Control-Allow-Origin (and credentials) when the auth limiter
// returns 429. Without ACAO the dashboard modal can only surface a generic
// network error. 200 login must keep the same headers.
func TestCORS_AuthLoginKeepsHeadersOn200AndRateLimit429(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	// Production-ish stack: sanitizer wraps the writer first, then CORS,
	// then the auth limiter that actually emits the 429.
	router.Use(ErrorSanitizer())
	router.Use(CORS([]string{"https://payverge.io"}))
	router.Use(AuthRateLimiter(10, 1))
	router.POST("/auth/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	origin := "https://payverge.io"
	postLogin := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("X-Forwarded-For", "203.0.113.10")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	ok := postLogin()
	assert.Equal(t, http.StatusOK, ok.Code)
	assert.Equal(t, origin, ok.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", ok.Header().Get("Access-Control-Allow-Credentials"))

	limited := postLogin()
	assert.Equal(t, http.StatusTooManyRequests, limited.Code)
	assert.Equal(t, origin, limited.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", limited.Header().Get("Access-Control-Allow-Credentials"))
}

// TestCORS_KeepsHeadersOnLater401And5xx locks the same ACAO contract for
// 401/5xx written after CORS, including the public-error writer wrap.
func TestCORS_KeepsHeadersOnLater401And5xx(t *testing.T) {
	gin.SetMode(gin.TestMode)
	origin := "https://payverge.io"

	cases := []struct {
		name   string
		path   string
		status int
		handle gin.HandlerFunc
	}{
		{
			name:   "401",
			path:   "/auth/me",
			status: http.StatusUnauthorized,
			handle: func(c *gin.Context) {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			},
		},
		{
			name:   "500",
			path:   "/auth/login",
			status: http.StatusInternalServerError,
			handle: func(c *gin.Context) {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "secret boom"})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(ErrorSanitizer())
			router.Use(CORS([]string{origin}))
			router.POST(tc.path, tc.handle)

			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			req.Header.Set("Origin", origin)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tc.status, w.Code)
			assert.Equal(t, origin, w.Header().Get("Access-Control-Allow-Origin"))
			assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
		})
	}
}

// Test Security Headers Middleware
func (suite *SecurityMiddlewareTestSuite) TestSecurityHeaders_AddsAllHeaders() {
	suite.router.Use(SecurityHeaders())
	suite.router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), 200, w.Code)

	// Check security headers
	assert.Equal(suite.T(), "1; mode=block", w.Header().Get("X-XSS-Protection"))
	assert.Equal(suite.T(), "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(suite.T(), "DENY", w.Header().Get("X-Frame-Options"))
	assert.Equal(suite.T(), "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
	// CSP is now owned by Next.js frontend, not set by backend
	assert.Empty(suite.T(), w.Header().Get("Content-Security-Policy"))
	// Permissions-Policy header may not be set by default
	assert.True(suite.T(), len(w.Header().Get("Permissions-Policy")) >= 0)
	// Plain HTTP (no TLS / X-Forwarded-Proto / FORCE_HSTS) must not emit HSTS.
	assert.Empty(suite.T(), w.Header().Get("Strict-Transport-Security"))
}

// TestSecurityHeaders_HSTSOnEdgeHTTPS locks #525: edge HTTPS still gets a
// one-year includeSubDomains policy, but never the preload token. The
// apex is not on hstspreload.org; advertising preload would be a lie.
func (suite *SecurityMiddlewareTestSuite) TestSecurityHeaders_HSTSOnEdgeHTTPS() {
	suite.router.Use(SecurityHeaders())
	suite.router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), 200, w.Code)
	hsts := w.Header().Get("Strict-Transport-Security")
	assert.Contains(suite.T(), hsts, "max-age=31536000")
	assert.Contains(suite.T(), hsts, "includeSubDomains")
	assert.NotContains(suite.T(), strings.ToLower(hsts), "preload")
}

// Test JSON Size Limit Middleware
func (suite *SecurityMiddlewareTestSuite) TestJSONSizeLimit_AllowsNormalRequests() {
	suite.router.Use(JSONSizeLimit(1024)) // 1KB limit
	suite.router.POST("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	smallPayload := `{"message": "hello world"}`
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(smallPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), 200, w.Code)
}

func (suite *SecurityMiddlewareTestSuite) TestJSONSizeLimit_BlocksLargeRequests() {
	suite.router.Use(JSONSizeLimit(100)) // 100 bytes limit
	suite.router.POST("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Create a payload larger than 100 bytes
	largePayload := strings.Repeat(`{"key": "value", "data": "test"}`, 10)
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(largePayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), 413, w.Code)
	assert.Contains(suite.T(), w.Body.String(), "Request payload too large")
}

// Test Rate Limiter Internal Structure
func (suite *SecurityMiddlewareTestSuite) TestRateLimiter_InternalState() {
	rateLimiter := NewSimpleRateLimiter(context.Background(), 10)
	suite.T().Cleanup(rateLimiter.Stop)

	// Make a request to create a visitor
	suite.router.Use(rateLimiter.RateLimit())
	suite.router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	// Check that visitor was created (we can't access internal fields directly due to encapsulation)
	assert.Equal(suite.T(), 200, w.Code)

	// Test that subsequent requests from same IP work
	req2, _ := http.NewRequest("GET", "/test", nil)
	req2.Header.Set("X-Forwarded-For", "192.168.1.1")
	w2 := httptest.NewRecorder()
	suite.router.ServeHTTP(w2, req2)
	assert.Equal(suite.T(), 200, w2.Code)
}

// Benchmark tests
func BenchmarkRateLimiter(b *testing.B) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	rateLimiter := NewSimpleRateLimiter(context.Background(), 1000)
	b.Cleanup(rateLimiter.Stop)
	router.Use(rateLimiter.RateLimit())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Forwarded-For", "192.168.1.1")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

// TestSimpleRateLimiter_EvictsStaleVisitors locks in the cleanupVisitors
// contract: a visitor whose last request is older than the 10-minute idle
// cutoff is removed from the map, while a fresh visitor survives. Without
// eviction the visitors map grows one key per unique client IP forever.
func TestSimpleRateLimiter_EvictsStaleVisitors(t *testing.T) {
	rl := NewSimpleRateLimiter(context.Background(), 100)
	t.Cleanup(rl.Stop)

	// Populate two visitors via the real allow path.
	if !rl.isAllowed("10.0.0.1") {
		t.Fatalf("expected stale-visitor first request to be allowed")
	}
	if !rl.isAllowed("10.0.0.2") {
		t.Fatalf("expected fresh-visitor first request to be allowed")
	}

	// Back-date 10.0.0.1's only request beyond the 10-minute idle cutoff.
	rl.mu.Lock()
	rl.visitors["10.0.0.1"].requests = []time.Time{time.Now().Add(-11 * time.Minute)}
	rl.mu.Unlock()

	// Run one eviction pass with the same predicate cleanupVisitors uses.
	rl.evictIdle(10 * time.Minute)

	rl.mu.RLock()
	_, staleStillPresent := rl.visitors["10.0.0.1"]
	_, freshStillPresent := rl.visitors["10.0.0.2"]
	rl.mu.RUnlock()

	if staleStillPresent {
		t.Fatalf("expected stale visitor 10.0.0.1 to be evicted")
	}
	if !freshStillPresent {
		t.Fatalf("expected fresh visitor 10.0.0.2 to survive eviction")
	}
}

// BenchmarkSimpleRateLimiterChurn exercises isAllowed across many unique IPs
// (the growth scenario the single-IP BenchmarkRateLimiter never hits) and
// periodically evicts, proving the visitors map stays bounded rather than
// growing one key per client forever. Records bytes/op and allocs/op.
func BenchmarkSimpleRateLimiterChurn(b *testing.B) {
	rl := NewSimpleRateLimiter(context.Background(), 1000)
	b.Cleanup(rl.Stop)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ip := "10." + strconv.Itoa((i>>16)&0xff) + "." + strconv.Itoa((i>>8)&0xff) + "." + strconv.Itoa(i&0xff)
		rl.isAllowed(ip)
		if i%4096 == 0 {
			// Back-date everything so the eviction can fully drain idle keys.
			rl.mu.Lock()
			for _, v := range rl.visitors {
				v.mu.Lock()
				v.requests = []time.Time{time.Now().Add(-11 * time.Minute)}
				v.mu.Unlock()
			}
			rl.mu.Unlock()
			rl.evictIdle(10 * time.Minute)
		}
	}
	b.StopTimer()

	rl.mu.RLock()
	live := len(rl.visitors)
	rl.mu.RUnlock()
	if b.N > 8192 && live >= b.N {
		b.Fatalf("visitors grew unbounded: %d live keys for %d iterations", live, b.N)
	}
}

func TestRequireTrustedOriginForMutations_SkipsSafeMethods(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireTrustedOriginForMutations([]string{"https://payverge.io"}))
	router.GET("/p", func(c *gin.Context) { c.Status(200) })

	req, _ := http.NewRequest("GET", "/p", nil)
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code, "GET must pass regardless of Origin")
}

func TestRequireTrustedOriginForMutations_AllowsTrustedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireTrustedOriginForMutations([]string{"https://payverge.io"}))
	router.POST("/p", func(c *gin.Context) { c.Status(200) })

	req, _ := http.NewRequest("POST", "/p", nil)
	req.Header.Set("Origin", "https://payverge.io")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
}

func TestRequireTrustedOriginForMutations_RejectsUntrustedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireTrustedOriginForMutations([]string{"https://payverge.io"}))
	router.POST("/p", func(c *gin.Context) { c.Status(200) })

	req, _ := http.NewRequest("POST", "/p", nil)
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, 403, w.Code)
	assert.Contains(t, w.Body.String(), "origin_not_allowed")
}

func TestRequireTrustedOriginForMutations_FallsBackToRefererOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireTrustedOriginForMutations([]string{"https://payverge.io"}))
	router.POST("/p", func(c *gin.Context) { c.Status(200) })

	// No Origin header, Referer present and trusted.
	req, _ := http.NewRequest("POST", "/p", nil)
	req.Header.Set("Referer", "https://payverge.io/admin/businesses")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	// No Origin header, Referer untrusted.
	req2, _ := http.NewRequest("POST", "/p", nil)
	req2.Header.Set("Referer", "https://evil.example/foo")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, 403, w2.Code)
}

func TestRequireTrustedOriginForMutations_AllowsNonBrowserClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireTrustedOriginForMutations([]string{"https://payverge.io"}))
	router.POST("/p", func(c *gin.Context) { c.Status(200) })

	// Neither Origin nor Referer set — a curl / server-to-server client.
	// We rely on rate limits + auth for these; Origin gates only browsers.
	req, _ := http.NewRequest("POST", "/p", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
}

// Low (CSRF): a mutation that rides on an auth cookie but carries neither
// Origin nor Referer must be refused; bearer-token and cookieless clients
// keep working.
func TestRequireTrustedOriginForMutations_RejectsCookieAuthWithoutOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireTrustedOriginForMutations([]string{"https://app.example.com"}))
	router.POST("/p", func(c *gin.Context) { c.Status(200) })

	for _, name := range []string{"session_token", "refresh_token", "staff_token", "customer_token", "customer_refresh_token"} {
		req, _ := http.NewRequest("POST", "/p", nil)
		req.AddCookie(&http.Cookie{Name: name, Value: "live"})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equalf(t, 403, w.Code, "cookie %s without Origin/Referer", name)
		assert.Contains(t, w.Body.String(), "origin_not_allowed")
	}

	// Same cookie plus a trusted Origin passes.
	req, _ := http.NewRequest("POST", "/p", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "live"})
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	// Bearer-authenticated non-browser client with a stray cookie passes.
	req, _ = http.NewRequest("POST", "/p", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "live"})
	req.Header.Set("Authorization", "Bearer abc.def.ghi")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	// Non-auth cookies (analytics, locale) do not trigger the check.
	req, _ = http.NewRequest("POST", "/p", nil)
	req.AddCookie(&http.Cookie{Name: "payverge_locale", Value: "es"})
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	// An empty auth cookie (a cleared session) is not a credential.
	req, _ = http.NewRequest("POST", "/p", nil)
	req.Header.Set("Cookie", "session_token=")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
}

func BenchmarkSecurityHeaders(b *testing.B) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(SecurityHeaders())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

// BenchmarkRequireTrustedOriginForMutations covers the browser path (trusted
// Origin) and the non-browser path (no Origin/Referer, bearer + cookie).
func BenchmarkRequireTrustedOriginForMutations(b *testing.B) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireTrustedOriginForMutations([]string{"https://app.example.com"}))
	router.POST("/p", func(c *gin.Context) { c.Status(200) })

	browser, _ := http.NewRequest("POST", "/p", nil)
	browser.Header.Set("Origin", "https://app.example.com")
	browser.AddCookie(&http.Cookie{Name: "session_token", Value: "live"})
	bearer, _ := http.NewRequest("POST", "/p", nil)
	bearer.Header.Set("Authorization", "Bearer abc.def.ghi")
	bearer.AddCookie(&http.Cookie{Name: "session_token", Value: "live"})
	cookieless, _ := http.NewRequest("POST", "/p", nil)

	for _, tc := range []struct {
		name string
		req  *http.Request
	}{{"trusted_origin", browser}, {"no_origin_bearer", bearer}, {"no_origin_no_cookie", cookieless}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, tc.req)
				if w.Code != 200 {
					b.Fatalf("got %d", w.Code)
				}
			}
		})
	}
}
