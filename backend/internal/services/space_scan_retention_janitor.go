package services

import (
	"context"
	"log"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/spaces/scan"
)

const (
	// DefaultSpaceScanRawRetentionDays is how long raw scan uploads are kept.
	DefaultSpaceScanRawRetentionDays = 7

	defaultSpaceScanRetentionInterval = time.Hour
	spaceScanRetentionBatchSize       = 50
)

// RunSpaceScanRawRetention deletes expired raw upload artifacts from storage
// and clears s3_key on the upload rows. retain_raw_until on the session drives
// eligibility (set at session create from SPACE_SCAN_RAW_RETENTION_DAYS).
func RunSpaceScanRawRetention(store scan.ArtifactStore, now time.Time) (int, error) {
	if store == nil {
		return 0, nil
	}
	deleted := 0
	for i := 0; i < 20; i++ {
		uploads, err := database.ListSpaceScanUploadsPastRetention(now, spaceScanRetentionBatchSize)
		if err != nil {
			return deleted, err
		}
		if len(uploads) == 0 {
			break
		}
		for _, u := range uploads {
			if u.S3Key == nil || *u.S3Key == "" {
				continue
			}
			if err := store.Delete(*u.S3Key); err != nil {
				log.Printf("space scan retention: delete %s: %v", *u.S3Key, err)
				continue
			}
			_ = database.ClearSpaceScanUploadS3Key(u.BusinessID, u.ID)
			deleted++
		}
		if len(uploads) < spaceScanRetentionBatchSize {
			break
		}
	}
	return deleted, nil
}

// StartSpaceScanRetentionJanitor runs retention on a ticker for the lifetime of ctx.
// retentionDays is documented for env; actual cutoff is per-session retain_raw_until.
// retentionDays <= 0 disables the janitor.
func StartSpaceScanRetentionJanitor(ctx context.Context, retentionDays int, store scan.ArtifactStore, interval time.Duration) {
	if retentionDays <= 0 {
		logger.Logger.Warn("space scan raw retention janitor DISABLED (SPACE_SCAN_RAW_RETENTION_DAYS=0)")
		return
	}
	if interval <= 0 {
		interval = defaultSpaceScanRetentionInterval
	}
	if store == nil {
		store = scan.DefaultArtifactStore("")
	}

	logger.SafeGo(func() {
		runSpaceScanRetentionTick(store)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Also expire stale active sessions.
				if n, err := database.ExpireStaleSpaceScanSessions(time.Now().UTC()); err != nil {
					log.Printf("space scan expire stale: %v", err)
				} else if n > 0 {
					log.Printf("space scan expired %d stale sessions", n)
				}
				runSpaceScanRetentionTick(store)
			}
		}
	})
}

func runSpaceScanRetentionTick(store scan.ArtifactStore) {
	logger.SafeTick("space-scan-retention-janitor", func() {
		n, err := RunSpaceScanRawRetention(store, time.Now().UTC())
		if err != nil {
			logger.Logger.Warnf("space scan retention pass failed: %v", err)
			return
		}
		if n > 0 {
			logger.Logger.Infof("space scan retention deleted %d raw uploads", n)
		}
	})
}
