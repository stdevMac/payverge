package print

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

// Backoff schedule per IMP-02 acceptance criteria. Indexes correspond to
// the attempt number that just failed (0 -> first wait, etc). After the
// last bucket is exhausted the job is marked failed_permanent.
//
//	attempt #1 fails -> retry in 5s
//	attempt #2 fails -> retry in 15s
//	attempt #3 fails -> retry in 1m
//	attempt #4 fails -> retry in 5m
//	attempt #5 fails -> retry in 30m
//	attempt #6 fails -> failed_permanent
var defaultBackoff = []time.Duration{
	5 * time.Second,
	15 * time.Second,
	1 * time.Minute,
	5 * time.Minute,
	30 * time.Minute,
}

// SendFunc is the transport-level send hook the worker calls when it claims
// a retryable job. Implementations should return nil on success and an error
// on failure; the worker will record the error and schedule the next attempt.
//
// The worker exposes this as a function so the cloudprnt/browser transports
// can be swapped in tests without dragging in HTTP servers.
type SendFunc func(ctx context.Context, job database.PrintJob) error

// RetryWorker drains stale and retryable print_jobs rows on a fixed interval.
// One goroutine per process; safe across replicas thanks to the FOR UPDATE
// SKIP LOCKED claim in queue.ClaimRetryable.
type RetryWorker struct {
	db           *gorm.DB
	queue        *Queue
	send         SendFunc
	interval     time.Duration
	batch        int
	backoff      []time.Duration
	staleAfter   time.Duration
	abandonAfter time.Duration

	mu     sync.Mutex
	stopCh chan struct{}
	done   chan struct{}
}

// RetryWorkerConfig is the public knobs for NewRetryWorker.
type RetryWorkerConfig struct {
	Interval time.Duration
	Batch    int
	Backoff  []time.Duration
	// StaleAfter is how long a job may sit in "printing" before it is presumed
	// orphaned (worker crashed mid-send) and reclaimed for retry. Must exceed any
	// real send duration. Defaults to 2 minutes.
	StaleAfter time.Duration
	// AbandonAfter is how long a browser-routed job may wait for a station to
	// claim it before it is marked permanently failed (#833). Defaults to
	// DefaultBrowserAbandonAfter; a negative value disables the sweep.
	AbandonAfter time.Duration
}

// NewRetryWorker wires a worker against the given DB and transport send hook.
// Defaults: 15s interval, batch=16, the schedule defined above.
func NewRetryWorker(db *gorm.DB, send SendFunc, cfg RetryWorkerConfig) *RetryWorker {
	if cfg.Interval <= 0 {
		cfg.Interval = 15 * time.Second
	}
	if cfg.Batch <= 0 {
		cfg.Batch = 16
	}
	if len(cfg.Backoff) == 0 {
		cfg.Backoff = defaultBackoff
	}
	if cfg.StaleAfter <= 0 {
		cfg.StaleAfter = 2 * time.Minute
	}
	if cfg.AbandonAfter == 0 {
		cfg.AbandonAfter = DefaultBrowserAbandonAfter
	}
	return &RetryWorker{
		db:           db,
		queue:        NewQueue(db),
		send:         send,
		interval:     cfg.Interval,
		batch:        cfg.Batch,
		backoff:      cfg.Backoff,
		staleAfter:   cfg.StaleAfter,
		abandonAfter: cfg.AbandonAfter,
	}
}

// Start launches the background goroutine. Returns immediately. Safe to call
// once; subsequent calls are a no-op.
func (w *RetryWorker) Start(ctx context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopCh != nil {
		return
	}
	stopCh := make(chan struct{})
	done := make(chan struct{})
	w.stopCh = stopCh
	w.done = done
	// Pass channels into the goroutine — Stop zeroes w.stopCh/w.done, so a
	// deferred close(w.done) inside the goroutine would race onto a nil
	// channel and panic. Owning local copies keeps the lifecycle clean.
	go w.run(ctx, stopCh, done)
}

// Stop signals the worker to exit and blocks until the goroutine returns.
// Idempotent.
func (w *RetryWorker) Stop() {
	w.mu.Lock()
	stopCh := w.stopCh
	done := w.done
	w.stopCh = nil
	w.done = nil
	w.mu.Unlock()
	if stopCh == nil {
		return
	}
	close(stopCh)
	<-done
}

// RunOnce performs a single sweep. Useful in tests; the goroutine calls it
// on every tick.
func (w *RetryWorker) RunOnce(ctx context.Context) (processed int, err error) {
	// Reclaim jobs orphaned in "printing" by a crashed/restarted worker before
	// claiming, so a deploy mid-print doesn't permanently lose the ticket.
	if n, rErr := w.queue.ReclaimStalePrinting(ctx, time.Now().Add(-w.staleAfter)); rErr != nil {
		log.Printf("[print-retry] reclaim stale printing error: %v", rErr)
	} else if n > 0 {
		log.Printf("[print-retry] reclaimed %d stale printing job(s)", n)
	}

	// #833: browser-routed jobs are exempt from ClaimRetryable on purpose, so
	// nothing else can move them off "routed". Without this they wait forever
	// and every status-based view files them as queued work in progress.
	if w.abandonAfter > 0 {
		if n, aErr := w.queue.AbandonStaleBrowserRouted(ctx, time.Now().Add(-w.abandonAfter)); aErr != nil {
			log.Printf("[print-retry] abandon stale browser-routed error: %v", aErr)
		} else if n > 0 {
			log.Printf("[print-retry] abandoned %d browser-routed job(s) past the %s claim SLA", n, w.abandonAfter)
		}
	}

	jobs, err := w.queue.ClaimRetryable(ctx, time.Now(), w.batch)
	if err != nil {
		return 0, err
	}
	for _, job := range jobs {
		w.processJob(ctx, job)
		processed++
	}
	return processed, nil
}

// run is the goroutine entry point. Channels are passed in so the goroutine
// owns its own references, independent of struct mutations under Stop().
func (w *RetryWorker) run(ctx context.Context, stopCh, done chan struct{}) {
	defer close(done)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Sweep immediately on start so a backend restart drains the queue
	// without waiting a full interval. SafeTick so a panic in a single sweep
	// (this runs in its own goroutine, outside gin.Recovery) logs and the loop
	// survives instead of crashing the whole process — and so a poison row
	// can't crash-loop the backend via this immediate-on-start sweep.
	logger.SafeTick("print-retry", func() {
		if _, err := w.RunOnce(ctx); err != nil {
			log.Printf("[print-retry] initial sweep error: %v", err)
		}
	})

	for {
		select {
		case <-ctx.Done():
			return
		case <-stopCh:
			return
		case <-ticker.C:
			logger.SafeTick("print-retry", func() {
				if _, err := w.RunOnce(ctx); err != nil {
					log.Printf("[print-retry] sweep error: %v", err)
				}
			})
		}
	}
}

// processJob attempts a single send and records the outcome.
func (w *RetryWorker) processJob(ctx context.Context, job database.PrintJob) {
	if w.send == nil {
		// No transport wired (tests). Treat as transient failure so the row
		// is retried with backoff.
		w.recordFailure(ctx, job, errors.New("no transport configured"))
		return
	}
	if err := w.send(ctx, job); err != nil {
		w.recordFailure(ctx, job, err)
		return
	}
	if err := w.queue.MarkAttemptSucceeded(ctx, job.ID); err != nil {
		log.Printf("[print-retry] mark success failed job_id=%d: %v", job.ID, err)
		return
	}
	// X-4: stamp the order's kitchen ack on successful kitchen/bar prints.
	recordKitchenAck(ctx, w.db, job)
}

// recordFailure picks the next attempt time based on the backoff schedule
// or marks the job permanently failed if attempts are exhausted.
func (w *RetryWorker) recordFailure(ctx context.Context, job database.PrintJob, sendErr error) {
	attempt := job.AttemptCount + 1
	maxAttempts := job.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = len(w.backoff) + 1
	}

	if attempt >= maxAttempts {
		if err := w.queue.MarkPermanentFailure(ctx, job.ID, attempt, sendErr.Error()); err != nil {
			log.Printf("[print-retry] mark permanent failure failed job_id=%d: %v", job.ID, err)
		}
		log.Printf("[print-retry] job_id=%d permanently failed after %d attempts: %v", job.ID, attempt, sendErr)
		return
	}

	idx := attempt - 1
	if idx >= len(w.backoff) {
		idx = len(w.backoff) - 1
	}
	next := time.Now().Add(w.backoff[idx])
	if err := w.queue.ScheduleRetry(ctx, job.ID, attempt, next, sendErr.Error()); err != nil {
		log.Printf("[print-retry] schedule retry failed job_id=%d: %v", job.ID, err)
	}
}
