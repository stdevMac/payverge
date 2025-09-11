package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiter stores rate limiters for different clients
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     r,
		burst:    b,
	}
}

// GetLimiter returns the rate limiter for a specific key
func (rl *RateLimiter) GetLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[key]
	if !exists {
		limiter = rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[key] = limiter
	}

	return limiter
}

// RateLimit middleware for Gin
func RateLimit(requestsPerSecond int, burst int) gin.HandlerFunc {
	limiter := NewRateLimiter(rate.Limit(requestsPerSecond), burst)

	return func(c *gin.Context) {
		// Use IP address as the key
		key := c.ClientIP()
		
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

// BusinessRateLimit applies rate limiting per business
func BusinessRateLimit(requestsPerMinute int) gin.HandlerFunc {
	limiter := NewRateLimiter(rate.Every(time.Minute/time.Duration(requestsPerMinute)), 1)

	return func(c *gin.Context) {
		businessID := c.Param("businessId")
		if businessID == "" {
			businessID = c.Param("id")
		}

		if businessID != "" {
			key := "business:" + businessID
			clientLimiter := limiter.GetLimiter(key)
			
			if !clientLimiter.Allow() {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error": "Business rate limit exceeded. Please try again later.",
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// PaymentRateLimit applies stricter rate limiting for payment endpoints
func PaymentRateLimit() gin.HandlerFunc {
	limiter := NewRateLimiter(rate.Every(10*time.Second), 1) // 1 payment per 10 seconds

	return func(c *gin.Context) {
		key := c.ClientIP() + ":payment"
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
