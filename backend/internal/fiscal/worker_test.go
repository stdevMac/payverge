package fiscal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/runtimecontrol"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// stubProvider is a fiscal.Provider whose method behavior is injectable per-test.
type stubProvider struct {
	country    string
	name       string
	IssueFn    func(ctx context.Context, in IssueInput) (*ReceiptResult, error)
	CreditFn   func(ctx context.Context, in CreditNoteInput) (*ReceiptResult, error)
	StatusFn   func(ctx context.Context, providerReceiptID string) (*ReceiptStatus, error)
	ValidateFn func(ctx context.Context, settings Settings) error
}

func (f *stubProvider) Country() string {
	if f.country == "" {
		return "AR"
	}
	return f.country
}

func (f *stubProvider) Name() string {
	if f.name == "" {
		return "arca"
	}
	return f.name
}

func (f *stubProvider) ValidateSettings(ctx context.Context, settings Settings) error {
	if f.ValidateFn != nil {
		return f.ValidateFn(ctx, settings)
	}
	return nil
}

func (f *stubProvider) IssueReceipt(ctx context.Context, in IssueInput) (*ReceiptResult, error) {
	if f.IssueFn != nil {
		return f.IssueFn(ctx, in)
	}
	return &ReceiptResult{Status: StatusAuthorized}, nil
}

func (f *stubProvider) IssueCreditNote(ctx context.Context, in CreditNoteInput) (*ReceiptResult, error) {
	if f.CreditFn != nil {
		return f.CreditFn(ctx, in)
	}
	return &ReceiptResult{Status: StatusAuthorized}, nil
}

func (f *stubProvider) GetStatus(ctx context.Context, providerReceiptID string) (*ReceiptStatus, error) {
	if f.StatusFn != nil {
		return f.StatusFn(ctx, providerReceiptID)
	}
	return &ReceiptStatus{Status: StatusAuthorized}, nil
}

// fakeFactory returns a fixed provider regardless of settings, so the worker can
// resolve the per-business provider without real credentials.
type fakeFactory struct {
	provider Provider
	err      error
}

func (f *fakeFactory) Build(ctx context.Context, settings *database.BusinessFiscalSettings) (Provider, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.provider, nil
}

func newTestFiscalWorker(t *testing.T) (*FiscalWorker, *gorm.DB, *stubProvider) {
	t.Helper()
	db := newFiscalTestDB(t)
	prov := &stubProvider{}
	w := NewFiscalWorker(db, &fakeFactory{provider: prov})
	w.WorkerID = "test-worker"
	return w, db, prov
}

func seedPaidBill(t *testing.T, db *gorm.DB, businessID, billID uint, totalCents int64) {
	t.Helper()
	require.NoError(t, db.Create(&database.Business{
		ID:             businessID,
		BusinessId:     "biz-" + strings.Repeat("x", int(businessID)),
		OwnerAddress:   "owner",
		Name:           "Test Biz",
		SettlementAddr: "settle",
		TippingAddr:    "tip",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID:          billID,
		BusinessID:  businessID,
		BillNumber:  "PV-worker-test",
		Status:      database.BillStatusPaid,
		Items:       "[]",
		TotalAmount: totalCents,
		PaidAmount:  totalCents,
	}).Error)
}

// seedReadySettings creates fiscal settings in automatic_non_blocking mode with a
// point of sale and tax condition that the receipt-type resolver can use. The
// worker resolves its provider via the injected factory, so no real creds are
// required here.
func seedReadySettings(t *testing.T, db *gorm.DB, businessID uint) uint {
	t.Helper()
	pos := 3
	s := &database.BusinessFiscalSettings{
		BusinessID:   businessID,
		Country:      "AR",
		Provider:     "arca",
		Mode:         database.FiscalModeAutomaticNonBlocking,
		Environment:  "sandbox",
		TaxID:        "20123456789",
		TaxCondition: "monotributo",
		PointOfSale:  &pos,
		SetupStatus:  "ready",
	}
	require.NoError(t, db.Create(s).Error)
	return s.ID
}

func enqueueIssueJob(t *testing.T, db *gorm.DB, businessID, billID uint) uint {
	return enqueueJob(t, db, businessID, billID, ActionIssueReceipt, nil)
}

func enqueueJob(t *testing.T, db *gorm.DB, businessID, billID uint, action string, receiptID *uint) uint {
	t.Helper()
	var settings database.BusinessFiscalSettings
	require.NoError(t, db.Where("business_id = ?", businessID).First(&settings).Error)
	job := &database.FiscalJob{
		BusinessID:     businessID,
		SettingsID:     settings.ID,
		BillID:         billID,
		ReceiptID:      receiptID,
		Action:         action,
		IdempotencyKey: action + ":biz:" + strings.Repeat("k", int(businessID)) + ":bill:" + strings.Repeat("b", int(billID)),
		Status:         database.FiscalStatusPending,
		MaxAttempts:    5,
		CreatedBy:      "system",
	}
	require.NoError(t, db.Create(job).Error)
	return job.ID
}

func TestFiscalWorkerIssuesReceipt(t *testing.T) {
	w, db, prov := newTestFiscalWorker(t)
	prov.IssueFn = func(ctx context.Context, in IssueInput) (*ReceiptResult, error) {
		exp := time.Now().Add(240 * time.Hour)
		return &ReceiptResult{
			Status:            StatusAuthorized,
			ProviderReceiptID: "71000000000001",
			AuthCode:          "71000000000001",
			AuthExpiresAt:     &exp,
			ReceiptNumber:     "43",
			QRPayload:         "https://www.afip.gob.ar/fe/qr/?p=abc",
			ReceiptType:       "factura_c",
		}, nil
	}
	seedPaidBill(t, db, 1, 1, 12100)
	seedReadySettings(t, db, 1)
	enqueueIssueJob(t, db, 1, 1)

	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	var r database.FiscalReceipt
	require.NoError(t, db.Where("bill_id = ?", 1).First(&r).Error)
	require.Equal(t, database.FiscalStatusAuthorized, r.Status)
	require.NotNil(t, r.AuthCode)
	require.Equal(t, "71000000000001", *r.AuthCode)
	require.NotNil(t, r.QRPayload)
	require.Equal(t, "factura_c", r.ReceiptType)
	require.Equal(t, ActionIssueReceipt, r.Action)
	require.Equal(t, int64(12100), r.TotalAmountCents)

	var job database.FiscalJob
	require.NoError(t, db.Where("bill_id = ?", 1).First(&job).Error)
	require.Equal(t, database.FiscalStatusAuthorized, job.Status)
	require.Nil(t, job.LockedAt)
	require.Nil(t, job.LockedBy)
	require.NotNil(t, job.ReceiptID)
	require.Equal(t, r.ID, *job.ReceiptID)

	var audits []database.FiscalAuditEvent
	require.NoError(t, db.Find(&audits).Error)
	require.Len(t, audits, 1)
	require.Equal(t, "fiscal_receipt_authorized", audits[0].EventType)
}

func TestFiscalWorkerDisabledLeavesQueuedJobsUnclaimed(t *testing.T) {
	w, db, _ := newTestFiscalWorker(t)
	seedPaidBill(t, db, 81, 81, 12100)
	seedReadySettings(t, db, 81)
	jobID := enqueueIssueJob(t, db, 81, 81)
	require.NoError(t, db.Model(&runtimecontrol.Control{}).
		Where("key = ?", runtimecontrol.ControlFiscal).
		Update("enabled", false).Error)

	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Zero(t, n)

	var job database.FiscalJob
	require.NoError(t, db.First(&job, jobID).Error)
	require.Equal(t, database.FiscalStatusPending, job.Status)
	require.Zero(t, job.Attempts)
	require.Empty(t, job.LockedBy)
}

func TestFiscalWorkerTransientErrorRetries(t *testing.T) {
	w, db, prov := newTestFiscalWorker(t)
	prov.IssueFn = func(ctx context.Context, in IssueInput) (*ReceiptResult, error) {
		return nil, errors.New("wsfe temporarily unavailable")
	}
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	w.Now = func() time.Time { return now }

	seedPaidBill(t, db, 1, 1, 12100)
	seedReadySettings(t, db, 1)
	jobID := enqueueIssueJob(t, db, 1, 1)

	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	var job database.FiscalJob
	require.NoError(t, db.First(&job, jobID).Error)
	require.Equal(t, database.FiscalStatusFailedRetryable, job.Status)
	require.Equal(t, 1, job.Attempts)
	require.NotNil(t, job.NextAttemptAt)
	require.True(t, job.NextAttemptAt.After(now), "next_attempt_at must be in the future")
	require.Nil(t, job.LockedAt)
	require.Nil(t, job.LockedBy)
	require.NotNil(t, job.LastErrorMessage)

	// No authorized receipt persisted.
	var authorized int64
	require.NoError(t, db.Model(&database.FiscalReceipt{}).
		Where("status = ?", database.FiscalStatusAuthorized).Count(&authorized).Error)
	require.Zero(t, authorized)

	var audits []database.FiscalAuditEvent
	require.NoError(t, db.Find(&audits).Error)
	require.Len(t, audits, 1)
	require.Equal(t, "fiscal_receipt_retry", audits[0].EventType)
}

func TestFiscalWorkerPermanentRejectionDoesNotRetry(t *testing.T) {
	w, db, prov := newTestFiscalWorker(t)
	prov.IssueFn = func(ctx context.Context, in IssueInput) (*ReceiptResult, error) {
		return &ReceiptResult{
			Status: StatusRejected,
			ProviderErrors: []ProviderError{
				{Code: "10016", Message: "CAE rejected: invalid point of sale"},
			},
		}, nil
	}
	seedPaidBill(t, db, 1, 1, 12100)
	seedReadySettings(t, db, 1)
	jobID := enqueueIssueJob(t, db, 1, 1)

	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	var job database.FiscalJob
	require.NoError(t, db.First(&job, jobID).Error)
	require.Equal(t, database.FiscalStatusFailedPermanent, job.Status)
	require.Equal(t, 1, job.Attempts)
	require.Nil(t, job.NextAttemptAt, "permanent rejections must not be scheduled for retry")
	require.Nil(t, job.LockedAt)
	require.NotNil(t, job.LastErrorCode)
	require.Equal(t, "10016", *job.LastErrorCode)

	// A failed_permanent receipt row is persisted (so the rejection is auditable),
	// but no authorized receipt.
	var r database.FiscalReceipt
	require.NoError(t, db.Where("bill_id = ?", 1).First(&r).Error)
	require.Equal(t, database.FiscalStatusFailedPermanent, r.Status)
	require.Nil(t, r.AuthCode)

	var audits []database.FiscalAuditEvent
	require.NoError(t, db.Find(&audits).Error)
	require.Len(t, audits, 1)
	require.Equal(t, "fiscal_receipt_failed_permanent", audits[0].EventType)
}

func TestFiscalWorkerExhaustsRetriesToPermanent(t *testing.T) {
	w, db, prov := newTestFiscalWorker(t)
	w.MaxAttempts = 3
	prov.IssueFn = func(ctx context.Context, in IssueInput) (*ReceiptResult, error) {
		return nil, errors.New("transient")
	}
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	w.Now = func() time.Time { return now }

	seedPaidBill(t, db, 1, 1, 12100)
	seedReadySettings(t, db, 1)
	jobID := enqueueIssueJob(t, db, 1, 1)
	// Pre-set attempts to one below the cap so this attempt is the last. The retry
	// budget is the JOB's max_attempts (F-MAXATTEMPTS), so set it to 3 here.
	require.NoError(t, db.Model(&database.FiscalJob{}).Where("id = ?", jobID).
		Updates(map[string]interface{}{"attempts": 2, "max_attempts": 3}).Error)

	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	var job database.FiscalJob
	require.NoError(t, db.First(&job, jobID).Error)
	require.Equal(t, database.FiscalStatusFailedPermanent, job.Status)
	require.Equal(t, 3, job.Attempts)
	require.Nil(t, job.NextAttemptAt)
}

func TestFiscalWorkerLoadUsesProjectionNoSelectStar(t *testing.T) {
	w, db, prov := newTestFiscalWorker(t)
	prov.IssueFn = func(ctx context.Context, in IssueInput) (*ReceiptResult, error) {
		return &ReceiptResult{Status: StatusAuthorized, ReceiptType: "factura_c", AuthCode: "1", ReceiptNumber: "1"}, nil
	}

	var sawSelectStar bool
	var sawBillLoad bool
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:capture_sql", func(tx *gorm.DB) {
		sql := strings.ToLower(tx.Statement.SQL.String())
		if strings.Contains(sql, "from `bills`") || strings.Contains(sql, "from \"bills\"") {
			sawBillLoad = true
			if strings.Contains(sql, "select *") {
				sawSelectStar = true
			}
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove("test:capture_sql") })

	seedPaidBill(t, db, 1, 1, 12100)
	seedReadySettings(t, db, 1)
	enqueueIssueJob(t, db, 1, 1)

	_, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.True(t, sawBillLoad, "expected the worker to load the bill")
	require.False(t, sawSelectStar, "bill load must use explicit column projection, not SELECT *")
}

func TestFiscalWorkerProviderBuildFailureRetries(t *testing.T) {
	db := newFiscalTestDB(t)
	w := NewFiscalWorker(db, &fakeFactory{err: errors.New("decrypt credentials: bad key")})
	w.WorkerID = "test-worker"
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	w.Now = func() time.Time { return now }

	seedPaidBill(t, db, 1, 1, 12100)
	seedReadySettings(t, db, 1)
	jobID := enqueueIssueJob(t, db, 1, 1)

	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	var job database.FiscalJob
	require.NoError(t, db.First(&job, jobID).Error)
	require.Equal(t, database.FiscalStatusFailedRetryable, job.Status)
	require.Equal(t, 1, job.Attempts)
	require.NotNil(t, job.NextAttemptAt)
}

func TestFiscalWorkerErrPermanentIsTerminal(t *testing.T) {
	w, db, prov := newTestFiscalWorker(t)
	prov.IssueFn = func(ctx context.Context, in IssueInput) (*ReceiptResult, error) {
		return nil, fmt.Errorf("rejected by AFIP: %w", ErrPermanent)
	}
	seedPaidBill(t, db, 1, 1, 12100)
	seedReadySettings(t, db, 1)
	jobID := enqueueIssueJob(t, db, 1, 1)

	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	var job database.FiscalJob
	require.NoError(t, db.First(&job, jobID).Error)
	require.Equal(t, database.FiscalStatusFailedPermanent, job.Status)
	require.Nil(t, job.NextAttemptAt)
}

func TestFiscalWorkerDispatchesCreditNote(t *testing.T) {
	w, db, prov := newTestFiscalWorker(t)
	var called bool
	prov.CreditFn = func(ctx context.Context, in CreditNoteInput) (*ReceiptResult, error) {
		called = true
		require.Equal(t, uint(1), in.OriginalReceipt.ID)
		// Regression: the credit-note input MUST carry the issuer settings
		// (CUIT + point of sale) or the real AR mapper rejects every credit note.
		require.Equal(t, "20123456789", in.Settings.TaxID, "credit note must carry issuer CUIT")
		require.NotNil(t, in.Settings.PointOfSale, "credit note must carry point of sale")
		require.Equal(t, 3, *in.Settings.PointOfSale)
		return &ReceiptResult{Status: StatusAuthorized, AuthCode: "99", ReceiptNumber: "7", ReceiptType: "nota_credito_b"}, nil
	}
	seedPaidBill(t, db, 1, 1, 12100)
	settingsID := seedReadySettings(t, db, 1)
	createFiscalReceiptWithStatus(t, db, 1, 1, settingsID, 1, database.FiscalStatusAuthorized)
	receiptID := uint(1)
	jobID := enqueueJob(t, db, 1, 1, ActionCreditNote, &receiptID)

	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.True(t, called, "credit note provider method must be dispatched")

	var job database.FiscalJob
	require.NoError(t, db.First(&job, jobID).Error)
	require.Equal(t, database.FiscalStatusAuthorized, job.Status)

	var credit database.FiscalReceipt
	require.NoError(t, db.Where("action = ?", ActionCreditNote).First(&credit).Error)
	require.Equal(t, database.FiscalStatusAuthorized, credit.Status)
	require.Equal(t, ActionCreditNote, credit.Action)
}

func TestFiscalWorkerDispatchesStatusCheck(t *testing.T) {
	w, db, prov := newTestFiscalWorker(t)
	var called bool
	prov.StatusFn = func(ctx context.Context, providerReceiptID string) (*ReceiptStatus, error) {
		called = true
		return &ReceiptStatus{Status: StatusAuthorized}, nil
	}
	seedPaidBill(t, db, 1, 1, 12100)
	settingsID := seedReadySettings(t, db, 1)
	createFiscalReceiptWithStatus(t, db, 1, 1, settingsID, 1, database.FiscalStatusFailedRetryable)
	receiptID := uint(1)
	jobID := enqueueJob(t, db, 1, 1, ActionStatusCheck, &receiptID)

	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.True(t, called, "status-check provider method must be dispatched")

	var job database.FiscalJob
	require.NoError(t, db.First(&job, jobID).Error)
	require.Equal(t, database.FiscalStatusAuthorized, job.Status)
}

// TestFiscalWorkerStatusCheckReconcilesReceiptToAuthorized is the T20 reconcile
// guard: a status_check on a failed_retryable receipt that the provider now
// reports authorized must flip BOTH the job AND the underlying receipt row to
// authorized. Without the receipt reconcile, only the job would record the
// terminal state and the receipt would stay stuck at failed_retryable forever.
func TestFiscalWorkerStatusCheckReconcilesReceiptToAuthorized(t *testing.T) {
	w, db, prov := newTestFiscalWorker(t)
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	w.Now = func() time.Time { return now }
	prov.StatusFn = func(ctx context.Context, providerReceiptID string) (*ReceiptStatus, error) {
		return &ReceiptStatus{Status: StatusAuthorized, ProviderState: "A"}, nil
	}
	seedPaidBill(t, db, 1, 1, 12100)
	settingsID := seedReadySettings(t, db, 1)
	createFiscalReceiptWithStatus(t, db, 1, 1, settingsID, 1, database.FiscalStatusFailedRetryable)
	receiptID := uint(1)
	jobID := enqueueJob(t, db, 1, 1, ActionStatusCheck, &receiptID)

	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	var job database.FiscalJob
	require.NoError(t, db.First(&job, jobID).Error)
	require.Equal(t, database.FiscalStatusAuthorized, job.Status, "job must reach terminal authorized")

	// The RECEIPT ROW must be reconciled to authorized, not left at failed_retryable.
	var receipt database.FiscalReceipt
	require.NoError(t, db.First(&receipt, receiptID).Error)
	require.Equal(t, database.FiscalStatusAuthorized, receipt.Status, "receipt row must be reconciled to authorized")
}

// TestFiscalWorkerStatusCheckRetryableLeavesReceiptUnchanged verifies a still-
// failing (retryable / not-found) status check reschedules the JOB with a future
// next_attempt_at but does NOT mutate the receipt row — it stays failed_retryable
// for the next sweep to re-check.
func TestFiscalWorkerStatusCheckRetryableLeavesReceiptUnchanged(t *testing.T) {
	w, db, prov := newTestFiscalWorker(t)
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	w.Now = func() time.Time { return now }
	prov.StatusFn = func(ctx context.Context, providerReceiptID string) (*ReceiptStatus, error) {
		return &ReceiptStatus{
			Status:         StatusFailedRetryable,
			ProviderErrors: []ProviderError{{Code: "602", Message: "comprobante inexistente"}},
		}, nil
	}
	seedPaidBill(t, db, 1, 1, 12100)
	settingsID := seedReadySettings(t, db, 1)
	createFiscalReceiptWithStatus(t, db, 1, 1, settingsID, 1, database.FiscalStatusFailedRetryable)
	receiptID := uint(1)
	jobID := enqueueJob(t, db, 1, 1, ActionStatusCheck, &receiptID)

	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	// Reload into a FRESH struct: a reused struct's *time.Time won't reset on NULL.
	var job database.FiscalJob
	require.NoError(t, db.First(&job, jobID).Error)
	require.Equal(t, database.FiscalStatusFailedRetryable, job.Status, "job must remain retryable")
	require.NotNil(t, job.NextAttemptAt, "retryable status check must schedule a future retry")
	require.True(t, job.NextAttemptAt.After(now), "next_attempt_at must be in the future")

	// The receipt row must be UNTOUCHED on a retryable result.
	var receipt database.FiscalReceipt
	require.NoError(t, db.First(&receipt, receiptID).Error)
	require.Equal(t, database.FiscalStatusFailedRetryable, receipt.Status, "receipt must be left unchanged on a retryable status check")
}

// TestFiscalWorkerRetriesFailedRetryableJobOnceDue is the regression guard for
// the bug where a transient failure parked a job in failed_retryable that the
// claim query (which only selected "pending") never picked up again — making the
// entire backoff machinery dead code. It verifies the full retry cycle: a job is
// NOT reclaimed before next_attempt_at, and IS reclaimed and retried (to success)
// once the backoff elapses.
func TestFiscalWorkerRetriesFailedRetryableJobOnceDue(t *testing.T) {
	w, db, prov := newTestFiscalWorker(t)
	cur := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	w.Now = func() time.Time { return cur }

	var calls int
	prov.IssueFn = func(ctx context.Context, in IssueInput) (*ReceiptResult, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("wsfe temporarily unavailable")
		}
		return &ReceiptResult{
			Status:        StatusAuthorized,
			AuthCode:      "71000000000002",
			ReceiptNumber: "44",
			ReceiptType:   "factura_c",
		}, nil
	}

	seedPaidBill(t, db, 1, 1, 12100)
	seedReadySettings(t, db, 1)
	jobID := enqueueIssueJob(t, db, 1, 1)

	// Pass 1: transient failure → failed_retryable, scheduled in the future.
	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	// Reload into a fresh struct each time: GORM's First into a reused struct does
	// not reset an already-populated *time.Time pointer when the column is NULL.
	var afterPass1 database.FiscalJob
	require.NoError(t, db.First(&afterPass1, jobID).Error)
	require.Equal(t, database.FiscalStatusFailedRetryable, afterPass1.Status)
	require.Equal(t, 1, afterPass1.Attempts)
	require.NotNil(t, afterPass1.NextAttemptAt)

	// Pass 2: still before next_attempt_at → not yet due, must NOT be re-claimed.
	n, err = w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, n, "a failed_retryable job must not be reclaimed before next_attempt_at")
	require.Equal(t, 1, calls, "provider must not be invoked again before the backoff elapses")

	// Advance the clock past the backoff; the job is now due.
	cur = afterPass1.NextAttemptAt.Add(time.Second)

	n, err = w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n, "a due failed_retryable job must be reclaimed and retried")
	require.Equal(t, 2, calls)

	var afterPass3 database.FiscalJob
	require.NoError(t, db.First(&afterPass3, jobID).Error)
	require.Equal(t, database.FiscalStatusAuthorized, afterPass3.Status, "second attempt should authorize")
	require.Equal(t, 2, afterPass3.Attempts)
	require.Nil(t, afterPass3.NextAttemptAt, "a terminal success must clear next_attempt_at")
	require.Nil(t, afterPass3.LockedAt)
}

func TestStartFiscalWorkerProcessesThenStopsOnCancel(t *testing.T) {
	w, db, prov := newTestFiscalWorker(t)
	// The worker runs ProcessDue in its own goroutine while this test polls the
	// DB; an in-memory SQLite database is per-connection, so pin the pool to a
	// single connection or the goroutine gets a fresh (empty) DB ("no such table").
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	prov.IssueFn = func(ctx context.Context, in IssueInput) (*ReceiptResult, error) {
		return &ReceiptResult{Status: StatusAuthorized, AuthCode: "1", ReceiptNumber: "1", ReceiptType: "factura_c"}, nil
	}
	seedPaidBill(t, db, 1, 1, 12100)
	seedReadySettings(t, db, 1)
	enqueueIssueJob(t, db, 1, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		StartFiscalWorker(ctx, w, 10*time.Millisecond)
		close(done)
	}()

	// The loop must run ProcessDue at least once within a short interval.
	deadline := time.Now().Add(2 * time.Second)
	for {
		var job database.FiscalJob
		require.NoError(t, db.Where("bill_id = ?", 1).First(&job).Error)
		if job.Status == database.FiscalStatusAuthorized {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job not processed by the worker loop within deadline (status=%v)", job.Status)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Cancelling the context must stop the loop promptly.
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("StartFiscalWorker did not return after context cancel")
	}
}

func TestFiscalWorkerRecordsMetrics(t *testing.T) {
	w, db, prov := newTestFiscalWorker(t)
	prov.IssueFn = func(ctx context.Context, in IssueInput) (*ReceiptResult, error) {
		return &ReceiptResult{Status: StatusAuthorized, AuthCode: "1", ReceiptNumber: "1", ReceiptType: "factura_c"}, nil
	}
	seedPaidBill(t, db, 1, 1, 12100)
	seedReadySettings(t, db, 1)
	enqueueIssueJob(t, db, 1, 1)

	// Counters are process-global; assert deltas, not absolutes.
	beforeAuth := testutil.ToFloat64(metrics.FiscalReceiptsTotal.WithLabelValues(string(database.FiscalStatusAuthorized)))
	beforeAttempts := testutil.ToFloat64(metrics.FiscalJobAttempts.WithLabelValues(ActionIssueReceipt))

	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	require.Equal(t, beforeAuth+1, testutil.ToFloat64(metrics.FiscalReceiptsTotal.WithLabelValues(string(database.FiscalStatusAuthorized))),
		"an authorized job must increment fiscal_receipts_total{status=authorized}")
	require.Equal(t, beforeAttempts+1, testutil.ToFloat64(metrics.FiscalJobAttempts.WithLabelValues(ActionIssueReceipt)),
		"processing a job must increment fiscal_job_attempts_total{action=issue_receipt}")
	// The depth gauge is sampled before the claim, so it reflects the one due job.
	require.Equal(t, float64(1), testutil.ToFloat64(metrics.FiscalJobsQueueDepth),
		"queue-depth gauge must be sampled to the pre-claim backlog")
}

func TestFiscalWorkerReclaimsStaleLockBeforeClaim(t *testing.T) {
	w, db, prov := newTestFiscalWorker(t)
	prov.IssueFn = func(ctx context.Context, in IssueInput) (*ReceiptResult, error) {
		return &ReceiptResult{Status: StatusAuthorized, AuthCode: "1", ReceiptNumber: "1", ReceiptType: "factura_c"}, nil
	}
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	w.Now = func() time.Time { return now }
	w.ReclaimStaleAfter = time.Minute

	seedPaidBill(t, db, 1, 1, 12100)
	seedReadySettings(t, db, 1)
	jobID := enqueueIssueJob(t, db, 1, 1)
	// Simulate a crashed worker that locked the job 10 minutes ago.
	staleLock := now.Add(-10 * time.Minute)
	require.NoError(t, db.Model(&database.FiscalJob{}).Where("id = ?", jobID).
		Updates(map[string]interface{}{"locked_at": staleLock, "locked_by": "dead-worker"}).Error)

	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n, "stale-locked job must be reclaimed then claimed")

	var job database.FiscalJob
	require.NoError(t, db.First(&job, jobID).Error)
	require.Equal(t, database.FiscalStatusAuthorized, job.Status)
}

// TestApplyOutcomeFailedPermanentRaisesAlert locks Task 13c: when a retryable
// failure exhausts the job's MaxAttempts budget, the operator alert inbox must
// receive exactly one fiscal_issue_failed alert.
func TestApplyOutcomeFailedPermanentRaisesAlert(t *testing.T) {
	db := newFiscalTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.OperationalAlert{}, &database.OperationalAlertEvent{}))

	w := NewFiscalWorker(db, &fakeFactory{provider: &stubProvider{}})
	w.WorkerID = "test-worker"
	now := time.Now()
	lockedBy := w.workerID()
	job := database.FiscalJob{
		BusinessID:  7,
		SettingsID:  1,
		BillID:      13,
		Action:      ActionIssueReceipt,
		Status:      database.FiscalStatusPending,
		Attempts:    4, // this attempt is the 5th and last
		MaxAttempts: 5,
		LockedBy:    &lockedBy,
		LockedAt:    &now,
	}
	require.NoError(t, db.Create(&job).Error)

	w.applyOutcome(job, &JobOutcome{
		Retryable:    true,
		ErrorCode:    "arca_5xx",
		ErrorMessage: "upstream down",
	})

	var count int64
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("business_id = ? AND alert_type = ?", 7, database.OperationalAlertTypeFiscalIssueFailed).
		Count(&count).Error)
	require.EqualValues(t, 1, count, "exhausted fiscal job must raise an urgent operator alert")
}
