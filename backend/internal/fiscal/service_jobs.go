package fiscal

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/metrics"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// JobOutcome reports the result of processing a single claimed fiscal job. It is
// deliberately status-shaped (not error-shaped) so the worker can decide on
// backoff vs terminal transition without re-interpreting provider internals.
type JobOutcome struct {
	// TerminalStatus, when set, is the final status the job should take with no
	// further retries (authorized, failed_permanent, rejected, credited, …).
	TerminalStatus database.FiscalStatus
	// Retryable is true when the failure is transient and the job should be
	// rescheduled with backoff (subject to MaxAttempts).
	Retryable bool
	// ReceiptID is the receipt row created/updated by this job, if any.
	ReceiptID *uint
	// ErrorCode / ErrorMessage describe the failure for audit + last_error_* on
	// the job. Empty on success.
	ErrorCode    string
	ErrorMessage string
}

// ProcessClaimedJob loads the job's bill/settings/business, resolves the
// per-business provider via the factory, builds the provider input, dispatches
// to the right provider method, and persists the resulting receipt. It returns a
// status-shaped JobOutcome; the worker applies the backoff/terminal transition
// and writes the audit event.
//
// ProcessClaimedJob never mutates the job's status/lock itself — that is the
// worker's job — so a single claim path owns the lifecycle.
func (s *Service) ProcessClaimedJob(ctx context.Context, job database.FiscalJob) (*JobOutcome, error) {
	jobCtx, err := s.repo.LoadJobContext(job)
	if err != nil {
		// A missing bill/settings/business is a permanent, non-retryable data
		// error: nothing the worker can do by retrying.
		return &JobOutcome{
			TerminalStatus: database.FiscalStatusFailedPermanent,
			ErrorCode:      "context_load_failed",
			ErrorMessage:   err.Error(),
		}, nil
	}

	// Short-circuit expired credentials before attempting any SOAP/provider call.
	// An expired certificate will certainly fail WSAA authentication, so retrying
	// only burns the job's retry budget. Mark the job failed_permanent so the
	// operator is clearly prompted to renew and re-upload the certificate bundle.
	if exp := jobCtx.Settings.CredentialsExpiresAt; exp != nil && time.Now().After(*exp) {
		return &JobOutcome{
			TerminalStatus: database.FiscalStatusFailedPermanent,
			ErrorCode:      "credentials_expired",
			ErrorMessage:   fmt.Sprintf("fiscal credentials expired at %s", exp.Format(time.RFC3339)),
		}, nil
	}

	provider, err := s.resolveProvider(ctx, &jobCtx.Settings)
	if err != nil {
		// Provider construction (credential decrypt/parse, unsupported combo) can
		// be transient (e.g. a not-yet-rotated cred) — treat as retryable by
		// default. Deterministic build failures (bad ciphertext, missing PEM, unknown
		// country/provider) wrap ErrPermanent and are classified terminal so a
		// mis-decryptable credential stops burning the 5x retry budget on every job.
		if errors.Is(err, ErrPermanent) {
			return &JobOutcome{
				TerminalStatus: database.FiscalStatusFailedPermanent,
				ErrorCode:      "provider_build_permanent",
				ErrorMessage:   err.Error(),
			}, nil
		}
		return &JobOutcome{
			Retryable:    true,
			ErrorCode:    "provider_unavailable",
			ErrorMessage: err.Error(),
		}, nil
	}

	switch job.Action {
	case ActionIssueReceipt:
		return s.processIssueJob(ctx, job, jobCtx, provider)
	case ActionCreditNote:
		return s.processCreditNoteJob(ctx, job, jobCtx, provider)
	case ActionStatusCheck:
		return s.processStatusCheckJob(ctx, job, jobCtx, provider)
	default:
		return &JobOutcome{
			TerminalStatus: database.FiscalStatusFailedPermanent,
			ErrorCode:      "unsupported_action",
			ErrorMessage:   fmt.Sprintf("unsupported fiscal job action %q", job.Action),
		}, nil
	}
}

func (s *Service) processIssueJob(ctx context.Context, job database.FiscalJob, jobCtx *JobContext, provider Provider) (*JobOutcome, error) {
	// Idempotency guard against a crash BETWEEN receipt-persist and job-mark: a
	// prior run of this job can commit the authorized receipt (+ job.receipt_id)
	// yet die before the worker writes status=authorized, leaving the job
	// re-claimable while a real CAE already exists at AFIP. Re-issuing would
	// request a NEW receipt number and emit a DUPLICATE legal invoice. A bill has
	// at most one authorized issue receipt (enqueue idempotency collapses to one
	// issue job per bill), so if one already exists, adopt it instead of calling
	// the provider again. Delivery below is a no-op via its DeliveredAt guard.
	if adopted := s.adoptExistingAuthorizedIssueReceipt(ctx, job.BillID); adopted != nil {
		return adopted, nil
	}

	// M-545: the guest fiscal identity route is reachable by any bill token
	// holder. If the guest session that set the receptor has no confirmed
	// payment on the bill (others paid, or staff settled it), the identity is
	// a squat — drop it and issue to consumidor final rather than invoice the
	// payer's purchase to a stranger's CUIT/email.
	if outcome := s.dropUnpaidGuestFiscalIdentity(ctx, job.BillID, jobCtx); outcome != nil {
		return outcome, nil
	}

	input := s.buildIssueInput(job, jobCtx)
	input.AttemptedProviderReceiptID = attemptedProviderReceiptID(job)
	input.OnAttempt = func(id string) error { return s.repo.RecordJobAttempt(job.ID, jobLockOwner(job), id) }
	input.VoucherClaimed = func(id string) (bool, error) { return s.repo.ProviderReceiptClaimedByOtherJob(job, id) }
	input.LockSeries = s.lockFiscalSeries
	result, err := provider.IssueReceipt(ctx, input)
	// Delivery intent is enqueued inside SaveReceiptForJob's transaction when
	// the authorized receipt is persisted (Wave 4 durable tasks). The delivery
	// worker executes artifact/email/print with retries — no inline single-shot
	// delivery after authorization.
	return s.persistReceiptResult(job, jobCtx, ActionIssueReceipt, input.ReceiptType, nil, result, err)
}

// lockFiscalSeries serializes voucher-number allocation for one provider series
// across workers and replicas. On Postgres it takes a session-level advisory
// lock on a dedicated connection, so the lock is held across the AFIP network
// round-trips without an open transaction. Other dialects, and a nil db,
// return a no-op release.
func (s *Service) lockFiscalSeries(ctx context.Context, key string) (func(), error) {
	if s.db == nil || s.db.Dialector == nil || s.db.Dialector.Name() != "postgres" {
		return func() {}, nil
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return nil, err
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock(hashtext($1))", key); err != nil {
		// The lock may already have been granted when ctx cancels. Close()
		// returns this session to the pool, and an idle pooled connection
		// would keep the advisory lock and wedge the series. Mark it bad so
		// the pool discards it (closing the session releases the lock).
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		_ = conn.Close()
		return nil, err
	}
	release := func() {
		// The caller's ctx may already be cancelled when we unlock. Detach from
		// that cancellation and bound the unlock so a stuck session cannot
		// linger, then return the connection to the pool.
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, uerr := conn.ExecContext(unlockCtx, "SELECT pg_advisory_unlock(hashtext($1))", key); uerr != nil {
			// A connection whose unlock failed may still hold the session
			// lock. Mark it bad so the pool discards it (closing the session
			// releases the lock) instead of handing it to another caller.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		_ = conn.Close()
	}
	return release, nil
}

// dropUnpaidGuestFiscalIdentity enforces the guest fiscal identity payer
// binding at issuance (database.ClearUnpaidGuestFiscalIdentity) and mirrors a
// cleared identity onto the loaded job context. A database error is retryable:
// issuing with an unverified receptor is worse than issuing a little later.
func (s *Service) dropUnpaidGuestFiscalIdentity(ctx context.Context, billID uint, jobCtx *JobContext) *JobOutcome {
	if jobCtx == nil || jobCtx.Bill.FiscalCustomerGuestSession == nil {
		return nil
	}
	cleared, err := database.ClearUnpaidGuestFiscalIdentity(s.db.WithContext(ctx), billID)
	if err != nil {
		return &JobOutcome{
			Retryable:    true,
			ErrorCode:    "fiscal_identity_check_failed",
			ErrorMessage: err.Error(),
		}
	}
	if cleared {
		logger.Logger.Warnf("Fiscal issue: dropped guest-set receptor on bill %d (setter session did not pay)", billID)
		jobCtx.Bill.FiscalCustomerDocType = nil
		jobCtx.Bill.FiscalCustomerDocNumber = nil
		jobCtx.Bill.FiscalCustomerTaxCondition = nil
		jobCtx.Bill.FiscalCustomerName = nil
		jobCtx.Bill.FiscalCustomerEmail = nil
		jobCtx.Bill.FiscalCustomerGuestSession = nil
	}
	return nil
}

// adoptExistingAuthorizedIssueReceipt returns an authorized JobOutcome for the
// bill's existing authorized issue receipt, or nil if none exists. Used to make a
// re-claimed issue job idempotent on the AFIP side after a crash between
// receipt-persist and job-mark, so a bill can never receive two facturas.
func (s *Service) adoptExistingAuthorizedIssueReceipt(ctx context.Context, billID uint) *JobOutcome {
	var existing database.FiscalReceipt
	err := s.db.WithContext(ctx).
		Select("id").
		Where("bill_id = ? AND action = ? AND status = ?",
			billID, ActionIssueReceipt, database.FiscalStatusAuthorized).
		Order("id DESC").
		First(&existing).Error
	if err != nil {
		return nil
	}
	id := existing.ID
	return &JobOutcome{TerminalStatus: database.FiscalStatusAuthorized, ReceiptID: &id}
}

// enqueueDeliveryForAuthorizedReceipt ensures durable delivery tasks exist for
// an authorized issue receipt that was authorized outside SaveReceiptForJob
// (e.g. status_check reconcile backfill). Idempotent on (receipt_id, channel).
func (s *Service) enqueueDeliveryForAuthorizedReceipt(receiptID uint) {
	var receipt database.FiscalReceipt
	if err := s.db.Where("id = ?", receiptID).First(&receipt).Error; err != nil {
		return
	}
	if receipt.Status != database.FiscalStatusAuthorized || receipt.Action != ActionIssueReceipt {
		return
	}
	if err := s.repo.enqueueAuthorizedDeliveryTasksInTx(s.db, &receipt, s.now()); err != nil {
		logger.Logger.Warnf("Fiscal delivery: enqueue tasks for receipt %d failed: %v", receiptID, err)
	}
}

func (s *Service) processCreditNoteJob(ctx context.Context, job database.FiscalJob, jobCtx *JobContext, provider Provider) (*JobOutcome, error) {
	var original database.FiscalReceipt
	if job.ReceiptID != nil {
		if err := s.db.WithContext(ctx).Where("id = ?", *job.ReceiptID).First(&original).Error; err != nil {
			return &JobOutcome{
				TerminalStatus: database.FiscalStatusFailedPermanent,
				ErrorCode:      "original_receipt_not_found",
				ErrorMessage:   err.Error(),
			}, nil
		}
		// A previous run of this job may have authorized the credit note and
		// died before the worker marked the job. receipt_id stays the original
		// issue receipt; produced_receipt_id is the nota de crédito. Adopt that
		// receipt instead of requesting a second CAE.
		if job.ProducedReceiptID != nil {
			var produced database.FiscalReceipt
			err := s.db.WithContext(ctx).Where("id = ?", *job.ProducedReceiptID).First(&produced).Error
			if err != nil {
				return &JobOutcome{Retryable: true, ErrorCode: "produced_receipt_lookup_failed", ErrorMessage: err.Error()}, nil
			}
			if produced.Status == database.FiscalStatusAuthorized || produced.Status == database.FiscalStatusCredited {
				// The earlier run may have died before moving the original
				// to credited. Finish that step before going terminal.
				var adoptedAmount int64
				if job.CreditAmountCents != nil {
					adoptedAmount = *job.CreditAmountCents
				}
				if merr := s.markOriginalCreditedIfFull(original, adoptedAmount); merr != nil {
					return &JobOutcome{Retryable: true, ErrorCode: "mark_original_credited_failed", ErrorMessage: merr.Error()}, nil
				}
				id := produced.ID
				return &JobOutcome{TerminalStatus: database.FiscalStatusAuthorized, ReceiptID: &id}, nil
			}
		}
	} else {
		// Deferred credit note (Wave 4): the refund arrived before the issue
		// receipt authorized. Resolve the authorized issue receipt by bill on
		// each attempt; until it exists the job stays retryable so the
		// worker's backoff keeps polling instead of dropping the nota de
		// crédito (the pre-Wave-4 behavior was a silent skip in the handler).
		err := s.db.WithContext(ctx).
			Where("bill_id = ? AND business_id = ? AND action = ? AND status = ?",
				job.BillID, job.BusinessID, ActionIssueReceipt, database.FiscalStatusAuthorized).
			Order("created_at DESC, id DESC").
			First(&original).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &JobOutcome{
					Retryable:    true,
					ErrorCode:    "awaiting_issue_receipt",
					ErrorMessage: "issue receipt not authorized yet; deferring credit note",
				}, nil
			}
			return &JobOutcome{Retryable: true, ErrorCode: "issue_receipt_lookup_failed", ErrorMessage: err.Error()}, nil
		}
		// Over-credit guard for the deferred path (IssueCreditNote's in-tx
		// guard could not run at enqueue time — there was no receipt). Reject
		// terminally if this amount plus other still-live credits would
		// exceed the original total. The current job is excluded from the sum.
		requested := int64(0)
		if job.CreditAmountCents != nil {
			requested = *job.CreditAmountCents
		}
		if requested <= 0 {
			requested = original.TotalAmountCents
		}
		priorLive, sumErr := s.repo.SumLiveCreditCents(s.db, original, job.ID, "")
		if sumErr != nil {
			return &JobOutcome{Retryable: true, ErrorCode: "credit_sum_failed", ErrorMessage: sumErr.Error()}, nil
		}
		if priorLive+requested > original.TotalAmountCents {
			return &JobOutcome{
				TerminalStatus: database.FiscalStatusRejected,
				ErrorCode:      "credit_note_exceeds_receipt",
				ErrorMessage:   "deferred credit note would exceed the original receipt total",
			}, nil
		}
	}
	// Defensive: a credit note may only be issued against an issue receipt.
	// Crediting a credit note would emit a second nota de crédito for the same
	// sale (or a malformed comprobante asociado), so refuse terminally.
	if original.Action == ActionCreditNote {
		return &JobOutcome{
			TerminalStatus: database.FiscalStatusFailedPermanent,
			ErrorCode:      "credit_note_original_is_credit_note",
			ErrorMessage:   "a credit note cannot be issued against another credit note",
		}, nil
	}
	// A partial credit-note amount (set by IssueCreditNote for a partial refund)
	// rides on the job; nil means credit the full original receipt total.
	var amountCents int64
	if job.CreditAmountCents != nil {
		amountCents = *job.CreditAmountCents
	}
	result, err := provider.IssueCreditNote(ctx, CreditNoteInput{
		OriginalReceipt: original,
		// Settings carries the issuer CUIT + point of sale that the AR mapper
		// (MapCreditNoteToWSFE) requires; without it every real credit note
		// fails CUIT/PoS validation and churns the whole retry budget.
		Settings:                   settingsFromModel(jobCtx.Settings),
		IssuedAt:                   s.now(),
		IdempotencyKey:             job.IdempotencyKey,
		AmountCents:                amountCents,
		Bill:                       jobCtx.Bill,
		BusinessTaxRate:            jobCtx.Business.TaxRate,
		AttemptedProviderReceiptID: attemptedProviderReceiptID(job),
		OnAttempt:                  func(id string) error { return s.repo.RecordJobAttempt(job.ID, jobLockOwner(job), id) },
		VoucherClaimed:             func(id string) (bool, error) { return s.repo.ProviderReceiptClaimedByOtherJob(job, id) },
		LockSeries:                 s.lockFiscalSeries,
	})
	outcome, perr := s.persistReceiptResult(job, jobCtx, ActionCreditNote, original.ReceiptType, &original, result, err)
	// F-CREDITED: once a credit note authorizes, move the original issue receipt to
	// credited if the authorized credits now cover its full total. A failure
	// keeps the job retryable: the nota de crédito is already stored as
	// produced_receipt_id, so the retry adopts it and finishes this step.
	if perr == nil && outcome != nil && outcome.TerminalStatus == database.FiscalStatusAuthorized {
		if merr := s.markOriginalCreditedIfFull(original, amountCents); merr != nil {
			return &JobOutcome{Retryable: true, ErrorCode: "mark_original_credited_failed", ErrorMessage: merr.Error()}, nil
		}
	}
	return outcome, perr
}

// markOriginalCreditedIfFull CAS-transitions the original issue receipt to
// credited when the sum of already-authorized credit notes PLUS this one reaches
// the original total. The current credit-note job's status is still pending here
// (the worker writes its terminal status after this returns), so its amount is
// added explicitly rather than read back from the DB.
func (s *Service) markOriginalCreditedIfFull(original database.FiscalReceipt, thisAmountCents int64) error {
	thisCredit := thisAmountCents
	if thisCredit <= 0 {
		thisCredit = original.TotalAmountCents // 0/nil == full credit
	}
	priorAuthorized, err := s.repo.SumAuthorizedCreditCents(original.ID, original.TotalAmountCents)
	if err != nil {
		logger.Logger.Errorf("fiscal: summing authorized credits for receipt %d failed: %v", original.ID, err)
		return fmt.Errorf("sum authorized credits for receipt %d: %w", original.ID, err)
	}
	if priorAuthorized+thisCredit >= original.TotalAmountCents {
		if _, err := s.repo.MarkReceiptCreditedIfAuthorized(original.ID, s.now()); err != nil {
			logger.Logger.Errorf("fiscal: marking receipt %d credited failed: %v", original.ID, err)
			return fmt.Errorf("mark receipt %d credited: %w", original.ID, err)
		}
	}
	return nil
}

func (s *Service) processStatusCheckJob(ctx context.Context, job database.FiscalJob, jobCtx *JobContext, provider Provider) (*JobOutcome, error) {
	providerReceiptID := ""
	if job.ReceiptID != nil {
		var receipt database.FiscalReceipt
		if err := s.db.WithContext(ctx).
			Select("id", "provider_receipt_id").
			Where("id = ?", *job.ReceiptID).First(&receipt).Error; err == nil && receipt.ProviderReceiptID != nil {
			providerReceiptID = *receipt.ProviderReceiptID
		}
	}
	status, err := provider.GetStatus(ctx, providerReceiptID)
	if err != nil {
		return classifyProviderError(err), nil
	}
	if status == nil {
		return &JobOutcome{Retryable: true, ErrorCode: "empty_status", ErrorMessage: "provider returned no status"}, nil
	}
	outcome := outcomeForStatus(status.Status, status.ProviderErrors, job.ReceiptID)

	// Reconcile the underlying receipt row to the resolved terminal state. Only a
	// NON-retryable outcome (authorized/credited or rejected/failed_permanent) is
	// terminal; a retryable/not-found result is left for the next sweep so the
	// receipt is not prematurely mutated. The job transition is still driven by the
	// returned outcome.
	if !outcome.Retryable && job.ReceiptID != nil && outcome.TerminalStatus != "" {
		if outcome.TerminalStatus == database.FiscalStatusAuthorized {
			// D-3: a reconcile that resolves to authorized backfills the FULL
			// authorization tuple (CAE, receipt number, CAE expiry, QR) — not just
			// the status — so a reconciled-authorized receipt can render a compliant
			// PDF. Empty fields are not written, so a status-only confirmation never
			// clobbers existing columns.
			now := s.now()
			fields := BackfillAuthorizedReceiptFields{
				AuthCode:      status.AuthCode,
				ReceiptNumber: status.ReceiptNumber,
				AuthExpiresAt: status.AuthExpiresAt,
				QRPayload:     status.QRPayload,
				IssuedAt:      &now,
			}
			if recErr := s.repo.BackfillAuthorizedReceipt(*job.ReceiptID, fields, now); recErr != nil {
				return &JobOutcome{Retryable: true, ErrorCode: "receipt_reconcile_failed", ErrorMessage: recErr.Error()}, nil
			}
			// Enqueue durable delivery tasks for the newly-reconciled authorized
			// receipt (idempotent). Worker executes delivery — no inline single-shot.
			s.enqueueDeliveryForAuthorizedReceipt(*job.ReceiptID)
		} else if recErr := s.repo.UpdateReceiptStatus(*job.ReceiptID, outcome.TerminalStatus, s.now()); recErr != nil {
			// A reconcile-write failure must not lose the resolved state: ask the
			// worker to retry the status check so the receipt is reconciled next sweep.
			return &JobOutcome{Retryable: true, ErrorCode: "receipt_reconcile_failed", ErrorMessage: recErr.Error()}, nil
		}
	}
	return outcome, nil
}

// persistReceiptResult classifies the provider's (result, err) pair and persists
// a FiscalReceipt row reflecting the outcome (authorized or failed_permanent).
// Transient errors persist no receipt and ask the worker to back off.
func (s *Service) persistReceiptResult(
	job database.FiscalJob,
	jobCtx *JobContext,
	action string,
	receiptType string,
	original *database.FiscalReceipt, // credit notes snapshot receptor data from here; nil for issue jobs
	result *ReceiptResult,
	err error,
) (*JobOutcome, error) {
	if err != nil {
		return classifyProviderError(err), nil
	}
	if result == nil {
		return &JobOutcome{Retryable: true, ErrorCode: "empty_result", ErrorMessage: "provider returned no result"}, nil
	}

	var originalID *uint
	if original != nil {
		originalID = &original.ID
	}

	switch result.Status {
	case database.FiscalStatusAuthorized:
		receipt := s.buildAuthorizedReceipt(job, jobCtx, action, receiptType, original, result)
		id, saveErr := s.saveAuthorizedReceiptWithRetry(receipt, job.ID, originalID)
		if saveErr != nil {
			// The inline save retries are exhausted after AFIP authorized.
			// When the provider recorded a numbered attempt, OnAttempt
			// persisted that voucher id before RequestCAE, so a retry
			// consults the same number and adopts it. Rescheduling does not
			// mint a second factura. With no recorded number, a retry would
			// allocate LastAuthorized+1 and duplicate the legal invoice, so
			// that case stays terminal for manual reconciliation.
			metrics.FiscalOrphanedCAETotal.Inc()
			if result.ProviderReceiptID != "" {
				logger.Logger.WithFields(logrus.Fields{
					"event":          "persist_failed_retryable",
					"job_id":         job.ID,
					"cae":            result.AuthCode,
					"receipt_number": result.ReceiptNumber,
					"attempts":       maxReceiptSaveAttempts,
				}).Errorf("authorized receipt could not be saved after %d attempts; retry will reconcile the attempted voucher: %v",
					maxReceiptSaveAttempts, saveErr)
				return &JobOutcome{
					Retryable:    true,
					ErrorCode:    "receipt_persist_failed",
					ErrorMessage: saveErr.Error(),
				}, nil
			}
			logger.Logger.WithFields(logrus.Fields{
				"event":          "persist_failed_terminal",
				"job_id":         job.ID,
				"cae":            result.AuthCode,
				"receipt_number": result.ReceiptNumber,
				"attempts":       maxReceiptSaveAttempts,
			}).Errorf("orphaned CAE: authorized receipt could not be saved after %d attempts — manual reconciliation required: %v",
				maxReceiptSaveAttempts, saveErr)
			return &JobOutcome{
				TerminalStatus: database.FiscalStatusFailedPermanent,
				ErrorCode:      "persist_failed_terminal",
				ErrorMessage:   saveErr.Error(),
			}, nil
		}
		return &JobOutcome{TerminalStatus: database.FiscalStatusAuthorized, ReceiptID: &id}, nil

	case database.FiscalStatusFailedRetryable:
		return &JobOutcome{
			Retryable:    true,
			ErrorCode:    firstErrorCode(result.ProviderErrors),
			ErrorMessage: firstErrorMessage(result.ProviderErrors, "provider reported a retryable failure"),
		}, nil

	default:
		// Rejected / failed_permanent / anything else terminal: record a failed
		// receipt row so the rejection is auditable, then stop retrying.
		receipt := s.buildFailedReceipt(job, jobCtx, action, receiptType, original, result)
		id, saveErr := s.repo.SaveReceiptForJob(receipt, job.ID, originalID)
		outcome := &JobOutcome{
			TerminalStatus: terminalStatusFor(result.Status),
			ErrorCode:      firstErrorCode(result.ProviderErrors),
			ErrorMessage:   firstErrorMessage(result.ProviderErrors, "provider rejected the receipt"),
		}
		if saveErr == nil {
			outcome.ReceiptID = &id
		}
		return outcome, nil
	}
}

func (s *Service) buildIssueInput(job database.FiscalJob, jobCtx *JobContext) IssueInput {
	settings := settingsFromModel(jobCtx.Settings)
	receiptType := resolveReceiptType(settings, jobCtx.Bill)
	total := jobCtx.Bill.TotalAmount
	if total <= 0 {
		total = jobCtx.Bill.PaidAmount
	}

	// The AR mapper derives IVA from the bill when tax is itemized. This line
	// is only a one-line summary for providers that read Lines.
	vatRate := 0.0
	if !strings.EqualFold(receiptType, "factura_c") {
		vatRate = DefaultArgentinaVATRate
	}

	docType, docNumber := customerDocumentFromBill(jobCtx.Bill)
	taxCond := strings.TrimSpace(strValue(jobCtx.Bill.FiscalCustomerTaxCondition))
	custName := strings.TrimSpace(strValue(jobCtx.Bill.FiscalCustomerName))

	return IssueInput{
		Business:             jobCtx.Business,
		Settings:             settings,
		Bill:                 jobCtx.Bill,
		Lines:                []IssueLine{{Description: "Total", Quantity: 1, UnitCents: total, TotalCents: total, TaxRate: vatRate}},
		Currency:             billCurrency(jobCtx.Bill, jobCtx.Business),
		ReceiptType:          receiptType,
		TotalAmountCents:     total,
		TipAmountCents:       jobCtx.Bill.TipAmount,
		IssuedAt:             s.now(),
		IdempotencyKey:       job.IdempotencyKey,
		CustomerDocType:      docType,
		CustomerDocNumber:    docNumber,
		CustomerTaxCondition: taxCond,
		CustomerName:         custName,
	}
}

func (s *Service) buildAuthorizedReceipt(job database.FiscalJob, jobCtx *JobContext, action, receiptType string, original *database.FiscalReceipt, result *ReceiptResult) *database.FiscalReceipt {
	rt := receiptType
	if result.ReceiptType != "" {
		rt = result.ReceiptType
	}
	issuedAt := s.now()
	if result.IssuedAt != nil && !result.IssuedAt.IsZero() {
		issuedAt = *result.IssuedAt
	}
	receipt := s.baseReceipt(job, jobCtx, action, rt, original)
	receipt.Status = database.FiscalStatusAuthorized
	receipt.IssuedAt = &issuedAt
	if result.ProviderReceiptID != "" {
		receipt.ProviderReceiptID = strPtr(result.ProviderReceiptID)
	}
	if result.ReceiptNumber != "" {
		receipt.ReceiptNumber = strPtr(result.ReceiptNumber)
	}
	if result.AuthCode != "" {
		receipt.AuthCode = strPtr(result.AuthCode)
	}
	receipt.AuthExpiresAt = result.AuthExpiresAt
	if result.QRPayload != "" {
		receipt.QRPayload = strPtr(result.QRPayload)
	}
	if result.PDFPath != "" {
		receipt.PDFPath = strPtr(result.PDFPath)
	}
	return receipt
}

func (s *Service) buildFailedReceipt(job database.FiscalJob, jobCtx *JobContext, action, receiptType string, original *database.FiscalReceipt, result *ReceiptResult) *database.FiscalReceipt {
	rt := receiptType
	if result.ReceiptType != "" {
		rt = result.ReceiptType
	}
	receipt := s.baseReceipt(job, jobCtx, action, rt, original)
	receipt.Status = terminalStatusFor(result.Status)
	if code := firstErrorCode(result.ProviderErrors); code != "" {
		receipt.ErrorCode = strPtr(code)
	}
	if msg := firstErrorMessage(result.ProviderErrors, ""); msg != "" {
		receipt.ErrorMessage = strPtr(msg)
	}
	return receipt
}

func (s *Service) baseReceipt(job database.FiscalJob, jobCtx *JobContext, action, receiptType string, original *database.FiscalReceipt) *database.FiscalReceipt {
	total := jobCtx.Bill.TotalAmount
	if total <= 0 {
		total = jobCtx.Bill.PaidAmount
	}
	tip := jobCtx.Bill.TipAmount
	if action == ActionCreditNote {
		// A partial nota de crédito stores the credited amount, not the bill
		// total. A nil or non-positive amount is a full credit of the original
		// receipt. Tips never enter a nota de crédito.
		tip = 0
		if job.CreditAmountCents != nil && *job.CreditAmountCents > 0 {
			total = *job.CreditAmountCents
		} else if original != nil {
			total = original.TotalAmountCents
		}
	}
	docType, docNumber := customerDocumentFromBill(jobCtx.Bill)
	taxCond := strings.TrimSpace(strValue(jobCtx.Bill.FiscalCustomerTaxCondition))
	custName := strings.TrimSpace(strValue(jobCtx.Bill.FiscalCustomerName))
	if original != nil {
		// Credit notes must reflect the receptor that was actually on the
		// original authorized invoice — the bill's fiscal fields are mutable
		// (issue overrides keep writing to them) and may have drifted since.
		// The WSFE submission already snapshots from the original
		// (MapCreditNoteToWSFE); the stored row and its PDF must match it.
		docType = strValue(original.CustomerDocType)
		docNumber = strValue(original.CustomerDocNumber)
		taxCond = strings.TrimSpace(strValue(original.CustomerTaxCondition))
		custName = strings.TrimSpace(strValue(original.CustomerName))
	}
	receipt := &database.FiscalReceipt{
		BusinessID:           job.BusinessID,
		SettingsID:           job.SettingsID,
		BillID:               job.BillID,
		PaymentID:            job.PaymentID,
		AlternativePaymentID: job.AlternativePaymentID,
		Country:              jobCtx.Settings.Country,
		Provider:             jobCtx.Settings.Provider,
		Action:               action,
		ReceiptType:          receiptType,
		TotalAmountCents:     total,
		TipAmountCents:       tip,
		Currency:             billCurrency(jobCtx.Bill, jobCtx.Business),
	}
	if docType != "" {
		receipt.CustomerDocType = strPtr(docType)
	}
	if docNumber != "" {
		receipt.CustomerDocNumber = strPtr(docNumber)
	}
	if taxCond != "" {
		receipt.CustomerTaxCondition = strPtr(taxCond)
	}
	if custName != "" {
		receipt.CustomerName = strPtr(custName)
	}
	return receipt
}

func (s *Service) now() time.Time {
	if s.nowFn != nil {
		return s.nowFn().UTC()
	}
	return time.Now().UTC()
}

// resolveProvider builds a credential-aware provider via the factory; if no
// factory is configured it falls back to the static registry (e.g. for stateless
// providers registered at boot).
func (s *Service) resolveProvider(ctx context.Context, settings *database.BusinessFiscalSettings) (Provider, error) {
	if s.factory != nil {
		return s.factory.Build(ctx, settings)
	}
	if s.providers != nil {
		if p, ok := s.providers.Get(settings.Country, settings.Provider); ok {
			return p, nil
		}
	}
	return nil, errors.New("no fiscal provider factory configured")
}

// classifyProviderError maps a provider call error to a retryable outcome by
// default. A provider may signal a permanent failure by wrapping ErrPermanent.
func classifyProviderError(err error) *JobOutcome {
	if errors.Is(err, ErrPermanent) {
		return &JobOutcome{
			TerminalStatus: database.FiscalStatusFailedPermanent,
			ErrorCode:      "provider_permanent",
			ErrorMessage:   err.Error(),
		}
	}
	return &JobOutcome{
		Retryable:    true,
		ErrorCode:    "provider_error",
		ErrorMessage: err.Error(),
	}
}

// outcomeForStatus maps a provider-reported status into a job outcome.
func outcomeForStatus(status database.FiscalStatus, provErrors []ProviderError, receiptID *uint) *JobOutcome {
	switch status {
	case database.FiscalStatusAuthorized, database.FiscalStatusCredited:
		return &JobOutcome{TerminalStatus: status, ReceiptID: receiptID}
	case database.FiscalStatusFailedRetryable, database.FiscalStatusPending:
		return &JobOutcome{
			Retryable:    true,
			ErrorCode:    firstErrorCode(provErrors),
			ErrorMessage: firstErrorMessage(provErrors, "provider reported a retryable status"),
		}
	default:
		return &JobOutcome{
			TerminalStatus: terminalStatusFor(status),
			ReceiptID:      receiptID,
			ErrorCode:      firstErrorCode(provErrors),
			ErrorMessage:   firstErrorMessage(provErrors, "provider reported a terminal status"),
		}
	}
}

// terminalStatusFor maps a provider-reported terminal status onto the status the
// job/receipt should take. Per the queue contract a provider rejection (e.g. CAE
// rejected) is recorded as failed_permanent — a non-retryable, operator-visible
// terminal failure — rather than the transient-looking "rejected" wire value.
func terminalStatusFor(status database.FiscalStatus) database.FiscalStatus {
	switch status {
	case database.FiscalStatusAuthorized,
		database.FiscalStatusFailedPermanent,
		database.FiscalStatusCancelled,
		database.FiscalStatusCredited:
		return status
	default:
		// rejected, pending (unexpected), or any unknown terminal → permanent fail.
		return database.FiscalStatusFailedPermanent
	}
}

// resolveReceiptType is the worker-side, ISSUABLE receipt-type derivation.
// Country gates the catalogue: AR keeps the AFIP letter scheme via
// ResolveIssuableReceiptType (D-2); US/other locales get invoice/receipt so a
// United States venue never issues Factura A/B/C.
func resolveReceiptType(settings Settings, bill database.Bill) string {
	return ResolveIssuableReceiptTypeForCountry(
		settings.Country,
		settings.TaxCondition,
		strValue(bill.FiscalCustomerTaxCondition),
		strValue(bill.FiscalCustomerDocType),
		strValue(bill.FiscalCustomerDocNumber),
	)
}

// jobLockOwner returns the worker id holding the job's lock, or "" when the
// job carries none (RecordJobAttempt then refuses to write).
func jobLockOwner(job database.FiscalJob) string {
	if job.LockedBy == nil {
		return ""
	}
	return *job.LockedBy
}

func attemptedProviderReceiptID(job database.FiscalJob) string {
	if job.AttemptedProviderReceiptID == nil {
		return ""
	}
	return *job.AttemptedProviderReceiptID
}

func customerDocumentFromBill(bill database.Bill) (docType, docNumber string) {
	return strings.TrimSpace(strValue(bill.FiscalCustomerDocType)), strings.TrimSpace(strValue(bill.FiscalCustomerDocNumber))
}

// billCurrency resolves the currency a fiscal receipt is denominated in. Bill.Currency
// is a gorm:"-" transient field that is never loaded in the worker path, so it is
// almost always empty here; the authoritative source is the business's configured
// default currency. The final "ARS" fallback only applies when BOTH are empty
// (an AFIP-enabled business with no currency set is assumed to bill in pesos).
//
// Crucially this no longer blanket-defaults a USD/USDC business to ARS: a non-ARS
// currency now flows through to the AR mapper, which rejects it with ErrPermanent
// (F-CURRENCY) — fail closed rather than silently declaring foreign amounts as pesos.
func billCurrency(bill database.Bill, business database.Business) string {
	if c := strings.TrimSpace(bill.Currency); c != "" {
		return c
	}
	if c := strings.TrimSpace(business.DefaultCurrency); c != "" {
		return c
	}
	return "ARS"
}

func settingsFromModel(m database.BusinessFiscalSettings) Settings {
	return Settings{
		ID:                     m.ID,
		BusinessID:             m.BusinessID,
		Country:                m.Country,
		Provider:               m.Provider,
		Mode:                   m.Mode,
		Environment:            m.Environment,
		TaxID:                  m.TaxID,
		TaxCondition:           m.TaxCondition,
		PointOfSale:            m.PointOfSale,
		CredentialsEncrypted:   m.CredentialsEncrypted,
		CredentialsFingerprint: m.CredentialsFingerprint,
		CredentialsExpiresAt:   m.CredentialsExpiresAt,
		ProviderConfig:         m.ProviderConfig,
		SetupStatus:            m.SetupStatus,
	}
}

func firstErrorCode(errs []ProviderError) string {
	if len(errs) == 0 {
		return ""
	}
	return strings.TrimSpace(errs[0].Code)
}

func firstErrorMessage(errs []ProviderError, fallback string) string {
	if len(errs) == 0 {
		return fallback
	}
	if msg := strings.TrimSpace(errs[0].Message); msg != "" {
		return msg
	}
	return fallback
}

func strValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func strPtr(s string) *string {
	return &s
}
