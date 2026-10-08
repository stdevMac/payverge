package fiscal

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// authorizedCreditStub returns a credit-note provider stub that authorizes.
func authorizedCreditStub(prov *stubProvider) {
	prov.CreditFn = func(ctx context.Context, in CreditNoteInput) (*ReceiptResult, error) {
		return &ReceiptResult{Status: StatusAuthorized, AuthCode: "99", ReceiptNumber: "7", ReceiptType: "nota_credito_b"}, nil
	}
}

// TestFiscalWorkerMarksOriginalCreditedWhenFullyCredited locks F-CREDITED: when a
// credit note that covers the full original total authorizes, the original issue
// receipt transitions authorized → credited, giving consumers a real terminal
// state and making the receipt non-creditable again.
func TestFiscalWorkerMarksOriginalCreditedWhenFullyCredited(t *testing.T) {
	w, db, prov := newTestFiscalWorker(t)
	authorizedCreditStub(prov)
	seedPaidBill(t, db, 1, 1, 2400)
	settingsID := seedReadySettings(t, db, 1)
	createFiscalReceiptWithStatus(t, db, 1, 1, settingsID, 1, database.FiscalStatusAuthorized) // total 2400
	receiptID := uint(1)
	enqueueJob(t, db, 1, 1, ActionCreditNote, &receiptID) // full credit (nil amount == whole total)

	_, err := w.ProcessDue(context.Background())
	require.NoError(t, err)

	var original database.FiscalReceipt
	require.NoError(t, db.First(&original, receiptID).Error)
	require.Equal(t, database.FiscalStatusCredited, original.Status,
		"a fully-credited original receipt must move to credited")
}

// TestFiscalWorkerKeepsOriginalAuthorizedOnPartialCredit locks the other side: a
// partial credit that does not reach the original total must leave the original
// authorized (still creditable for the remainder).
func TestFiscalWorkerKeepsOriginalAuthorizedOnPartialCredit(t *testing.T) {
	w, db, prov := newTestFiscalWorker(t)
	authorizedCreditStub(prov)
	seedPaidBill(t, db, 1, 1, 2400)
	settingsID := seedReadySettings(t, db, 1)
	createFiscalReceiptWithStatus(t, db, 1, 1, settingsID, 1, database.FiscalStatusAuthorized) // total 2400
	receiptID := uint(1)
	amt := int64(1000)
	require.NoError(t, db.Create(&database.FiscalJob{
		BusinessID: 1, SettingsID: settingsID, BillID: 1, ReceiptID: &receiptID,
		Action: ActionCreditNote, IdempotencyKey: "cn-partial-1000",
		CreditAmountCents: &amt, Status: database.FiscalStatusPending, MaxAttempts: 5, CreatedBy: "system",
	}).Error)

	_, err := w.ProcessDue(context.Background())
	require.NoError(t, err)

	var original database.FiscalReceipt
	require.NoError(t, db.First(&original, receiptID).Error)
	require.Equal(t, database.FiscalStatusAuthorized, original.Status,
		"a partial credit must not mark the original credited")
}
