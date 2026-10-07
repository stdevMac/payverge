package fiscal

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// TestMarkJobResultIsOwnerScoped locks F-RECLAIM's write half: a worker whose lock
// was reclaimed mid-flight (and re-owned by another worker) must NOT be able to
// write its late result. MarkJobResult is CAS'd on locked_by so only the current
// lock owner can mark the job and clear the lock.
func TestMarkJobResultIsOwnerScoped(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)
	lockedBy := "worker-A"
	lockedAt := time.Now()
	job := &database.FiscalJob{
		BusinessID: 1, SettingsID: 1, BillID: 1, Action: ActionIssueReceipt,
		IdempotencyKey: "reclaim-k1", Status: database.FiscalStatusPending,
		MaxAttempts: 5, CreatedBy: "system", LockedAt: &lockedAt, LockedBy: &lockedBy,
	}
	require.NoError(t, db.Create(job).Error)

	// A worker that no longer owns the lock cannot write the result or clear the lock.
	require.NoError(t, repo.MarkJobResult(job.ID, "worker-B",
		map[string]interface{}{"status": database.FiscalStatusAuthorized}, time.Now()))
	var reload database.FiscalJob
	require.NoError(t, db.First(&reload, job.ID).Error)
	require.Equal(t, database.FiscalStatusPending, reload.Status, "non-owner must not mark the job")
	require.NotNil(t, reload.LockedBy, "non-owner must not clear the lock")

	// The owning worker writes the result and clears the lock.
	require.NoError(t, repo.MarkJobResult(job.ID, "worker-A",
		map[string]interface{}{"status": database.FiscalStatusAuthorized}, time.Now()))
	require.NoError(t, db.First(&reload, job.ID).Error)
	require.Equal(t, database.FiscalStatusAuthorized, reload.Status)
	require.Nil(t, reload.LockedBy, "owner clears the lock on result")
}

// TestFiscalWorkerJobTimeoutBelowReclaim locks F-RECLAIM's time half: each job is
// processed under a deadline strictly shorter than the reclaim window, so an
// in-flight provider call is aborted before its lock can be reclaimed and the job
// re-issued by another worker. The default reclaim is raised to 5m to clear the
// worst-case AFIP SOAP chain.
func TestFiscalWorkerJobTimeoutBelowReclaim(t *testing.T) {
	w := &FiscalWorker{}
	require.Equal(t, 5*time.Minute, w.reclaimStaleAfter(), "default reclaim should be 5m")
	require.Less(t, w.jobProcessTimeout(), w.reclaimStaleAfter(),
		"per-job timeout must be < reclaim so an in-flight job aborts before reclaim")
	require.GreaterOrEqual(t, w.jobProcessTimeout(), 30*time.Second)

	w2 := &FiscalWorker{ReclaimStaleAfter: 90 * time.Second}
	require.Less(t, w2.jobProcessTimeout(), w2.reclaimStaleAfter())
	require.GreaterOrEqual(t, w2.jobProcessTimeout(), 30*time.Second)

	// Even a misconfigured tiny reclaim must keep the timeout strictly below it, or
	// the "abort before reclaim" guarantee is defeated. The reclaim window is
	// floored to a sane minimum so the invariant always holds.
	w3 := &FiscalWorker{ReclaimStaleAfter: 10 * time.Second}
	require.Less(t, w3.jobProcessTimeout(), w3.reclaimStaleAfter(),
		"per-job timeout must stay below the (floored) reclaim window")
}
