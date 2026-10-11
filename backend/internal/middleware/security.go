package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/metrics"

	"github.com/gin-gonic/gin"
)

// SimpleRateLimiter is an in-memory rate limiter.
// LIMITATION: State resets on process restart and is not shared across instances.
// For multi-instance deployments, migrate to a Redis-backed rate limiter.
type SimpleRateLimiter struct {
	visitors map[string]*Visitor
	mu       sync.RWMutex
	rate     int           // requests per minute
	window   time.Duration // time window
	stopOnce sync.Once
	stop     chan struct{}
}

// Visitor represents a client with request tracking
type Visitor struct {
	requests []time.Time
	mu       sync.Mutex
}

// NewSimpleRateLimiter creates a new simple rate limiter whose cleanup
// goroutine can be stopped via Stop() or by cancelling ctx. Pass
// context.Background() for lifetime-of-process limiters.
func NewSimpleRateLimiter(ctx context.Context, requestsPerMinute int) *SimpleRateLimiter {
	rl := &SimpleRateLimiter{
		visitors: make(map[string]*Visitor),
		rate:     requestsPerMinute,
		window:   time.Minute,
		stop:     make(chan struct{}),
	}

	// Clean up old visitors every 5 minutes; exits on Stop() or ctx.Done().
	go rl.cleanupVisitors(ctx)

	return rl
}

// Stop terminates the background cleanup goroutine. Safe to call multiple times.
func (rl *SimpleRateLimiter) Stop() {
	rl.stopOnce.Do(func() { close(rl.stop) })
}

// RateLimit middleware function
func (rl *SimpleRateLimiter) RateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Health/liveness/readiness probes originate from the orchestrator/proxy
		// IP and must never be 429'd — a non-2xx triggers a pod kill mid-spike (EXT-6).
		// /media/* serves immutable public objects (menu photos, logos): one
		// guest menu page fans out to dozens of them, and they cost a stat +
		// file read, so the per-IP API budget must not starve them. The route
		// carries its own per-IP bucket instead (MediaRateLimit).
		if path := c.Request.URL.Path; strings.HasPrefix(path, "/api/v1/health") || strings.HasPrefix(path, "/media/") {
			c.Next()
			return
		}

		// WebSocket exemption removed - using polling instead

		ip := ClientRateLimitKey(c)

		if !rl.isAllowed(ip) {
			if c.FullPath() == "/api/v1/guest/bill/:bill_token/request-alternative-payment" ||
				c.FullPath() == "/guest/bill/:bill_token/request-alternative-payment" {
				metrics.RecordAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestRateLimited)
			}
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded. Please try again later.",
				"retry_after": 60,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// isAllowed checks if the request is within rate limits
func (rl *SimpleRateLimiter) isAllowed(ip string) bool {
	rl.mu.Lock()
	visitor, exists := rl.visitors[ip]
	if !exists {
		visitor = &Visitor{
			requests: make([]time.Time, 0),
		}
		rl.visitors[ip] = visitor
	}
	rl.mu.Unlock()

	visitor.mu.Lock()
	defer visitor.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Remove old requests outside the time window
	var validRequests []time.Time
	for _, reqTime := range visitor.requests {
		if reqTime.After(cutoff) {
			validRequests = append(validRequests, reqTime)
		}
	}
	visitor.requests = validRequests

	// Check if we're within the rate limit
	if len(visitor.requests) >= rl.rate {
		return false
	}

	// Add current request
	visitor.requests = append(visitor.requests, now)
	return true
}

// cleanupVisitors removes old visitors to prevent memory leaks.
// Exits when Stop() is called or ctx is cancelled so repeated limiter
// construction in tests doesn't leak tickers.
func (rl *SimpleRateLimiter) cleanupVisitors(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rl.stop:
			return
		case <-ticker.C:
			rl.evictIdle(10 * time.Minute)
		}
	}
}

// evictIdle removes visitors whose last request is older than idleFor (or who
// have no requests), bounding the visitors map under high unique-IP churn.
// Extracted from cleanupVisitors so the eviction predicate is unit-testable
// without waiting on the 5-minute ticker.
func (rl *SimpleRateLimiter) evictIdle(idleFor time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	cutoff := time.Now().Add(-idleFor)
	for ip, visitor := range rl.visitors {
		visitor.mu.Lock()
		if len(visitor.requests) == 0 || visitor.requests[len(visitor.requests)-1].Before(cutoff) {
			delete(rl.visitors, ip)
		}
		visitor.mu.Unlock()
	}
}

// SecurityHeaders adds security headers to responses
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent clickjacking
		c.Header("X-Frame-Options", "DENY")

		// Prevent MIME type sniffing
		c.Header("X-Content-Type-Options", "nosniff")

		// XSS protection
		c.Header("X-XSS-Protection", "1; mode=block")

		// Referrer policy
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// CSP is set by Next.js (frontend/next.config.js) with a comprehensive policy
		// that includes external image sources, fonts, and API connections.
		// Setting a second CSP here would conflict. Only the frontend should own CSP.

		// Strict Transport Security — emitted when the *edge* request was
		// HTTPS. Direct TLS shows up as c.Request.TLS != nil; behind our
		// Caddy reverse proxy, TLS terminates at the edge and the app sees
		// plain HTTP, so we also honor X-Forwarded-Proto. Operators can
		// also force-enable HSTS via env in environments where neither
		// signal is available (e.g. Cloud Run). Do not add `preload` until
		// the deployment's apex domain is accepted on hstspreload.org.
		if isEdgeHTTPS(c) {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		c.Next()
	}
}

// isEdgeHTTPS reports whether the *client-facing* connection was HTTPS. It
// trusts X-Forwarded-Proto only because upstream requests hit the Gin server
// through our reverse proxy that sets this header — direct internet traffic
// should never reach the app.
func isEdgeHTTPS(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	if proto := c.GetHeader("X-Forwarded-Proto"); strings.EqualFold(proto, "https") {
		return true
	}
	if os.Getenv("FORCE_HSTS") == "true" {
		return true
	}
	return false
}

// CORS middleware for handling Cross-Origin Resource Sharing
func CORS(allowedOrigins []string) gin.HandlerFunc {

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Debug logging
		if gin.Mode() == gin.DebugMode {
			fmt.Printf("[CORS] Request from origin: %s, Method: %s, Path: %s\n", origin, c.Request.Method, SafeRequestPath(c))
		}

		allowed := false
		for _, allowedOrigin := range allowedOrigins {
			if origin == allowedOrigin {
				allowed = true
				break
			}
		}

		emitCORSMetadata := allowed || origin == ""

		// Always set CORS headers for allowed origins
		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		if emitCORSMetadata {
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-Requested-With, X-Request-Id")
			c.Header("Access-Control-Max-Age", "3600") // 1 hour — allows faster CORS policy updates
		}

		// Handle preflight OPTIONS request
		if c.Request.Method == "OPTIONS" {
			if gin.Mode() == gin.DebugMode {
				fmt.Printf("[CORS] Handling OPTIONS preflight for: %s\n", origin)
			}
			if !allowed && origin != "" {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// RequireTrustedOriginForMutations blocks state-changing requests (POST,
// PUT, PATCH, DELETE) whose Origin header (or Referer origin, when Origin
// is absent) isn't in `allowedOrigins`. Apply on authenticated route groups
// — not on webhook / guest / public-read endpoints, which must accept
// server-to-server or cross-origin calls.
//
// Why: cookies are SameSite=Lax, which permits top-level navigations to
// carry credentials. An attacker site cannot directly POST cross-origin
// because Lax strips the cookie, but a same-registrable-domain subdomain
// (example.com ↔ api.example.com) or a misconfigured origin could.
// Layering an Origin-allowlist check on top of SameSite closes the
// subdomain-XSS pivot and catches CORS misconfigurations early.
//
// If neither Origin nor Referer is present, the request is permitted only
// when it does not ride on an auth cookie: non-browser clients legitimately
// omit both headers but authenticate with an Authorization: Bearer header,
// while browsers always send Origin on credentialed cross-origin mutations.
// A cookie-authenticated mutation with no Origin and no Referer (a stripped
// or privacy-suppressed header on a forged request) is refused, so the guard
// no longer fails open for the one case it exists to stop.
func RequireTrustedOriginForMutations(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = struct{}{}
	}
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}

		reqOrigin := strings.TrimSpace(c.Request.Header.Get("Origin"))
		if reqOrigin == "" {
			if ref := c.Request.Header.Get("Referer"); ref != "" {
				if u, err := url.Parse(ref); err == nil && u.Scheme != "" && u.Host != "" {
					reqOrigin = u.Scheme + "://" + u.Host
				}
			}
		}

		if reqOrigin == "" {
			// Cheapest checks first: no Cookie header at all, then an
			// explicit bearer credential, and only then a cookie parse.
			if c.Request.Header.Get("Cookie") == "" || requestHasBearerAuth(c.Request) || !requestHasAuthCookie(c.Request) {
				c.Next()
				return
			}
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "request origin required for cookie-authenticated changes",
				"code":  "origin_not_allowed",
			})
			return
		}

		if _, ok := allowed[reqOrigin]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "untrusted request origin",
				"code":  "origin_not_allowed",
			})
			return
		}

		c.Next()
	}
}

// csrfAuthCookieNames are the cookies that authenticate a browser session
// (owner/admin, staff, customer). A mutation carrying one of them is the
// cross-site request forgery target RequireTrustedOriginForMutations guards.
var csrfAuthCookieNames = []string{
	"session_token",
	"refresh_token",
	"staff_token",
	"customer_token",
	"customer_refresh_token",
}

func requestHasAuthCookie(r *http.Request) bool {
	for _, name := range csrfAuthCookieNames {
		if ck, err := r.Cookie(name); err == nil && strings.TrimSpace(ck.Value) != "" {
			return true
		}
	}
	return false
}

// requestHasBearerAuth reports an explicit Authorization: Bearer credential.
// A cross-site page cannot attach one without a CORS preflight, so it marks a
// deliberate (non-ambient) client.
func requestHasBearerAuth(r *http.Request) bool {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	return len(auth) > len("Bearer ") && strings.EqualFold(auth[:len("Bearer ")], "Bearer ")
}
