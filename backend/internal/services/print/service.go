package print

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services/print/formatters"
)

// kitchenTicketKinds are the print kinds that must exist at most once per order.
// Two independent producers (approve-time enqueue and the orphan sweep) race to
// create these, so their check-then-insert is serialized on the order row.
var kitchenTicketKinds = []database.PrintJobKind{
	database.PrintJobKindKitchen,
	database.PrintJobKindBar,
}

func isKitchenTicketKind(k database.PrintJobKind) bool {
	for _, kk := range kitchenTicketKinds {
		if k == kk {
			return true
		}
	}
	return false
}

// ErrKindNotReprintable is returned by Reprint when the job's kind does not
// support reprinting (e.g. kitchen tickets).
var ErrKindNotReprintable = errors.New("kind not reprintable")

// ErrJobNotReprintableState prevents a manual copy while the original job can
// still print or retry. Creating another job in those states can put two
// physical receipts in front of the guest.
var ErrJobNotReprintableState = errors.New("job is not in a reprintable terminal state")

// ErrJobNotReroutable is returned when an operator asks to assign a printer to
// a job that already has one, or that is past the pending/unassigned window.
var ErrJobNotReroutable = errors.New("job is not waiting for a printer")

// ErrReceiptRequiresPaidBill prevents an operator or internal caller from
// producing a receipt (a representation of collected money) for an open or
// partially paid bill.
var ErrReceiptRequiresPaidBill = errors.New("receipt requires a paid bill")

// Service composes the Queue and Router to provide the high-level print
// operations used by HTTP handlers and background jobs.
type Service struct {
	db     *gorm.DB
	queue  *Queue
	router *Router
}

// NewService creates a Service backed by db.
func NewService(db *gorm.DB) *Service {
	return &Service{
		db:     db,
		queue:  NewQueue(db),
		router: NewRouter(db),
	}
}

// Enqueue validates params, inserts a pending PrintJob, and attempts to route
// it to an enabled printer. If a printer is found, the HTML payload is rendered
// and stored; the returned job has status=routed. When no printer is registered
// for the requested role, the job remains pending and is returned as-is.
func (s *Service) Enqueue(ctx context.Context, p EnqueueParams) (*database.PrintJob, error) {
	return s.EnqueueWithTx(ctx, s.db, p)
}

// EnqueueWithTx is the transactional variant: callers (notably order create
// in IMP-04) pass a *gorm.DB tied to an open transaction so the print_jobs
// row commits atomically with the order row. The render/route step still
// runs through the parent DB (read-only) so missing printers don't tank
// the order create.
func (s *Service) EnqueueWithTx(ctx context.Context, tx *gorm.DB, p EnqueueParams) (*database.PrintJob, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if tx == nil {
		tx = s.db
	}

	job := &database.PrintJob{
		BusinessID: p.BusinessID,
		LocationID: p.LocationID,
		OrderID:    p.OrderID,
		Kind:       p.Kind,
		SourceType: p.SourceType,
		SourceID:   p.SourceID,
		Status:     database.PrintJobStatusPending,
		Language:   normalizeJobLanguage(p.Language),
		CreatedBy:  defaultActor(p.CreatedBy),
	}
	// Default maxAttempts via tag, but be explicit so SQLite tests don't
	// inherit the column default (which sometimes lags on auto-migrate).
	if job.MaxAttempts == 0 {
		job.MaxAttempts = 6
	}
	now := time.Now()
	job.NextAttemptAt = &now

	// Kitchen/bar tickets must exist at most once per order. The approve-time
	// enqueue and the orphan sweep both do check(OrderHasKitchenJob)-then-insert
	// with no DB backstop, so a narrow TOCTOU window (approve firing while the
	// 60s sweep tick runs its NOT EXISTS query) yields two tickets for one order.
	// Serialize the check+insert on the order row: lock it, re-check inside the
	// same tx, and no-op back the existing job if a peer producer won the race.
	if p.OrderID != nil && isKitchenTicketKind(p.Kind) {
		existing, err := s.insertKitchenTicketDeduped(ctx, tx, job, *p.OrderID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	} else if p.Kind == database.PrintJobKindReceipt && p.SourceType == "bill" && p.SourceID != 0 {
		// Receipt auto-fire can land from multiple settlement paths (plugin,
		// cash/alt, crypto, fiscal delivery). Dedup so a dual producer does not
		// double-print the same bill. Reprint intentionally creates a new job
		// with a different SourceType/flow and still uses plain Create.
		existing, err := s.insertReceiptDeduped(ctx, tx, job)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	} else if p.Kind == database.PrintJobKindReceipt && p.SourceType == "reprint" && p.SourceID != 0 {
		if err := requirePaidReceiptBill(ctx, tx, p.BusinessID, p.SourceID, false); err != nil {
			return nil, err
		}
		if err := tx.WithContext(ctx).Create(job).Error; err != nil {
			return nil, err
		}
	} else if err := tx.WithContext(ctx).Create(job).Error; err != nil {
		return nil, err
	}

	role := roleForKind(p.Kind)
	printer, err := s.router.Route(p.BusinessID, p.LocationID, role)
	if err != nil {
		if errors.Is(err, ErrNoPrinterForRole) {
			// No printer registered — leave job as pending and return it.
			// The retry worker will keep trying once one comes online.
			return job, nil
		}
		return nil, err
	}

	html, err := s.renderHTML(ctx, p, *printer)
	if err != nil {
		// Don't leak the render error onto the open transaction — let the
		// retry worker reschedule the job after the parent commits. We log
		// the failure on the job row using a follow-up update.
		msg := err.Error()
		// Best-effort: record the render failure on the job row. We deliberately
		// do NOT return this error — that would roll back the parent transaction
		// (the bill/order being created) over a best-effort, async print render.
		// If the status write itself fails, log it for visibility; the job stays
		// Pending with NextAttemptAt set, so the retry worker still reschedules it.
		if uErr := tx.WithContext(ctx).Model(&database.PrintJob{}).Where("id = ?", job.ID).
			Updates(map[string]interface{}{
				"status":     database.PrintJobStatusFailedRetryable,
				"last_error": msg,
			}).Error; uErr != nil {
			log.Printf("[print] mark job %d failed-retryable failed: %v", job.ID, uErr)
		}
		return job, nil
	}

	updates := map[string]interface{}{
		"printer_id":   printer.ID,
		"payload_html": html,
		"status":       database.PrintJobStatusRouted,
		"updated_at":   time.Now(),
	}
	if err := tx.WithContext(ctx).Model(&database.PrintJob{}).Where("id = ?", job.ID).Updates(updates).Error; err != nil {
		return nil, err
	}

	// Re-read in the same tx so callers get the freshly stamped row.
	var refreshed database.PrintJob
	if err := tx.WithContext(ctx).First(&refreshed, job.ID).Error; err != nil {
		return nil, err
	}
	// Wake events are best-effort hints. EnqueueWithTx may run inside an outer
	// transaction, so the durable print_jobs row and the recovery poll remain
	// authoritative if a browser claims before the outer commit is visible.
	publishPrintWake(&refreshed)
	return &refreshed, nil
}

// insertKitchenTicketDeduped inserts job atomically-once per order. It locks the
// order row (Postgres FOR UPDATE; a no-op but harmless on SQLite, where the
// single writer already serializes) so the approve-time producer and the orphan
// sweep cannot both pass the existence check and insert. If a kitchen/bar job
// for the order already exists, it returns that row and does NOT insert (nil
// error, non-nil existing). Otherwise it creates job and returns (nil, nil).
//
// The whole check+insert runs in one transaction. Callers pass either s.db or an
// already-open tx; tx.Transaction nests via savepoint so both are safe.
func (s *Service) insertKitchenTicketDeduped(ctx context.Context, tx *gorm.DB, job *database.PrintJob, orderID uint) (*database.PrintJob, error) {
	var existing *database.PrintJob
	err := tx.WithContext(ctx).Transaction(func(txn *gorm.DB) error {
		// Lock the order row so concurrent producers serialize on it. A missing
		// order (e.g. a bare source with no real order row) is not fatal — fall
		// through to the re-check, which then simply inserts.
		if isPostgres(txn) {
			_ = txn.Model(&database.Order{}).
				Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("id = ?", orderID).
				Select("id").
				First(&struct{ ID uint }{}).Error
		}

		var found database.PrintJob
		lookupErr := txn.Where("order_id = ? AND kind IN ?", orderID, kitchenTicketKinds).
			First(&found).Error
		if lookupErr == nil {
			existing = &found
			return nil
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return lookupErr
		}
		return txn.Create(job).Error
	})
	if err != nil {
		return nil, err
	}
	return existing, nil
}

// insertReceiptDeduped inserts a receipt job at most once per paid cycle for
// (business_id, source_type=bill, source_id=bill.ID) among non-cancelled jobs.
// The paid-cycle boundary is bill.ClosedAt, then SettledAt, then CreatedAt for
// legacy rows; refund/reversal clears ClosedAt so repayment starts a new cycle
// and allows one new receipt. Concurrent settlement paths (plugin + fiscal
// delivery, cash confirm + webhook) both call Enqueue; without this they
// double-print. Returns the existing row when a peer won the race (nil error,
// non-nil existing); otherwise creates job and returns (nil, nil).
func (s *Service) insertReceiptDeduped(ctx context.Context, tx *gorm.DB, job *database.PrintJob) (*database.PrintJob, error) {
	var existing *database.PrintJob
	err := tx.WithContext(ctx).Transaction(func(txn *gorm.DB) error {
		// All automatic final receipts use source_type=bill/source_id=bill.ID.
		// Lock that durable parent before the existence check so independent
		// settlement callbacks cannot both observe an empty queue and insert.
		// SQLite already serializes its single writer; PostgreSQL needs the row
		// lock to close the READ COMMITTED check-then-insert race.
		cycleStart, err := paidReceiptCycleStart(ctx, txn, job.BusinessID, job.SourceID, true)
		if err != nil {
			return err
		}

		var found database.PrintJob
		lookupErr := txn.
			Where(
				"business_id = ? AND kind = ? AND source_type = ? AND source_id = ? AND status <> ?",
				job.BusinessID, database.PrintJobKindReceipt, job.SourceType, job.SourceID, database.PrintJobStatusCancelled,
			).
			Order("created_at DESC").
			Order("id DESC").
			First(&found).Error
		if lookupErr == nil {
			// Compare time.Time values in Go. SQLite stores timezone offsets as
			// text, whose lexical ordering can disagree with chronological order;
			// PostgreSQL returns the same absolute instant here.
			if !found.CreatedAt.Before(cycleStart) {
				existing = &found
				return nil
			}
			return txn.Create(job).Error
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return lookupErr
		}
		return txn.Create(job).Error
	})
	if err != nil {
		return nil, err
	}
	return existing, nil
}

func requirePaidReceiptBill(ctx context.Context, tx *gorm.DB, businessID, billID uint, lock bool) error {
	_, err := paidReceiptCycleStart(ctx, tx, businessID, billID, lock)
	return err
}

// paidReceiptCycleStart returns the durable boundary for the bill's current
// transition to paid. closed_at is cleared by refund/reversal and written again
// on repayment, so it distinguishes legitimate later receipt cycles while
// concurrent producers in the same cycle still serialize on the bill row.
func paidReceiptCycleStart(ctx context.Context, tx *gorm.DB, businessID, billID uint, lock bool) (time.Time, error) {
	var bill database.Bill
	query := tx.WithContext(ctx).
		Select("id", "status", "closed_at", "settled_at", "created_at", "updated_at").
		Where("id = ? AND business_id = ?", billID, businessID)
	if lock && isPostgres(tx) {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.First(&bill).Error; err != nil {
		return time.Time{}, err
	}
	if bill.Status != database.BillStatusPaid {
		return time.Time{}, ErrReceiptRequiresPaidBill
	}
	if bill.ClosedAt != nil && !bill.ClosedAt.IsZero() {
		return bill.ClosedAt.UTC(), nil
	}
	if bill.SettledAt != nil && !bill.SettledAt.IsZero() {
		return bill.SettledAt.UTC(), nil
	}
	// Compatibility for legacy paid rows without lifecycle timestamps. This
	// intentionally deduplicates their whole known history rather than risking
	// a duplicate physical receipt from an ambiguous cycle.
	if !bill.CreatedAt.IsZero() {
		return bill.CreatedAt.UTC(), nil
	}
	return time.Unix(0, 0).UTC(), nil
}

// MarkPrinted transitions a job to the Printed status and records PrintedAt.
// actor is informational and reserved for future audit-log use.
func (s *Service) MarkPrinted(ctx context.Context, id uint, actor string) error {
	// Only a job actually in flight (routed/printing) can be marked printed —
	// a browser double-fire or a late callback after cancel must not re-stamp a
	// terminal job's printed_at.
	if err := s.queue.TransitionGuarded(ctx, id, database.PrintJobStatusPrinted,
		[]database.PrintJobStatus{database.PrintJobStatusRouted, database.PrintJobStatusPrinting}, nil); err != nil {
		return err
	}
	// X-4: stamp the order's kitchen ack on successful kitchen/bar prints.
	if job, err := s.queue.Get(ctx, id); err == nil {
		recordKitchenAck(ctx, s.db, *job)
	}
	return nil
}

// Cancel transitions a job to the Cancelled status.
// actor is informational and reserved for future audit-log use.
func (s *Service) Cancel(ctx context.Context, id uint, actor string) error {
	// A job may only be cancelled while it is still non-terminal. Cancelling an
	// already-printed job (or one already failed_permanent/cancelled) must be
	// rejected rather than overwriting the terminal state.
	return s.queue.TransitionGuarded(ctx, id, database.PrintJobStatusCancelled,
		[]database.PrintJobStatus{
			database.PrintJobStatusPending,
			database.PrintJobStatusRouted,
			database.PrintJobStatusPrinting,
			database.PrintJobStatusFailedRetryable,
		}, nil)
}

// Reroute attempts to assign a printer to a pending/unassigned job after an
// operator has added or re-enabled a station. Used by the Recent Jobs recovery
// CTA when a receipt sat queued with printer_id NULL.
func (s *Service) Reroute(ctx context.Context, jobID uint, actor string) (*database.PrintJob, error) {
	job, err := s.queue.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.PrinterID != nil || (job.Status != database.PrintJobStatusPending && job.Status != database.PrintJobStatusFailedRetryable) {
		return nil, ErrJobNotReroutable
	}

	role := roleForKind(job.Kind)
	printer, err := s.router.Route(job.BusinessID, job.LocationID, role)
	if err != nil {
		if errors.Is(err, ErrNoPrinterForRole) {
			return nil, ErrNoPrinterForRole
		}
		return nil, err
	}

	params := EnqueueParams{
		BusinessID: job.BusinessID,
		LocationID: job.LocationID,
		OrderID:    job.OrderID,
		Kind:       job.Kind,
		SourceType: job.SourceType,
		SourceID:   job.SourceID,
		Language:   job.Language,
		CreatedBy:  defaultActor(actor),
	}
	html, err := s.renderHTML(ctx, params, *printer)
	if err != nil {
		msg := err.Error()
		_ = s.db.WithContext(ctx).Model(&database.PrintJob{}).Where("id = ?", job.ID).
			Updates(map[string]interface{}{
				"status":     database.PrintJobStatusFailedRetryable,
				"last_error": msg,
			}).Error
		return nil, err
	}
	if err := s.queue.SetPrinterAndPayload(ctx, job.ID, printer.ID, html); err != nil {
		return nil, err
	}
	s.writePrintAudit(ctx, job.ID, defaultActor(actor), "reroute", map[string]interface{}{
		"printer_id": printer.ID,
	})
	refreshed, err := s.queue.Get(ctx, job.ID)
	if err != nil {
		return nil, err
	}
	publishPrintWake(refreshed)
	return refreshed, nil
}

// Reprint creates a new print job from an existing one's parameters.
// Only bill and receipt kinds support reprinting; kitchen/bar/void/modify
// return ErrKindNotReprintable.
func (s *Service) Reprint(ctx context.Context, originalID uint, actor string) (*database.PrintJob, error) {
	original, err := s.queue.Get(ctx, originalID)
	if err != nil {
		return nil, err
	}
	if original.Kind != database.PrintJobKindBill && original.Kind != database.PrintJobKindReceipt {
		return nil, ErrKindNotReprintable
	}
	if !manualReprintState(original.Status) {
		return nil, ErrJobNotReprintableState
	}
	return s.Enqueue(ctx, EnqueueParams{
		BusinessID: original.BusinessID,
		LocationID: original.LocationID,
		Kind:       original.Kind,
		// Reprints are deliberate operator actions and must bypass the
		// source_type=bill auto-receipt dedupe path. SourceID still identifies
		// the bill used to render the payload.
		SourceType: "reprint",
		SourceID:   original.SourceID,
		Language:   original.Language,
		CreatedBy:  actor,
	})
}

func manualReprintState(status database.PrintJobStatus) bool {
	switch status {
	case database.PrintJobStatusPrinted,
		database.PrintJobStatusFailed,
		database.PrintJobStatusFailedPermanent,
		database.PrintJobStatusCancelled:
		return true
	default:
		return false
	}
}

// renderHTML produces the HTML payload for the given job params and printer.
func (s *Service) renderHTML(ctx context.Context, p EnqueueParams, printer database.Printer) (string, error) {
	switch p.Kind {
	case database.PrintJobKindBill:
		in, err := buildBillInputFromBill(ctx, s.db, p.SourceID, p.BusinessID, p.Language)
		if err != nil {
			return "", err
		}
		return formatters.Bill(in, printer.PaperWidthMM)

	case database.PrintJobKindReceipt:
		in, err := buildReceiptInputFromBill(ctx, s.db, p.SourceID, p.BusinessID, p.Language)
		if err != nil {
			return "", err
		}
		return formatters.Receipt(in, printer.PaperWidthMM)

	case database.PrintJobKindKitchen, database.PrintJobKindBar:
		// IMP-13: kitchen + bar tickets share the same modifier/allergen
		// payload. SourceID is the Order.ID per the orphan_sweep contract.
		orderID := p.SourceID
		if p.OrderID != nil && *p.OrderID != 0 {
			orderID = *p.OrderID
		}
		in, err := buildKitchenTicketInputFromOrder(ctx, s.db, orderID, p.BusinessID, p.Language)
		if err != nil {
			return "", err
		}
		return formatters.KitchenTicket(in, printer.PaperWidthMM)

	default:
		return "", fmt.Errorf("kind %q not implemented in Sprint 1", p.Kind)
	}
}

// normalizeJobLanguage collapses a requested job language onto the canonical
// label-bundle tier ("en" / "es" / "es-AR"). Explicit fallback chain:
// recognized Spanish family (es, es-AR in any tolerated spelling) → its
// bundle; everything else — empty string, the 18 other guest locales,
// unknown tags — → "en".
func normalizeJobLanguage(in string) string {
	return formatters.CanonicalPrintLanguage(in)
}

// defaultActor returns "system" when in is empty.
func defaultActor(in string) string {
	if in == "" {
		return "system"
	}
	return in
}

// roleForKind maps a PrintJobKind to the printer role string used by the Router.
func roleForKind(k database.PrintJobKind) string {
	switch k {
	case database.PrintJobKindBill, database.PrintJobKindReceipt:
		return "bill"
	case database.PrintJobKindBar:
		return "bar"
	default:
		return "kitchen"
	}
}

// ---- Wave 4 browser-agent lease API (wrappers over Queue) ----

// DefaultBrowserLease is the default lease duration granted on claim/renew.
const DefaultBrowserLease = 45 * time.Second

// ClaimBrowserJob leases the next browser-transport job for the business to
// clientID. Returns (nil, nil) when the queue is empty.
func (s *Service) ClaimBrowserJob(ctx context.Context, businessID uint, clientID string, now time.Time, leaseDur time.Duration) (*database.PrintJob, error) {
	if leaseDur <= 0 {
		leaseDur = DefaultBrowserLease
	}
	return s.queue.ClaimBrowserJob(ctx, businessID, clientID, now, leaseDur)
}

// ClaimBrowserJobForPrinter leases work only for the browser station selected
// by the operator.
func (s *Service) ClaimBrowserJobForPrinter(ctx context.Context, businessID, printerID uint, clientID string, now time.Time, leaseDur time.Duration) (*database.PrintJob, error) {
	if leaseDur <= 0 {
		leaseDur = DefaultBrowserLease
	}
	return s.queue.ClaimBrowserJobForPrinter(ctx, businessID, printerID, clientID, now, leaseDur)
}

// ClaimAuthorizedBrowserJobForPrinter limits a station lease to job kinds the
// authenticated operator may actually print.
func (s *Service) ClaimAuthorizedBrowserJobForPrinter(ctx context.Context, businessID, printerID uint, allowedKinds []database.PrintJobKind, clientID string, now time.Time, leaseDur time.Duration) (*database.PrintJob, error) {
	if leaseDur <= 0 {
		leaseDur = DefaultBrowserLease
	}
	return s.queue.ClaimAuthorizedBrowserJobForPrinter(ctx, businessID, printerID, allowedKinds, clientID, now, leaseDur)
}

// RenewBrowserLease extends the lease held by clientID.
func (s *Service) RenewBrowserLease(ctx context.Context, jobID uint, clientID string, now time.Time, leaseDur time.Duration) error {
	if leaseDur <= 0 {
		leaseDur = DefaultBrowserLease
	}
	return s.queue.RenewBrowserLease(ctx, jobID, clientID, now, leaseDur)
}

// MarkPresented records dialog presentation without confirming the print.
// actor is recorded on the print audit log when available.
func (s *Service) MarkPresented(ctx context.Context, jobID uint, clientID string, now time.Time, actor string) error {
	if err := s.queue.MarkPresented(ctx, jobID, clientID, now); err != nil {
		return err
	}
	s.writePrintAudit(ctx, jobID, actor, "browser_presented", nil)
	return nil
}

// ConfirmPrinted is the explicit operator "Printed" confirmation.
func (s *Service) ConfirmPrinted(ctx context.Context, jobID uint, clientID string, now time.Time, actor string) error {
	if err := s.queue.ConfirmPrinted(ctx, jobID, clientID, now); err != nil {
		return err
	}
	if job, err := s.queue.Get(ctx, jobID); err == nil {
		recordKitchenAck(ctx, s.db, *job)
	}
	s.writePrintAudit(ctx, jobID, actor, "browser_confirmed", nil)
	return nil
}

// RetryBrowserJob returns the job to the routed queue for re-presentation.
func (s *Service) RetryBrowserJob(ctx context.Context, jobID uint, clientID string, now time.Time, actor string) error {
	if err := s.queue.RetryBrowserJob(ctx, jobID, clientID, now); err != nil {
		return err
	}
	s.writePrintAudit(ctx, jobID, actor, "browser_retry", nil)
	return nil
}

// FailBrowserJob permanently fails a leased browser job (Cancel from the
// confirmation panel).
func (s *Service) FailBrowserJob(ctx context.Context, jobID uint, clientID string, now time.Time, actor, reason string) error {
	if err := s.queue.FailBrowserJob(ctx, jobID, clientID, now, reason); err != nil {
		return err
	}
	s.writePrintAudit(ctx, jobID, actor, "browser_failed", map[string]interface{}{"reason": reason})
	return nil
}

// ReclaimExpiredBrowserLeases recovers jobs whose browser agent died mid-lease.
func (s *Service) ReclaimExpiredBrowserLeases(ctx context.Context, now time.Time) (int64, error) {
	return s.queue.ReclaimExpiredBrowserLeases(ctx, now)
}

// writePrintAudit best-effort records an operator-visible audit row. Failures
// are swallowed so audit never blocks the print state machine.
func (s *Service) writePrintAudit(ctx context.Context, jobID uint, actor, action string, meta map[string]interface{}) {
	if s.db == nil {
		return
	}
	job, err := s.queue.Get(ctx, jobID)
	if err != nil {
		return
	}
	if actor == "" {
		actor = "system"
	}
	entry := database.PrintAuditLog{
		BusinessID: job.BusinessID,
		Actor:      actor,
		Action:     action,
		PrintJobID: &jobID,
		PrinterID:  job.PrinterID,
	}
	if meta != nil {
		if raw, mErr := json.Marshal(meta); mErr == nil {
			entry.Metadata = database.JSONRawMessage(raw)
		}
	}
	_ = s.db.WithContext(ctx).Create(&entry).Error
}

// BusinessPrintLanguage resolves the print language for auto-enqueued jobs
// (auto-receipt on paid, kitchen tickets on approve, printer test pages): the
// business's default_language column normalized onto the label-bundle tier.
// Fallback chain: business default language → "en" when the business row is
// missing, the column is empty, or db is unavailable.
func BusinessPrintLanguage(db *gorm.DB, businessID uint) string {
	if db == nil || businessID == 0 {
		return "en"
	}
	var defaultLanguage string
	if err := db.Model(&database.Business{}).
		Where("id = ?", businessID).
		Pluck("default_language", &defaultLanguage).Error; err != nil {
		return "en"
	}
	return formatters.CanonicalPrintLanguage(defaultLanguage)
}
