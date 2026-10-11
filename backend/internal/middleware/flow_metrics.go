package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/metrics"
)

// FlowMetrics records FlowRequestDuration for handlers that tag themselves
// via c.Set("perf_flow", "<flow_name>"). Handlers without the tag are ignored
// so this can sit in the global chain without polluting cardinality.
func FlowMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		raw, ok := c.Get("perf_flow")
		if !ok {
			return
		}
		flow, _ := raw.(string)
		if flow == "" {
			return
		}
		metrics.FlowRequestDuration.
			WithLabelValues(flow, strconv.Itoa(c.Writer.Status())).
			Observe(time.Since(start).Seconds())
	}
}
