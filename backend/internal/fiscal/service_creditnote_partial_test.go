package fiscal

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// T7: each partial refund (distinct discriminator) against one receipt gets its
// own credit-note job with its own amount; a retried refund of the SAME payment
// (same discriminator) is a no-op.
func TestServiceIssueCreditNote_PartialRefundsPerDiscriminatorCreateDistinctJobs(t *testing.T) {
	db := newFiscalTestDB(t)
	createFiscalReceiptWithStatus(t, db, 40, 4, 5, 44, database.FiscalStatusAuthorized)
	svc := NewService(db, NewProviderRegistry())

	// 1000 + 1400 = 2400 == TotalAmountCents: exactly at the limit (not over).
	require.NoError(t, svc.IssueCreditNote(context.Background(), 4, 40, 1000, "payment:7", "refund", "system"))
	require.NoError(t, svc.IssueCreditNote(context.Background(), 4, 40, 1400, "payment:8", "refund", "system"))
	require.Equal(t, int64(2), countFiscalJobs(t, db))

	// Retried refund of the same payment → no second job for that payment.
	require.NoError(t, svc.IssueCreditNote(context.Background(), 4, 40, 1000, "payment:7", "refund", "system"))
	require.Equal(t, int64(2), countFiscalJobs(t, db))

	var jobs []database.FiscalJob
	require.NoError(t, db.Order("id").Find(&jobs).Error)
	require.Len(t, jobs, 2)

	require.Equal(t, "business:4:receipt:40:action:credit_note:payment:7", jobs[0].IdempotencyKey)
	require.NotNil(t, jobs[0].CreditAmountCents)
	require.Equal(t, int64(1000), *jobs[0].CreditAmountCents)

	require.Equal(t, "business:4:receipt:40:action:credit_note:payment:8", jobs[1].IdempotencyKey)
	require.NotNil(t, jobs[1].CreditAmountCents)
	require.Equal(t, int64(1400), *jobs[1].CreditAmountCents)
}

// A full credit (operator dashboard: amount 0, no discriminator) keeps the legacy
// single-credit-note key and a nil amount (mapper falls back to original total).
func TestServiceIssueCreditNote_FullCreditKeepsLegacyKeyAndNilAmount(t *testing.T) {
	db := newFiscalTestDB(t)
	createFiscalReceiptWithStatus(t, db, 40, 4, 5, 44, database.FiscalStatusAuthorized)
	svc := NewService(db, NewProviderRegistry())

	require.NoError(t, svc.IssueCreditNote(context.Background(), 4, 40, 0, "", "manual", "manager"))

	var job database.FiscalJob
	require.NoError(t, db.First(&job).Error)
	require.Equal(t, "business:4:receipt:40:action:credit_note", job.IdempotencyKey)
	require.Nil(t, job.CreditAmountCents, "full credit stores nil amount")
}

// A partial credit-note job stores the credited amount on the receipt row.
// A full credit (nil amount) stores the original receipt total. The bill
// total and tip must not leak onto the nota de crédito.
func TestProcessClaimedJob_CreditNoteStoresCreditedTotal(t *testing.T) {
	t.Run("partial", func(t *testing.T) {
		amt := int64(300)
		credit := authorizeCreditNote(t, &amt)
		require.Equal(t, int64(300), credit.TotalAmountCents)
		require.Equal(t, int64(0), credit.TipAmountCents)
	})
	t.Run("full", func(t *testing.T) {
		credit := authorizeCreditNote(t, nil)
		require.Equal(t, int64(1000), credit.TotalAmountCents)
		require.Equal(t, int64(0), credit.TipAmountCents)
	})
}

func authorizeCreditNote(t *testing.T, creditAmount *int64) database.FiscalReceipt {
	t.Helper()
	db := newFiscalTestDB(t)
	require.NoError(t, db.Create(&database.Business{
		ID: 4, BusinessId: "biz-credit-total", OwnerAddress: "owner",
		Name: "Credit SA", SettlementAddr: "settle", TippingAddr: "tip",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 44, BusinessID: 4, BillNumber: "PV-credit-total",
		Status: database.BillStatusPaid, Items: "[]",
		TotalAmount: 5000, PaidAmount: 5000, TipAmount: 400,
	}).Error)
	pos := 1
	settings := database.BusinessFiscalSettings{
		BusinessID: 4, Country: "AR", Provider: "arca",
		Mode: database.FiscalModeAutomaticNonBlocking, Environment: "sandbox",
		TaxID: "20123456789", TaxCondition: "monotributo", PointOfSale: &pos,
		SetupStatus: "ready",
	}
	require.NoError(t, db.Create(&settings).Error)
	original := database.FiscalReceipt{
		BusinessID: 4, SettingsID: settings.ID, BillID: 44,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_b", TotalAmountCents: 1000, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}
	require.NoError(t, db.Create(&original).Error)

	key := "cn-full"
	if creditAmount != nil {
		key = "cn-partial"
	}
	job := database.FiscalJob{
		BusinessID: 4, SettingsID: settings.ID, BillID: 44, ReceiptID: &original.ID,
		Action: ActionCreditNote, IdempotencyKey: key,
		CreditAmountCents: creditAmount, Status: database.FiscalStatusPending,
		MaxAttempts: 5, CreatedBy: "system",
	}
	require.NoError(t, db.Create(&job).Error)

	prov := &stubProvider{
		CreditFn: func(ctx context.Context, in CreditNoteInput) (*ReceiptResult, error) {
			return &ReceiptResult{
				Status: StatusAuthorized, AuthCode: "99", ReceiptNumber: "7",
				ReceiptType: "nota_de_credito_b",
			}, nil
		},
	}
	svc := NewService(db, NewProviderRegistry()).WithProviderFactory(&fakeFactory{provider: prov})
	outcome, err := svc.ProcessClaimedJob(context.Background(), job)
	require.NoError(t, err)
	require.NotNil(t, outcome)
	require.Equal(t, database.FiscalStatusAuthorized, outcome.TerminalStatus)

	var credit database.FiscalReceipt
	require.NoError(t, db.Where("action = ?", ActionCreditNote).First(&credit).Error)
	return credit
}

// Over-credit guard: a full credit (amount 0 == whole receipt) plus a later
// partial credit on the SAME receipt would exceed the original total — the
// second IssueCreditNote must be rejected, not silently enqueued.
func TestServiceIssueCreditNote_RejectsOverCreditAcrossPaths(t *testing.T) {
	db := newFiscalTestDB(t)
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 40, BusinessID: 4, SettingsID: 5, BillID: 44,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "B", TotalAmountCents: 1000, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}).Error)
	svc := NewService(db, NewProviderRegistry())

	// Operator full credit (amount 0 => credits the whole 1000).
	require.NoError(t, svc.IssueCreditNote(context.Background(), 4, 40, 0, "", "manual", "manager"))
	// A partial refund of $3 on the same receipt would push credited to 1300 > 1000.
	err := svc.IssueCreditNote(context.Background(), 4, 40, 300, "payment:7", "refund", "system")
	require.ErrorIs(t, err, ErrCreditNoteExceedsReceipt)
	require.Equal(t, int64(1), countFiscalJobs(t, db), "the over-credit job must not be enqueued")
}

// Two partials that together stay within the total are both allowed.
func TestServiceIssueCreditNote_AllowsPartialsWithinTotal(t *testing.T) {
	db := newFiscalTestDB(t)
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 40, BusinessID: 4, SettingsID: 5, BillID: 44,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "B", TotalAmountCents: 1000, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}).Error)
	svc := NewService(db, NewProviderRegistry())

	require.NoError(t, svc.IssueCreditNote(context.Background(), 4, 40, 400, "payment:7", "refund", "system"))
	require.NoError(t, svc.IssueCreditNote(context.Background(), 4, 40, 600, "payment:8", "refund", "system"))
	require.Equal(t, int64(2), countFiscalJobs(t, db))
}
