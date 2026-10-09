package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/stdevmac/payverge/backend/internal/metrics"
)

func TestFlowMetricsMiddlewareRecordsLabeledDuration(t *testing.T) {
	metrics.Init()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(FlowMetrics())
	r.GET("/menu", func(c *gin.Context) {
		c.Set("perf_flow", "menu_browse")
		time.Sleep(5 * time.Millisecond)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/menu", nil)
	r.ServeHTTP(w, req)

	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	var family *dto.MetricFamily
	for _, mf := range mfs {
		if mf.GetName() == "flow_request_duration_seconds" {
			family = mf
			break
		}
	}
	if family == nil {
		t.Fatalf("flow_request_duration_seconds not registered")
	}
	found := false
	for _, m := range family.Metric {
		labels := map[string]string{}
		for _, l := range m.Label {
			labels[l.GetName()] = l.GetValue()
		}
		if labels["flow"] == "menu_browse" && labels["status"] == "200" {
			if m.Histogram.GetSampleCount() < 1 {
				t.Fatalf("expected at least 1 observation, got %d", m.Histogram.GetSampleCount())
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("no histogram sample for flow=menu_browse status=200")
	}
}
