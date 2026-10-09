package health

import (
	"context"
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// detailTokenEnv unlocks detailed health/readiness payloads (DB latency,
// uptime, goroutine count, component matrix). Same operational pattern as
// METRICS_TOKEN: unset means the public surface stays minimal.
const detailTokenEnv = "HEALTH_DETAIL_TOKEN"

// hasValidDetailToken accepts only Authorization: Bearer <token>.
// A ?token= query parameter is not accepted; query strings land in access logs.
func hasValidDetailToken(c *gin.Context) bool {
	expected := strings.TrimSpace(os.Getenv(detailTokenEnv))
	if expected == "" {
		return false
	}
	auth := c.GetHeader("Authorization")
	raw, ok := strings.CutPrefix(auth, "Bearer ")
	if !ok {
		return false
	}
	provided := strings.TrimSpace(raw)
	if provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

// PublicHandler wraps the detailed health Handler. Without a valid detail
// token it pings the database (1s timeout) and returns only {"status":"ok"}
// or 503 {"status":"unavailable"}. A down database must not look healthy, and
// public probes still cannot fingerprint latency, uptime, or goroutine counts (#286).
func PublicHandler(db *gorm.DB, detailed gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hasValidDetailToken(c) {
			detailed(c)
			return
		}
		parent := context.Background()
		if c.Request != nil {
			parent = c.Request.Context()
		}
		ctx, cancel := context.WithTimeout(parent, time.Second)
		defer cancel()
		if checkDatabaseCtx(ctx, db).Status == "unhealthy" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

// PublicReadinessHandler wraps ReadinessHandler. Without a valid detail token
// it returns a binary ready/not-ready status (HTTP 200/503) with no component
// matrix. Orchestrators that need the matrix must present HEALTH_DETAIL_TOKEN.
func PublicReadinessHandler(state ReadinessState) gin.HandlerFunc {
	detailed := ReadinessHandler(state)
	return func(c *gin.Context) {
		if hasValidDetailToken(c) {
			detailed(c)
			return
		}
		if !evaluateReady(c, state) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	}
}
