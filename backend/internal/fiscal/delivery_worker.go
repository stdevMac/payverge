package fiscal

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/metrics"

	"gorm.io/gorm"
)

// DeliveryWorker drains fiscal_delivery_tasks: claims due tasks with a lease,
// executes the channel side effect via ReceiptDispatcher, and applies
// success/retry/dead transitions with bounded exponential backoff + jitter.
type DeliveryWorker struct {
	db         *gorm.DB
	repo       *Repository
	service    *Service
	dispatcher ReceiptDispatcher

	// WorkerID identifies this instance in lease_owner.
	WorkerID string
	// BatchSize bounds claims per ProcessDue (default 25).
	BatchSize int
	// Concurrency is reserved for future fan-out; ProcessDue is sequential today.
	Concurrency int
	// LeaseDuration is how long a claim holds exclusive rights (default 2m).
	LeaseDuration time.Duration
	// Now is an injectable clock.
	Now func() time.Time
}

// NewDeliveryWorker constructs a worker bound to db and the service's delivery
// transports. dispatcher may be nil (worker no-ops processing).
func NewDeliveryWorker(db *gorm.DB, svc *Service, dispatcher ReceiptDispatcher) *DeliveryWorker {
	w := &DeliveryWorker{
		db:            db,
		repo:          NewRepository(db),
		service:       svc,
		dispatcher:    dispatcher,
		BatchSize:     25,
		Concurrency:   1,
		LeaseDuration: 2 * time.Minute,
	}
	if svc != nil {
		w.service = svc
		if dispatcher == nil {
			w.dispatcher = svc.dispatcher
		}
	}
	return w
}

// ProcessDue claims due delivery tasks and executes each channel. Returns the
// number of tasks claimed this round.
func (w *DeliveryWorker) ProcessDue(ctx context.Context) (int, error) {
	if w == nil || w.repo == nil {
		return 0, nil
	}
	enabled, controlErr := automaticFiscalEnabled(ctx, w.db)
	if controlErr != nil {
		return 0, fmt.Errorf("read fiscal runtime control: %w", controlErr)
	}
	if !enabled {
		return 0, nil
	}
	now := w.now()

	if pending, err := w.repo.CountPendingDeliveryTasks(); err == nil {
		metrics.FiscalDeliveryPending.Set(float64(pending))
	}
	if age, err := w.repo.OldestPendingDeliveryAge(now); err == nil {
		metrics.FiscalDeliveryOldestAgeSeconds.Set(age.Seconds())
	}

	tasks, err := w.repo.ClaimDueDeliveryTasks(w.workerID(), now, w.leaseDuration(), w.batchSize())
	if err != nil {
		return 0, err
	}

	for i := range tasks {
		if ctx.Err() != nil {
			return i, ctx.Err()
		}
		w.processTask(ctx, tasks[i])
	}
	return len(tasks), nil
}

func (w *DeliveryWorker) processTask(ctx context.Context, task database.FiscalDeliveryTask) {
	start := w.now()
	metrics.FiscalDeliveryAttempts.WithLabelValues(task.Channel).Inc()

	err := w.executeChannel(ctx, task)
	duration := w.now().Sub(start).Seconds()
	metrics.FiscalDeliveryDuration.WithLabelValues(task.Channel).Observe(duration)

	if err == nil {
		if markErr := w.repo.MarkDeliverySucceeded(task.ID, w.workerID(), "", w.now()); markErr != nil {
			logger.Logger.Warnf("Fiscal delivery: mark succeeded task %d: %v", task.ID, markErr)
		} else {
			metrics.FiscalDeliverySuccesses.WithLabelValues(task.Channel).Inc()
			w.maybeStampReceiptDelivered(task.ReceiptID)
		}
		return
	}

	var deferred *DeliveryDeferredError
	if errors.As(err, &deferred) {
		w.deferTask(task, deferred)
		return
	}

	if isPermanentDeliveryError(err) {
		if markErr := w.repo.MarkDeliveryDead(task.ID, w.workerID(), err.Error(), w.now()); markErr != nil {
			logger.Logger.Warnf("Fiscal delivery: mark dead task %d: %v", task.ID, markErr)
		} else {
			metrics.FiscalDeliveryDeadLetters.WithLabelValues(task.Channel).Inc()
		}
		return
	}

	attempts := task.Attempts + 1
	next := w.now().Add(w.backoffForAttempt(attempts))
	if markErr := w.repo.MarkDeliveryFailed(task.ID, w.workerID(), err.Error(), next, false, w.now()); markErr != nil {
		logger.Logger.Warnf("Fiscal delivery: mark failed task %d: %v", task.ID, markErr)
		return
	}
	// Detect dead-letter transition after max attempts.
	var reloaded database.FiscalDeliveryTask
	if w.db.Select("status").Where("id = ?", task.ID).First(&reloaded).Error == nil &&
		reloaded.Status == database.FiscalDeliveryStatusDead {
		metrics.FiscalDeliveryDeadLetters.WithLabelValues(task.Channel).Inc()
	}
}

func (w *DeliveryWorker) executeChannel(ctx context.Context, task database.FiscalDeliveryTask) error {
	if w.dispatcher == nil {
		return errors.New("delivery dispatcher not configured")
	}
	var receipt database.FiscalReceipt
	if err := w.db.WithContext(ctx).Where("id = ?", task.ReceiptID).First(&receipt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return permanentDeliveryError("receipt_not_found")
		}
		return err
	}
	if receipt.Status != database.FiscalStatusAuthorized {
		return permanentDeliveryError("receipt_not_authorized")
	}
	// Demo-provider receipts are simulated end-to-end: succeed without touching
	// real transports (S3, email provider, print queue). Running real delivery
	// for fake receipts dead-letters and floods needs_attention.
	if receipt.Provider == demoProviderName {
		return nil
	}

	jobCtx, err := w.repo.LoadJobContext(database.FiscalJob{
		BillID:     receipt.BillID,
		SettingsID: receipt.SettingsID,
		BusinessID: receipt.BusinessID,
	})
	if err != nil {
		return err
	}

	switch task.Channel {
	case database.FiscalDeliveryChannelArtifact:
		return w.execArtifact(ctx, &receipt, jobCtx)
	case database.FiscalDeliveryChannelEmail:
		return w.execEmail(ctx, &receipt, jobCtx, task)
	case database.FiscalDeliveryChannelPrint:
		return w.execPrint(ctx, &receipt, jobCtx, task)
	default:
		return permanentDeliveryError(fmt.Sprintf("unknown_channel:%s", task.Channel))
	}
}

func (w *DeliveryWorker) execArtifact(ctx context.Context, receipt *database.FiscalReceipt, jobCtx *JobContext) error {
	// Idempotent: if PDF already stored, succeed without re-upload.
	if receipt.PDFPath != nil && strings.TrimSpace(*receipt.PDFPath) != "" {
		return nil
	}
	pdf, err := RenderReceiptPDF(*receipt, jobCtx.Business, jobCtx.Settings)
	if err != nil {
		return fmt.Errorf("render pdf: %w", err)
	}
	// Use deterministic S3 key via uploadReceiptArtifacts path on service. Pass the
	// worker's own dispatcher explicitly — do NOT mutate the shared w.service field
	// (latent data race once ProcessDue fans out concurrently).
	if w.service != nil {
		w.service.uploadReceiptArtifacts(w.dispatcher, receipt, pdf)
		// Re-check: upload failures are swallowed by uploadReceiptArtifacts; treat
		// missing path as retryable failure.
		if receipt.PDFPath == nil || strings.TrimSpace(*receipt.PDFPath) == "" {
			return errors.New("artifact upload did not persist pdf_path")
		}
		return nil
	}
	folder := fmt.Sprintf("fiscal-receipts/%d", receipt.BusinessID)
	name := fmt.Sprintf("receipt-%d.pdf", receipt.ID)
	loc, err := w.dispatcher.UploadProtected(pdf, name, folder, "application/pdf")
	if err != nil {
		return err
	}
	if loc == "" {
		return errors.New("empty upload location")
	}
	return w.repo.UpdateReceiptArtifactPaths(receipt.ID, map[string]interface{}{"pdf_path": loc})
}

func (w *DeliveryWorker) execEmail(ctx context.Context, receipt *database.FiscalReceipt, jobCtx *JobContext, task database.FiscalDeliveryTask) error {
	// Already-succeeded path is handled by status; if provider_message_id set, ok.
	if task.ProviderMessageID != nil && strings.TrimSpace(*task.ProviderMessageID) != "" {
		return nil
	}
	recipient := ""
	if w.service != nil {
		recipient = w.service.customerEmail(ctx, jobCtx.Bill)
	}
	if recipient == "" {
		// No recipient (Consumidor Final): succeed without sending — same as
		// legacy deliverReceipt which stamps DeliveredAt with no email.
		return nil
	}
	pdf, err := RenderReceiptPDF(*receipt, jobCtx.Business, jobCtx.Settings)
	if err != nil {
		return fmt.Errorf("render pdf: %w", err)
	}
	lang := receiptEmailLanguage(jobCtx.Settings)
	if task.Locale != nil && strings.TrimSpace(*task.Locale) != "" {
		lang = *task.Locale
	}
	// Email delivery is at-least-once at the provider boundary (SendReceiptEmail
	// returns no provider message id we can pre-reserve). The narrow duplicate
	// window is: the provider accepts the send, then the process dies before
	// MarkDeliverySucceeded and the lease expires → a re-claim re-sends. We close
	// that window locally by stamping a dispatch marker (provider_message_id) the
	// instant the send is accepted; the guard at the top of execEmail then short-
	// circuits any re-claim. A fully provider-side Idempotency-Key would also
	// dedup at the mail provider, but that is a cross-cutting change to the shared
	// email infrastructure (EmailMessage / provider / Resend SDK) used by ALL
	// transactional mail — deferred as its own initiative; this marker is the
	// correct local mitigation and is exercised by
	// TestDeliveryWorker_EmailNotResentAfterCrashWindow.
	if err := sendBillReceiptEmail(
		w.dispatcher,
		jobCtx.Bill,
		[]string{recipient},
		emitterName(jobCtx.Business),
		receipt.ReceiptType,
		deref(receipt.ReceiptNumber),
		formatARS(receipt.TotalAmountCents),
		lang,
		pdf,
	); err != nil {
		return err
	}
	// Record the dispatch marker before the task is finalized as succeeded, so a
	// crash/lease-expiry re-claim between here and MarkDeliverySucceeded does not
	// resend. Failure to stamp is logged but does not fail the task — the email
	// WAS sent; the marker is a best-effort duplicate suppressor.
	if err := w.repo.MarkDeliveryDispatched(task.ID, w.workerID(), task.IdempotencyKey, w.now()); err != nil {
		logger.Logger.Warnf("Fiscal delivery: mark dispatched task %d: %v", task.ID, err)
	}
	return nil
}

func (w *DeliveryWorker) execPrint(ctx context.Context, receipt *database.FiscalReceipt, jobCtx *JobContext, task database.FiscalDeliveryTask) error {
	lang := receiptEmailLanguage(jobCtx.Settings)
	if task.Locale != nil && strings.TrimSpace(*task.Locale) != "" {
		lang = *task.Locale
	}
	// Idempotency: the print service does NOT dedup receipt-kind jobs, so a
	// retry / lease-expiry re-run would enqueue a SECOND print job → duplicate
	// physical receipt. Guard with a read-check for an existing (non-cancelled)
	// receipt-kind print job for this bill CREATED BY THE FISCAL CHANNEL
	// (created_by = 'fiscal-delivery', stamped by dispatcher.EnqueueReceiptPrint).
	// Courtesy receipt jobs from payment flows must NOT count: their HTML is
	// rendered at enqueue time, before the CAE exists, so they carry no fiscal
	// block — deduping on them would silently drop the legal comprobante.
	// Accepted trade-off: a courtesy ticket and a fiscal ticket may both print
	// for one bill. The task lease serializes execution, so check-then-enqueue
	// is not racy across workers.
	var existing int64
	if err := w.db.WithContext(ctx).Model(&database.PrintJob{}).
		Where("business_id = ? AND source_type = ? AND source_id = ? AND kind = ? AND created_by = ? AND status <> ?",
			jobCtx.Bill.BusinessID, "bill", jobCtx.Bill.ID,
			database.PrintJobKindReceipt, "fiscal-delivery", database.PrintJobStatusCancelled).
		Count(&existing).Error; err == nil && existing > 0 {
		return nil
	}
	return w.dispatcher.EnqueueReceiptPrint(ctx, jobCtx.Bill.BusinessID, jobCtx.Bill.ID, lang)
}

// maybeStampReceiptDelivered sets delivered_at when all channels for the receipt
// have succeeded (aggregate legacy field for dashboards / sweep gate).
func (w *DeliveryWorker) maybeStampReceiptDelivered(receiptID uint) {
	var pending int64
	if err := w.db.Model(&database.FiscalDeliveryTask{}).
		Where("receipt_id = ? AND status != ?", receiptID, database.FiscalDeliveryStatusSucceeded).
		Count(&pending).Error; err != nil || pending > 0 {
		return
	}
	now := w.now()
	_ = w.repo.MarkReceiptDelivered(receiptID, now)
}

func (w *DeliveryWorker) backoffForAttempt(attempt int) time.Duration {
	// Bounded exponential: 15s * 2^(attempt-1), cap 15m, ± ±20%.
	if attempt < 1 {
		attempt = 1
	}
	base := 15 * time.Second
	max := 15 * time.Minute
	secs := float64(base) * math.Pow(2, float64(attempt-1))
	d := time.Duration(secs)
	if d > max {
		d = max
	}
	// Jitter ±20%
	j := 0.2 * (rand.Float64()*2 - 1)
	return time.Duration(float64(d) * (1 + j))
}

func (w *DeliveryWorker) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

func (w *DeliveryWorker) workerID() string {
	if strings.TrimSpace(w.WorkerID) == "" {
		return "fiscal-delivery-worker"
	}
	return w.WorkerID
}

func (w *DeliveryWorker) batchSize() int {
	if w.BatchSize <= 0 {
		return 25
	}
	return w.BatchSize
}

func (w *DeliveryWorker) leaseDuration() time.Duration {
	if w.LeaseDuration <= 0 {
		return 2 * time.Minute
	}
	return w.LeaseDuration
}

// DeliveryDeferredError means the side effect was refused by a quota that
// resets on its own (the tenant outbound email budget), not by a fault that a
// fast retry could fix. The worker puts the task back to pending after After
// WITHOUT consuming an attempt, so a spent daily budget cannot dead-letter a
// legally required receipt within the backoff schedule. A task still deferred
// MaxDeliveryDeferral after it was created is dead-lettered so it surfaces to
// the operator instead of waiting forever.
type DeliveryDeferredError struct {
	Err   error
	After time.Duration
}

func (e *DeliveryDeferredError) Error() string {
	if e == nil || e.Err == nil {
		return "delivery deferred"
	}
	return "delivery deferred: " + e.Err.Error()
}

func (e *DeliveryDeferredError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// DeferDelivery wraps err so the delivery worker retries after `after`
// without consuming an attempt. A non-positive after uses DefaultDeliveryDeferral.
func DeferDelivery(err error, after time.Duration) error {
	if err == nil {
		return nil
	}
	if after <= 0 {
		after = DefaultDeliveryDeferral
	}
	return &DeliveryDeferredError{Err: err, After: after}
}

const (
	// DefaultDeliveryDeferral is how long a deferred task waits before the
	// next try. The tenant email budget is a rolling 24h window, so an hourly
	// retry delivers within an hour of budget becoming available again.
	DefaultDeliveryDeferral = time.Hour
	// MaxDeliveryDeferral bounds how long deferrals may keep a task alive,
	// counted from the task's creation.
	MaxDeliveryDeferral = 72 * time.Hour
)

func (w *DeliveryWorker) deferTask(task database.FiscalDeliveryTask, deferred *DeliveryDeferredError) {
	now := w.now()
	if !task.CreatedAt.IsZero() && now.Sub(task.CreatedAt) >= MaxDeliveryDeferral {
		msg := "deferred past " + MaxDeliveryDeferral.String() + ": " + deferred.Error()
		if markErr := w.repo.MarkDeliveryDead(task.ID, w.workerID(), msg, now); markErr != nil {
			logger.Logger.Warnf("Fiscal delivery: mark dead task %d after deferral horizon: %v", task.ID, markErr)
		} else {
			metrics.FiscalDeliveryDeadLetters.WithLabelValues(task.Channel).Inc()
		}
		return
	}
	after := deferred.After
	if after <= 0 {
		after = DefaultDeliveryDeferral
	}
	// ±10% jitter so a venue's deferred receipts do not all retry at once.
	after = time.Duration(float64(after) * (1 + 0.1*(rand.Float64()*2-1)))
	if markErr := w.repo.DeferDelivery(task.ID, w.workerID(), deferred.Error(), now.Add(after), now); markErr != nil {
		logger.Logger.Warnf("Fiscal delivery: defer task %d: %v", task.ID, markErr)
		return
	}
	logger.Logger.Warnf("Fiscal delivery: task %d (business %d, %s) deferred %s: %v",
		task.ID, task.BusinessID, task.Channel, after.Round(time.Second), deferred.Err)
}

// permanentDeliveryError marks non-retryable failures.
type permanentDeliveryError string

func (e permanentDeliveryError) Error() string { return string(e) }

func isPermanentDeliveryError(err error) bool {
	var p permanentDeliveryError
	return errors.As(err, &p)
}

// StartDeliveryWorker runs w.ProcessDue on interval until ctx is cancelled.
func StartDeliveryWorker(ctx context.Context, w *DeliveryWorker, interval time.Duration) {
	if w == nil {
		logger.Logger.Error("StartDeliveryWorker called with nil worker")
		return
	}
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	logger.Logger.Infof("Fiscal delivery worker started (interval=%s concurrency=%d)", interval, w.Concurrency)
	for {
		select {
		case <-ctx.Done():
			logger.Logger.Info("Fiscal delivery worker stopping (context cancelled)")
			return
		case <-ticker.C:
			logger.SafeTick("fiscal-delivery-worker", func() {
				if n, err := w.ProcessDue(ctx); err != nil {
					logger.Logger.Warnf("Fiscal delivery worker sweep failed: %v", err)
				} else if n > 0 {
					logger.Logger.Debugf("Fiscal delivery worker processed %d tasks", n)
				}
			})
		}
	}
}
