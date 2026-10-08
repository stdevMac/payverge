package services

import (
	"context"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

const (
	// DefaultWebhookEventRetentionDays is the default number of days to keep
	// processed webhook_events rows. Set WEBHOOK_EVENT_RETENTION_DAYS=0 to
	// disable the janitor and retain payloads indefinitely.
	DefaultWebhookEventRetentionDays = 60

	defaultWebhookRetentionBatchSize  = 500
	webhookRetentionMaxBatchesPerPass = 200
	defaultWebhookRetentionInterval   = time.Hour
)

// RunWebhookEventRetention deletes processed webhook_events older than
// retentionDays in bounded batches. retentionDays <= 0 disables deletion.
// 'failed' rows are never touched (kept for forensics). It is pure with
// respect to the database package global, so it is directly testable.
func RunWebhookEventRetention(retentionDays, batchSize int, now time.Time) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	if batchSize <= 0 {
		batchSize = defaultWebhookRetentionBatchSize
	}
	cutoff := now.UTC().AddDate(0, 0, -retentionDays)
	var total int64
	for i := 0; i < webhookRetentionMaxBatchesPerPass; i++ {
		n, err := database.DeleteProcessedWebhookEventsBefore(cutoff, batchSize)
		if err != nil {
			return total, err
		}
		total += n
		if n == 0 {
			break
		}
	}
	return total, nil
}

// StartWebhookEventRetentionJanitor runs RunWebhookEventRetention on a ticker
// for the lifetime of ctx. retentionDays <= 0 means the janitor logs that
// retention is disabled and returns without scheduling work.
func StartWebhookEventRetentionJanitor(ctx context.Context, retentionDays int, interval time.Duration) {
	if retentionDays <= 0 {
		logger.Logger.Warn("webhook_events retention janitor DISABLED: processed webhook payloads retained indefinitely")
		return
	}
	if interval <= 0 {
		interval = defaultWebhookRetentionInterval
	}

	logger.SafeGo(func() {
		runWebhookRetentionTick(retentionDays)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runWebhookRetentionTick(retentionDays)
			}
		}
	})
}

func runWebhookRetentionTick(retentionDays int) {
	logger.SafeTick("webhook-events-retention-janitor", func() {
		n, err := RunWebhookEventRetention(retentionDays, defaultWebhookRetentionBatchSize, time.Now().UTC())
		if err != nil {
			logger.Logger.Warnf("webhook_events retention janitor pass failed: %v", err)
			return
		}
		if n > 0 {
			logger.Logger.Infof("webhook_events retention janitor swept %d processed rows older than %d days", n, retentionDays)
		}
	})
}
