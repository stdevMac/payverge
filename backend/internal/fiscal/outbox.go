package fiscal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"

	"gorm.io/gorm"
)

// EnqueueIssueJobInTx writes the fiscal issue job on the CALLER's payment
// settlement transaction. The fiscal_jobs row IS the outbox: once the
// settlement commit succeeds the job is durably queued for the FiscalWorker
// (which polls fiscal_jobs via database.ClaimDueFiscalJobs), so a crash
// between settlement and a post-commit enqueue can no longer lose the factura.
//
// Gating mirrors Service.HandleBillPaid exactly (service.go:151-170): only
// fully-paid bills, only businesses with active fiscal settings in an
// automatic mode; every skip returns nil. The idempotency key is the same
// per-bill key as the post-commit path (buildIdempotencyKey), so both paths
// collapse to a single job via idx_fiscal_jobs_idempotency.
//
// An enqueue error rolls back the settlement transaction — acceptable because
// the job row lives in the same database, so the only realistic failure is a
// DB fault that would fail the commit anyway (same posture as the delivery
// Telegram transactional outbox in services.GuestDeliveryCheckout). The one
// exception: a unique-key violation means the job ALREADY exists (either via
// the idempotency key or the bill-scoped idx_fiscal_jobs_bill_issue_unique
// backstop in the genesis schema, if keys ever diverge) — that is the outbox's
// success condition, so it is swallowed as a no-op rather than allowed to
// abort the settlement.
func EnqueueIssueJobInTx(tx *gorm.DB, bill *database.Bill, paymentID, alternativePaymentID *uint, actor string) error {
	if bill == nil || bill.Status != database.BillStatusPaid || bill.PaidAmount <= 0 {
		return nil
	}
	enabled, controlErr := automaticFiscalEnabled(context.Background(), tx)
	if controlErr != nil {
		// Fiscal containment must fail closed without rolling back money already
		// collected. The worker is independently gated, so queued work cannot run.
		logger.Logger.Warnf("Fiscal runtime control unavailable; automatic enqueue skipped: %v", controlErr)
		return nil
	}
	if !enabled {
		return nil
	}
	// Task 15: transactional outbox must mirror HandleBillPaid and never queue
	// AFIP jobs for demo/test businesses into the production fiscal worker.
	if !isProductionFiscalBusiness(tx, bill.BusinessID) {
		return nil
	}
	repo := NewRepository(tx)
	settings, err := repo.GetActiveSettings(bill.BusinessID)
	if err != nil || settings == nil {
		return err // GetActiveSettings already returns nil,nil for mode=off
	}
	if settings.Mode == database.FiscalModeManual {
		return nil
	}
	_, err = repo.CreateJobIfNotExists(CreateJobInput{
		BusinessID:           bill.BusinessID,
		SettingsID:           settings.ID,
		BillID:               bill.ID,
		PaymentID:            paymentID,
		AlternativePaymentID: alternativePaymentID,
		Action:               ActionIssueReceipt,
		IdempotencyKey:       buildIdempotencyKey(bill.BusinessID, bill.ID, ActionIssueReceipt),
		CreatedBy:            actor,
	})
	if err != nil && isDuplicateKeyError(err) {
		// The bill already carries an issue job (bill-scoped unique backstop
		// fired past the idempotency-key DO NOTHING). Already enqueued — the
		// fiscal backstop must never kill a payment settlement.
		return nil
	}
	return err
}

// EnqueueRefundCreditNoteInTx writes the credit-note intent on the caller's
// crypto-refund ledger transaction. The fiscal_jobs row is the durable outbox:
// a confirmed refund can no longer commit locally and then lose its tax
// reversal because the process crashed before a post-commit enqueue.
//
// When the original issue receipt is not authorized yet, the existing deferred
// credit-note path is used. Fiscal-off businesses remain a no-op. Any database
// failure is returned so the ledger transaction rolls back and the already
// verified on-chain transaction is reconciled on the next worker tick without
// sending money again.
func EnqueueRefundCreditNoteInTx(tx *gorm.DB, bill *database.Bill, payment *database.Payment, discriminator, actor string) error {
	if tx == nil || bill == nil || payment == nil {
		return fmt.Errorf("refund fiscal outbox requires transaction, bill, and payment")
	}
	if payment.Amount <= 0 {
		return fmt.Errorf("refund fiscal outbox requires positive fiscal amount")
	}

	var receipt database.FiscalReceipt
	err := tx.Select("id", "status", "total_amount_cents").
		Where("bill_id = ? AND business_id = ? AND action = ? AND status IN ?",
			bill.ID, bill.BusinessID, ActionIssueReceipt,
			[]database.FiscalStatus{database.FiscalStatusAuthorized, database.FiscalStatusCredited}).
		Order("created_at DESC, id DESC").
		First(&receipt).Error
	svc := NewService(tx, NewProviderRegistry())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return svc.IssueCreditNoteDeferred(context.Background(), bill.BusinessID, bill.ID, payment.Amount, discriminator, actor)
	}
	if err != nil {
		return fmt.Errorf("load authorized fiscal receipt for refund: %w", err)
	}
	if receipt.Status == database.FiscalStatusCredited {
		return recordRefundFiscalSkipInTx(tx, bill, payment, &receipt, actor,
			"original fiscal receipt was already fully credited; no duplicate credit note was enqueued")
	}
	if err := svc.IssueCreditNote(context.Background(), bill.BusinessID, receipt.ID, payment.Amount, discriminator, "refund", actor); err != nil {
		if errors.Is(err, ErrCreditNoteExceedsReceipt) {
			fullyCredited, sumErr := fiscalReceiptFullyCreditedInTx(tx, &receipt)
			if sumErr != nil {
				return fmt.Errorf("check existing fiscal credits for refund: %w", sumErr)
			}
			if fullyCredited {
				return recordRefundFiscalSkipInTx(tx, bill, payment, &receipt, actor,
					"original fiscal receipt was already fully credited; no duplicate credit note was enqueued")
			}
		}
		return fmt.Errorf("enqueue fiscal credit note for refund: %w", err)
	}
	return nil
}

func fiscalReceiptFullyCreditedInTx(tx *gorm.DB, receipt *database.FiscalReceipt) (bool, error) {
	if receipt == nil || receipt.TotalAmountCents <= 0 {
		return false, nil
	}
	var jobs []database.FiscalJob
	if err := tx.Select("credit_amount_cents").
		Where("receipt_id = ? AND action = ? AND status <> ? AND status <> ?",
			receipt.ID, ActionCreditNote, database.FiscalStatusFailedPermanent, database.FiscalStatusRejected).
		Find(&jobs).Error; err != nil {
		return false, err
	}
	credited := int64(0)
	for _, job := range jobs {
		if job.CreditAmountCents == nil {
			credited += receipt.TotalAmountCents
		} else {
			credited += *job.CreditAmountCents
		}
	}
	return credited >= receipt.TotalAmountCents, nil
}

func recordRefundFiscalSkipInTx(tx *gorm.DB, bill *database.Bill, payment *database.Payment, receipt *database.FiscalReceipt, actor, message string) error {
	receiptID := receipt.ID
	return tx.Create(&database.FiscalAuditEvent{
		BusinessID: bill.BusinessID,
		ReceiptID:  &receiptID,
		Actor:      defaultActor(actor),
		EventType:  "refund_credit_note_not_enqueued",
		Message:    message,
		Metadata: map[string]interface{}{
			"bill_id":    bill.ID,
			"payment_id": payment.ID,
		},
	}).Error
}

// isDuplicateKeyError reports whether err is a unique-constraint violation, on
// any supported dialect. GORM only yields ErrDuplicatedKey when TranslateError
// is enabled, so the raw Postgres (SQLSTATE 23505 / "duplicate key value
// violates unique constraint") and SQLite ("UNIQUE constraint failed")
// messages are matched as well.
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key value violates unique constraint") ||
		strings.Contains(msg, "SQLSTATE 23505") ||
		strings.Contains(msg, "UNIQUE constraint failed")
}
