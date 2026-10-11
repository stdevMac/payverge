package print

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// backdatePrintJob rewrites both timestamps the abandon sweep reads. GORM stamps
// created_at/updated_at on insert, so a test job is always "new" until this runs.
func backdatePrintJob(t *testing.T, db *gorm.DB, id uint, at time.Time) {
	t.Helper()
	if err := db.Model(&database.PrintJob{}).Where("id = ?", id).
		UpdateColumns(map[string]interface{}{"created_at": at, "updated_at": at}).Error; err != nil {
		t.Fatalf("backdate job %d: %v", id, err)
	}
}

// TestQueue_AbandonStaleBrowserRouted reproduces live venue 2 (#833): four jobs
// sat `routed` for 67h / 133h / 133h / 813h with attempt_count 0, printed_at and
// failed_at null, because ClaimRetryable deliberately skips browser-routed rows
// and nothing else ever moved them. Every status-grouped view filed them under
// "queued", so a print that never happened read as work still in progress.
func TestQueue_AbandonStaleBrowserRouted(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)
	q := NewQueue(db)
	ctx := context.Background()

	browserPrinter := database.Printer{
		BusinessID: business.ID, Name: "Station", Role: "kitchen",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	if err := db.Create(&browserPrinter).Error; err != nil {
		t.Fatalf("create browser printer: %v", err)
	}

	// The live ages, in hours.
	var stale []*database.PrintJob
	for i, ageHours := range []int{67, 133, 133, 813} {
		job := &database.PrintJob{
			BusinessID: business.ID, PrinterID: &browserPrinter.ID,
			Kind: database.PrintJobKindKitchen, SourceType: "order", SourceID: uint(i + 1),
			Status: database.PrintJobStatusRouted, AttemptCount: 0, MaxAttempts: 6,
		}
		if err := db.Create(job).Error; err != nil {
			t.Fatalf("seed stale job: %v", err)
		}
		backdatePrintJob(t, db, job.ID, time.Now().Add(-time.Duration(ageHours)*time.Hour))
		stale = append(stale, job)
	}

	// Waiting 20 minutes for the tab to come back is normal service, not a
	// failure: the operator list already flags it stalled at 10 minutes.
	fresh := &database.PrintJob{
		BusinessID: business.ID, PrinterID: &browserPrinter.ID,
		Kind: database.PrintJobKindKitchen, SourceType: "order", SourceID: 99,
		Status: database.PrintJobStatusRouted, AttemptCount: 0, MaxAttempts: 6,
	}
	if err := db.Create(fresh).Error; err != nil {
		t.Fatalf("seed fresh job: %v", err)
	}
	backdatePrintJob(t, db, fresh.ID, time.Now().Add(-20*time.Minute))

	n, err := q.AbandonStaleBrowserRouted(ctx, time.Now().Add(-DefaultBrowserAbandonAfter))
	if err != nil {
		t.Fatalf("AbandonStaleBrowserRouted: %v", err)
	}
	if n != 4 {
		t.Fatalf("expected the 4 day-old routed jobs abandoned, got %d", n)
	}

	for _, job := range stale {
		var reloaded database.PrintJob
		if err := db.First(&reloaded, job.ID).Error; err != nil {
			t.Fatalf("reload job %d: %v", job.ID, err)
		}
		if reloaded.Status != database.PrintJobStatusFailedPermanent {
			t.Fatalf("job %d: a browser job nobody claimed for days must not stay routed, got %q",
				job.ID, reloaded.Status)
		}
		if reloaded.PrintedAt != nil {
			t.Fatalf("job %d: abandoned job must not claim it printed", job.ID)
		}
		if reloaded.LastError == nil || *reloaded.LastError == "" {
			t.Fatalf("job %d: abandoned job must say why it failed", job.ID)
		}
	}

	var reloadedFresh database.PrintJob
	if err := db.First(&reloadedFresh, fresh.ID).Error; err != nil {
		t.Fatalf("reload fresh: %v", err)
	}
	if reloadedFresh.Status != database.PrintJobStatusRouted {
		t.Fatalf("a 20-minute-old routed job is still claimable, got %q", reloadedFresh.Status)
	}
}

// A job that a lease reclaim or an operator retry put back on the queue is
// freshly routed even though it was created days ago: the wait restarts from
// that moment, so created_at alone must not condemn it.
func TestQueue_AbandonStaleBrowserRouted_RespectsRequeue(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)
	q := NewQueue(db)

	browserPrinter := database.Printer{
		BusinessID: business.ID, Name: "Station", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	if err := db.Create(&browserPrinter).Error; err != nil {
		t.Fatalf("create browser printer: %v", err)
	}

	job := &database.PrintJob{
		BusinessID: business.ID, PrinterID: &browserPrinter.ID,
		Kind: database.PrintJobKindBill, SourceType: "bill", SourceID: 1,
		Status: database.PrintJobStatusRouted, AttemptCount: 1, MaxAttempts: 6,
	}
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("seed job: %v", err)
	}
	if err := db.Model(&database.PrintJob{}).Where("id = ?", job.ID).
		UpdateColumns(map[string]interface{}{
			"created_at": time.Now().Add(-48 * time.Hour),
			"updated_at": time.Now().Add(-1 * time.Minute),
		}).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}

	n, err := q.AbandonStaleBrowserRouted(context.Background(), time.Now().Add(-DefaultBrowserAbandonAfter))
	if err != nil {
		t.Fatalf("AbandonStaleBrowserRouted: %v", err)
	}
	if n != 0 {
		t.Fatalf("a just-requeued job must get the full window again, abandoned %d", n)
	}
}

// Non-browser routed jobs already have an exit: ClaimRetryable picks them up and
// max_attempts bounds them. The sweep must not race that path.
func TestQueue_AbandonStaleBrowserRouted_LeavesNetworkPrinters(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)
	q := NewQueue(db)

	networkPrinter := database.Printer{
		BusinessID: business.ID, Name: "Star", Role: "kitchen",
		Transport: "cloudprnt", PaperWidthMM: 80, Enabled: true,
	}
	if err := db.Create(&networkPrinter).Error; err != nil {
		t.Fatalf("create printer: %v", err)
	}

	job := &database.PrintJob{
		BusinessID: business.ID, PrinterID: &networkPrinter.ID,
		Kind: database.PrintJobKindKitchen, SourceType: "order", SourceID: 1,
		Status: database.PrintJobStatusRouted, AttemptCount: 0, MaxAttempts: 6,
	}
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("seed job: %v", err)
	}
	backdatePrintJob(t, db, job.ID, time.Now().Add(-72*time.Hour))

	n, err := q.AbandonStaleBrowserRouted(context.Background(), time.Now().Add(-DefaultBrowserAbandonAfter))
	if err != nil {
		t.Fatalf("AbandonStaleBrowserRouted: %v", err)
	}
	if n != 0 {
		t.Fatalf("network-routed jobs belong to the retry worker, abandoned %d", n)
	}
}

// The sweep has to run from the worker, not just exist: nothing else can move a
// browser-routed row, so an un-wired sweep is the same bug.
func TestRetryWorker_RunOnceAbandonsStaleBrowserRouted(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)

	browserPrinter := database.Printer{
		BusinessID: business.ID, Name: "Station", Role: "kitchen",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	if err := db.Create(&browserPrinter).Error; err != nil {
		t.Fatalf("create browser printer: %v", err)
	}

	job := &database.PrintJob{
		BusinessID: business.ID, PrinterID: &browserPrinter.ID,
		Kind: database.PrintJobKindKitchen, SourceType: "order", SourceID: 1,
		Status: database.PrintJobStatusRouted, AttemptCount: 0, MaxAttempts: 6,
	}
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("seed job: %v", err)
	}
	backdatePrintJob(t, db, job.ID, time.Now().Add(-813*time.Hour))

	worker := NewRetryWorker(db, nil, RetryWorkerConfig{})
	if _, err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	var reloaded database.PrintJob
	if err := db.First(&reloaded, job.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != database.PrintJobStatusFailedPermanent {
		t.Fatalf("worker sweep must retire an abandoned browser job, got %q", reloaded.Status)
	}
	// The exemption that caused #833 still has to hold: the sweep retires the
	// job, it never sends it, so attempts stay where the station left them.
	if reloaded.AttemptCount != 0 {
		t.Fatalf("abandon must not burn attempts, got %d", reloaded.AttemptCount)
	}
}

// A worker configured with AbandonAfter < 0 leaves routed jobs alone, so an
// operator can pause the sweep without rebuilding.
func TestRetryWorker_AbandonSweepCanBeDisabled(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)

	browserPrinter := database.Printer{
		BusinessID: business.ID, Name: "Station", Role: "kitchen",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	if err := db.Create(&browserPrinter).Error; err != nil {
		t.Fatalf("create browser printer: %v", err)
	}

	job := &database.PrintJob{
		BusinessID: business.ID, PrinterID: &browserPrinter.ID,
		Kind: database.PrintJobKindKitchen, SourceType: "order", SourceID: 1,
		Status: database.PrintJobStatusRouted, AttemptCount: 0, MaxAttempts: 6,
	}
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("seed job: %v", err)
	}
	backdatePrintJob(t, db, job.ID, time.Now().Add(-813*time.Hour))

	worker := NewRetryWorker(db, nil, RetryWorkerConfig{AbandonAfter: -1})
	if _, err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	var reloaded database.PrintJob
	if err := db.First(&reloaded, job.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != database.PrintJobStatusRouted {
		t.Fatalf("disabled sweep must leave the job alone, got %q", reloaded.Status)
	}
}
