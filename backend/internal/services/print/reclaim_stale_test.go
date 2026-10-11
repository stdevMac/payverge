package print

import (
	"context"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestQueue_ReclaimStalePrinting verifies that a job orphaned in "printing" (its
// worker crashed/restarted before recording success/failure) is reclaimed for
// retry once it is older than the stale cutoff, while a job that is actively
// printing within the cutoff is left untouched.
func TestQueue_ReclaimStalePrinting(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)
	q := NewQueue(db)
	ctx := context.Background()

	stale := &database.PrintJob{
		BusinessID: business.ID, Kind: database.PrintJobKindBill, SourceType: "bill", SourceID: 1,
		Status: database.PrintJobStatusPrinting, AttemptCount: 0, MaxAttempts: 3,
	}
	if err := db.Create(stale).Error; err != nil {
		t.Fatalf("seed stale job: %v", err)
	}
	// Backdate updated_at so it is past the cutoff (worker died 10m ago).
	if err := db.Model(&database.PrintJob{}).Where("id = ?", stale.ID).
		Update("updated_at", time.Now().Add(-10*time.Minute)).Error; err != nil {
		t.Fatalf("backdate stale job: %v", err)
	}

	fresh := &database.PrintJob{
		BusinessID: business.ID, Kind: database.PrintJobKindBill, SourceType: "bill", SourceID: 2,
		Status: database.PrintJobStatusPrinting, AttemptCount: 0, MaxAttempts: 3,
	}
	if err := db.Create(fresh).Error; err != nil {
		t.Fatalf("seed fresh job: %v", err)
	}

	n, err := q.ReclaimStalePrinting(ctx, time.Now().Add(-2*time.Minute))
	if err != nil {
		t.Fatalf("ReclaimStalePrinting: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 job reclaimed, got %d", n)
	}

	var reloadedStale, reloadedFresh database.PrintJob
	if err := db.First(&reloadedStale, stale.ID).Error; err != nil {
		t.Fatalf("reload stale: %v", err)
	}
	if err := db.First(&reloadedFresh, fresh.ID).Error; err != nil {
		t.Fatalf("reload fresh: %v", err)
	}

	if reloadedStale.Status != database.PrintJobStatusFailedRetryable {
		t.Fatalf("orphaned printing job must be reclaimed to failed_retryable, got %q", reloadedStale.Status)
	}
	if reloadedStale.AttemptCount != 1 {
		t.Fatalf("reclaim must bump attempt_count to bound retries, got %d", reloadedStale.AttemptCount)
	}
	if reloadedFresh.Status != database.PrintJobStatusPrinting {
		t.Fatalf("actively-printing job must NOT be reclaimed, got %q", reloadedFresh.Status)
	}

	// The reclaimed job must be re-claimable by the worker.
	jobs, err := q.ClaimRetryable(ctx, time.Now(), 10)
	if err != nil {
		t.Fatalf("ClaimRetryable: %v", err)
	}
	found := false
	for _, j := range jobs {
		if j.ID == stale.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("reclaimed job %d should be re-claimable", stale.ID)
	}
}
