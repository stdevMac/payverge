package middleware

import (
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const RequestIDHeader = "X-Request-Id"
const RequestIDKey = "request_id"

// Only accept alphanumeric + dashes, max 64 chars
var validRequestID = regexp.MustCompile(`^[a-zA-Z0-9\-]{1,64}$`)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(RequestIDHeader)
		if requestID == "" || !validRequestID.MatchString(requestID) {
			requestID = uuid.New().String()
		}
		c.Set(RequestIDKey, requestID)
		c.Header(RequestIDHeader, requestID)
		c.Next()
	}
}
