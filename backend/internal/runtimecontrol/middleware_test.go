package runtimecontrol

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMiddlewareMaintenancePreservesHealthAndAdminRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := New(newRuntimeControlTestDB(t))
	require.NoError(t, svc.SetControl(t.Context(), SetControlInput{
		Key: ControlMaintenance, Enabled: true, Owner: "platform", Reason: "drill",
		ExpiresAt: time.Now().Add(time.Hour), Actor: "admin@example.test",
	}))
	r := gin.New()
	r.Use(Middleware(svc))
	r.GET("/api/v1/health/live", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.PUT("/api/v1/admin/runtime-controls/payments", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.POST("/api/v1/inside/businesses/1/menu", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	require.Equal(t, http.StatusNoContent, runtimeRequest(r, http.MethodGet, "/api/v1/health/live").Code)
	require.Equal(t, http.StatusNoContent, runtimeRequest(r, http.MethodPut, "/api/v1/admin/runtime-controls/payments").Code)
	require.Equal(t, http.StatusServiceUnavailable, runtimeRequest(r, http.MethodPost, "/api/v1/inside/businesses/1/menu").Code)
}

func TestMiddlewareFeatureSwitchesBlockOnlyNewAttempts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := New(newRuntimeControlTestDB(t))
	for _, key := range []ControlKey{ControlPayments, ControlFiscal, ControlAI, ControlUploads, ControlGuestOrders} {
		require.NoError(t, svc.SetControl(t.Context(), SetControlInput{
			Key: key, Enabled: false, Owner: "launch", Reason: "drill",
			ExpiresAt: time.Now().Add(time.Hour), Actor: "admin@example.test",
		}))
	}
	r := gin.New()
	r.Use(Middleware(svc))
	for _, path := range []string{
		"/api/v1/guest/bill/token/plugin-payment",
		"/api/v1/inside/businesses/1/fiscal/receipts/issue",
		"/api/v1/inside/businesses/1/ai/director/ask",
		"/api/v1/inside/businesses/1/uploads",
		"/api/v1/guest/table/code/order",
	} {
		r.POST(path, func(c *gin.Context) { c.Status(http.StatusNoContent) })
		require.Equal(t, http.StatusServiceUnavailable, runtimeRequest(r, http.MethodPost, path).Code, path)
	}
	// Provider callbacks reconcile prior attempts and remain available.
	r.POST("/api/v1/webhooks/stripe", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	require.Equal(t, http.StatusNoContent, runtimeRequest(r, http.MethodPost, "/api/v1/webhooks/stripe").Code)
}

func TestMiddlewareReadOnlyPreservesReadsAndBlocksMutations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := New(newRuntimeControlTestDB(t))
	require.NoError(t, svc.SetControl(t.Context(), SetControlInput{
		Key: ControlReadOnly, Enabled: true, Owner: "platform", Reason: "incident",
		ExpiresAt: time.Now().Add(time.Hour), Actor: "admin@example.test",
	}))
	r := gin.New()
	r.Use(Middleware(svc))
	r.GET("/api/v1/inside/businesses/1/menu", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.POST("/api/v1/inside/businesses/1/menu", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	require.Equal(t, http.StatusNoContent, runtimeRequest(r, http.MethodGet, "/api/v1/inside/businesses/1/menu").Code)
	require.Equal(t, http.StatusServiceUnavailable, runtimeRequest(r, http.MethodPost, "/api/v1/inside/businesses/1/menu").Code)
}

func runtimeRequest(r http.Handler, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	r.ServeHTTP(w, req)
	return w
}
