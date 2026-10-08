package services

import (
	"context"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

const (
	// DefaultOpsAssistantRetentionDays is how long an Ops Assistant thread
	// survives without a new message (Wave 5 privacy contract).
	DefaultOpsAssistantRetentionDays = 30

	defaultOpsRetentionBatchSize     = 500
	opsRetentionMaxBatchesPerPass    = 200
	defaultOpsAssistantRetentionTick = time.Hour
)

// RunOpsAssistantRetention deletes ops threads whose last message is older
// than retentionDays, in bounded batches. retentionDays <= 0 disables deletion.
func RunOpsAssistantRetention(retentionDays, batchSize int, now time.Time) (database.OpsAssistantRetentionResult, error) {
	total := database.OpsAssistantRetentionResult{}
	if retentionDays <= 0 {
		return total, nil
	}
	if batchSize <= 0 {
		batchSize = defaultOpsRetentionBatchSize
	}
	cutoff := now.UTC().AddDate(0, 0, -retentionDays)
	for i := 0; i < opsRetentionMaxBatchesPerPass; i++ {
		res, err := database.DeleteInactiveOpsAssistantThreads(cutoff, batchSize)
		if err != nil {
			return total, err
		}
		total.ThreadsDeleted += res.ThreadsDeleted
		total.MessagesDeleted += res.MessagesDeleted
		total.ToolCallsDeleted += res.ToolCallsDeleted
		total.RequestsDeleted += res.RequestsDeleted
		if res.ThreadsDeleted == 0 {
			break
		}
	}
	return total, nil
}

// StartOpsAssistantRetentionJanitor purges inactive ops history hourly.
// retentionDays <= 0 logs a warning and does not schedule work.
func StartOpsAssistantRetentionJanitor(ctx context.Context, retentionDays int, interval time.Duration) {
	if retentionDays <= 0 {
		logger.Logger.Warn("ops assistant retention janitor DISABLED (OPS_ASSISTANT_RETENTION_DAYS=0): ops assistant threads will be retained indefinitely")
		return
	}
	if interval <= 0 {
		interval = defaultOpsAssistantRetentionTick
	}
	logger.SafeGo(func() {
		runOpsAssistantRetentionTick(retentionDays)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runOpsAssistantRetentionTick(retentionDays)
			}
		}
	})
}

func runOpsAssistantRetentionTick(retentionDays int) {
	logger.SafeTick("ops-assistant-retention-janitor", func() {
		res, err := RunOpsAssistantRetention(retentionDays, defaultOpsRetentionBatchSize, time.Now().UTC())
		if err != nil {
			logger.Logger.Warnf("ops assistant retention janitor pass failed: %v", err)
			return
		}
		if res.ThreadsDeleted > 0 {
			logger.Logger.Infof("ops assistant retention janitor swept %d threads / %d messages / %d tool_calls with no message in %d days",
				res.ThreadsDeleted, res.MessagesDeleted, res.ToolCallsDeleted, retentionDays)
		}
	})
}
