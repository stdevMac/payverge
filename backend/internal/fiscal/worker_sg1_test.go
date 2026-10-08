package fiscal

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// SG-1: when the retry budget is exhausted and a retryable outcome carries a
// linked receipt id, the worker must reconcile that receipt row to
// failed_permanent too — otherwise a terminal job leaves an orphaned
// non-terminal receipt behind.
func TestApplyOutcome_BudgetExhaustedReconcilesLinkedReceipt(t *testing.T) {
	w, db, _ := newTestFiscalWorker(t)
	w.MaxAttempts = 3
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	w.Now = func() time.Time { return now }

	receipt := &database.FiscalReceipt{
		BusinessID: 1, SettingsID: 1, BillID: 1,
		Status:      database.FiscalStatusFailedRetryable,
		ReceiptType: "factura_c",
	}
	require.NoError(t, db.Create(receipt).Error)
	rid := receipt.ID

	// Attempts = cap-1 so this outcome exhausts the budget (attempts+1 == cap).
	// applyOutcome only ever runs on a job this worker just claimed, so it must be
	// locked by the worker for MarkJobResult's owner CAS to write (F-RECLAIM).
	lockedBy := w.workerID()
	job := database.FiscalJob{
		BusinessID: 1, SettingsID: 1, BillID: 1,
		Action:      ActionStatusCheck,
		Status:      database.FiscalStatusFailedRetryable,
		MaxAttempts: 3,
		Attempts:    2,
		ReceiptID:   &rid,
		LockedAt:    &now,
		LockedBy:    &lockedBy,
	}
	require.NoError(t, db.Create(&job).Error)

	w.applyOutcome(job, &JobOutcome{Retryable: true, ReceiptID: &rid, ErrorCode: "transient", ErrorMessage: "temporary outage"})

	var gotJob database.FiscalJob
	require.NoError(t, db.First(&gotJob, job.ID).Error)
	require.Equal(t, database.FiscalStatusFailedPermanent, gotJob.Status)
	require.Nil(t, gotJob.NextAttemptAt)

	var gotReceipt database.FiscalReceipt
	require.NoError(t, db.First(&gotReceipt, rid).Error)
	require.Equal(t, database.FiscalStatusFailedPermanent, gotReceipt.Status,
		"linked receipt must be reconciled to failed_permanent on budget exhaustion")
}

// Budget exhaustion with NO linked receipt must not error or panic.
func TestApplyOutcome_BudgetExhaustedNoReceiptIsSafe(t *testing.T) {
	w, db, _ := newTestFiscalWorker(t)
	w.MaxAttempts = 3
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	w.Now = func() time.Time { return now }

	lockedBy := w.workerID()
	job := database.FiscalJob{
		BusinessID: 1, SettingsID: 1, BillID: 1,
		Action:      ActionIssueReceipt,
		Status:      database.FiscalStatusFailedRetryable,
		MaxAttempts: 3,
		Attempts:    2,
		LockedAt:    &now,
		LockedBy:    &lockedBy,
	}
	require.NoError(t, db.Create(&job).Error)

	require.NotPanics(t, func() {
		w.applyOutcome(job, &JobOutcome{Retryable: true, ErrorCode: "transient", ErrorMessage: "temporary outage"})
	})

	var gotJob database.FiscalJob
	require.NoError(t, db.First(&gotJob, job.ID).Error)
	require.Equal(t, database.FiscalStatusFailedPermanent, gotJob.Status)
}

// A worker whose lock was reclaimed must not touch the linked receipt, raise
// the failed-permanent alert, or write an audit event for a result it dropped.
func TestApplyOutcome_LostLockWritesNoSideEffects(t *testing.T) {
	w, db, _ := newTestFiscalWorker(t)
	w.MaxAttempts = 3
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	w.Now = func() time.Time { return now }

	receipt := &database.FiscalReceipt{
		BusinessID: 1, SettingsID: 1, BillID: 1,
		Status: database.FiscalStatusFailedRetryable, ReceiptType: "factura_c",
	}
	require.NoError(t, db.Create(receipt).Error)
	rid := receipt.ID
	other := "another-worker"
	job := database.FiscalJob{
		BusinessID: 1, SettingsID: 1, BillID: 1, Action: ActionStatusCheck,
		Status: database.FiscalStatusPending, MaxAttempts: 3, Attempts: 2,
		ReceiptID: &rid, LockedAt: &now, LockedBy: &other,
	}
	require.NoError(t, db.Create(&job).Error)

	w.applyOutcome(job, &JobOutcome{Retryable: true, ReceiptID: &rid, ErrorCode: "transient"})

	var gotJob database.FiscalJob
	require.NoError(t, db.First(&gotJob, job.ID).Error)
	require.Equal(t, database.FiscalStatusPending, gotJob.Status)
	require.NotNil(t, gotJob.LockedBy)
	var gotReceipt database.FiscalReceipt
	require.NoError(t, db.First(&gotReceipt, rid).Error)
	require.Equal(t, database.FiscalStatusFailedRetryable, gotReceipt.Status, "stale worker must not flip the receipt")
	var audits int64
	require.NoError(t, db.Model(&database.FiscalAuditEvent{}).Count(&audits).Error)
	require.Equal(t, int64(0), audits, "no audit event for a dropped result")
}
