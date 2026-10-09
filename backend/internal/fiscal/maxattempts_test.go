package fiscal

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// TestApplyOutcomeHonorsPerJobMaxAttempts locks F-MAXATTEMPTS: the retry budget is
// the job's own MaxAttempts (set per-job at enqueue / by RetryReceipt), not the
// worker-global default. Here the job caps at 2 while the worker default is 5, so
// the second failed attempt must go terminal.
func TestApplyOutcomeHonorsPerJobMaxAttempts(t *testing.T) {
	w, db, _ := newTestFiscalWorker(t) // worker MaxAttempts defaults to 5
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	w.Now = func() time.Time { return now }
	lockedBy := w.workerID()

	job := database.FiscalJob{
		BusinessID: 1, SettingsID: 1, BillID: 1,
		Action:      ActionIssueReceipt,
		Status:      database.FiscalStatusFailedRetryable,
		MaxAttempts: 2, // per-job budget, lower than the worker default
		Attempts:    1, // this outcome is attempt 2 == cap
		LockedAt:    &now,
		LockedBy:    &lockedBy,
	}
	require.NoError(t, db.Create(&job).Error)

	w.applyOutcome(job, &JobOutcome{Retryable: true, ErrorCode: "transient", ErrorMessage: "temporary outage"})

	var got database.FiscalJob
	require.NoError(t, db.First(&got, job.ID).Error)
	require.Equal(t, database.FiscalStatusFailedPermanent, got.Status,
		"per-job MaxAttempts=2 must terminate at attempt 2 even though the worker default is 5")
	require.Nil(t, got.NextAttemptAt)
}

// TestApplyOutcomeFallsBackToWorkerMaxAttempts confirms a job with no per-job cap
// (MaxAttempts 0) still uses the worker default.
func TestApplyOutcomeFallsBackToWorkerMaxAttempts(t *testing.T) {
	w, db, _ := newTestFiscalWorker(t)
	w.MaxAttempts = 4
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	w.Now = func() time.Time { return now }
	lockedBy := w.workerID()

	job := database.FiscalJob{
		BusinessID: 1, SettingsID: 1, BillID: 1,
		Action:      ActionIssueReceipt,
		Status:      database.FiscalStatusFailedRetryable,
		MaxAttempts: 0, // no per-job cap → fall back to worker default (4)
		Attempts:    1, // attempt 2 of 4 → still retryable
		LockedAt:    &now,
		LockedBy:    &lockedBy,
	}
	require.NoError(t, db.Create(&job).Error)

	w.applyOutcome(job, &JobOutcome{Retryable: true, ErrorCode: "transient"})

	var got database.FiscalJob
	require.NoError(t, db.First(&got, job.ID).Error)
	require.Equal(t, database.FiscalStatusFailedRetryable, got.Status,
		"with no per-job cap, attempt 2 of worker-default 4 stays retryable")
}
