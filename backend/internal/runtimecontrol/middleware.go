package runtimecontrol

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Middleware applies global maintenance/read-only controls and narrow feature
// switches before handlers can create side effects.
func Middleware(service *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isReadMethod(c.Request.Method) || isRecoveryPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		for _, key := range []ControlKey{ControlMaintenance, ControlReadOnly} {
			enabled, err := service.Enabled(c.Request.Context(), key)
			if err != nil || enabled {
				block(c, key)
				return
			}
		}
		if key, ok := featureControlForPath(c.Request.URL.Path); ok {
			enabled, err := service.Enabled(c.Request.Context(), key)
			if err != nil || !enabled {
				block(c, key)
				return
			}
		}
		c.Next()
	}
}

func isReadMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func isRecoveryPath(path string) bool {
	return strings.HasPrefix(path, "/api/v1/health") || strings.HasPrefix(path, "/api/v1/admin/runtime-controls") || strings.HasPrefix(path, "/api/v1/webhooks/")
}

func featureControlForPath(path string) (ControlKey, bool) {
	switch {
	case strings.Contains(path, "/guest/table/") && strings.Contains(path, "/order"):
		return ControlGuestOrders, true
	case strings.Contains(path, "/upload"):
		return ControlUploads, true
	case strings.Contains(path, "/fiscal/"):
		return ControlFiscal, true
	case strings.Contains(path, "/ai/") || strings.Contains(path, "/ai-"):
		return ControlAI, true
	case strings.Contains(path, "payment") || strings.Contains(path, "/checkout") || strings.Contains(path, "/crypto-quote"):
		return ControlPayments, true
	default:
		return "", false
	}
}

func block(c *gin.Context, key ControlKey) {
	c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": "RUNTIME_CONTROL_DISABLED", "error": "This operation is temporarily unavailable.", "control": key})
}
