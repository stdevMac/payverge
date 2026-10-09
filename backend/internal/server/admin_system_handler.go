package server

import (
	"context"
	"net/http"
	"time"

	"github.com/stdevmac/payverge/backend/internal/adminruntime"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/health"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type adminQueueMetric struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
	Label string `json:"label,omitempty"`
}

type adminSystemHealthResponse struct {
	Status    string                      `json:"status"`
	Uptime    string                      `json:"uptime"`
	Version   string                      `json:"version"`
	Timestamp time.Time                   `json:"timestamp"`
	Checks    []health.HealthCheck        `json:"checks"`
	Workers   []adminruntime.WorkerStatus `json:"workers"`
	Queues    []adminQueueMetric          `json:"queues"`
}

// GetAdminSystemHealth GET /api/v1/admin/system/health — operational snapshot
// for the command-center dashboard (DB, queues, background workers).
func GetAdminSystemHealth(c *gin.Context) {
	db := database.GetDB()
	checks := []health.HealthCheck{}
	overall := "healthy"

	dbCheck := healthCheckDatabase(c.Request.Context(), db)
	checks = append(checks, dbCheck)
	if dbCheck.Status == "unhealthy" {
		overall = "unhealthy"
	} else if dbCheck.Status == "degraded" && overall == "healthy" {
		overall = "degraded"
	}

	sysCheck := healthCheckRuntime()
	checks = append(checks, sysCheck)
	if sysCheck.Status == "degraded" && overall == "healthy" {
		overall = "degraded"
	}

	queues := collectAdminQueueMetrics(db)
	for _, q := range queues {
		if q.Count > 0 && (q.Name == "failed_webhooks" || q.Name == "open_escalations") {
			if overall == "healthy" {
				overall = "degraded"
			}
		}
	}

	c.JSON(http.StatusOK, adminSystemHealthResponse{
		Status:    overall,
		Uptime:    time.Since(adminruntime.StartedAt).Round(time.Second).String(),
		Version:   "1.0.0",
		Timestamp: time.Now(),
		Checks:    checks,
		Workers:   adminruntime.ListWorkers(),
		Queues:    queues,
	})
}

func healthCheckDatabase(ctx context.Context, db *gorm.DB) health.HealthCheck {
	if db == nil {
		return health.HealthCheck{Service: "postgres", Status: "unhealthy", Message: "database not initialized"}
	}
	start := time.Now()
	sqlDB, err := db.DB()
	if err != nil {
		return health.HealthCheck{Service: "postgres", Status: "unhealthy", Message: err.Error(), Latency: time.Since(start).String()}
	}
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err = sqlDB.PingContext(pingCtx)
	latency := time.Since(start)
	if err != nil {
		return health.HealthCheck{Service: "postgres", Status: "unhealthy", Message: err.Error(), Latency: latency.String()}
	}
	status := "healthy"
	if latency > 100*time.Millisecond {
		status = "degraded"
	}
	return health.HealthCheck{Service: "postgres", Status: status, Latency: latency.String()}
}

func healthCheckRuntime() health.HealthCheck {
	return health.HealthCheck{
		Service: "runtime",
		Status:  "healthy",
		Message: "process alive",
	}
}

func collectAdminQueueMetrics(db *gorm.DB) []adminQueueMetric {
	if db == nil {
		return nil
	}
	metrics := make([]adminQueueMetric, 0, 6)

	var failedWebhooks int64
	if db.Migrator().HasTable("webhook_events") {
		_ = db.Model(&database.WebhookEvent{}).Where("status = ?", "failed").Count(&failedWebhooks).Error
	}
	metrics = append(metrics, adminQueueMetric{Name: "failed_webhooks", Count: failedWebhooks, Label: "Failed payment webhooks"})

	var openEscalations int64
	if db.Migrator().HasTable("escalations") {
		_ = db.Model(&database.Escalation{}).Where("status IN ?", []string{"open", "in_progress"}).Count(&openEscalations).Error
	}
	metrics = append(metrics, adminQueueMetric{Name: "open_escalations", Count: openEscalations, Label: "Open escalations"})

	var errors1h int64
	since := time.Now().Add(-1 * time.Hour)
	if db.Migrator().HasTable("error_logs") {
		_ = db.Model(&database.ErrorLog{}).Where("created_at >= ?", since).Count(&errors1h).Error
	}
	metrics = append(metrics, adminQueueMetric{Name: "errors_1h", Count: errors1h, Label: "Errors (last hour)"})

	var pendingPrint int64
	if db.Migrator().HasTable("print_jobs") {
		_ = db.Model(&database.PrintJob{}).Where("status IN ?", []string{"pending", "queued"}).Count(&pendingPrint).Error
	}
	metrics = append(metrics, adminQueueMetric{Name: "pending_print_jobs", Count: pendingPrint, Label: "Pending print jobs"})

	summary, _ := database.GetAdminFiscalSummary()
	metrics = append(metrics, adminQueueMetric{Name: "due_fiscal_jobs", Count: summary.DueJobs, Label: "Due fiscal jobs"})
	metrics = append(metrics, adminQueueMetric{Name: "failed_fiscal_jobs", Count: summary.FailedRetryableJobs + summary.FailedPermanentJobs, Label: "Failed fiscal jobs"})

	return metrics
}
