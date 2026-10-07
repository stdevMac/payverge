package server

import (
	"net/http"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
)

// ErrCodeBusinessSuspended / ErrCodeBusinessClosed are the operator-route
// denials for a business the server administrator suspended (is_active=false)
// or closed (closed_at). They are the only business locks Payverge has.
const (
	ErrCodeBusinessSuspended = database.BusinessLockCodeSuspended
	ErrCodeBusinessClosed    = database.BusinessLockCodeClosed
)

// RespondIfBusinessLocked writes 403 business_suspended / business_closed and
// returns true when b is not operational; otherwise it returns false and
// writes nothing.
func RespondIfBusinessLocked(c *gin.Context, b *database.Business) bool {
	code, message, locked := database.BusinessLockDenial(b)
	if !locked {
		return false
	}
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"error":   message,
		"code":    code,
		"message": message,
	})
	return true
}
