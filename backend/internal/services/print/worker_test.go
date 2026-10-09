package print

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// stubSender returns the configured response on every call and records how
// many times it was invoked. Concurrency-safe.
type stubSender struct {
	mu       sync.Mutex
	calls    int32
	failures []error // pop the front of failures on each call; nil after exhausted
}

func (s *stubSender) Send(ctx context.Context, _ database.PrintJob) error {
	atomic.AddInt32(&s.calls, 1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.failures) == 0 {
		return nil
	}
	err := s.failures[0]
	s.failures = s.failures[1:]
	return err
}

func TestRetryWorker_RetriesWithBackoff(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)

	job := &database.PrintJob{
		BusinessID:    business.ID,
		Kind:          database.PrintJobKindBill,
		SourceType:    "bill",
		SourceID:      1,
		Status:        database.PrintJobStatusFailedRetryable,
		AttemptCount:  0,
		MaxAttempts:   3,
		NextAttemptAt: ptrTime(time.Now().Add(-time.Second)),
	}
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("seed job: %v", err)
	}

	sender := &stubSender{failures: []error{errors.New("boom"), errors.New("boom")}}
	worker := NewRetryWorker(db, sender.Send, RetryWorkerConfig{
		Interval: 10 * time.Millisecond,
		Batch:    4,
		Backoff:  []time.Duration{1 * time.Millisecond, 1 * time.Millisecond},
	})

	// First sweep: send fails -> reschedule.
	if _, err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	var updated database.PrintJob
	if err := db.First(&updated, job.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if updated.Status != database.PrintJobStatusFailedRetryable {
		t.Fatalf("expected status failed_retryable, got %q", updated.Status)
	}
	if updated.AttemptCount != 1 {
		t.Fatalf("expected attempt_count=1, got %d", updated.AttemptCount)
	}
	if updated.NextAttemptAt == nil || updated.NextAttemptAt.After(time.Now().Add(time.Second)) {
		t.Fatalf("expected next_attempt_at scheduled within 1s, got %v", updated.NextAttemptAt)
	}

	// Force the row eligible immediately, run again.
	now := time.Now().Add(-time.Second)
	if err := db.Model(&database.PrintJob{}).Where("id = ?", job.ID).Update("next_attempt_at", &now).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}

	if _, err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("run 2: %v", err)
	}
	if err := db.First(&updated, job.ID).Error; err != nil {
		t.Fatalf("reload 2: %v", err)
	}
	if updated.AttemptCount != 2 {
		t.Fatalf("expected attempt_count=2, got %d", updated.AttemptCount)
	}

	// Final sweep: sender now succeeds.
	now = time.Now().Add(-time.Second)
	if err := db.Model(&database.PrintJob{}).Where("id = ?", job.ID).Update("next_attempt_at", &now).Error; err != nil {
		t.Fatalf("backdate 2: %v", err)
	}
	if _, err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("run 3: %v", err)
	}
	if err := db.First(&updated, job.ID).Error; err != nil {
		t.Fatalf("reload 3: %v", err)
	}
	if updated.Status != database.PrintJobStatusPrinted {
		t.Fatalf("expected printed after success, got %q", updated.Status)
	}
	if updated.PrintedAt == nil {
		t.Fatalf("expected printed_at to be set")
	}
	if atomic.LoadInt32(&sender.calls) != 3 {
		t.Fatalf("expected 3 send attempts, got %d", sender.calls)
	}
}

// TestClaimRetryable_SkipsBrowserTransportJobs guards the browser-pull invariant:
// a job routed to a browser printer must never be claimed by the retry worker.
// Browser printing is pull-based (the frontend fetches routed jobs and calls
// mark-printed), so claiming one would churn it through
// printing->failed_retryable->failed_permanent and lose the kitchen ticket while
// the tab was briefly closed. A job routed to a non-browser printer, and a
// pre-routing job with no printer, must still be claimable.
func TestClaimRetryable_SkipsBrowserTransportJobs(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)
	q := NewQueue(db)

	browser := database.Printer{
		BusinessID: business.ID, Name: "browser", Role: "kitchen",
		Transport: "browser", Enabled: true,
	}
	if err := db.Create(&browser).Error; err != nil {
		t.Fatalf("create browser printer: %v", err)
	}
	// cloudprnt is rejected at the handler today but is the canonical server-push
	// transport the worker exists to serve; use it to prove non-browser jobs are
	// still claimed.
	push := database.Printer{
		BusinessID: business.ID, Name: "push", Role: "kitchen",
		Transport: "cloudprnt", Enabled: true,
	}
	if err := db.Create(&push).Error; err != nil {
		t.Fatalf("create push printer: %v", err)
	}

	routedBrowser := &database.PrintJob{
		BusinessID: business.ID, Kind: database.PrintJobKindKitchen,
		SourceType: "order", SourceID: 1, PrinterID: &browser.ID,
		Status: database.PrintJobStatusRouted, MaxAttempts: 6,
	}
	routedPush := &database.PrintJob{
		BusinessID: business.ID, Kind: database.PrintJobKindKitchen,
		SourceType: "order", SourceID: 2, PrinterID: &push.ID,
		Status: database.PrintJobStatusRouted, MaxAttempts: 6,
	}
	unrouted := &database.PrintJob{
		BusinessID: business.ID, Kind: database.PrintJobKindKitchen,
		SourceType: "order", SourceID: 3,
		Status: database.PrintJobStatusPending, MaxAttempts: 6,
	}
	for _, j := range []*database.PrintJob{routedBrowser, routedPush, unrouted} {
		if err := db.Create(j).Error; err != nil {
			t.Fatalf("seed job: %v", err)
		}
	}

	claimed, err := q.ClaimRetryable(context.Background(), time.Now(), 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	got := map[uint]bool{}
	for _, j := range claimed {
		got[j.ID] = true
	}
	if got[routedBrowser.ID] {
		t.Errorf("browser-transport routed job %d was claimed; it must only leave routed via mark-printed/cancel", routedBrowser.ID)
	}
	if !got[routedPush.ID] {
		t.Errorf("server-push routed job %d was NOT claimed; the worker must still drain it", routedPush.ID)
	}
	if !got[unrouted.ID] {
		t.Errorf("unrouted pending job %d (NULL printer) was NOT claimed; pre-routing rows must stay bounded by max_attempts", unrouted.ID)
	}

	// The browser job must be untouched (still routed), not bumped to printing.
	var reload database.PrintJob
	if err := db.First(&reload, routedBrowser.ID).Error; err != nil {
		t.Fatalf("reload browser job: %v", err)
	}
	if reload.Status != database.PrintJobStatusRouted {
		t.Errorf("browser job status = %q, want routed (untouched)", reload.Status)
	}
}

func TestRetryWorker_MarksPermanentFailureAfterMaxAttempts(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)

	job := &database.PrintJob{
		BusinessID:    business.ID,
		Kind:          database.PrintJobKindBill,
		SourceType:    "bill",
		SourceID:      2,
		Status:        database.PrintJobStatusFailedRetryable,
		AttemptCount:  1, // one failure already on record
		MaxAttempts:   2, // next failure should be terminal
		NextAttemptAt: ptrTime(time.Now().Add(-time.Second)),
	}
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	sender := &stubSender{failures: []error{errors.New("boom")}}
	worker := NewRetryWorker(db, sender.Send, RetryWorkerConfig{
		Interval: time.Hour,
		Backoff:  []time.Duration{time.Millisecond, time.Millisecond},
	})

	if _, err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}

	var updated database.PrintJob
	if err := db.First(&updated, job.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if updated.Status != database.PrintJobStatusFailedPermanent {
		t.Fatalf("expected failed_permanent, got %q", updated.Status)
	}
	if updated.LastError == nil || *updated.LastError != "boom" {
		t.Fatalf("expected last_error=boom, got %v", updated.LastError)
	}
}

func TestRetryWorker_SkipsJobsScheduledInTheFuture(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)

	future := time.Now().Add(10 * time.Minute)
	job := &database.PrintJob{
		BusinessID:    business.ID,
		Kind:          database.PrintJobKindBill,
		SourceType:    "bill",
		SourceID:      3,
		Status:        database.PrintJobStatusFailedRetryable,
		AttemptCount:  1,
		MaxAttempts:   6,
		NextAttemptAt: &future,
	}
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	sender := &stubSender{}
	worker := NewRetryWorker(db, sender.Send, RetryWorkerConfig{Interval: time.Hour})

	processed, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if processed != 0 {
		t.Fatalf("expected 0 processed for future job, got %d", processed)
	}
	if atomic.LoadInt32(&sender.calls) != 0 {
		t.Fatalf("expected sender not called, got %d", sender.calls)
	}
}

func TestRetryWorker_StartStopIsIdempotent(t *testing.T) {
	db := setupTestDB(t)
	worker := NewRetryWorker(db, nil, RetryWorkerConfig{Interval: 5 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker.Start(ctx)
	worker.Start(ctx) // no-op
	time.Sleep(20 * time.Millisecond)
	worker.Stop()
	worker.Stop() // no-op
}

func ptrTime(t time.Time) *time.Time { return &t }
