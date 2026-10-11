package middleware

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/logger"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiter stores rate limiters for different clients with automatic
// eviction of entries that have not been seen for a configurable TTL.
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	lastSeen map[string]time.Time
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
}

// NewRateLimiter creates a new rate limiter and starts a background
// goroutine that evicts entries idle for more than 10 minutes.
func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	rl := &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		lastSeen: make(map[string]time.Time),
		rate:     r,
		burst:    b,
	}
	logger.SafeGo(func() { rl.cleanupLoop(10 * time.Minute) })
	return rl
}

// GetLimiter returns the rate limiter for a specific key and records the
// current time so the entry is not evicted while still active.
func (rl *RateLimiter) GetLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[key]
	if !exists {
		limiter = rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[key] = limiter
	}
	rl.lastSeen[key] = time.Now()

	return limiter
}

// evictStale removes entries that have not been seen for longer than ttl.
func (rl *RateLimiter) evictStale(ttl time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-ttl)
	for key, seen := range rl.lastSeen {
		if seen.Before(cutoff) {
			delete(rl.limiters, key)
			delete(rl.lastSeen, key)
		}
	}
}

// cleanupLoop runs evictStale on a recurring interval.
func (rl *RateLimiter) cleanupLoop(ttl time.Duration) {
	ticker := time.NewTicker(ttl)
	defer ticker.Stop()
	for range ticker.C {
		rl.evictStale(ttl)
	}
}

// RateLimit middleware for Gin
func RateLimit(requestsPerSecond int, burst int) gin.HandlerFunc {
	limiter := NewRateLimiter(rate.Limit(requestsPerSecond), burst)

	return func(c *gin.Context) {
		// Use the client IP (IPv6 bucketed per /64) as the key
		key := ClientRateLimitKey(c)

		// Get limiter for this client
		clientLimiter := limiter.GetLimiter(key)

		if !clientLimiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded. Please try again later.",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// BusinessRateLimit applies rate limiting per business with a default burst of 1.
// Use BusinessRateLimitWithBurst when a higher burst is needed for legitimate
// multi-step flows (e.g. delivery quote → re-quote → checkout).
// It tries Gin route params in this order: "id", "business_id", "businessId".
func BusinessRateLimit(requestsPerMinute int) gin.HandlerFunc {
	return BusinessRateLimitWithBurst(requestsPerMinute, 1)
}

// BusinessRateLimitWithBurst applies rate limiting per business with an
// explicit burst budget. Burst should match the maximum number of legitimate
// back-to-back requests a single user action can trigger.
//
// Routes without a business route param (for example the public
// /delivery/:delivery_number/track lookup) fall back to a per-client bucket
// instead of being waved through (M-track): an absent param used to make the
// limiter a no-op, leaving delivery-number enumeration unthrottled.
//
// Keep this conservative: a higher burst expands the attack window for
// abuse-sensitive endpoints (AI prompts, search). The delivery quote +
// re-quote + checkout sequence is currently the highest legitimate burst
// at 3, served by burst=5.
func BusinessRateLimitWithBurst(requestsPerMinute int, burst int) gin.HandlerFunc {
	limiter := NewRateLimiter(rate.Every(time.Minute/time.Duration(requestsPerMinute)), burst)

	return func(c *gin.Context) {
		businessID := c.Param("id")
		if businessID == "" {
			businessID = c.Param("business_id")
		}
		if businessID == "" {
			businessID = c.Param("businessId")
		}

		clientKey := ClientRateLimitKey(c)
		key := "client:" + clientKey
		if businessID != "" {
			key = "business:" + businessID + ":client:" + clientKey
		}

		if !limiter.GetLimiter(key).Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Business rate limit exceeded. Please try again later.",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// AuthRateLimiter applies rate limiting for authentication endpoints, keyed by client IP.
// requestsPerMinute controls the sustained rate; burst controls the token bucket capacity.
// 429 responses include Retry-After and X-RateLimit-* so clients can back off (#295).
func AuthRateLimiter(requestsPerMinute int, burst int) gin.HandlerFunc {
	if requestsPerMinute <= 0 {
		requestsPerMinute = 1
	}
	if burst <= 0 {
		burst = 1
	}
	limiter := NewRateLimiter(rate.Every(time.Minute/time.Duration(requestsPerMinute)), burst)
	limitHeader := strconv.Itoa(requestsPerMinute)

	return func(c *gin.Context) {
		key := ClientRateLimitKey(c)
		clientLimiter := limiter.GetLimiter(key)

		reservation := clientLimiter.Reserve()
		if !reservation.OK() {
			writeAuthRateLimitExceeded(c, limitHeader, 60)
			return
		}
		if delay := reservation.Delay(); delay > 0 {
			reservation.Cancel()
			secs := int(math.Ceil(delay.Seconds()))
			if secs < 1 {
				secs = 1
			}
			writeAuthRateLimitExceeded(c, limitHeader, secs)
			return
		}

		c.Header("X-RateLimit-Limit", limitHeader)
		c.Next()
	}
}

func writeAuthRateLimitExceeded(c *gin.Context, limitHeader string, retryAfterSeconds int) {
	c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
	c.Header("X-RateLimit-Limit", limitHeader)
	c.Header("X-RateLimit-Remaining", "0")
	// Reset is an epoch hint for when the bucket is expected to replenish.
	c.Header("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Duration(retryAfterSeconds)*time.Second).Unix(), 10))
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error": "Too many requests. Please try again later.",
	})
	c.Abort()
}

// PaymentRateLimit applies stricter rate limiting for payment endpoints.
// The rate-limit bucket is scoped per (business_id, bill_id) when both route
// params are present, so guests at a NATed venue (shared public IP) are not
// mutually throttled across different bills. When either param is absent (e.g.
// PayPal/Tron return routes), the key falls back to the client IP to preserve
// the original single-IP throttle.
func PaymentRateLimit() gin.HandlerFunc {
	limiter := NewRateLimiter(rate.Every(10*time.Second), 1) // 1 payment per 10 seconds

	return func(c *gin.Context) {
		// Scope the bucket per (business, bill) so one guest's payment does
		// not throttle another at the same NATed venue. Fall back to IP-only
		// when route params are absent (non-guest payment routes).
		bizID := c.Param("business_id")
		if bizID == "" {
			bizID = c.Param("id")
		}
		billID := c.Param("bill_id")
		if billID == "" {
			billID = c.Param("bill_token")
		}
		if billID == "" {
			billID = c.Param("bill_number")
		}
		key := "payment:ip:" + ClientRateLimitKey(c)
		if bizID != "" && billID != "" {
			key = "payment:biz:" + bizID + ":bill:" + billID
		}
		clientLimiter := limiter.GetLimiter(key)

		if !clientLimiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Payment rate limit exceeded. Please wait before making another payment.",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// ManagerActionRateLimit throttles the manager-PIN-gated correction routes
// (bill void/refund and the crypto-refund lifecycle). These used to reuse
// PaymentRateLimit, which was wrong twice over (issue 909):
//
//   - /bills/:bill_id/void carries no business_id/id param, so the payment key
//     collapsed to "payment:ip:<ClientIP>" — one 1-per-10s token shared by every
//     void, refund and crypto-refund from a venue. A single 404 probe spent it,
//     and the next real void came back 429 before the handler could run its own
//     validation.
//   - the 429 body told an expo correcting a check to "wait before making
//     another payment".
//
// This limiter has its own bucket (never shared with guest payments), is scoped
// per (authenticated actor, route, target resource) so distinct corrections do
// not starve each other, and still fails closed: unauthenticated or
// unidentifiable callers fall back to an IP bucket rather than being waved
// through, and sustained hammering of one target is throttled.
func ManagerActionRateLimit() gin.HandlerFunc {
	// A human correcting checks at the pass does a handful of actions in a
	// burst, then pauses. 5 immediate actions, replenishing one every 2s.
	limiter := NewRateLimiter(rate.Every(2*time.Second), 5)

	return func(c *gin.Context) {
		key := "manager:" + managerActionActorKey(c) + ":" + c.FullPath() + ":" + managerActionTargetKey(c)

		if !limiter.GetLimiter(key).Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":  "MANAGER_ACTION_RATE_LIMITED",
				"error": "Too many void/refund actions in a row. Wait a moment and try again.",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// managerActionActorKey identifies the operator behind a PIN-gated correction.
// Group-level authentication has already populated the context by the time this
// route-level middleware runs. Falls back to the client IP so an unauthenticated
// or unrecognized caller is still bucketed (fail closed), never exempted.
func managerActionActorKey(c *gin.Context) string {
	if staffID, ok := c.Get("staff_id"); ok {
		if s := formatContextID(staffID); s != "" {
			return "staff:" + s
		}
	}
	if userID, ok := c.Get("user_id"); ok {
		if s := formatContextID(userID); s != "" {
			return "user:" + s
		}
	}
	if address, ok := c.Get("address"); ok {
		if s, isStr := address.(string); isStr && s != "" {
			return "addr:" + s
		}
	}
	return "ip:" + ClientRateLimitKey(c)
}

// managerActionTargetKey names the row the correction acts on so voiding two
// different checks does not consume one shared token.
func managerActionTargetKey(c *gin.Context) string {
	target := c.Param("bill_id")
	if target == "" {
		target = c.Param("refund_id")
	}
	if target == "" {
		target = c.Param("id")
	}
	if target == "" {
		return "-"
	}
	return target
}

// formatContextID renders the numeric/string identities the auth middlewares
// stash (uint from live staff hydration, float64 from JWT claims, string from
// OAuth subjects) as a stable bucket key fragment.
func formatContextID(v interface{}) string {
	switch id := v.(type) {
	case uint:
		return strconv.FormatUint(uint64(id), 10)
	case uint64:
		return strconv.FormatUint(id, 10)
	case int:
		return strconv.Itoa(id)
	case int64:
		return strconv.FormatInt(id, 10)
	case float64:
		return strconv.FormatInt(int64(id), 10)
	case string:
		return id
	default:
		return ""
	}
}
