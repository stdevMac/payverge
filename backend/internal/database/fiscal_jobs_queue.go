package database

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// fiscalJobsClaimUsesSkipLocked reports whether the dialect supports the
// FOR UPDATE SKIP LOCKED clause.  PostgreSQL 9.5+ supports it; SQLite does
// not and will error if the clause is rendered.
func fiscalJobsClaimUsesSkipLocked(dialect string) bool {
	return dialect == "postgres"
}

// claimableFiscalJobStatuses are the job statuses a worker may (re)claim: a
// brand-new "pending" job, or a "failed_retryable" job that previously hit a
// transient error and was scheduled for a backoff retry.  The worker's
// applyOutcome writes "failed_retryable" (not "pending") on a transient failure,
// so the claim — and the stale-lock reclaim — MUST include it or the backoff
// machinery would be dead code and a job would never be retried automatically.
// A job in "failed_retryable" only becomes claimable again once its
// next_attempt_at is due.  Terminal statuses (authorized, rejected,
// failed_permanent) are never claimed; the manual operator retry path resets a
// job to "pending" explicitly.
var claimableFiscalJobStatuses = []FiscalStatus{FiscalStatusPending, FiscalStatusFailedRetryable}

// ClaimDueFiscalJobs atomically selects up to limit claimable FiscalJob rows
// (status pending or failed_retryable; see claimableFiscalJobStatuses) whose
// NextAttemptAt is in the past (or NULL) and that are not yet locked, marks them
// as locked by workerID, and returns the updated rows.
//
// Under PostgreSQL the SELECT uses FOR UPDATE SKIP LOCKED so that concurrent
// workers do not race on the same rows.  Under SQLite (used in tests) the
// locking clause is omitted; correctness is still verified through the
// locked_at / locked_by transition.
func ClaimDueFiscalJobs(db *gorm.DB, workerID string, limit int, now time.Time) ([]FiscalJob, error) {
	if limit <= 0 {
		return nil, nil
	}

	var jobs []FiscalJob
	err := db.Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&FiscalJob{}).
			Where(
				"status IN ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?) AND locked_at IS NULL",
				claimableFiscalJobStatuses,
				now,
			).
			Order("created_at ASC, id ASC").
			Limit(limit)

		// Apply SKIP LOCKED only on Postgres.
		if fiscalJobsClaimUsesSkipLocked(tx.Name()) {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}

		var ids []uint
		if err := query.Pluck("id", &ids).Error; err != nil {
			return fmt.Errorf("fiscal job claim select: %w", err)
		}
		if len(ids) == 0 {
			return nil
		}

		result := tx.Model(&FiscalJob{}).
			Where("id IN ? AND status IN ? AND locked_at IS NULL", ids, claimableFiscalJobStatuses).
			Updates(map[string]interface{}{
				"locked_at":  now,
				"locked_by":  workerID,
				"updated_at": now,
			})
		if result.Error != nil {
			return fmt.Errorf("fiscal job claim update: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			// Concurrent worker raced us on all rows; nothing to return.
			return nil
		}

		// Reload only the rows we successfully locked, matching the exact
		// locked_at timestamp written by the UPDATE above to exclude rows
		// that a concurrent worker may have re-locked between our UPDATE
		// and this SELECT.
		if err := tx.Where("id IN ? AND locked_at = ? AND locked_by = ?", ids, now, workerID).
			Order("created_at ASC, id ASC").
			Find(&jobs).Error; err != nil {
			return fmt.Errorf("fiscal job claim reload: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ClaimDueFiscalJobs: %w", err)
	}
	return jobs, nil
}

// CountDueFiscalJobs returns the number of claimable jobs (status pending or
// failed_retryable) whose next_attempt_at is due and that are not currently
// locked — i.e. the worker backlog depth, for the queue-depth gauge.  It uses
// the same predicate as ClaimDueFiscalJobs (minus the limit/locking) so the
// gauge reflects exactly what the next sweep would pick up.
func CountDueFiscalJobs(db *gorm.DB, now time.Time) (int64, error) {
	var count int64
	err := db.Model(&FiscalJob{}).
		Where(
			"status IN ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?) AND locked_at IS NULL",
			claimableFiscalJobStatuses,
			now,
		).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("CountDueFiscalJobs: %w", err)
	}
	return count, nil
}

// ReclaimStaleFiscalJobs clears locked_at / locked_by on claimable FiscalJob
// rows (status pending or failed_retryable) whose lock is older than staleAfter,
// making them eligible for a new claim attempt.  It returns the number of rows
// reset.  A worker that crashed mid-process on a retry attempt leaves the row in
// failed_retryable + locked, so the reclaim must cover that status too.
//
// This should be called by a background sweeper on a schedule shorter than
// the worker's processing deadline to bound the time a crashed worker can
// hold a lock.
func ReclaimStaleFiscalJobs(db *gorm.DB, staleAfter time.Duration, now time.Time) (int64, error) {
	staleThreshold := now.Add(-staleAfter)
	result := db.Model(&FiscalJob{}).
		Where("status IN ? AND locked_at IS NOT NULL AND locked_at < ?", claimableFiscalJobStatuses, staleThreshold).
		Updates(map[string]interface{}{
			"locked_at":  nil,
			"locked_by":  nil,
			"updated_at": now,
		})
	if result.Error != nil {
		return 0, fmt.Errorf("ReclaimStaleFiscalJobs: %w", result.Error)
	}
	return result.RowsAffected, nil
}
