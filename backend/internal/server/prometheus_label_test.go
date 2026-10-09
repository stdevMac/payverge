package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stretchr/testify/require"
)

// Prometheus labels must use the route template so a capability token in the
// URL path never becomes a label value (and never explodes cardinality).
func TestPrometheusMiddlewareLabelsRouteTemplateNotRawPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(PrometheusMiddleware())
	r.GET("/api/v1/guest/bill/:bill_token/promlabel-split", func(c *gin.Context) { c.Status(http.StatusOK) })

	before := testutil.ToFloat64(metrics.TotalRequests.WithLabelValues("/api/v1/guest/bill/:bill_token/promlabel-split", "GET", "200"))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/guest/bill/PROMSECRET/promlabel-split", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/guest/bill/PROMSECRET2/unmatched", nil))

	after := testutil.ToFloat64(metrics.TotalRequests.WithLabelValues("/api/v1/guest/bill/:bill_token/promlabel-split", "GET", "200"))
	require.Equal(t, before+1, after)
	// Unmatched requests carry an empty route label, never the raw path.
	require.Equal(t, float64(1), testutil.ToFloat64(metrics.TotalRequests.WithLabelValues("", "GET", "404")))
}
