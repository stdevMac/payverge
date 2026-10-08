package services

import (
	"context"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

const (
	DefaultAiTranscriptRetentionDays = 90

	defaultAiRetentionBatchSize = 500

	aiRetentionMaxBatchesPerPass = 200

	defaultAiRetentionInterval = time.Hour
)

// RunAiTranscriptRetention performs one retention pass: it deletes AI waiter
// conversations (and their messages) older than retentionDays, in batches of
// batchSize, until the window is drained or the per-pass batch cap is hit.
// retentionDays <= 0 disables deletion. It is pure with respect to the database
// package global, so it is directly testable.
func RunAiTranscriptRetention(retentionDays, batchSize int, now time.Time) (database.AiTranscriptDeletionResult, error) {
	total := database.AiTranscriptDeletionResult{}
	if retentionDays <= 0 {
		return total, nil
	}
	if batchSize <= 0 {
		batchSize = defaultAiRetentionBatchSize
	}

	cutoff := now.UTC().AddDate(0, 0, -retentionDays)
	for i := 0; i < aiRetentionMaxBatchesPerPass; i++ {
		res, err := database.DeleteExpiredAiWaiterTranscripts(cutoff, batchSize)
		if err != nil {
			return total, err
		}
		total.ConversationsDeleted += res.ConversationsDeleted
		total.MessagesDeleted += res.MessagesDeleted
		if res.ConversationsDeleted == 0 {
			break
		}
	}
	return total, nil
}

// StartAiTranscriptRetentionJanitor runs RunAiTranscriptRetention on a ticker for
// the lifetime of ctx. retentionDays <= 0 means the janitor logs that retention
// is disabled and returns without scheduling work.
func StartAiTranscriptRetentionJanitor(ctx context.Context, retentionDays int, interval time.Duration) {
	if retentionDays <= 0 {
		logger.Logger.Warn("AI transcript retention janitor DISABLED (AI_TRANSCRIPT_RETENTION_DAYS=0): guest transcripts will be retained indefinitely")
		return
	}
	if interval <= 0 {
		interval = defaultAiRetentionInterval
	}

	logger.SafeGo(func() {
		runAiRetentionTick(retentionDays)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runAiRetentionTick(retentionDays)
			}
		}
	})
}

func runAiRetentionTick(retentionDays int) {
	logger.SafeTick("ai-transcript-retention-janitor", func() {
		res, err := RunAiTranscriptRetention(retentionDays, defaultAiRetentionBatchSize, time.Now().UTC())
		if err != nil {
			logger.Logger.Warnf("AI transcript retention janitor pass failed: %v", err)
			return
		}
		if res.ConversationsDeleted > 0 || res.MessagesDeleted > 0 {
			logger.Logger.Infof("AI transcript retention janitor swept %d conversations / %d messages older than %d days",
				res.ConversationsDeleted, res.MessagesDeleted, retentionDays)
		}
	})
}
