package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Public error codes returned on the job row (never raw provider errors).
const (
	MenuExtractionErrLoadInputs  = "load_inputs_failed"
	MenuExtractionErrProvider    = "provider_failed"
	MenuExtractionErrEmptyResult = "empty_result"
	MenuExtractionErrInternal    = "internal_error"
	MenuExtractionErrMaxAttempts = "max_attempts_exceeded"
)

const (
	menuExtractionWorkerQueueSize = 64
	menuExtractionMaxAttempts     = 5
	menuExtractionJobTimeout      = 5 * time.Minute
)

// MenuExtractionDownloader loads verified page bytes for a job.
type MenuExtractionDownloader func(images []database.MenuExtractionImage) ([]MenuExtractionInput, error)

// MenuExtractionCleaner removes protected/legacy assets after terminal success.
type MenuExtractionCleaner func(images []database.MenuExtractionImage)

// MenuExtractionWorker claims jobs atomically and runs extraction under a
// timeout. Enqueue is non-blocking wake; the DB is the source of truth.
type MenuExtractionWorker struct {
	service    *MenuAIService
	download   MenuExtractionDownloader
	cleanup    MenuExtractionCleaner
	lease      time.Duration
	queue      chan uint
	now        func() time.Time
	maxAttempt int64

	startOnce sync.Once
	stopOnce  sync.Once
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// NewMenuExtractionWorker builds a worker. download/cleanup are required for
// production; tests inject fakes.
func NewMenuExtractionWorker(
	service *MenuAIService,
	download MenuExtractionDownloader,
	cleanup MenuExtractionCleaner,
) *MenuExtractionWorker {
	return &MenuExtractionWorker{
		service:    service,
		download:   download,
		cleanup:    cleanup,
		lease:      database.DefaultMenuExtractionClaimLease,
		queue:      make(chan uint, menuExtractionWorkerQueueSize),
		now:        func() time.Time { return time.Now().UTC() },
		maxAttempt: menuExtractionMaxAttempts,
	}
}

// Start runs the background consumer until ctx is cancelled.
func (w *MenuExtractionWorker) Start(ctx context.Context) {
	w.startOnce.Do(func() {
		runCtx, cancel := context.WithCancel(ctx)
		w.cancel = cancel
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			w.loop(runCtx)
		}()
	})
}

// Stop cancels the worker and waits for the in-flight job, which goes back to
// pending (attempt refunded) rather than failing.
func (w *MenuExtractionWorker) Stop() {
	w.stopOnce.Do(func() {
		if w.cancel != nil {
			w.cancel()
		}
	})
	w.wg.Wait()
}

// Enqueue wakes the worker for jobID. Dropped when the queue is full; startup
// recovery and retries re-list due work.
func (w *MenuExtractionWorker) Enqueue(jobID uint) {
	if jobID == 0 {
		return
	}
	select {
	case w.queue <- jobID:
	default:
		log.Printf("menu extraction worker queue full; job %d will rely on recovery sweep", jobID)
	}
}

// RestorePending enqueues jobs that need work (startup recovery).
func (w *MenuExtractionWorker) RestorePending(limit int) (int, error) {
	ids, err := database.ListMenuExtractionJobsNeedingWork(w.now(), w.lease, limit)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		w.Enqueue(id)
	}
	return len(ids), nil
}

// ProcessOne claims and runs a single job (exported for tests).
func (w *MenuExtractionWorker) ProcessOne(ctx context.Context, jobID uint) {
	w.process(ctx, jobID)
}

func (w *MenuExtractionWorker) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-w.queue:
			w.process(ctx, id)
		}
	}
}

func (w *MenuExtractionWorker) process(parent context.Context, jobID uint) {
	if w.service == nil || w.download == nil {
		log.Printf("menu extraction worker misconfigured; skipping job %d", jobID)
		return
	}

	token, err := newClaimToken()
	if err != nil {
		log.Printf("menu extraction claim token error job=%d: %v", jobID, err)
		return
	}
	now := w.now()
	claim, err := database.ClaimMenuExtractionJob(jobID, token, now, w.lease)
	if err != nil {
		log.Printf("menu extraction claim failed job=%d: %v", jobID, err)
		return
	}
	if !claim.Claimed {
		// Another worker owns it, or it is already terminal.
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("menu extraction job=%d panicked: %v\n%s", jobID, r, debug.Stack())
			w.fail(jobID, token, claim.Job.AttemptCount, MenuExtractionErrInternal)
		}
	}()
	if claim.Job.AttemptCount > w.maxAttempt {
		_, _ = database.FailMenuExtractionJob(jobID, token, MenuExtractionErrMaxAttempts, nil, w.now())
		return
	}

	jobCtx, cancel := context.WithTimeout(parent, menuExtractionJobTimeout)
	defer cancel()

	inputs, loadErr := w.download(claim.Job.Images)
	if parent.Err() != nil {
		w.release(jobID, token)
		return
	}
	if loadErr != nil {
		log.Printf("menu extraction load inputs job=%d: %v", jobID, loadErr)
		w.fail(jobID, token, claim.Job.AttemptCount, MenuExtractionErrLoadInputs)
		return
	}

	menu, extractErr := w.service.ExtractMenuFromImages(jobCtx, claim.Job.BusinessID, inputs)
	if extractErr != nil {
		if parent.Err() != nil {
			// Shutdown cancelled the call; the provider did not fail.
			w.release(jobID, token)
			return
		}
		// A provider error and the per-job timeout both count as a failed attempt.
		log.Printf("menu extraction provider job=%d: %v", jobID, extractErr)
		w.fail(jobID, token, claim.Job.AttemptCount, MenuExtractionErrProvider)
		return
	}
	if menu == nil {
		w.fail(jobID, token, claim.Job.AttemptCount, MenuExtractionErrEmptyResult)
		return
	}

	menuJSON, _ := json.Marshal(menu)
	ok, completeErr := database.CompleteMenuExtractionJob(jobID, token, string(menuJSON), w.now())
	if completeErr != nil {
		log.Printf("menu extraction complete failed job=%d: %v", jobID, completeErr)
		return
	}
	if !ok {
		log.Printf("menu extraction complete skipped job=%d (claim lost)", jobID)
		return
	}
	if w.cleanup != nil {
		w.cleanup(claim.Job.Images)
	}
}

func (w *MenuExtractionWorker) fail(jobID uint, token string, attempt int64, publicCode string) {
	var next *time.Time
	if attempt < w.maxAttempt {
		// Exponential backoff: 30s, 60s, 120s, ... capped at 15m.
		backoff := time.Duration(30*(1<<min64(attempt-1, 5))) * time.Second
		if backoff > 15*time.Minute {
			backoff = 15 * time.Minute
		}
		t := w.now().Add(backoff)
		next = &t
	}
	ok, err := database.FailMenuExtractionJob(jobID, token, publicCode, next, w.now())
	if err != nil {
		log.Printf("menu extraction fail write job=%d: %v", jobID, err)
		return
	}
	if !ok {
		log.Printf("menu extraction fail skipped job=%d (claim lost)", jobID)
	}
}

// release returns an in-flight job to pending so the next process start picks
// it up at once instead of after the claim lease.
func (w *MenuExtractionWorker) release(jobID uint, token string) {
	ok, err := database.ReleaseMenuExtractionJob(jobID, token, w.now())
	if err != nil {
		log.Printf("menu extraction release on shutdown job=%d: %v", jobID, err)
		return
	}
	if !ok {
		log.Printf("menu extraction release skipped job=%d (claim lost)", jobID)
	}
}

func newClaimToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// Global worker used by HTTP handlers. Set from main after S3/DB init.
var globalMenuExtractionWorker *MenuExtractionWorker
var globalMenuExtractionWorkerMu sync.RWMutex

// SetMenuExtractionWorker installs the process-wide worker.
func SetMenuExtractionWorker(w *MenuExtractionWorker) {
	globalMenuExtractionWorkerMu.Lock()
	defer globalMenuExtractionWorkerMu.Unlock()
	globalMenuExtractionWorker = w
}

// GetMenuExtractionWorker returns the process-wide worker (may be nil in tests).
func GetMenuExtractionWorker() *MenuExtractionWorker {
	globalMenuExtractionWorkerMu.RLock()
	defer globalMenuExtractionWorkerMu.RUnlock()
	return globalMenuExtractionWorker
}

// EnqueueMenuExtraction is a nil-safe enqueue for handlers.
func EnqueueMenuExtraction(jobID uint) error {
	w := GetMenuExtractionWorker()
	if w == nil {
		return fmt.Errorf("menu extraction worker not configured")
	}
	w.Enqueue(jobID)
	return nil
}
