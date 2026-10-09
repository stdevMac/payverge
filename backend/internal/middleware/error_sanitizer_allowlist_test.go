package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func serveSanitized(t *testing.T, h gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ErrorSanitizer())
	r.GET("/t", h)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/t", nil))
	return w
}

// Decision D-3: an allowlisted 5xx code passes through with a fixed,
// server-authored message; the handler's own bytes never reach the client.
func TestErrorSanitizer_AllowlistedCodePassesWithFixedMessage(t *testing.T) {
	w := serveSanitized(t, func(c *gin.Context) {
		AllowPublicServerError(c, PublicServerErrorAINotConfigured)
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "ai_not_configured", "message": "leak: db=10.0.0.5"})
	})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	var got publicErrorEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("body %q: %v", w.Body.String(), err)
	}
	if got.Code != "ai_not_configured" || got.Error != "ai_not_configured" || got.Message != AINotConfiguredMessage {
		t.Fatalf("unexpected envelope %+v", got)
	}
	if strings.Contains(w.Body.String(), "leak") {
		t.Fatalf("handler bytes leaked: %s", w.Body.String())
	}
}

// Unknown codes, and allowlisted codes nobody marked, stay sanitized.
func TestErrorSanitizer_UnlistedCodeStaysSanitized(t *testing.T) {
	for name, h := range map[string]gin.HandlerFunc{
		"unknown code": func(c *gin.Context) {
			AllowPublicServerError(c, "db_down")
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": "db_down", "error": "pq: connection refused"})
		},
		"unmarked ai code": func(c *gin.Context) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": "ai_not_configured", "error": "custom"})
		},
	} {
		w := serveSanitized(t, h)
		if body := w.Body.String(); body != `{"error":"Service temporarily unavailable","code":"SERVICE_UNAVAILABLE"}` {
			t.Fatalf("%s: body = %s", name, body)
		}
	}
}
