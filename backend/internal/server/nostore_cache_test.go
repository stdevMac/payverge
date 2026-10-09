package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/server"
)

// TestAuthMiddlewaresSetNoStore verifies the authenticated middlewares emit
// Cache-Control: no-store so per-user/business sensitive data (bills, payments,
// profile) isn't retained in browser/shared caches — even on the 401 path
// (the header is set before the auth check). SSE handlers override to no-cache
// separately; this guards the default.
func TestAuthMiddlewaresSetNoStore(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mws := map[string]gin.HandlerFunc{
		"hybrid": server.HybridAuthenticationMiddleware(),
		"staff":  server.StaffAuthenticationMiddleware(),
		"admin":  server.AuthenticationAdminMiddleware(),
		"user":   server.AuthenticationMiddleware(),
	}

	for name, mw := range mws {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			// No auth token → the middleware sets the header then 401-aborts.
			c.Request = httptest.NewRequest("GET", "/api/v1/inside/whatever", nil)

			mw(c)

			if got := rec.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("%s middleware: Cache-Control = %q, want \"no-store\"", name, got)
			}
			// Sanity: with no token it should reject (not pass through unauthenticated).
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s middleware: expected 401 with no token, got %d", name, rec.Code)
			}
		})
	}
}
