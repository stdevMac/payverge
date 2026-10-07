package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSecMedRouteWiring pins the main.go route hardening from the
// open-source-release security workstream (sec-med). String-presence checks
// match rate_limit_wiring_test.go.
func TestSecMedRouteWiring(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	src, err := os.ReadFile(filepath.Join(wd, "main.go"))
	require.NoError(t, err)
	body := string(src)

	required := []string{
		// M-logs: route-level body cap in front of the public error ingest.
		`publicRoutes.POST("/logs/error", errorLogRateLimiter.RateLimit(), middleware.JSONSizeLimit(handlers.MaxErrorLogBodyBytes), errorLogHandler.IngestError)`,
		// M-track: delivery tracking is throttled per client, not by a
		// business limiter that no-ops without a business param.
		`deliveryTrackRateLimit := middleware.AuthRateLimiter(120, 20)`,
		`publicRoutes.GET("/delivery/:delivery_number/track", deliveryTrackRateLimit, deliveryHandler.TrackDelivery)`,
		// M-siwe: explicitly configured origins are acceptable SIWE domains
		// besides PUBLIC_URL.
		`server.SetSIWETrustedOrigins(strings.Split(os.Getenv("ALLOWED_ORIGINS"), ","))`,
		// Low (missing limiters): public reads that had no route limiter.
		`publicRoutes.GET("/reservations/:confirmationCode", publicReservationLimiter, server.GetPublicReservationByCode)`,
		`publicRoutes.POST("/staff/accept-invitation", staffAuthLimiter, server.AcceptInvitation)`,
		`guestCatalogReadLimiter := newGuestCatalogReadLimiter()`,
		`publicRoutes.GET("/guest/table/:code/menu-translations", guestCatalogReadLimiter, currencyHandler.GetGuestMenuTranslations)`,
		`publicRoutes.GET("/businesses/:business_id/payment-plugins", guestCatalogReadLimiter, pluginHandlers.GetBusinessPaymentPlugins)`,
	}
	for _, snippet := range required {
		assert.Containsf(t, body, snippet, "main.go must keep sec-med wiring %q", snippet)
	}

	forbidden := []string{
		`publicRoutes.GET("/reservations/:confirmationCode", server.GetPublicReservationByCode)`,
		`publicRoutes.POST("/staff/accept-invitation", server.AcceptInvitation)`,
		`publicRoutes.GET("/guest/table/:code/menu-translations", currencyHandler.GetGuestMenuTranslations)`,
		`publicRoutes.GET("/businesses/:business_id/payment-plugins", pluginHandlers.GetBusinessPaymentPlugins)`,
		// M-siwe: resolveAllowedOrigins carries built-in hosted defaults; a
		// self-hosted instance must not accept SIWE messages naming them.
		`server.SetSIWETrustedOrigins(allowedOrigins)`,
	}
	for _, snippet := range forbidden {
		assert.NotContainsf(t, body, snippet, "main.go must not register %q without its guard", snippet)
	}
}

// The catalog-read limiter throttles per client and honours the shared
// GUEST_TABLE_READ_RATE_LIMIT_REQUESTS_PER_MINUTE override.
func TestGuestCatalogReadLimiter_ThrottlesPerClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("GUEST_TABLE_READ_RATE_LIMIT_REQUESTS_PER_MINUTE", "3")

	r := gin.New()
	r.GET("/api/v1/businesses/:business_id/payment-plugins", newGuestCatalogReadLimiter(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	get := func(ip string) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/businesses/1/payment-plugins", nil)
		req.RemoteAddr = ip + ":1234"
		r.ServeHTTP(w, req)
		return w.Code
	}
	for i := 0; i < 3; i++ {
		require.Equal(t, http.StatusOK, get("203.0.113.7"), "request %d within budget", i+1)
	}
	assert.Equal(t, http.StatusTooManyRequests, get("203.0.113.7"))
	assert.Equal(t, http.StatusOK, get("198.51.100.9"), "another client has its own bucket")
}
