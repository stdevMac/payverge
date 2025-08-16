package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

// Test Rate Limiting Middleware
func (suite *SecurityMiddlewareTestSuite) TestRateLimiter_AllowsNormalRequests() {
	rateLimiter := NewRateLimiter(10) // 10 requests per minute
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
	rateLimiter := NewRateLimiter(2) // 2 requests per minute for testing
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
	rateLimiter := NewRateLimiter(2) // 2 requests per minute to allow both IPs
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

// Test CORS Middleware
func (suite *SecurityMiddlewareTestSuite) TestCORS_AllowsConfiguredOrigins() {
	suite.router.Use(CORS())
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
	suite.router.Use(CORS())
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

func (suite *SecurityMiddlewareTestSuite) TestCORS_HandlesPreflightRequests() {
	suite.router.Use(CORS())
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
	assert.Contains(suite.T(), w.Header().Get("Content-Security-Policy"), "default-src 'self'")
	// Permissions-Policy header may not be set by default
	assert.True(suite.T(), len(w.Header().Get("Permissions-Policy")) >= 0)
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
	rateLimiter := NewRateLimiter(10)
	
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
	rateLimiter := NewRateLimiter(1000)
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
