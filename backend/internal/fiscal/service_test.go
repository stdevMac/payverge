package fiscal

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newSelfSignedPEMStrings generates an RSA self-signed cert+key and returns them as strings.
func newSelfSignedPEMStrings(t *testing.T) (certPEM, keyPEM string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "fiscal-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	certBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return string(certBytes), string(keyBytes)
}

func seedSettings(t *testing.T, db *gorm.DB, s *database.BusinessFiscalSettings) {
	t.Helper()
	require.NoError(t, db.Create(s).Error)
}

func TestServiceHandleBillPaid_DoesNothingWhenSettingsMissing(t *testing.T) {
	db := newFiscalTestDB(t)
	svc := NewService(db, NewProviderRegistry())

	err := svc.HandleBillPaid(context.Background(), BillPaidInput{
		BillID: 1,
		Actor:  "system",
	})
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Model(&database.FiscalJob{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestServiceHandleBillPaid_EnqueuesAutomaticNonBlocking(t *testing.T) {
	db := newFiscalTestDB(t)
	createFiscalSettings(t, db, 1, database.FiscalModeAutomaticNonBlocking)
	createFiscalBill(t, db, 7, 1, database.BillStatusPaid, 1200)

	svc := NewService(db, NewProviderRegistry())
	err := svc.HandleBillPaid(context.Background(), BillPaidInput{
		BillID: 7,
		Actor:  "system",
	})
	require.NoError(t, err)

	var jobs []database.FiscalJob
	require.NoError(t, db.Find(&jobs).Error)
	require.Len(t, jobs, 1)
	require.Equal(t, database.FiscalStatusPending, jobs[0].Status)
	require.Contains(t, jobs[0].IdempotencyKey, "bill:7")
}

func TestServiceHandleBillPaid_DoesNothingWhenModeOff(t *testing.T) {
	db := newFiscalTestDB(t)
	createFiscalSettings(t, db, 3, database.FiscalModeOff)
	createFiscalBill(t, db, 9, 3, database.BillStatusPaid, 1200)

	svc := NewService(db, NewProviderRegistry())
	err := svc.HandleBillPaid(context.Background(), BillPaidInput{
		BillID: 9,
		Actor:  "system",
	})
	require.NoError(t, err)

	require.Zero(t, countFiscalJobs(t, db))
}

func TestServiceHandleBillPaid_DoesNothingWhenModeManual(t *testing.T) {
	db := newFiscalTestDB(t)
	createFiscalSettings(t, db, 4, database.FiscalModeManual)
	createFiscalBill(t, db, 10, 4, database.BillStatusPaid, 1200)

	svc := NewService(db, NewProviderRegistry())
	err := svc.HandleBillPaid(context.Background(), BillPaidInput{
		BillID: 10,
		Actor:  "system",
	})
	require.NoError(t, err)

	require.Zero(t, countFiscalJobs(t, db))
}

func TestServiceHandleBillPaid_DoesNothingWhenBillUnpaid(t *testing.T) {
	db := newFiscalTestDB(t)
	createFiscalSettings(t, db, 6, database.FiscalModeAutomaticNonBlocking)
	createFiscalBill(t, db, 12, 6, database.BillStatusOpen, 0)

	svc := NewService(db, NewProviderRegistry())
	err := svc.HandleBillPaid(context.Background(), BillPaidInput{
		BillID: 12,
		Actor:  "system",
	})
	require.NoError(t, err)

	require.Zero(t, countFiscalJobs(t, db))
}

// TestServiceHandleBillPaid_SkipsNonRealBusiness locks Task 15: automatic
// enqueue must not feed demo/test businesses into the production fiscal queue.
func TestServiceHandleBillPaid_SkipsNonRealBusiness(t *testing.T) {
	db := newFiscalTestDB(t)

	require.NoError(t, db.Create(&database.Business{
		ID: 20, BusinessId: "demo-20", Name: "Demo Lounge",
		Kind: database.BusinessKindDemo, IsDemo: true,
	}).Error)
	createFiscalSettings(t, db, 20, database.FiscalModeAutomaticNonBlocking)
	require.NoError(t, db.Create(&database.Bill{
		ID: 200, BusinessID: 20, BillNumber: "PV-demo-200",
		Status: database.BillStatusPaid, Items: "[]",
		TotalAmount: 5000, PaidAmount: 5000,
	}).Error)

	svc := NewService(db, NewProviderRegistry())
	require.NoError(t, svc.HandleBillPaid(context.Background(), BillPaidInput{
		BillID: 200,
		Actor:  "system",
	}))
	require.Zero(t, countFiscalJobs(t, db), "kind=demo must not enqueue AFIP jobs")

	// kind=test is also excluded.
	require.NoError(t, db.Create(&database.Business{
		ID: 21, BusinessId: "test-21", Name: "CI Fixture",
		Kind: database.BusinessKindTest, IsDemo: false,
	}).Error)
	createFiscalSettings(t, db, 21, database.FiscalModeAutomaticNonBlocking)
	require.NoError(t, db.Create(&database.Bill{
		ID: 201, BusinessID: 21, BillNumber: "PV-test-201",
		Status: database.BillStatusPaid, Items: "[]",
		TotalAmount: 5000, PaidAmount: 5000,
	}).Error)
	require.NoError(t, svc.HandleBillPaid(context.Background(), BillPaidInput{
		BillID: 201,
		Actor:  "system",
	}))
	require.Zero(t, countFiscalJobs(t, db), "kind=test must not enqueue AFIP jobs")
}

func TestServiceHandleBillPaid_DuplicateCallsCreateOneJob(t *testing.T) {
	db := newFiscalTestDB(t)
	createFiscalSettings(t, db, 7, database.FiscalModeAutomaticNonBlocking)
	createFiscalBill(t, db, 13, 7, database.BillStatusPaid, 1200)

	svc := NewService(db, NewProviderRegistry())
	input := BillPaidInput{
		BillID: 13,
		Actor:  "system",
	}
	require.NoError(t, svc.HandleBillPaid(context.Background(), input))
	require.NoError(t, svc.HandleBillPaid(context.Background(), input))

	require.Equal(t, int64(1), countFiscalJobs(t, db))
}

// TestServiceHandleBillPaid_PerBillIssueKeyDedupesAcrossPaths locks F-DUPISSUE:
// the issue key is per-bill (no payment dimension), so the auto path (with a
// concrete paymentID) and the manual operator path (IssueReceipt, nil/nil) for the
// SAME bill collapse to exactly one issue job — never two full-bill facturas.
func TestServiceHandleBillPaid_PerBillIssueKeyDedupesAcrossPaths(t *testing.T) {
	db := newFiscalTestDB(t)
	createFiscalSettings(t, db, 8, database.FiscalModeAutomaticNonBlocking)
	createFiscalBill(t, db, 14, 8, database.BillStatusPaid, 1200)
	paymentID := uint(21)
	alternativePaymentID := uint(34)

	svc := NewService(db, NewProviderRegistry())

	// Auto path: paid bill with a concrete payment.
	require.NoError(t, svc.HandleBillPaid(context.Background(), BillPaidInput{
		BillID:               14,
		PaymentID:            &paymentID,
		AlternativePaymentID: &alternativePaymentID,
		Actor:                "system",
	}))

	var job database.FiscalJob
	require.NoError(t, db.First(&job).Error)
	require.Equal(t, "business:8:bill:14:action:issue_receipt:split:none", job.IdempotencyKey)

	// Manual operator path for the same bill must NOT create a second factura job.
	require.NoError(t, svc.IssueReceipt(context.Background(), 8, 14, "manager"))
	require.Equal(t, int64(1), countFiscalJobs(t, db))
}

func TestServiceIssueReceipt_EnqueuesManualMode(t *testing.T) {
	db := newFiscalTestDB(t)
	createFiscalSettings(t, db, 2, database.FiscalModeManual)
	createFiscalBill(t, db, 8, 2, database.BillStatusPaid, 2400)

	svc := NewService(db, NewProviderRegistry())
	err := svc.IssueReceipt(context.Background(), 2, 8, "manager")
	require.NoError(t, err)

	var job database.FiscalJob
	require.NoError(t, db.First(&job).Error)
	require.Equal(t, "manager", job.CreatedBy)
	require.Equal(t, database.FiscalStatusPending, job.Status)
}

func TestServiceIssueReceipt_DoesNothingWhenBillUnpaid(t *testing.T) {
	db := newFiscalTestDB(t)
	createFiscalSettings(t, db, 15, database.FiscalModeManual)
	createFiscalBill(t, db, 15, 15, database.BillStatusOpen, 0)

	svc := NewService(db, NewProviderRegistry())
	err := svc.IssueReceipt(context.Background(), 15, 15, "manager")
	require.NoError(t, err)

	require.Zero(t, countFiscalJobs(t, db))
}

func TestServiceIssueReceipt_DoesNotEnqueueForDifferentBusiness(t *testing.T) {
	db := newFiscalTestDB(t)
	createFiscalSettings(t, db, 1, database.FiscalModeManual)
	createFiscalSettings(t, db, 2, database.FiscalModeManual)
	createFiscalBill(t, db, 21, 2, database.BillStatusPaid, 2400)

	svc := NewService(db, NewProviderRegistry())
	err := svc.IssueReceipt(context.Background(), 1, 21, "manager")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.Zero(t, countFiscalJobs(t, db))
}

func TestServiceIssueReceipt_EnqueuesAutomaticModes(t *testing.T) {
	tests := []struct {
		name string
		mode database.FiscalMode
	}{
		{name: "non blocking", mode: database.FiscalModeAutomaticNonBlocking},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newFiscalTestDB(t)
			businessID := uint(16 + i)
			billID := uint(16 + i)
			createFiscalSettings(t, db, businessID, tt.mode)
			createFiscalBill(t, db, billID, businessID, database.BillStatusPaid, 1200)

			svc := NewService(db, NewProviderRegistry())
			err := svc.IssueReceipt(context.Background(), businessID, billID, "manager")
			require.NoError(t, err)

			require.Equal(t, int64(1), countFiscalJobs(t, db))
		})
	}
}

// #907: the invoice picker deliberately keeps already-invoiced bills visible and
// marks them with existing_receipt_id (#260) — the drawer swaps Issue for "View
// invoice" on those rows. But the manual bill-number field bypasses the picker
// entirely, and the per-bill idempotency key then swallowed the duplicate
// silently: CreateJobIfNotExists returned the existing job, enqueueIssueJob
// returned nil, and the operator was told "invoice issue queued" for work that
// never happened. Refuse the re-issue out loud instead.
func TestServiceIssueReceipt_RefusesBillThatAlreadyHasLiveReceipt(t *testing.T) {
	db := newFiscalTestDB(t)
	createFiscalSettings(t, db, 3, database.FiscalModeManual)
	createFiscalBill(t, db, 33, 3, database.BillStatusPaid, 2400)
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 71, BusinessID: 3, SettingsID: 1, BillID: 33,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_b", TotalAmountCents: 2400, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}).Error)

	svc := NewService(db, NewProviderRegistry())
	err := svc.IssueReceipt(context.Background(), 3, 33, "manager")
	require.ErrorIs(t, err, ErrReceiptAlreadyIssued)
	require.Zero(t, countFiscalJobs(t, db))

	// The refusal and the picker's badge are the same rule: whatever the list
	// marks with existing_receipt_id is exactly what the write path refuses.
	rows, listErr := NewRepository(db).ListIssuableBills(3, "", 20)
	require.NoError(t, listErr)
	require.Len(t, rows, 1)
	require.Equal(t, uint(33), rows[0].BillID)
	require.NotNil(t, rows[0].ExistingReceiptID,
		"the bill the write path refuses must be the bill the list flags")
	require.Equal(t, uint(71), *rows[0].ExistingReceiptID)
}

// The guard blocks on live receipts only. A bill whose issue attempt died is
// genuinely still issuable — the picker leaves existing_receipt_id nil for it,
// and the manual issue must keep working (it is how an operator recovers).
func TestServiceIssueReceipt_StillIssuableAfterFailedReceipt(t *testing.T) {
	db := newFiscalTestDB(t)
	createFiscalSettings(t, db, 4, database.FiscalModeManual)
	createFiscalBill(t, db, 44, 4, database.BillStatusPaid, 2400)
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 72, BusinessID: 4, SettingsID: 1, BillID: 44,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_b", TotalAmountCents: 2400, Currency: "ARS",
		Status: database.FiscalStatusFailedPermanent,
	}).Error)

	svc := NewService(db, NewProviderRegistry())
	require.NoError(t, svc.IssueReceipt(context.Background(), 4, 44, "manager"))
	require.Equal(t, int64(1), countFiscalJobs(t, db))

	rows, listErr := NewRepository(db).ListIssuableBills(4, "", 20)
	require.NoError(t, listErr)
	require.Len(t, rows, 1)
	require.Nil(t, rows[0].ExistingReceiptID)
}

// Another business's receipt on the same bill number must not block this
// business's issue — the guard is tenant-scoped like every other fiscal read.
func TestServiceIssueReceipt_ForeignReceiptDoesNotBlock(t *testing.T) {
	db := newFiscalTestDB(t)
	createFiscalSettings(t, db, 5, database.FiscalModeManual)
	createFiscalBill(t, db, 55, 5, database.BillStatusPaid, 2400)
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID: 73, BusinessID: 6, SettingsID: 1, BillID: 55,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_b", TotalAmountCents: 2400, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}).Error)

	svc := NewService(db, NewProviderRegistry())
	require.NoError(t, svc.IssueReceipt(context.Background(), 5, 55, "manager"))
	require.Equal(t, int64(1), countFiscalJobs(t, db))
}

func TestServiceRetryReceipt_DoesNotEnqueueForDifferentBusiness(t *testing.T) {
	db := newFiscalTestDB(t)
	createFiscalReceipt(t, db, 31, 2, 3, 33)

	svc := NewService(db, NewProviderRegistry())
	err := svc.RetryReceipt(context.Background(), 1, 31, "manager")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.Zero(t, countFiscalJobs(t, db))
}

func TestServiceRetryReceipt_CreatesPendingJobForOwnedReceipt(t *testing.T) {
	db := newFiscalTestDB(t)
	createFiscalReceipt(t, db, 32, 4, 5, 44)

	svc := NewService(db, NewProviderRegistry())
	err := svc.RetryReceipt(context.Background(), 4, 32, "manager")
	require.NoError(t, err)

	var job database.FiscalJob
	require.NoError(t, db.First(&job).Error)
	require.Equal(t, uint(4), job.BusinessID)
	require.Equal(t, uint(5), job.SettingsID)
	require.NotNil(t, job.ReceiptID)
	require.Equal(t, uint(32), *job.ReceiptID)
	require.Equal(t, uint(44), job.BillID)
	require.Equal(t, ActionStatusCheck, job.Action)
	require.Equal(t, "business:4:receipt:32:action:retry", job.IdempotencyKey)
	require.Equal(t, database.FiscalStatusPending, job.Status)
	require.Equal(t, "manager", job.CreatedBy)
}

func TestServiceRetryReceipt_RejectsNonRetryableReceiptStatus(t *testing.T) {
	db := newFiscalTestDB(t)
	createFiscalReceiptWithStatus(t, db, 33, 4, 5, 44, database.FiscalStatusAuthorized)

	svc := NewService(db, NewProviderRegistry())
	err := svc.RetryReceipt(context.Background(), 4, 33, "manager")
	require.ErrorIs(t, err, ErrReceiptNotRetryable)
	require.Zero(t, countFiscalJobs(t, db))
}

func TestServiceRetryReceipt_ResetsTerminalRetryJob(t *testing.T) {
	db := newFiscalTestDB(t)
	createFiscalReceipt(t, db, 34, 4, 5, 44)
	receiptID := uint(34)
	lastError := "provider unavailable"
	require.NoError(t, db.Create(&database.FiscalJob{
		BusinessID:       4,
		SettingsID:       5,
		ReceiptID:        &receiptID,
		BillID:           44,
		Action:           ActionStatusCheck,
		IdempotencyKey:   "business:4:receipt:34:action:retry",
		Status:           database.FiscalStatusFailedRetryable,
		Attempts:         3,
		MaxAttempts:      5,
		LastErrorCode:    stringPtr("TEMPORARY"),
		LastErrorMessage: &lastError,
		CreatedBy:        "system",
	}).Error)

	svc := NewService(db, NewProviderRegistry())
	err := svc.RetryReceipt(context.Background(), 4, 34, "manager")
	require.NoError(t, err)

	var jobs []database.FiscalJob
	require.NoError(t, db.Find(&jobs).Error)
	require.Len(t, jobs, 1)
	require.Equal(t, database.FiscalStatusPending, jobs[0].Status)
	require.Zero(t, jobs[0].Attempts)
	require.Nil(t, jobs[0].LastErrorCode)
	require.Nil(t, jobs[0].LastErrorMessage)
	require.Equal(t, "manager", jobs[0].CreatedBy)
}

func TestServiceIssueCreditNote_CreatesOneJobAndIsIdempotent(t *testing.T) {
	db := newFiscalTestDB(t)
	// Authorized issue receipt is the canonical credit-note target.
	createFiscalReceiptWithStatus(t, db, 40, 4, 5, 44, database.FiscalStatusAuthorized)

	svc := NewService(db, NewProviderRegistry())
	require.NoError(t, svc.IssueCreditNote(context.Background(), 4, 40, 0, "", "customer requested refund", "manager"))

	var jobs []database.FiscalJob
	require.NoError(t, db.Find(&jobs).Error)
	require.Len(t, jobs, 1)
	job := jobs[0]
	require.Equal(t, uint(4), job.BusinessID)
	require.Equal(t, uint(5), job.SettingsID)
	require.NotNil(t, job.ReceiptID)
	require.Equal(t, uint(40), *job.ReceiptID)
	require.Equal(t, uint(44), job.BillID)
	require.Equal(t, ActionCreditNote, job.Action)
	require.Equal(t, "business:4:receipt:40:action:credit_note", job.IdempotencyKey)
	require.Equal(t, database.FiscalStatusPending, job.Status)
	require.Equal(t, 5, job.MaxAttempts)
	require.Equal(t, "manager", job.CreatedBy)

	// Second call is a no-op: still exactly one job.
	require.NoError(t, svc.IssueCreditNote(context.Background(), 4, 40, 0, "", "customer requested refund", "manager"))
	require.Equal(t, int64(1), countFiscalJobs(t, db))
}

func TestServiceIssueCreditNote_RejectsNonAuthorizedReceipt(t *testing.T) {
	db := newFiscalTestDB(t)
	// A still-pending issue receipt is not yet creditable.
	createFiscalReceiptWithStatus(t, db, 41, 4, 5, 44, database.FiscalStatusFailedRetryable)

	svc := NewService(db, NewProviderRegistry())
	err := svc.IssueCreditNote(context.Background(), 4, 41, 0, "", "reason", "manager")
	require.ErrorIs(t, err, ErrReceiptNotCreditable)
	require.Zero(t, countFiscalJobs(t, db))
}

func TestServiceIssueCreditNote_RejectsCreditNoteReceipt(t *testing.T) {
	db := newFiscalTestDB(t)
	// An authorized receipt that is itself a credit note must not be credited again.
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID:               42,
		BusinessID:       4,
		SettingsID:       5,
		BillID:           44,
		Country:          "AR",
		Provider:         "arca",
		Action:           ActionCreditNote,
		ReceiptType:      "B",
		TotalAmountCents: 2400,
		Currency:         "ARS",
		Status:           database.FiscalStatusAuthorized,
	}).Error)

	svc := NewService(db, NewProviderRegistry())
	err := svc.IssueCreditNote(context.Background(), 4, 42, 0, "", "reason", "manager")
	require.ErrorIs(t, err, ErrReceiptNotCreditable)
	require.Zero(t, countFiscalJobs(t, db))
}

func TestServiceIssueCreditNote_DoesNotCreditForDifferentBusiness(t *testing.T) {
	db := newFiscalTestDB(t)
	createFiscalReceiptWithStatus(t, db, 43, 2, 5, 44, database.FiscalStatusAuthorized)

	svc := NewService(db, NewProviderRegistry())
	err := svc.IssueCreditNote(context.Background(), 1, 43, 0, "", "reason", "manager")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.Zero(t, countFiscalJobs(t, db))
}

func TestServiceUpdateSettings_NormalizesDefaultsAndRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name     string
		settings database.BusinessFiscalSettings
		wantErr  bool
	}{
		{
			name: "normalizes and defaults environment",
			settings: database.BusinessFiscalSettings{
				BusinessID: 1,
				Country:    " ar ",
				Provider:   " ARCA ",
				Mode:       database.FiscalModeManual,
			},
		},
		{
			name: "rejects country",
			settings: database.BusinessFiscalSettings{
				BusinessID: 1,
				Country:    "US",
				Provider:   "arca",
				Mode:       database.FiscalModeManual,
			},
			wantErr: true,
		},
		{
			name: "rejects provider",
			settings: database.BusinessFiscalSettings{
				BusinessID: 1,
				Country:    "AR",
				Provider:   "unknown",
				Mode:       database.FiscalModeManual,
			},
			wantErr: true,
		},
		{
			name: "rejects arca for ae",
			settings: database.BusinessFiscalSettings{
				BusinessID: 1,
				Country:    "AE",
				Provider:   "arca",
				Mode:       database.FiscalModeManual,
			},
			wantErr: true,
		},
		{
			name: "rejects edicom for ar",
			settings: database.BusinessFiscalSettings{
				BusinessID: 1,
				Country:    "AR",
				Provider:   "edicom",
				Mode:       database.FiscalModeManual,
			},
			wantErr: true,
		},
		{
			name: "rejects environment",
			settings: database.BusinessFiscalSettings{
				BusinessID:  1,
				Country:     "AR",
				Provider:    "arca",
				Mode:        database.FiscalModeManual,
				Environment: "staging",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newFiscalTestDB(t)
			svc := NewService(db, NewProviderRegistry())
			settings := tt.settings

			err := svc.UpdateSettings(context.Background(), &settings)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrInvalidSettings)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "AR", settings.Country)
			require.Equal(t, "arca", settings.Provider)
			require.Equal(t, "sandbox", settings.Environment)
		})
	}
}

func createFiscalSettings(t *testing.T, db interface {
	Create(value interface{}) *gorm.DB
}, businessID uint, mode database.FiscalMode) {
	t.Helper()
	require.NoError(t, db.Create(&database.BusinessFiscalSettings{
		BusinessID:  businessID,
		Country:     "AR",
		Provider:    "arca",
		Mode:        mode,
		Environment: "sandbox",
		SetupStatus: "valid",
	}).Error)
}

func createFiscalBill(t *testing.T, db interface {
	Create(value interface{}) *gorm.DB
}, id, businessID uint, status database.BillStatus, paidAmount int64) {
	t.Helper()
	require.NoError(t, db.Create(&database.Bill{
		ID:          id,
		BusinessID:  businessID,
		BillNumber:  "PV-test",
		Status:      status,
		Items:       "[]",
		TotalAmount: paidAmount,
		PaidAmount:  paidAmount,
	}).Error)
}

func createFiscalReceipt(t *testing.T, db interface {
	Create(value interface{}) *gorm.DB
}, id, businessID, settingsID, billID uint) {
	t.Helper()
	createFiscalReceiptWithStatus(t, db, id, businessID, settingsID, billID, database.FiscalStatusFailedRetryable)
}

func createFiscalReceiptWithStatus(t *testing.T, db interface {
	Create(value interface{}) *gorm.DB
}, id, businessID, settingsID, billID uint, status database.FiscalStatus) {
	t.Helper()
	require.NoError(t, db.Create(&database.FiscalReceipt{
		ID:               id,
		BusinessID:       businessID,
		SettingsID:       settingsID,
		BillID:           billID,
		Country:          "AR",
		Provider:         "arca",
		Action:           ActionIssueReceipt,
		ReceiptType:      "B",
		TotalAmountCents: 2400,
		Currency:         "ARS",
		Status:           status,
	}).Error)
}

func stringPtr(s string) *string {
	return &s
}

func countFiscalJobs(t *testing.T, db interface {
	Model(value interface{}) *gorm.DB
}) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&database.FiscalJob{}).Count(&count).Error)
	return count
}

func TestServiceSetCredentials_PersistsBundle(t *testing.T) {
	db := newFiscalTestDB(t)
	svc := NewService(db, NewProviderRegistry())
	seedSettings(t, db, &database.BusinessFiscalSettings{
		BusinessID:  1,
		Country:     "AR",
		Provider:    "arca",
		Mode:        database.FiscalModeManual,
		Environment: "sandbox",
		SetupStatus: "draft",
	})
	certPEM, keyPEM := newSelfSignedPEMStrings(t)
	if err := svc.SetCredentials(context.Background(), 1, certPEM, keyPEM); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetSettings(context.Background(), 1)
	require.NoError(t, err)
	if len(got.CredentialsEncrypted) == 0 || got.CredentialsFingerprint == "" || got.CredentialsExpiresAt == nil {
		t.Fatalf("credentials not persisted: %+v", got)
	}
	require.Equal(t, "credentials_set", got.SetupStatus)
}

func TestServiceSetCredentials_RejectsMismatchedPair(t *testing.T) {
	db := newFiscalTestDB(t)
	svc := NewService(db, NewProviderRegistry())
	seedSettings(t, db, &database.BusinessFiscalSettings{
		BusinessID:  2,
		Country:     "AR",
		Provider:    "arca",
		Mode:        database.FiscalModeManual,
		Environment: "sandbox",
		SetupStatus: "draft",
	})
	certPEM, _ := newSelfSignedPEMStrings(t)
	_, otherKey := newSelfSignedPEMStrings(t)
	err := svc.SetCredentials(context.Background(), 2, certPEM, otherKey)
	require.Error(t, err, "want error when cert/key do not match")

	// Credentials must NOT be persisted on error.
	got, gerr := svc.GetSettings(context.Background(), 2)
	require.NoError(t, gerr)
	require.Empty(t, got.CredentialsEncrypted, "credentials must not be stored on mismatch")
}

func TestServiceValidateSettings_FlipsToReadyOnSuccess(t *testing.T) {
	db := newFiscalTestDB(t)
	seedSettings(t, db, &database.BusinessFiscalSettings{
		BusinessID:  1,
		Country:     "AR",
		Provider:    "arca",
		Mode:        database.FiscalModeManual,
		Environment: "sandbox",
		SetupStatus: "credentials_set",
	})

	fixed := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	svc := NewService(db, NewProviderRegistry()).
		WithProviderFactory(&fakeFactory{provider: &stubProvider{}}).
		WithClock(func() time.Time { return fixed })

	got, err := svc.ValidateSettings(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "ready", got.SetupStatus)
	require.NotNil(t, got.LastValidatedAt)
	require.Nil(t, got.LastValidationError)

	// Reload from DB to confirm it was persisted.
	reloaded, err := svc.GetSettings(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "ready", reloaded.SetupStatus)
	require.NotNil(t, reloaded.LastValidatedAt)
	require.Equal(t, fixed.Unix(), reloaded.LastValidatedAt.Unix())
	require.Nil(t, reloaded.LastValidationError)
}

func TestServiceValidateSettings_RecordsErrorAndLeavesStatusOnFailure(t *testing.T) {
	db := newFiscalTestDB(t)
	seedSettings(t, db, &database.BusinessFiscalSettings{
		BusinessID:  2,
		Country:     "AR",
		Provider:    "arca",
		Mode:        database.FiscalModeManual,
		Environment: "sandbox",
		SetupStatus: "credentials_set",
	})

	validationErr := errors.New("wsfe FEDummy: AppServer down")
	provider := &stubProvider{ValidateFn: func(ctx context.Context, s Settings) error { return validationErr }}
	svc := NewService(db, NewProviderRegistry()).
		WithProviderFactory(&fakeFactory{provider: provider})

	got, err := svc.ValidateSettings(context.Background(), 2)
	require.Error(t, err)
	require.Equal(t, validationErr.Error(), err.Error())
	// Settings still returned so the handler can surface the failure inline.
	require.NotNil(t, got)
	require.Equal(t, "credentials_set", got.SetupStatus)

	reloaded, gerr := svc.GetSettings(context.Background(), 2)
	require.NoError(t, gerr)
	require.Equal(t, "credentials_set", reloaded.SetupStatus, "status must not flip to ready on failure")
	require.Nil(t, reloaded.LastValidatedAt, "LastValidatedAt unchanged on failure")
	require.NotNil(t, reloaded.LastValidationError)
	require.Equal(t, validationErr.Error(), *reloaded.LastValidationError)
}

func TestServiceValidateSettings_NoSettingsReturnsError(t *testing.T) {
	db := newFiscalTestDB(t)
	svc := NewService(db, NewProviderRegistry()).
		WithProviderFactory(&fakeFactory{provider: &stubProvider{}})

	_, err := svc.ValidateSettings(context.Background(), 99)
	require.ErrorIs(t, err, ErrNoFiscalSettings)
}

func TestServiceValidateSettings_NilFactoryReturnsError(t *testing.T) {
	db := newFiscalTestDB(t)
	seedSettings(t, db, &database.BusinessFiscalSettings{
		BusinessID:  3,
		Country:     "AR",
		Provider:    "arca",
		Mode:        database.FiscalModeManual,
		Environment: "sandbox",
		SetupStatus: "credentials_set",
	})
	svc := NewService(db, NewProviderRegistry())

	_, err := svc.ValidateSettings(context.Background(), 3)
	require.Error(t, err)
	require.Contains(t, err.Error(), "factory")
}

func TestServiceSetCredentials_NoSettingsReturnsError(t *testing.T) {
	db := newFiscalTestDB(t)
	svc := NewService(db, NewProviderRegistry())
	// No fiscal settings seeded for businessID=99.
	certPEM, keyPEM := newSelfSignedPEMStrings(t)
	err := svc.SetCredentials(context.Background(), 99, certPEM, keyPEM)
	require.ErrorIs(t, err, ErrNoFiscalSettings, "want ErrNoFiscalSettings when no settings row exists")

	// No settings row should have been created.
	got, gerr := svc.GetSettings(context.Background(), 99)
	require.NoError(t, gerr)
	require.Nil(t, got, "no settings row should exist for business 99")
}

// TestManualIssueRevivesFailedPermanentJob locks Task 13a: an operator clicking
// "issue receipt" after the worker exhausted retries must revive the dead job
// (pending, attempts=0) rather than silently no-op via CreateJobIfNotExists.
func TestManualIssueRevivesFailedPermanentJob(t *testing.T) {
	db := newFiscalTestDB(t)
	svc := NewService(db, NewProviderRegistry())
	createFiscalSettings(t, db, 1, database.FiscalModeAutomaticNonBlocking)
	createFiscalBill(t, db, 7, 1, database.BillStatusPaid, 1200)

	// First manual issue creates the job.
	require.NoError(t, svc.IssueReceipt(context.Background(), 1, 7, "owner"))

	// Simulate the worker exhausting retries.
	require.NoError(t, db.Model(&database.FiscalJob{}).Where("bill_id = ?", 7).
		Updates(map[string]interface{}{
			"status":   database.FiscalStatusFailedPermanent,
			"attempts": 5,
		}).Error)

	// Operator clicks "issue receipt" again: the dead job must revive.
	require.NoError(t, svc.IssueReceipt(context.Background(), 1, 7, "owner"))

	var job database.FiscalJob
	require.NoError(t, db.Where("bill_id = ?", 7).First(&job).Error)
	require.Equal(t, database.FiscalStatusPending, job.Status,
		"manual issue must revive a failed_permanent job")
	require.Equal(t, 0, job.Attempts)
}
