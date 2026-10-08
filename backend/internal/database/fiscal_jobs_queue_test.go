package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newSQLiteWithFiscal opens an in-memory SQLite DB and auto-migrates the fiscal
// tables.  NOTE: SQLite does not support FOR UPDATE … SKIP LOCKED; the
// ClaimDueFiscalJobs implementation omits the locking clause under SQLite and
// relies on the status/locked_at transition to prove correctness.
func newSQLiteWithFiscal(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	// SQLite has issues with concurrent connections sharing the same :memory:
	// URL; pin to a single connection so the shared cache works correctly.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(
		&BusinessFiscalSettings{},
		&FiscalReceipt{},
		&FiscalJob{},
		&FiscalAuditEvent{},
	))
	return db
}

func TestClaimDueFiscalJobs(t *testing.T) {
	db := newSQLiteWithFiscal(t)
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)

	// Seed one pending job with no NextAttemptAt (immediately claimable).
	job := FiscalJob{
		BusinessID:     1,
		SettingsID:     1,
		BillID:         1,
		Action:         "issue_receipt",
		IdempotencyKey: "k1",
		Status:         FiscalStatusPending,
		MaxAttempts:    5,
	}
	require.NoError(t, db.Create(&job).Error)

	// First claim must succeed and populate LockedBy / LockedAt.
	jobs, err := ClaimDueFiscalJobs(db, "worker-1", 10, now)
	require.NoError(t, err)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 claimed job, got %d", len(jobs))
	}
	if jobs[0].LockedBy == nil || *jobs[0].LockedBy != "worker-1" {
		t.Fatalf("claim failed: LockedBy = %v", jobs[0].LockedBy)
	}
	if jobs[0].LockedAt == nil {
		t.Fatal("claim failed: LockedAt is nil")
	}

	// Second claim by a different worker must return nothing (row is locked).
	again, err := ClaimDueFiscalJobs(db, "worker-2", 10, now)
	require.NoError(t, err)
	if len(again) != 0 {
		t.Fatalf("double claim should be empty, got %d rows: %+v", len(again), again)
	}
}

func TestClaimDueFiscalJobs_NotYetDue(t *testing.T) {
	db := newSQLiteWithFiscal(t)
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)

	// A job whose NextAttemptAt is in the future must NOT be claimed.
	job := FiscalJob{
		BusinessID:     2,
		SettingsID:     2,
		BillID:         2,
		Action:         "issue_receipt",
		IdempotencyKey: "k-future",
		Status:         FiscalStatusPending,
		MaxAttempts:    5,
		NextAttemptAt:  &future,
	}
	require.NoError(t, db.Create(&job).Error)

	claimed, err := ClaimDueFiscalJobs(db, "worker-1", 10, now)
	require.NoError(t, err)
	if len(claimed) != 0 {
		t.Fatalf("expected 0 claimed (future NextAttemptAt), got %d", len(claimed))
	}
}

func TestReclaimStaleFiscalJobs(t *testing.T) {
	db := newSQLiteWithFiscal(t)
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	staleLockTime := now.Add(-time.Hour) // locked 1 hour ago
	workerID := "worker-stale"

	// Seed a job that was claimed an hour ago but never finished (still pending).
	job := FiscalJob{
		BusinessID:     3,
		SettingsID:     3,
		BillID:         3,
		Action:         "issue_receipt",
		IdempotencyKey: "k-stale",
		Status:         FiscalStatusPending,
		MaxAttempts:    5,
		LockedAt:       &staleLockTime,
		LockedBy:       &workerID,
	}
	require.NoError(t, db.Create(&job).Error)

	// Reclaim jobs locked more than 30 minutes ago.
	affected, err := ReclaimStaleFiscalJobs(db, 30*time.Minute, now)
	require.NoError(t, err)
	if affected != 1 {
		t.Fatalf("expected 1 row affected, got %d", affected)
	}

	// The row must now be unlocked so ClaimDueFiscalJobs can pick it up.
	var reloaded FiscalJob
	require.NoError(t, db.First(&reloaded, job.ID).Error)
	if reloaded.LockedAt != nil {
		t.Fatalf("expected LockedAt to be nil after reclaim, got %v", reloaded.LockedAt)
	}
	if reloaded.LockedBy != nil {
		t.Fatalf("expected LockedBy to be nil after reclaim, got %v", reloaded.LockedBy)
	}

	// After reclaim the job must be claimable again.
	claimed, err := ClaimDueFiscalJobs(db, "worker-new", 10, now)
	require.NoError(t, err)
	if len(claimed) != 1 {
		t.Fatalf("expected 1 job after reclaim, got %d", len(claimed))
	}
}

func TestReclaimStaleFiscalJobs_DoesNotTouchRecentLocks(t *testing.T) {
	db := newSQLiteWithFiscal(t)
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	recentLockTime := now.Add(-5 * time.Minute) // only 5 minutes old
	workerID := "worker-recent"

	job := FiscalJob{
		BusinessID:     4,
		SettingsID:     4,
		BillID:         4,
		Action:         "issue_receipt",
		IdempotencyKey: "k-recent",
		Status:         FiscalStatusPending,
		MaxAttempts:    5,
		LockedAt:       &recentLockTime,
		LockedBy:       &workerID,
	}
	require.NoError(t, db.Create(&job).Error)

	// Stale threshold is 30 minutes — the 5-minute lock must survive.
	affected, err := ReclaimStaleFiscalJobs(db, 30*time.Minute, now)
	require.NoError(t, err)
	if affected != 0 {
		t.Fatalf("expected 0 rows affected (lock is fresh), got %d", affected)
	}

	var reloaded FiscalJob
	require.NoError(t, db.First(&reloaded, job.ID).Error)
	if reloaded.LockedBy == nil || *reloaded.LockedBy != workerID {
		t.Fatalf("fresh lock was unexpectedly cleared")
	}
}

func TestClaimDueFiscalJobs_SameWorkerSecondClaimEmpty(t *testing.T) {
	db := newSQLiteWithFiscal(t)
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	db.Create(&FiscalJob{BusinessID: 5, SettingsID: 5, BillID: 5, Action: "issue_receipt", IdempotencyKey: "k-same-worker", Status: FiscalStatusPending, MaxAttempts: 5})
	first, err := ClaimDueFiscalJobs(db, "worker-1", 10, now)
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim: n=%d err=%v", len(first), err)
	}
	again, err := ClaimDueFiscalJobs(db, "worker-1", 10, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("same worker re-claim should be empty: %+v", again)
	}
}

// TestFiscalJobsQueueDialectGuard verifies that the SKIP LOCKED helper
// correctly identifies the postgres dialect and skips it for sqlite.
func TestFiscalJobsQueueDialectGuard(t *testing.T) {
	if fiscalJobsClaimUsesSkipLocked("postgres") != true {
		t.Fatal("expected postgres to use SKIP LOCKED")
	}
	if fiscalJobsClaimUsesSkipLocked("sqlite") != false {
		t.Fatal("expected sqlite to skip SKIP LOCKED")
	}
}
