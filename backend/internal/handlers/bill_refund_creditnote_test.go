package handlers

import (
	"fmt"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupRefundCreditNoteDB wires a fresh sqlite DB as the global test DB (the
// refund hook reads database.GetDB()) and seeds an authorized issue receipt for
// bill 44 / business 4. Returns the db + the seeded receipt id; restores the
// prior global DB on cleanup.
func setupRefundCreditNoteDB(t *testing.T) (*gorm.DB, uint) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateFiscalHandlerDB(db))

	prev := database.GetDB()
	database.SetTestDB(db)
	t.Cleanup(func() { database.SetTestDB(prev) })

	receipt := &database.FiscalReceipt{
		BusinessID:  4,
		SettingsID:  5,
		BillID:      44,
		Action:      fiscal.ActionIssueReceipt,
		Status:      database.FiscalStatusAuthorized,
		ReceiptType: "factura_b",
		// A real fiscal receipt's total is always >= any credit note raised
		// against it. The P1.4 over-credit guard rejects credits that exceed
		// this total, so seed a realistic total that covers both the partial
		// (2500) and full (10000) credits these tests enqueue.
		TotalAmountCents: 10000,
	}
	require.NoError(t, db.Create(receipt).Error)
	return db, receipt.ID
}

// T7: a PARTIAL refund (bill still partly paid) now enqueues a credit note for
// the refunded fiscal amount — the old code skipped partials entirely.
func TestEnqueueFiscalCreditNoteForRefund_PartialEnqueuesForRefundedAmount(t *testing.T) {
	db, receiptID := setupRefundCreditNoteDB(t)

	bill := &database.Bill{ID: 44, BusinessID: 4, PaidAmount: 5000} // still partly paid
	enqueueFiscalCreditNoteForRefund(bill, 2500, "payment:7", "system")

	var jobs []database.FiscalJob
	require.NoError(t, db.Where("action = ?", fiscal.ActionCreditNote).Find(&jobs).Error)
	require.Len(t, jobs, 1, "a partial refund must enqueue a credit note")
	require.NotNil(t, jobs[0].CreditAmountCents)
	require.Equal(t, int64(2500), *jobs[0].CreditAmountCents)
	require.Equal(t, fmt.Sprintf("business:4:receipt:%d:action:credit_note:payment:7", receiptID), jobs[0].IdempotencyKey)
}

// A full refund still enqueues a credit note for the refunded amount.
func TestEnqueueFiscalCreditNoteForRefund_FullEnqueues(t *testing.T) {
	db, _ := setupRefundCreditNoteDB(t)

	bill := &database.Bill{ID: 44, BusinessID: 4, PaidAmount: 0} // fully reversed
	enqueueFiscalCreditNoteForRefund(bill, 10000, "payment:9", "system")

	var count int64
	require.NoError(t, db.Model(&database.FiscalJob{}).Where("action = ?", fiscal.ActionCreditNote).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

// Best-effort: a non-positive refunded amount (e.g. tip-only refund) enqueues
// nothing and never panics.
func TestEnqueueFiscalCreditNoteForRefund_NonPositiveAmountSkips(t *testing.T) {
	db, _ := setupRefundCreditNoteDB(t)

	require.NotPanics(t, func() {
		enqueueFiscalCreditNoteForRefund(&database.Bill{ID: 44, BusinessID: 4}, 0, "payment:7", "system")
	})
	var count int64
	require.NoError(t, db.Model(&database.FiscalJob{}).Where("action = ?", fiscal.ActionCreditNote).Count(&count).Error)
	require.Zero(t, count)
}
