package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ulule/limiter/v3"
	ginlimiter "github.com/ulule/limiter/v3/drivers/middleware/gin"
	"github.com/ulule/limiter/v3/drivers/store/memory"
)

func RateLimitMiddleware() gin.HandlerFunc {
	// Create a rate limiter: 100 requests per minute
	rate := limiter.Rate{
		Period: 1 * time.Minute,
		Limit:  100,
	}

	// Create an in-memory store
	store := memory.NewStore()

	// Create the limiter instance
	instance := limiter.New(store, rate)

	// Return the Gin middleware
	return ginlimiter.NewMiddleware(instance, ginlimiter.WithErrorHandler(func(c *gin.Context, err error) {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Rate limiter error",
		})
	}), ginlimiter.WithLimitReachedHandler(func(c *gin.Context) {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": "Rate limit exceeded",
		})
	}))
}
