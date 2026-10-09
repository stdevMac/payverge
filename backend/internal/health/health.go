package health

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// HealthCheck represents the health status of a service component
type HealthCheck struct {
	Service string `json:"service"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// HealthResponse represents the overall health response
type HealthResponse struct {
	Status    string        `json:"status"`
	Timestamp time.Time     `json:"timestamp"`
	Version   string        `json:"version"`
	Uptime    string        `json:"uptime"`
	Checks    []HealthCheck `json:"checks"`
}

var startTime = time.Now()

// Handler returns a health check handler
func Handler(db *gorm.DB, version string) gin.HandlerFunc {
	return func(c *gin.Context) {
		checks := []HealthCheck{}
		overallStatus := "healthy"

		// Check PostgreSQL database connection
		dbCheck := checkDatabase(db)
		checks = append(checks, dbCheck)
		if dbCheck.Status != "healthy" {
			overallStatus = "unhealthy"
		}

		// Check system resources
		systemCheck := checkSystem()
		checks = append(checks, systemCheck)
		if systemCheck.Status != "healthy" {
			overallStatus = "degraded"
		}

		response := HealthResponse{
			Status:    overallStatus,
			Timestamp: time.Now(),
			Version:   version,
			Uptime:    time.Since(startTime).String(),
			Checks:    checks,
		}

		statusCode := http.StatusOK
		switch overallStatus {
		case "unhealthy":
			statusCode = http.StatusServiceUnavailable
		case "degraded":
			statusCode = http.StatusPartialContent
		}

		c.JSON(statusCode, response)
	}
}

// ComponentResult is a non-secret readiness component status. Code is an
// issue code when Status is "failed"; it must never contain raw secrets.
type ComponentResult struct {
	Component string `json:"component"`
	Status    string `json:"status"`
	Code      string `json:"code,omitempty"`
	// Source is "config" for entries computed once at boot from configuration,
	// "live" for entries evaluated on every probe.
	Source string `json:"source,omitempty"`
}

// ReadinessState is the snapshot passed into ReadinessHandler at route
// registration. Components are typically mapped once at startup from
// production preflight (non-secret codes only). ExtraComponents, when set,
// is called on every probe so DB-backed checks stay
// current without a process restart. Readiness must not send email.
type ReadinessState struct {
	DB         *gorm.DB
	Components []ComponentResult
	// ExtraComponents returns additional (or overriding) component results
	// evaluated per request. When a component name appears in both the static
	// list and ExtraComponents, the ExtraComponents status wins.
	ExtraComponents func() []ComponentResult
}

// readinessSnapshot evaluates DB + component readiness for one probe.
func readinessSnapshot(ctx context.Context, state ReadinessState, static []ComponentResult, extraFn func() []ComponentResult) (ready bool, checks []ComponentResult) {
	// Use PingContext with a bounded deadline so a saturated pool (25
	// conns) doesn't make /health/ready hang — it must fail fast (EXT-4).
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	dbPing := checkDatabaseCtx(pingCtx, state.DB)

	dbStatus := "ok"
	dbCode := ""
	// Preserve the pre-component contract: anything other than a healthy
	// ping (including degraded/slow) fails readiness.
	if dbPing.Status != "healthy" {
		dbStatus = "failed"
		dbCode = "database.unavailable"
	}

	// Merge static + dynamic components. Dynamic entries override static
	// by component name so DB-backed checks refresh per probe.
	byName := make(map[string]ComponentResult, len(static)+4)
	order := make([]string, 0, len(static)+4)
	for _, ch := range static {
		if _, seen := byName[ch.Component]; !seen {
			order = append(order, ch.Component)
		}
		byName[ch.Component] = ch
	}
	if extraFn != nil {
		for _, ch := range extraFn() {
			if _, seen := byName[ch.Component]; !seen {
				order = append(order, ch.Component)
			}
			byName[ch.Component] = ch
		}
	}

	checks = make([]ComponentResult, 0, 1+len(order))
	checks = append(checks, ComponentResult{
		Component: "database",
		Status:    dbStatus,
		Code:      dbCode,
		Source:    "live",
	})
	for _, name := range order {
		checks = append(checks, byName[name])
	}

	ready = dbStatus == "ok"
	if ready {
		for _, ch := range checks[1:] {
			if ch.Status != "ok" {
				ready = false
				break
			}
		}
	}
	return ready, checks
}

func evaluateReady(c *gin.Context, state ReadinessState) bool {
	static := make([]ComponentResult, len(state.Components))
	copy(static, state.Components)
	ready, _ := readinessSnapshot(c.Request.Context(), state, static, state.ExtraComponents)
	return ready
}

// ReadinessHandler returns a readiness check handler. It performs a bounded
// DB ping (2s) plus the precomputed component list; it never sends email.
// Prefer PublicReadinessHandler on internet-facing routes (#286).
func ReadinessHandler(state ReadinessState) gin.HandlerFunc {
	// Snapshot static components so the handler is safe if callers reuse the slice.
	static := make([]ComponentResult, len(state.Components))
	copy(static, state.Components)
	extraFn := state.ExtraComponents

	return func(c *gin.Context) {
		ready, checks := readinessSnapshot(c.Request.Context(), state, static, extraFn)
		if !ready {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not ready",
				"checks": checks,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "ready",
			"checks": checks,
		})
	}
}

// LivenessHandler returns a minimal public liveness probe (process up).
// Intentionally omits timestamps and component details (#286).
func LivenessHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "alive",
		})
	}
}

// checkDatabase checks PostgreSQL database connection health
func checkDatabase(db *gorm.DB) HealthCheck {
	if db == nil {
		return HealthCheck{
			Service: "postgres",
			Status:  "unhealthy",
			Message: "database not initialized",
		}
	}

	start := time.Now()
	sqlDB, err := db.DB()
	if err != nil {
		return HealthCheck{
			Service: "postgres",
			Status:  "unhealthy",
			Message: err.Error(),
			Latency: time.Since(start).String(),
		}
	}

	err = sqlDB.Ping()
	latency := time.Since(start)

	if err != nil {
		return HealthCheck{
			Service: "postgres",
			Status:  "unhealthy",
			Message: err.Error(),
			Latency: latency.String(),
		}
	}

	status := "healthy"
	if latency > 100*time.Millisecond {
		status = "degraded"
	}

	return HealthCheck{
		Service: "postgres",
		Status:  status,
		Latency: latency.String(),
	}
}

// checkDatabaseCtx is checkDatabase with an explicit deadline. sqlDB.Ping()
// first waits for a free pool connection and so HANGS on a saturated pool —
// the exact moment readiness should shed load. PingContext fails fast (EXT-4).
func checkDatabaseCtx(ctx context.Context, db *gorm.DB) HealthCheck {
	if db == nil {
		return HealthCheck{Service: "postgres", Status: "unhealthy", Message: "database not initialized"}
	}
	start := time.Now()
	sqlDB, err := db.DB()
	if err != nil {
		return HealthCheck{Service: "postgres", Status: "unhealthy", Message: err.Error(), Latency: time.Since(start).String()}
	}
	err = sqlDB.PingContext(ctx)
	latency := time.Since(start)
	if err != nil {
		return HealthCheck{Service: "postgres", Status: "unhealthy", Message: err.Error(), Latency: latency.String()}
	}
	status := "healthy"
	if latency > 100*time.Millisecond {
		status = "degraded"
	}
	return HealthCheck{Service: "postgres", Status: status, Latency: latency.String()}
}

// checkSystem checks basic system health
func checkSystem() HealthCheck {
	numGoroutines := runtime.NumGoroutine()
	if numGoroutines > 10000 {
		return HealthCheck{
			Service: "system",
			Status:  "degraded",
			Message: fmt.Sprintf("high goroutine count: %d", numGoroutines),
		}
	}
	return HealthCheck{
		Service: "system",
		Status:  "healthy",
		Message: fmt.Sprintf("goroutines: %d", numGoroutines),
	}
}
