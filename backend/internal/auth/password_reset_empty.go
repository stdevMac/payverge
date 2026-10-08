package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/server"
)

// RejectEmptyPasswordResetEmail returns 400 before the auth rate limiter so
// blank submits cannot burn the bucket. Well-formed addresses still pass
// through to authLimiter unchanged.
func (h *AuthHandler) RejectEmptyPasswordResetEmail() gin.HandlerFunc {
	return rejectEmptyPasswordResetEmail
}

func rejectEmptyPasswordResetEmail(c *gin.Context) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid request")
		c.Abort()
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))

	var req struct {
		Email string `json:"email"`
	}
	_ = json.Unmarshal(raw, &req)
	if strings.TrimSpace(req.Email) == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Email is required")
		c.Abort()
		return
	}
	c.Next()
}
