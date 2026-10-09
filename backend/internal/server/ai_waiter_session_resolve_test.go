package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestResolveAiWaiterSessionToken locks the auth path that lets the guest UI
// stop holding a replayable AI-waiter bearer token in JS-readable storage
// (localStorage), which any XSS could dump and replay. The chat/poll handlers
// must accept the session via the HttpOnly pv_ai_waiter_session cookie when the
// client sends no explicit token.
func TestResolveAiWaiterSessionToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newCtx := func(explicitCookie string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		if explicitCookie != "" {
			req.AddCookie(&http.Cookie{Name: "pv_ai_waiter_session", Value: explicitCookie})
		}
		c.Request = req
		return c
	}

	// Explicit token wins (backwards-compatible during the deploy window).
	assert.Equal(t, "explicit-tok", resolveAiWaiterSessionToken(newCtx("cookie-tok"), "explicit-tok"))

	// No explicit token => fall back to the HttpOnly cookie.
	assert.Equal(t, "cookie-tok", resolveAiWaiterSessionToken(newCtx("cookie-tok"), ""))

	// Whitespace-only explicit token is treated as empty and falls back.
	assert.Equal(t, "cookie-tok", resolveAiWaiterSessionToken(newCtx("cookie-tok"), "   "))

	// Neither present => empty (caller 404s).
	assert.Equal(t, "", resolveAiWaiterSessionToken(newCtx(""), ""))
}
