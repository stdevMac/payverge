package fiscal

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

func TestCreditNote_TwoPartialsExceedingTotal_SecondRefused(t *testing.T) {
	db := newFiscalTestDB(t)
	require.NoError(t, db.Create(&database.Business{
		ID: 4, BusinessId: "biz-credit-guard", OwnerAddress: "owner",
		Name: "Guard SA", SettlementAddr: "settle", TippingAddr: "tip",
	}).Error)
	bill := database.Bill{
		ID: 44, BusinessID: 4, BillNumber: "PV-guard",
		Status: database.BillStatusPaid, Items: "[]",
		TotalAmount: 1000, PaidAmount: 1000,
	}
	require.NoError(t, db.Create(&bill).Error)
	pos := 1
	settings := database.BusinessFiscalSettings{
		BusinessID: 4, Country: "AR", Provider: "arca",
		Mode: database.FiscalModeAutomaticNonBlocking, Environment: "sandbox",
		TaxID: "20123456789", TaxCondition: "monotributo", PointOfSale: &pos,
		SetupStatus: "ready",
	}
	require.NoError(t, db.Create(&settings).Error)
	original := database.FiscalReceipt{
		BusinessID: 4, SettingsID: settings.ID, BillID: bill.ID,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_b", TotalAmountCents: 1000, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}
	require.NoError(t, db.Create(&original).Error)

	calls := 0
	prov := &stubProvider{
		CreditFn: func(ctx context.Context, in CreditNoteInput) (*ReceiptResult, error) {
			calls++
			return &ReceiptResult{
				Status: StatusAuthorized, AuthCode: "99", ReceiptNumber: "7",
				ReceiptType: "nota_de_credito_b",
			}, nil
		},
	}
	svc := NewService(db, NewProviderRegistry()).WithProviderFactory(&fakeFactory{provider: prov})

	require.NoError(t, svc.IssueCreditNote(context.Background(), 4, original.ID, 600, "payment:1", "refund", "system"))
	var job database.FiscalJob
	require.NoError(t, db.Where("action = ?", ActionCreditNote).First(&job).Error)

	outcome, err := svc.ProcessClaimedJob(context.Background(), job)
	require.NoError(t, err)
	require.NotNil(t, outcome)
	require.Equal(t, database.FiscalStatusAuthorized, outcome.TerminalStatus)
	require.Equal(t, 1, calls)

	require.NoError(t, db.First(&job, job.ID).Error)
	require.NotNil(t, job.ReceiptID)
	require.Equal(t, original.ID, *job.ReceiptID, "credit-note job must keep pointing at the original receipt")
	require.NotNil(t, job.ProducedReceiptID)
	var credit database.FiscalReceipt
	require.NoError(t, db.Where("action = ?", ActionCreditNote).First(&credit).Error)
	require.Equal(t, credit.ID, *job.ProducedReceiptID)
	require.NotEqual(t, original.ID, credit.ID)

	err = svc.IssueCreditNote(context.Background(), 4, original.ID, 500, "payment:2", "refund", "system")
	require.ErrorIs(t, err, ErrCreditNoteExceedsReceipt)
	require.Equal(t, int64(1), countFiscalJobs(t, db), "the over-credit job must not be enqueued")
}

func TestCreditNote_PendingDeferredBlocksManualOverCredit(t *testing.T) {
	db := newFiscalTestDB(t)
	require.NoError(t, db.Create(&database.Business{
		ID: 4, BusinessId: "biz-credit-defer-guard", OwnerAddress: "owner",
		Name: "Guard SA", SettlementAddr: "settle", TippingAddr: "tip",
	}).Error)
	bill := database.Bill{
		ID: 44, BusinessID: 4, BillNumber: "PV-defer-guard",
		Status: database.BillStatusPaid, Items: "[]",
		TotalAmount: 1000, PaidAmount: 1000,
	}
	require.NoError(t, db.Create(&bill).Error)
	settings := database.BusinessFiscalSettings{
		BusinessID: 4, Country: "AR", Provider: "arca",
		Mode: database.FiscalModeAutomaticNonBlocking, Environment: "sandbox",
		SetupStatus: "ready",
	}
	require.NoError(t, db.Create(&settings).Error)
	original := database.FiscalReceipt{
		BusinessID: 4, SettingsID: settings.ID, BillID: bill.ID,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_b", TotalAmountCents: 1000, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}
	require.NoError(t, db.Create(&original).Error)

	svc := NewService(db, NewProviderRegistry())
	require.NoError(t, svc.IssueCreditNoteDeferred(context.Background(), 4, bill.ID, 700, "payment:9", "refund"))

	err := svc.IssueCreditNote(context.Background(), 4, original.ID, 400, "payment:manual", "refund", "manager")
	require.ErrorIs(t, err, ErrCreditNoteExceedsReceipt)
	require.Equal(t, int64(1), countFiscalJobs(t, db))
}

func TestCreditNote_ReprocessAuthorizedSkipsProvider(t *testing.T) {
	db := newFiscalTestDB(t)
	require.NoError(t, db.Create(&database.Business{
		ID: 4, BusinessId: "biz-credit-reprocess", OwnerAddress: "owner",
		Name: "Guard SA", SettlementAddr: "settle", TippingAddr: "tip",
	}).Error)
	bill := database.Bill{
		ID: 44, BusinessID: 4, BillNumber: "PV-reprocess",
		Status: database.BillStatusPaid, Items: "[]",
		TotalAmount: 1000, PaidAmount: 1000,
	}
	require.NoError(t, db.Create(&bill).Error)
	settings := database.BusinessFiscalSettings{
		BusinessID: 4, Country: "AR", Provider: "arca",
		Mode: database.FiscalModeAutomaticNonBlocking, Environment: "sandbox",
		SetupStatus: "ready",
	}
	require.NoError(t, db.Create(&settings).Error)
	original := database.FiscalReceipt{
		BusinessID: 4, SettingsID: settings.ID, BillID: bill.ID,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_b", TotalAmountCents: 1000, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}
	require.NoError(t, db.Create(&original).Error)
	produced := database.FiscalReceipt{
		BusinessID: 4, SettingsID: settings.ID, BillID: bill.ID,
		Country: "AR", Provider: "arca", Action: ActionCreditNote,
		ReceiptType: "nota_de_credito_b", TotalAmountCents: 600, Currency: "ARS",
		Status: database.FiscalStatusAuthorized, AuthCode: strPtr("99"),
	}
	require.NoError(t, db.Create(&produced).Error)
	amt := int64(600)
	job := database.FiscalJob{
		BusinessID: 4, SettingsID: settings.ID, BillID: bill.ID,
		ReceiptID: &original.ID, ProducedReceiptID: &produced.ID,
		Action: ActionCreditNote, IdempotencyKey: "business:4:receipt:reprocess",
		CreditAmountCents: &amt, Status: database.FiscalStatusPending,
		MaxAttempts: 5, CreatedBy: "system",
	}
	require.NoError(t, db.Create(&job).Error)

	calls := 0
	prov := &stubProvider{
		CreditFn: func(ctx context.Context, in CreditNoteInput) (*ReceiptResult, error) {
			calls++
			return &ReceiptResult{Status: StatusAuthorized, AuthCode: "100", ReceiptNumber: "8"}, nil
		},
	}
	svc := NewService(db, NewProviderRegistry())
	outcome, err := svc.processCreditNoteJob(context.Background(), job, &JobContext{
		Bill: bill, Settings: settings, Business: database.Business{Name: "Guard SA"},
	}, prov)
	require.NoError(t, err)
	require.NotNil(t, outcome)
	require.Equal(t, database.FiscalStatusAuthorized, outcome.TerminalStatus)
	require.NotNil(t, outcome.ReceiptID)
	require.Equal(t, produced.ID, *outcome.ReceiptID)
	require.Equal(t, 0, calls, "an already-authorized credit job must not call the provider again")
	var orig database.FiscalReceipt
	require.NoError(t, db.First(&orig, original.ID).Error)
	require.Equal(t, database.FiscalStatusAuthorized, orig.Status, "a 600/1000 partial credit leaves the original authorized")
}

// seedAdoptableFullCredit seeds an authorized original factura and a full
// nota de crédito already stored as the job's produced receipt, as if the
// previous run died after persisting the nota but before marking the original.
func seedAdoptableFullCredit(t *testing.T, db *gorm.DB) (database.FiscalJob, database.FiscalReceipt, *JobContext) {
	t.Helper()
	require.NoError(t, db.Create(&database.Business{
		ID: 4, BusinessId: "biz-credit-adopt-full", OwnerAddress: "owner",
		Name: "Guard SA", SettlementAddr: "settle", TippingAddr: "tip",
	}).Error)
	bill := database.Bill{
		ID: 44, BusinessID: 4, BillNumber: "PV-adopt-full",
		Status: database.BillStatusPaid, Items: "[]",
		TotalAmount: 1000, PaidAmount: 1000,
	}
	require.NoError(t, db.Create(&bill).Error)
	settings := database.BusinessFiscalSettings{
		BusinessID: 4, Country: "AR", Provider: "arca",
		Mode: database.FiscalModeAutomaticNonBlocking, Environment: "sandbox",
		SetupStatus: "ready",
	}
	require.NoError(t, db.Create(&settings).Error)
	original := database.FiscalReceipt{
		BusinessID: 4, SettingsID: settings.ID, BillID: bill.ID,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_b", TotalAmountCents: 1000, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}
	require.NoError(t, db.Create(&original).Error)
	produced := database.FiscalReceipt{
		BusinessID: 4, SettingsID: settings.ID, BillID: bill.ID,
		Country: "AR", Provider: "arca", Action: ActionCreditNote,
		ReceiptType: "nota_de_credito_b", TotalAmountCents: 1000, Currency: "ARS",
		Status: database.FiscalStatusAuthorized, AuthCode: strPtr("99"),
	}
	require.NoError(t, db.Create(&produced).Error)
	job := database.FiscalJob{
		BusinessID: 4, SettingsID: settings.ID, BillID: bill.ID,
		ReceiptID: &original.ID, ProducedReceiptID: &produced.ID,
		Action: ActionCreditNote, IdempotencyKey: "business:4:receipt:adopt-full",
		Status: database.FiscalStatusPending, MaxAttempts: 5, CreatedBy: "system",
	}
	require.NoError(t, db.Create(&job).Error)
	return job, original, &JobContext{Bill: bill, Settings: settings, Business: database.Business{Name: "Guard SA"}}
}

// Adopting a full nota de crédito must also move the original to credited.
func TestCreditNote_AdoptFullCreditMarksOriginalCredited(t *testing.T) {
	db := newFiscalTestDB(t)
	job, original, jobCtx := seedAdoptableFullCredit(t, db)
	svc := NewService(db, NewProviderRegistry())
	outcome, err := svc.processCreditNoteJob(context.Background(), job, jobCtx, &stubProvider{})
	require.NoError(t, err)
	require.Equal(t, database.FiscalStatusAuthorized, outcome.TerminalStatus)

	var orig database.FiscalReceipt
	require.NoError(t, db.First(&orig, original.ID).Error)
	require.Equal(t, database.FiscalStatusCredited, orig.Status)
}

// If marking the original fails, the job stays retryable instead of going
// terminal with the factura still open.
func TestCreditNote_AdoptMarkFailureStaysRetryable(t *testing.T) {
	db := newFiscalTestDB(t)
	job, original, jobCtx := seedAdoptableFullCredit(t, db)
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:fail_receipt_update", func(tx *gorm.DB) {
		if tx.Statement.Table == "fiscal_receipts" {
			_ = tx.AddError(errors.New("db down"))
		}
	}))
	svc := NewService(db, NewProviderRegistry())
	outcome, err := svc.processCreditNoteJob(context.Background(), job, jobCtx, &stubProvider{})
	require.NoError(t, err)
	require.True(t, outcome.Retryable)
	require.Empty(t, outcome.TerminalStatus)
	require.Equal(t, "mark_original_credited_failed", outcome.ErrorCode)

	require.NoError(t, db.Callback().Update().Remove("test:fail_receipt_update"))
	var orig database.FiscalReceipt
	require.NoError(t, db.First(&orig, original.ID).Error)
	require.Equal(t, database.FiscalStatusAuthorized, orig.Status)
}
