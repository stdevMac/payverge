package fiscal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestIssueCreditNoteDeferred_CreatesBillScopedPendingJob: with NO authorized
// issue receipt yet, the refund path must persist a deferred credit-note job
// (receipt_id NULL, bill-scoped idempotency key, extended retry budget)
// instead of silently dropping the nota de crédito.
func TestIssueCreditNoteDeferred_CreatesBillScopedPendingJob(t *testing.T) {
	db := newFiscalTestDB(t)
	bill := database.Bill{BusinessID: 1, BillNumber: "B-defer-1",
		TotalAmount: 1000, PaidAmount: 1000, Status: database.BillStatusPaid}
	require.NoError(t, db.Create(&bill).Error)
	settings := database.BusinessFiscalSettings{BusinessID: 1, Country: "AR",
		Provider: "arca", Environment: "sandbox", Mode: database.FiscalModeAutomaticNonBlocking}
	require.NoError(t, db.Create(&settings).Error)

	svc := NewService(db, NewProviderRegistry())
	require.NoError(t, svc.IssueCreditNoteDeferred(context.Background(), 1, bill.ID, 400, "payment:7", "refund"))

	var job database.FiscalJob
	require.NoError(t, db.Where("bill_id = ? AND action = ?", bill.ID, ActionCreditNote).First(&job).Error)
	require.Nil(t, job.ReceiptID, "deferred job carries no receipt reference yet")
	require.Equal(t, database.FiscalStatusPending, job.Status)
	require.Equal(t, deferredCreditNoteMaxAttempts, job.MaxAttempts)
	require.NotNil(t, job.CreditAmountCents)
	require.Equal(t, int64(400), *job.CreditAmountCents)

	// Same discriminator retries are a no-op (idempotency).
	require.NoError(t, svc.IssueCreditNoteDeferred(context.Background(), 1, bill.ID, 400, "payment:7", "refund"))
	var count int64
	require.NoError(t, db.Model(&database.FiscalJob{}).
		Where("bill_id = ? AND action = ?", bill.ID, ActionCreditNote).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

// TestProcessCreditNoteJob_NilReceipt_RetriesUntilIssueAuthorizes: a deferred
// job processed BEFORE the issue receipt authorizes must come back retryable
// (awaiting_issue_receipt), never failed_permanent.
func TestProcessCreditNoteJob_NilReceipt_RetriesUntilIssueAuthorizes(t *testing.T) {
	db := newFiscalTestDB(t)
	bill := database.Bill{BusinessID: 1, BillNumber: "B-defer-2",
		TotalAmount: 1000, PaidAmount: 1000, Status: database.BillStatusPaid}
	require.NoError(t, db.Create(&bill).Error)
	settings := database.BusinessFiscalSettings{BusinessID: 1, Country: "AR",
		Provider: "arca", Environment: "sandbox", Mode: database.FiscalModeAutomaticNonBlocking}
	require.NoError(t, db.Create(&settings).Error)
	business := database.Business{Name: "Deferred SA"}
	require.NoError(t, db.Create(&business).Error)

	amount := int64(400)
	job := database.FiscalJob{BusinessID: 1, SettingsID: settings.ID, BillID: bill.ID,
		Action: ActionCreditNote, IdempotencyKey: "business:1:bill:1:action:credit_note_deferred:payment:7",
		CreditAmountCents: &amount, Status: database.FiscalStatusPending,
		MaxAttempts: deferredCreditNoteMaxAttempts, CreatedBy: "system"}
	require.NoError(t, db.Create(&job).Error)

	svc := NewService(db, NewProviderRegistry())
	outcome, err := svc.processCreditNoteJob(context.Background(), job, &JobContext{
		Bill: bill, Settings: settings, Business: business,
	}, nil /* provider never reached: no authorized issue receipt */)
	require.NoError(t, err)
	require.True(t, outcome.Retryable, "must retry until the issue receipt authorizes")
	require.Empty(t, outcome.TerminalStatus)
	require.Equal(t, "awaiting_issue_receipt", outcome.ErrorCode)
}

// TestProcessCreditNoteJob_NilReceipt_OverCreditGoesTerminal: once the issue
// receipt exists, a deferred amount exceeding the remaining creditable total
// must fail permanently (rejected), mirroring ErrCreditNoteExceedsReceipt.
func TestProcessCreditNoteJob_NilReceipt_OverCreditGoesTerminal(t *testing.T) {
	db := newFiscalTestDB(t)
	bill := database.Bill{BusinessID: 1, BillNumber: "B-defer-3",
		TotalAmount: 1000, PaidAmount: 1000, Status: database.BillStatusPaid}
	require.NoError(t, db.Create(&bill).Error)
	settings := database.BusinessFiscalSettings{BusinessID: 1, Country: "AR",
		Provider: "arca", Environment: "sandbox", Mode: database.FiscalModeAutomaticNonBlocking}
	require.NoError(t, db.Create(&settings).Error)
	business := database.Business{Name: "Deferred SA"}
	require.NoError(t, db.Create(&business).Error)

	receipt := database.FiscalReceipt{BusinessID: 1, SettingsID: settings.ID,
		BillID: bill.ID, Action: ActionIssueReceipt,
		Status: database.FiscalStatusAuthorized, TotalAmountCents: 1000}
	require.NoError(t, db.Create(&receipt).Error)

	amount := int64(1500) // exceeds the 1000-cent original receipt
	job := database.FiscalJob{BusinessID: 1, SettingsID: settings.ID, BillID: bill.ID,
		Action: ActionCreditNote, IdempotencyKey: "business:1:bill:1:action:credit_note_deferred:payment:8",
		CreditAmountCents: &amount, Status: database.FiscalStatusPending,
		MaxAttempts: deferredCreditNoteMaxAttempts, CreatedBy: "system"}
	require.NoError(t, db.Create(&job).Error)

	svc := NewService(db, NewProviderRegistry())
	outcome, err := svc.processCreditNoteJob(context.Background(), job, &JobContext{
		Bill: bill, Settings: settings, Business: business,
	}, nil)
	require.NoError(t, err)
	require.False(t, outcome.Retryable)
	require.Equal(t, database.FiscalStatusRejected, outcome.TerminalStatus)
	require.Equal(t, "credit_note_exceeds_receipt", outcome.ErrorCode)
}
