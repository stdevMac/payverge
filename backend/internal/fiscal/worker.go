package fiscal

import (
	"context"
	"fmt"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	operational_alerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"

	sentry "github.com/getsentry/sentry-go"
	"gorm.io/gorm"
)

// FiscalWorker drains the fiscal_jobs queue: it reclaims jobs orphaned by a
// crashed worker, atomically claims due jobs, processes each through the Service
// (load context → resolve provider → issue/credit/status → persist receipt), and
// applies the resulting terminal/backoff transition plus an audit event.
//
// It mirrors services.PluginNotificationWorker: injectable clock, bounded batch
// size, exponential capped backoff, and a stale-lock reclaim before claiming so
// a deploy/OOM/panic mid-flight never strands a row.
type FiscalWorker struct {
	db      *gorm.DB
	repo    *Repository
	service *Service

	// WorkerID identifies this worker instance in the claim's locked_by column.
	WorkerID string
	// BatchSize bounds how many jobs are claimed per ProcessDue call (default 25).
	BatchSize int
	// MaxAttempts is the retry ceiling; once Attempts reaches it a retryable
	// failure becomes terminal failed_permanent (default 5).
	MaxAttempts int
	// ReclaimStaleAfter is how long a job may hold a lock before being presumed
	// orphaned and reclaimed (default 2 minutes). Must exceed any real provider
	// round-trip so an in-flight job is never reclaimed mid-call.
	ReclaimStaleAfter time.Duration
	// Now is an injectable clock (default time.Now, normalized to UTC).
	Now func() time.Time
}

// NewFiscalWorker constructs a worker bound to db, using factory to resolve the
// per-business provider for each claimed job.
func NewFiscalWorker(db *gorm.DB, factory ProviderFactory) *FiscalWorker {
	w := &FiscalWorker{
		db:          db,
		repo:        NewRepository(db),
		BatchSize:   25,
		MaxAttempts: 5,
	}
	w.service = NewService(db, nil).WithProviderFactory(factory).WithClock(func() time.Time { return w.now() })
	return w
}

// WithDelivery configures the receipt dispatcher on the worker's service so an
// authorized issue job delivers the receipt (S3 upload + email + best-effort
// print) once per authorization. A nil dispatcher disables delivery. Returns the
// worker for chaining.
func (w *FiscalWorker) WithDelivery(dispatcher ReceiptDispatcher) *FiscalWorker {
	w.service.WithDelivery(dispatcher)
	return w
}

// ProcessDue reclaims stale locks, claims due jobs, processes each, and returns
// the number of jobs claimed (i.e. attempted) this round.
func (w *FiscalWorker) ProcessDue(ctx context.Context) (int, error) {
	enabled, controlErr := automaticFiscalEnabled(ctx, w.db)
	if controlErr != nil {
		return 0, fmt.Errorf("read fiscal runtime control: %w", controlErr)
	}
	if !enabled {
		return 0, nil
	}
	now := w.now()

	// Best-effort self-heal: a reclaim failure must not block normal processing.
	if reclaimed, err := database.ReclaimStaleFiscalJobs(w.db, w.reclaimStaleAfter(), now); err != nil {
		logger.Logger.Warnf("Fiscal worker reclaim failed: %v", err)
	} else if reclaimed > 0 {
		logger.Logger.Infof("Fiscal worker reclaimed %d stranded fiscal jobs", reclaimed)
	}

	// Sample backlog depth before claiming so the gauge reflects what is waiting.
	// Best-effort: a count failure must not block processing.
	if depth, err := database.CountDueFiscalJobs(w.db, now); err == nil {
		metrics.FiscalJobsQueueDepth.Set(float64(depth))
	}

	jobs, err := database.ClaimDueFiscalJobs(w.db, w.workerID(), w.batchSize(), now)
	if err != nil {
		return 0, err
	}

	for i := range jobs {
		w.processJob(ctx, jobs[i])
	}

	// Best-effort: re-deliver authorized receipts that a transient render/email
	// failure left undelivered. Runs after job processing; a sweep error must not
	// fail ProcessDue. The grace window keeps it from racing the inline post-issue
	// delivery of receipts authorized this very tick.
	if delivered, err := w.service.SweepUndeliveredReceipts(ctx, now.Add(-deliverySweepGrace), w.batchSize()); err != nil {
		logger.Logger.Warnf("Fiscal worker undelivered-receipt sweep failed: %v", err)
	} else if delivered > 0 {
		logger.Logger.Infof("Fiscal worker re-delivered %d previously-undelivered receipts", delivered)
	}

	return len(jobs), nil
}

// deliverySweepGrace is how long after issuance an authorized-but-undelivered
// receipt waits before the sweep re-drives delivery — long enough that the inline
// post-issue delivery has had its chance, short enough to recover quickly.
const deliverySweepGrace = 5 * time.Minute

func (w *FiscalWorker) processJob(ctx context.Context, job database.FiscalJob) {
	metrics.FiscalJobAttempts.WithLabelValues(job.Action).Inc()
	// F-RECLAIM: bound each job to a deadline strictly shorter than the reclaim
	// window so a slow/hung AFIP SOAP call is aborted before its lock can be
	// reclaimed and the job re-issued by another worker. The provider HTTP clients
	// honor the request context, so this force-aborts an in-flight call.
	jobCtx, cancel := context.WithTimeout(ctx, w.jobProcessTimeout())
	defer cancel()
	outcome, err := w.service.ProcessClaimedJob(jobCtx, job)
	if err != nil {
		// An infrastructure error processing the job (not a provider/data outcome)
		// — leave it retryable so the next sweep retries; log and move on.
		logger.Logger.Errorf("Fiscal worker job %d processing error: %v", job.ID, err)
		if sentry.CurrentHub().Client() != nil {
			sentry.CaptureException(fmt.Errorf("fiscal worker job %d unexpected error: %w", job.ID, err))
		}
		outcome = &JobOutcome{Retryable: true, ErrorCode: "worker_error", ErrorMessage: err.Error()}
	}
	w.applyOutcome(job, outcome)
}

func (w *FiscalWorker) applyOutcome(job database.FiscalJob, outcome *JobOutcome) {
	now := w.now()
	attempts := job.Attempts + 1

	updates := map[string]interface{}{
		"attempts": attempts,
	}
	setError(updates, outcome.ErrorCode, outcome.ErrorMessage)

	// F-MAXATTEMPTS: honor the job's own retry budget (set per-job at enqueue and by
	// RetryReceipt) and only fall back to the worker-global default when unset.
	attemptCap := w.effectiveMaxAttempts(job)

	var eventType string
	var reconcileReceiptID *uint
	switch {
	case outcome.Retryable && attempts < attemptCap:
		next := now.Add(w.backoffForAttempt(attempts))
		updates["status"] = database.FiscalStatusFailedRetryable
		updates["next_attempt_at"] = next
		eventType = "fiscal_receipt_retry"
	case outcome.Retryable:
		// Retry budget exhausted → terminal permanent failure.
		updates["status"] = database.FiscalStatusFailedPermanent
		updates["next_attempt_at"] = nil
		eventType = "fiscal_receipt_failed_permanent"
		// SG-1: a retryable outcome can carry a linked receipt id (e.g. a
		// status_check). When the budget is exhausted the job goes terminal, so the
		// linked receipt row must not be left orphaned in a non-terminal state —
		// reconcile it to failed_permanent too, once the job write is confirmed
		// (a worker that lost its lock must not touch the receipt).
		// Best-effort: a failure here is logged and must never block the job
		// transition.
		reconcileReceiptID = outcome.ReceiptID
	default:
		status := outcome.TerminalStatus
		if status == "" {
			status = database.FiscalStatusFailedPermanent
		}
		updates["status"] = status
		updates["next_attempt_at"] = nil
		eventType = auditEventForTerminalStatus(status)
	}

	applied, err := w.repo.MarkJobResultApplied(job.ID, w.workerID(), updates, now)
	if err != nil {
		logger.Logger.Errorf("Fiscal worker failed to mark job %d result: %v", job.ID, err)
	} else if !applied {
		// The lock was reclaimed mid-flight; the current owner decides the
		// outcome. Reporting this stale result as an alert, metric or audit
		// event would contradict the real job state.
		logger.Logger.Warnf("Fiscal worker dropped result for job %d: lock no longer held", job.ID)
		return
	} else if reconcileReceiptID != nil {
		if rerr := w.repo.UpdateReceiptStatus(*reconcileReceiptID, database.FiscalStatusFailedPermanent, now); rerr != nil {
			logger.Logger.Errorf("Fiscal worker failed to reconcile receipt %d to failed_permanent on budget exhaustion (job %d): %v", *reconcileReceiptID, job.ID, rerr)
		}
		w.createFailedPermanentAlert(job, outcome)
	} else if st, ok := updates["status"].(database.FiscalStatus); ok && st == database.FiscalStatusFailedPermanent {
		// Both permanent-failure paths (non-retryable and retryable-but-exhausted)
		// write FiscalStatusFailedPermanent into updates["status"], so one check covers both.
		w.createFailedPermanentAlert(job, outcome)
	}

	finalStatus := fmt.Sprint(updates["status"])
	metrics.FiscalReceiptsTotal.WithLabelValues(finalStatus).Inc()
	w.writeAudit(job, outcome, eventType, finalStatus)
}

// createFailedPermanentAlert surfaces an exhausted fiscal job to the operator
// alert inbox. Best-effort: alerting must never fail the worker loop.
func (w *FiscalWorker) createFailedPermanentAlert(job database.FiscalJob, outcome *JobOutcome) {
	if outcome == nil {
		outcome = &JobOutcome{}
	}
	svc := operational_alerts.NewService(w.db)
	if _, err := svc.UpsertAlert(context.Background(), operational_alerts.UpsertAlertInput{
		BusinessID:   job.BusinessID,
		AlertType:    database.OperationalAlertTypeFiscalIssueFailed,
		ResourceType: database.OperationalAlertResourceTypeBill,
		ResourceID:   job.BillID,
		Priority:     database.OperationalAlertPriorityUrgent,
		Title:        "Fiscal receipt permanently failed",
		Body: fmt.Sprintf("Fiscal job %d (%s) for bill %d exhausted all retries: %s %s. Fix the cause, then re-issue from the bill.",
			job.ID, job.Action, job.BillID, outcome.ErrorCode, outcome.ErrorMessage),
		Metadata: map[string]any{
			"fiscal_job_id": job.ID,
			"action":        job.Action,
			"error_code":    outcome.ErrorCode,
		},
	}); err != nil {
		logger.Logger.Errorf("fiscal worker: failed to raise failed_permanent alert for job %d: %v", job.ID, err)
	}
}

func (w *FiscalWorker) writeAudit(job database.FiscalJob, outcome *JobOutcome, eventType, status string) {
	metadata := map[string]interface{}{
		"action": job.Action,
		"status": status,
	}
	if outcome.ErrorCode != "" {
		metadata["error_code"] = outcome.ErrorCode
	}
	if outcome.ErrorMessage != "" {
		metadata["error_message"] = outcome.ErrorMessage
	}
	event := &database.FiscalAuditEvent{
		BusinessID: job.BusinessID,
		ReceiptID:  outcome.ReceiptID,
		JobID:      &job.ID,
		Actor:      "system",
		EventType:  eventType,
		Message:    auditMessage(job.Action, status, outcome.ErrorMessage),
		Metadata:   metadata,
	}
	if err := w.repo.WriteAudit(event); err != nil {
		logger.Logger.Errorf("Fiscal worker failed to write audit for job %d: %v", job.ID, err)
	}
}

func auditEventForTerminalStatus(status database.FiscalStatus) string {
	switch status {
	case database.FiscalStatusAuthorized:
		return "fiscal_receipt_authorized"
	case database.FiscalStatusCredited:
		return "fiscal_receipt_credited"
	case database.FiscalStatusRejected:
		return "fiscal_receipt_rejected"
	case database.FiscalStatusCancelled:
		return "fiscal_receipt_cancelled"
	default:
		return "fiscal_receipt_failed_permanent"
	}
}

func auditMessage(action, status, errMsg string) string {
	if errMsg != "" {
		return fmt.Sprintf("fiscal job action=%s status=%s: %s", action, status, errMsg)
	}
	return fmt.Sprintf("fiscal job action=%s status=%s", action, status)
}

func setError(updates map[string]interface{}, code, message string) {
	if code != "" {
		updates["last_error_code"] = code
	} else {
		updates["last_error_code"] = nil
	}
	if message != "" {
		updates["last_error_message"] = message
	} else {
		updates["last_error_message"] = nil
	}
}

func (w *FiscalWorker) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

func (w *FiscalWorker) workerID() string {
	if w.WorkerID == "" {
		return "fiscal-worker"
	}
	return w.WorkerID
}

func (w *FiscalWorker) batchSize() int {
	if w.BatchSize <= 0 {
		return 25
	}
	return w.BatchSize
}

func (w *FiscalWorker) maxAttempts() int {
	if w.MaxAttempts <= 0 {
		return 5
	}
	return w.MaxAttempts
}

// effectiveMaxAttempts is the retry ceiling for a specific job: its own
// MaxAttempts when set (>0), else the worker-global default. RetryReceipt and the
// enqueue path set a per-job budget, which must not be silently overridden by the
// worker constant (F-MAXATTEMPTS).
func (w *FiscalWorker) effectiveMaxAttempts(job database.FiscalJob) int {
	if job.MaxAttempts > 0 {
		return job.MaxAttempts
	}
	return w.maxAttempts()
}

func (w *FiscalWorker) reclaimStaleAfter() time.Duration {
	if w.ReclaimStaleAfter <= 0 {
		// 5m comfortably exceeds the worst-case AFIP SOAP chain (WSAA refresh +
		// LastAuthorized + RequestCAE + reconcile), so a genuinely in-flight job is
		// never reclaimed mid-call; F-RECLAIM's per-job deadline aborts it earlier.
		return 5 * time.Minute
	}
	// Floor a misconfigured tiny value so jobProcessTimeout (reclaim − 30s margin,
	// floored at 30s) is always strictly below the reclaim window — otherwise the
	// "abort an in-flight job before its lock is reclaimed" guarantee breaks.
	const minReclaim = 60 * time.Second
	if w.ReclaimStaleAfter < minReclaim {
		return minReclaim
	}
	return w.ReclaimStaleAfter
}

// jobProcessTimeout is the per-job deadline: the reclaim window minus a safety
// margin, floored at 30s. It is always strictly less than reclaimStaleAfter so an
// in-flight provider call is force-aborted before the lock can be reclaimed.
func (w *FiscalWorker) jobProcessTimeout() time.Duration {
	const margin = 30 * time.Second
	d := w.reclaimStaleAfter() - margin
	if d < 30*time.Second {
		return 30 * time.Second
	}
	return d
}

// backoffForAttempt returns an exponential backoff capped at 6h:
// min(2^(attempt-1) * base, maxBackoff), base = 1 minute.
func (w *FiscalWorker) backoffForAttempt(attempt int) time.Duration {
	const base = time.Minute
	const maxBackoff = 6 * time.Hour
	if attempt < 1 {
		attempt = 1
	}
	d := base
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= maxBackoff {
			return maxBackoff
		}
	}
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}

// StartFiscalWorker runs w.ProcessDue on a fixed interval until ctx is cancelled.
// It is meant to be launched in its own goroutine (go StartFiscalWorker(...)).
// Each tick is wrapped so a panic in one sweep can never kill the loop, and a
// ProcessDue error is logged but does not stop the worker — the next tick retries.
// If interval <= 0 it defaults to 10s.
func StartFiscalWorker(ctx context.Context, w *FiscalWorker, interval time.Duration) {
	if w == nil {
		logger.Logger.Error("StartFiscalWorker called with nil worker; fiscal jobs will not be processed")
		return
	}
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	logger.Logger.Infof("Fiscal worker started (interval=%s)", interval)
	for {
		select {
		case <-ctx.Done():
			logger.Logger.Info("Fiscal worker stopping (context cancelled)")
			return
		case <-ticker.C:
			logger.SafeTick("fiscal-worker", func() {
				if processed, err := w.ProcessDue(ctx); err != nil {
					logger.Logger.Warnf("Fiscal worker sweep failed: %v", err)
				} else if processed > 0 {
					logger.Logger.Debugf("Fiscal worker processed %d jobs", processed)
				}
			})
		}
	}
}
