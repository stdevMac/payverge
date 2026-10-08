package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/middleware"
	"github.com/stdevmac/payverge/backend/internal/server"
)

func TestRequestPasswordReset_EmptyBodyIs400WithoutConsumingLimiter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, _ := newAuthEnvelopeHandler(t)

	r := gin.New()
	// Burst 1: a second empty POST would 429 if the limiter ran first.
	r.POST("/password/reset-request",
		h.RejectEmptyPasswordResetEmail(),
		middleware.AuthRateLimiter(1, 1),
		h.RequestPasswordReset,
	)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/password/reset-request", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.9:5151"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equalf(t, http.StatusBadRequest, w.Code, "attempt %d body=%s", i+1, w.Body.String())
		require.NotEqual(t, http.StatusTooManyRequests, w.Code)
		var payload map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
		require.Equal(t, server.ErrCodeInvalidInput, payload["code"])
	}
}
