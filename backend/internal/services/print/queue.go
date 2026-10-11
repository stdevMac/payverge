package print

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
)

// Queue wraps the database layer for print job persistence.
type Queue struct {
	db *gorm.DB
}

// NewQueue creates a Queue backed by db.
func NewQueue(db *gorm.DB) *Queue { return &Queue{db: db} }

// Get retrieves a PrintJob by primary key.
func (q *Queue) Get(ctx context.Context, id uint) (*database.PrintJob, error) {
	var j database.PrintJob
	if err := q.db.WithContext(ctx).First(&j, id).Error; err != nil {
		return nil, err
	}
	return &j, nil
}

// ErrIllegalTransition is returned by TransitionGuarded when the job is not in
// one of the allowed source states (e.g. marking an already-cancelled job
// printed, or cancelling a job that already printed).
var ErrIllegalTransition = errors.New("print job not in an allowed source state for this transition")

// TransitionGuarded updates a job's status only when its current status is in
// allowedFrom (nil = any source, preserving the legacy unconditional behavior).
// The source check is folded into the UPDATE's WHERE clause so it is atomic:
// a concurrent double-fire or a late callback after cancel cannot flip a
// terminal job into an incoherent state. Returns ErrIllegalTransition when the
// row exists but is not in an allowed source state, and a not-found error when
// no row matches the id at all.
func (q *Queue) TransitionGuarded(ctx context.Context, id uint, want database.PrintJobStatus, allowedFrom []database.PrintJobStatus, lastError *string) error {
	updates := map[string]interface{}{
		"status":     want,
		"updated_at": time.Now(),
	}
	if want == database.PrintJobStatusPrinted {
		now := time.Now()
		updates["printed_at"] = &now
	}
	if lastError != nil {
		updates["last_error"] = *lastError
	}
	query := q.db.WithContext(ctx).Model(&database.PrintJob{}).Where("id = ?", id)
	if len(allowedFrom) > 0 {
		query = query.Where("status IN ?", allowedFrom)
	}
	res := query.Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// Distinguish "no such job" from "job exists but wrong source state" so
		// callers can map the latter to a 409 rather than a 404.
		if len(allowedFrom) > 0 {
			var exists int64
			if err := q.db.WithContext(ctx).Model(&database.PrintJob{}).Where("id = ?", id).Count(&exists).Error; err != nil {
				return err
			}
			if exists > 0 {
				return ErrIllegalTransition
			}
		}
		return fmt.Errorf("print job %d not found", id)
	}
	return nil
}

// SetPrinterAndPayload assigns a printer and HTML payload to a job and
// transitions it to the Routed status in a single UPDATE.
func (q *Queue) SetPrinterAndPayload(ctx context.Context, id uint, printerID uint, html string) error {
	res := q.db.WithContext(ctx).Model(&database.PrintJob{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"printer_id":   printerID,
			"payload_html": html,
			"status":       database.PrintJobStatusRouted,
			"updated_at":   time.Now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("print job not found for routing")
	}
	return nil
}

// ScheduleRetry bumps the attempt counter, records the failure, and schedules
// the next attempt at the given time. Used by PrintRetryWorker.
func (q *Queue) ScheduleRetry(ctx context.Context, id uint, attemptCount int, nextAttemptAt time.Time, lastError string) error {
	now := time.Now()
	res := q.db.WithContext(ctx).Model(&database.PrintJob{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":          database.PrintJobStatusFailedRetryable,
			"attempt_count":   attemptCount,
			"retries":         attemptCount, // keep legacy column in sync
			"next_attempt_at": nextAttemptAt,
			"last_error":      lastError,
			"updated_at":      now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("print job %d not found", id)
	}
	return nil
}

// MarkPermanentFailure transitions a job to failed_permanent with the
// supplied error message. Used after MaxAttempts is exhausted.
func (q *Queue) MarkPermanentFailure(ctx context.Context, id uint, attemptCount int, lastError string) error {
	res := q.db.WithContext(ctx).Model(&database.PrintJob{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        database.PrintJobStatusFailedPermanent,
			"attempt_count": attemptCount,
			"retries":       attemptCount,
			"last_error":    lastError,
			"updated_at":    time.Now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("print job %d not found", id)
	}
	return nil
}

// ClaimRetryable atomically claims up to `limit` retryable jobs whose
// next_attempt_at has elapsed. Each row is locked with FOR UPDATE SKIP
// LOCKED (Postgres) so multiple worker replicas never double-send.
//
// SQLite (used in tests) does not support SKIP LOCKED — the queue falls
// back to a vanilla SELECT in that case. Worker-level mutex serialises
// access in unit tests.
func (q *Queue) ClaimRetryable(ctx context.Context, now time.Time, limit int) ([]database.PrintJob, error) {
	var jobs []database.PrintJob

	statuses := []database.PrintJobStatus{
		database.PrintJobStatusPending,
		database.PrintJobStatusRouted,
		database.PrintJobStatusFailedRetryable,
	}

	err := q.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Never claim jobs routed to a browser printer. Browser printing is
		// pull-based: the frontend fetches routed jobs and reports completion via
		// the mark-printed/cancel callbacks, so a browser job must only leave
		// "routed" through those callbacks. If the worker claimed it, it would
		// churn the job through printing->failed_retryable->failed_permanent
		// while the tab was merely briefly closed, losing the kitchen ticket.
		// Jobs with no printer yet (NULL printer_id, pre-routing) stay claimable
		// so genuinely-orphaned pending rows are still bounded by max_attempts.
		browserPrinterIDs := tx.Model(&database.Printer{}).
			Select("id").
			Where("transport = ?", "browser")
		query := tx.Model(&database.PrintJob{}).
			Where("status IN ?", statuses).
			Where("(printer_id IS NULL OR printer_id NOT IN (?))", browserPrinterIDs).
			Where("(next_attempt_at IS NULL OR next_attempt_at <= ?)", now).
			Where("attempt_count < max_attempts").
			Order("printer_id NULLS FIRST, id ASC").
			Limit(limit)

		// Best-effort row-level lock for Postgres. SQLite ignores the clause
		// in tests and the worker mutex handles serialisation there.
		if isPostgres(tx) {
			query = query.Clauses(clause.Locking{
				Strength: "UPDATE",
				Options:  "SKIP LOCKED",
			})
		}

		if err := query.Find(&jobs).Error; err != nil {
			return err
		}
		if len(jobs) == 0 {
			return nil
		}

		ids := make([]uint, len(jobs))
		for i := range jobs {
			ids[i] = jobs[i].ID
			jobs[i].Status = database.PrintJobStatusPrinting
		}

		// Bump status to printing inside the lock so a second sweep skips them.
		return tx.Model(&database.PrintJob{}).
			Where("id IN ?", ids).
			Updates(map[string]interface{}{
				"status":     database.PrintJobStatusPrinting,
				"updated_at": time.Now(),
			}).Error
	})

	if err != nil {
		return nil, err
	}
	return jobs, nil
}

// ReclaimStalePrinting resets jobs stuck in "printing" past staleBefore back to
// a retryable state. A worker claims a job (status -> printing) and then sends;
// if it crashes or is restarted (deploy) before recording success/failure, the
// job would otherwise be orphaned forever — ClaimRetryable never re-claims
// "printing", and the orphan sweep skips orders that already have a job row, so
// the kitchen ticket is silently lost.
//
// Browser-leased jobs are excluded: they use ReclaimExpiredBrowserLeases so an
// open print dialog (status=printing, claimed_by set) is not force-failed while
// the operator is still confirming.
//
// staleBefore MUST be older than any legitimate send so an actively-printing job
// is never reclaimed mid-send; a reclaim can cause a re-print, which for kitchen
// tickets is preferable to losing one. attempt_count is incremented so a job
// that repeatedly orphans the worker is still bounded by max_attempts rather than
// looping forever. Returns the number of jobs reclaimed.
func (q *Queue) ReclaimStalePrinting(ctx context.Context, staleBefore time.Time) (int64, error) {
	now := time.Now()
	browserPrinterIDs := q.db.Model(&database.Printer{}).
		Select("id").
		Where("transport = ?", "browser")
	res := q.db.WithContext(ctx).Model(&database.PrintJob{}).
		Where("status = ? AND updated_at < ?", database.PrintJobStatusPrinting, staleBefore).
		Where("(printer_id IS NULL OR printer_id NOT IN (?))", browserPrinterIDs).
		Where("claimed_by IS NULL").
		Updates(map[string]interface{}{
			"status":          database.PrintJobStatusFailedRetryable,
			"attempt_count":   gorm.Expr("attempt_count + 1"),
			"retries":         gorm.Expr("attempt_count + 1"), // keep legacy column in sync
			"next_attempt_at": now,
			"last_error":      "reclaimed: worker presumed crashed mid-print",
			"updated_at":      now,
		})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// MarkAttemptSucceeded transitions a printing job back to Printed (terminal
// success) when the transport reports the print completed.
func (q *Queue) MarkAttemptSucceeded(ctx context.Context, id uint) error {
	now := time.Now()
	res := q.db.WithContext(ctx).Model(&database.PrintJob{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     database.PrintJobStatusPrinted,
			"printed_at": &now,
			"last_error": nil,
			"updated_at": now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("print job %d not found", id)
	}
	return nil
}

// isPostgres returns true when the underlying gorm dialect is Postgres.
// Used to opt into SKIP LOCKED only on the production driver.
func isPostgres(tx *gorm.DB) bool {
	return tx.Dialector != nil && tx.Dialector.Name() == "postgres"
}

// ErrNotLeaseOwner is returned when a browser-lease mutation is attempted by a
// client that does not currently hold the job's lease.
var ErrNotLeaseOwner = errors.New("print job lease not owned by this client")

// ErrNoBrowserJob is returned when a claim finds no claimable browser job
// (empty queue). Callers map this to a 204/empty response rather than an error.
var ErrNoBrowserJob = errors.New("no claimable browser print job")

// ErrInvalidBrowserPrinter means the requested station is not an enabled
// browser printer owned by the business. Callers should treat it as not found.
var ErrInvalidBrowserPrinter = errors.New("invalid browser printer")

// ClaimBrowserJob atomically leases the oldest claimable browser-transport job
// for businessID to clientID. Uses FOR UPDATE SKIP LOCKED on Postgres so two
// operator tabs cannot claim the same job. Constraints:
//   - printer.transport = 'browser'
//   - printer.enabled = true
//   - status = routed (expired leases are reclaimed to routed first)
//
// Returns (nil, nil) when the queue is empty for this business.
func (q *Queue) ClaimBrowserJob(ctx context.Context, businessID uint, clientID string, now time.Time, leaseDur time.Duration) (*database.PrintJob, error) {
	return q.claimBrowserJob(ctx, businessID, 0, nil, clientID, now, leaseDur)
}

// ClaimBrowserJobForPrinter scopes a browser agent to one explicitly selected
// physical station. It can never consume work routed to another printer.
func (q *Queue) ClaimBrowserJobForPrinter(ctx context.Context, businessID, printerID uint, clientID string, now time.Time, leaseDur time.Duration) (*database.PrintJob, error) {
	if printerID == 0 {
		return nil, ErrInvalidBrowserPrinter
	}
	return q.claimBrowserJob(ctx, businessID, printerID, nil, clientID, now, leaseDur)
}

// ClaimAuthorizedBrowserJobForPrinter applies both physical-station and job-kind
// scope. It is the only claim path exposed by the HTTP browser agent.
func (q *Queue) ClaimAuthorizedBrowserJobForPrinter(ctx context.Context, businessID, printerID uint, allowedKinds []database.PrintJobKind, clientID string, now time.Time, leaseDur time.Duration) (*database.PrintJob, error) {
	if printerID == 0 {
		return nil, ErrInvalidBrowserPrinter
	}
	if len(allowedKinds) == 0 {
		return nil, errors.New("at least one authorized print job kind is required")
	}
	return q.claimBrowserJob(ctx, businessID, printerID, allowedKinds, clientID, now, leaseDur)
}

func (q *Queue) claimBrowserJob(ctx context.Context, businessID, printerID uint, allowedKinds []database.PrintJobKind, clientID string, now time.Time, leaseDur time.Duration) (*database.PrintJob, error) {
	if businessID == 0 || clientID == "" {
		return nil, errors.New("business_id and client_id are required")
	}
	if leaseDur <= 0 {
		leaseDur = 30 * time.Second
	}
	// Opportunistically reclaim expired leases so a crashed tab does not block
	// the queue until a separate sweep runs.
	if _, err := q.ReclaimExpiredBrowserLeases(ctx, now); err != nil {
		return nil, err
	}

	var claimed *database.PrintJob
	err := q.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&database.PrintJob{}).
			Where("business_id = ?", businessID).
			Where("status = ?", database.PrintJobStatusRouted).
			Order("created_at ASC, id ASC").
			Limit(1)
		if len(allowedKinds) > 0 {
			query = query.Where("kind IN ?", allowedKinds)
		}
		if printerID > 0 {
			var printer database.Printer
			if err := tx.Select("id").
				Where("id = ? AND business_id = ? AND transport = ? AND enabled = ?", printerID, businessID, "browser", true).
				First(&printer).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrInvalidBrowserPrinter
				}
				return err
			}
			query = query.Where("printer_id = ?", printerID)
		} else {
			enabledBrowserIDs := tx.Model(&database.Printer{}).
				Select("id").
				Where("business_id = ? AND transport = ? AND enabled = ?", businessID, "browser", true)
			query = query.Where("printer_id IN (?)", enabledBrowserIDs)
		}

		if isPostgres(tx) {
			query = query.Clauses(clause.Locking{
				Strength: "UPDATE",
				Options:  "SKIP LOCKED",
			})
		}

		var job database.PrintJob
		if err := query.First(&job).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		expires := now.Add(leaseDur)
		res := tx.Model(&database.PrintJob{}).
			Where("id = ? AND status = ?", job.ID, database.PrintJobStatusRouted).
			Updates(map[string]interface{}{
				"status":           database.PrintJobStatusPrinting,
				"claimed_by":       clientID,
				"lease_expires_at": expires,
				"presented_at":     nil,
				"updated_at":       now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// Lost a race (SQLite path without SKIP LOCKED); treat as empty.
			return nil
		}

		if err := tx.First(&job, job.ID).Error; err != nil {
			return err
		}
		claimed = &job
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

// RenewBrowserLease extends lease_expires_at for the job owned by clientID.
func (q *Queue) RenewBrowserLease(ctx context.Context, jobID uint, clientID string, now time.Time, leaseDur time.Duration) error {
	if leaseDur <= 0 {
		leaseDur = 30 * time.Second
	}
	expires := now.Add(leaseDur)
	res := q.db.WithContext(ctx).Model(&database.PrintJob{}).
		Where("id = ? AND claimed_by = ? AND status = ?", jobID, clientID, database.PrintJobStatusPrinting).
		Updates(map[string]interface{}{
			"lease_expires_at": expires,
			"updated_at":       now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return q.browserLeaseMutationError(ctx, jobID, clientID)
	}
	return nil
}

// MarkPresented records that the browser print dialog was shown / window.print()
// was invoked. Does NOT transition to printed — that requires ConfirmPrinted.
func (q *Queue) MarkPresented(ctx context.Context, jobID uint, clientID string, now time.Time) error {
	res := q.db.WithContext(ctx).Model(&database.PrintJob{}).
		Where("id = ? AND claimed_by = ? AND status = ?", jobID, clientID, database.PrintJobStatusPrinting).
		Updates(map[string]interface{}{
			"presented_at": now,
			"updated_at":   now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return q.browserLeaseMutationError(ctx, jobID, clientID)
	}
	return nil
}

// ConfirmPrinted is the explicit operator confirmation that the physical print
// succeeded. Distinct from MarkPresented (dialog shown).
func (q *Queue) ConfirmPrinted(ctx context.Context, jobID uint, clientID string, now time.Time) error {
	res := q.db.WithContext(ctx).Model(&database.PrintJob{}).
		Where("id = ? AND claimed_by = ? AND status = ?", jobID, clientID, database.PrintJobStatusPrinting).
		Updates(map[string]interface{}{
			"status":           database.PrintJobStatusPrinted,
			"printed_at":       now,
			"last_error":       nil,
			"lease_expires_at": nil,
			"updated_at":       now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return q.browserLeaseMutationError(ctx, jobID, clientID)
	}
	return nil
}

// RetryBrowserJob returns a leased job to routed so another (or the same)
// browser agent can re-present it. Clears lease ownership and presented_at.
func (q *Queue) RetryBrowserJob(ctx context.Context, jobID uint, clientID string, now time.Time) error {
	res := q.db.WithContext(ctx).Model(&database.PrintJob{}).
		Where("id = ? AND claimed_by = ? AND status = ?", jobID, clientID, database.PrintJobStatusPrinting).
		Updates(map[string]interface{}{
			"status":           database.PrintJobStatusRouted,
			"claimed_by":       nil,
			"lease_expires_at": nil,
			"presented_at":     nil,
			"attempt_count":    gorm.Expr("attempt_count + 1"),
			"retries":          gorm.Expr("attempt_count + 1"),
			"last_error":       "browser agent retry requested",
			"updated_at":       now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return q.browserLeaseMutationError(ctx, jobID, clientID)
	}
	return nil
}

// FailBrowserJob marks a leased browser job permanently failed (operator
// cancel from the confirmation panel, or unrecoverable station error).
func (q *Queue) FailBrowserJob(ctx context.Context, jobID uint, clientID string, now time.Time, reason string) error {
	if reason == "" {
		reason = "browser agent failed job"
	}
	res := q.db.WithContext(ctx).Model(&database.PrintJob{}).
		Where("id = ? AND claimed_by = ? AND status = ?", jobID, clientID, database.PrintJobStatusPrinting).
		Updates(map[string]interface{}{
			"status":           database.PrintJobStatusFailedPermanent,
			"claimed_by":       nil,
			"lease_expires_at": nil,
			"last_error":       reason,
			"updated_at":       now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return q.browserLeaseMutationError(ctx, jobID, clientID)
	}
	return nil
}

// ReclaimExpiredBrowserLeases returns printing browser jobs whose lease has
// expired back to routed so another tab can claim them. Clears claim fields.
// Returns the number of jobs reclaimed.
func (q *Queue) ReclaimExpiredBrowserLeases(ctx context.Context, now time.Time) (int64, error) {
	// Only reclaim rows that are on browser printers (or already have a
	// claimed_by set from a browser agent). Network/cloudprnt printing uses
	// ReclaimStalePrinting instead.
	browserPrinterIDs := q.db.Model(&database.Printer{}).
		Select("id").
		Where("transport = ?", "browser")

	res := q.db.WithContext(ctx).Model(&database.PrintJob{}).
		Where("status = ?", database.PrintJobStatusPrinting).
		Where("claimed_by IS NOT NULL").
		Where("lease_expires_at IS NOT NULL AND lease_expires_at < ?", now).
		Where("printer_id IN (?)", browserPrinterIDs).
		Updates(map[string]interface{}{
			"status":           database.PrintJobStatusRouted,
			"claimed_by":       nil,
			"lease_expires_at": nil,
			"presented_at":     nil,
			"last_error":       "reclaimed: browser lease expired",
			"updated_at":       now,
		})
	if res.Error != nil {
		return 0, res.Error
	}
	if res.RowsAffected > 0 {
		metrics.PrintBrowserExpiredLeases.Add(float64(res.RowsAffected))
	}
	return res.RowsAffected, nil
}

// browserLeaseMutationError distinguishes "wrong owner / wrong state" from
// "job does not exist".
func (q *Queue) browserLeaseMutationError(ctx context.Context, jobID uint, clientID string) error {
	var job database.PrintJob
	if err := q.db.WithContext(ctx).Select("id", "claimed_by", "status").First(&job, jobID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("print job %d not found", jobID)
		}
		return err
	}
	if job.ClaimedBy == nil || *job.ClaimedBy != clientID {
		return ErrNotLeaseOwner
	}
	return ErrIllegalTransition
}

// DefaultBrowserAbandonAfter is how long a browser-routed job may wait for a
// station to claim it before the queue gives up on it.
//
// #833: browser printing is pull-based, so ClaimRetryable deliberately skips
// jobs routed to a browser printer — a tab that is merely closed over lunch
// must not burn the job's attempts. The cost of that exemption is that a job
// nobody ever claims has no exit at all: live venue 2 carried four `routed`
// rows aged 67h / 133h / 133h / 813h with attempt_count 0 and both printed_at
// and failed_at null, and every surface that groups by status filed them under
// "queued" — a print that never happened, reported as work still in progress.
//
// Six hours is well past any plausible service gap (the station-offline alert
// fires at 3 minutes and the operator list flags a stalled job at 10), so a job
// this old did not print and will not: printing it now would push a ticket from
// a previous service onto the pass. Marking it failed is the honest end state,
// and it is the one that lets the operator reprint a bill or receipt.
const DefaultBrowserAbandonAfter = 6 * time.Hour

// AbandonStaleBrowserRouted marks browser-routed jobs that have been waiting
// since before routedBefore as permanently failed.
//
// Both created_at and updated_at must predate the cutoff: a job returned to
// routed by a lease reclaim or an operator retry has a fresh updated_at and
// deserves the full window again from that moment. Returns the number of jobs
// abandoned.
func (q *Queue) AbandonStaleBrowserRouted(ctx context.Context, routedBefore time.Time) (int64, error) {
	now := time.Now()
	browserPrinterIDs := q.db.Model(&database.Printer{}).
		Select("id").
		Where("transport = ?", "browser")

	res := q.db.WithContext(ctx).Model(&database.PrintJob{}).
		Where("status = ?", database.PrintJobStatusRouted).
		Where("printer_id IN (?)", browserPrinterIDs).
		Where("created_at < ? AND updated_at < ?", routedBefore, routedBefore).
		Updates(map[string]interface{}{
			"status":           database.PrintJobStatusFailedPermanent,
			"claimed_by":       nil,
			"lease_expires_at": nil,
			"last_error":       "abandoned: no browser print station claimed this job",
			"updated_at":       now,
		})
	if res.Error != nil {
		return 0, res.Error
	}
	if res.RowsAffected > 0 {
		metrics.PrintBrowserAbandoned.Add(float64(res.RowsAffected))
	}
	return res.RowsAffected, nil
}
