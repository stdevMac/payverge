package fiscal

import (
	"context"
	"errors"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

// countingIssueProvider counts IssueReceipt calls so a test can assert the
// provider (AFIP) is NOT called when an authorized receipt already exists.
type countingIssueProvider struct {
	fakeProvider
	issueCalls int
}

func (p *countingIssueProvider) IssueReceipt(context.Context, IssueInput) (*ReceiptResult, error) {
	p.issueCalls++
	return &ReceiptResult{Status: database.FiscalStatusAuthorized, AuthCode: "DUPLICATE-CAE", ReceiptNumber: "99"}, nil
}

// TestProcessIssueJob_AdoptsExistingAuthorizedReceipt guards the CRITICAL
// crash-window: if a prior run authorized a factura (receipt committed) but the
// process died before the job was marked authorized, the re-claimed job must
// adopt the existing receipt and NOT request a second CAE (duplicate legal
// invoice) from AFIP.
func TestProcessIssueJob_AdoptsExistingAuthorizedReceipt(t *testing.T) {
	db := newFiscalTestDB(t)
	svc := NewService(db, NewProviderRegistry())

	existing := &database.FiscalReceipt{
		BusinessID:       1,
		SettingsID:       1,
		BillID:           1,
		Action:           ActionIssueReceipt,
		ReceiptType:      "factura_b",
		Status:           database.FiscalStatusAuthorized,
		TotalAmountCents: 12100,
		AuthCode:         strPtr("70123456789012"),
		ReceiptNumber:    strPtr("42"),
	}
	require.NoError(t, db.Create(existing).Error)

	job := database.FiscalJob{BusinessID: 1, SettingsID: 1, BillID: 1}
	job.ID = 7
	jobCtx := &JobContext{
		Bill:     database.Bill{TotalAmount: 12100, PaidAmount: 12100},
		Settings: database.BusinessFiscalSettings{Country: "AR", Provider: "arca"},
	}

	provider := &countingIssueProvider{}
	out, err := svc.processIssueJob(context.Background(), job, jobCtx, provider)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, database.FiscalStatusAuthorized, out.TerminalStatus)
	require.NotNil(t, out.ReceiptID)
	require.Equal(t, existing.ID, *out.ReceiptID, "must adopt the existing authorized receipt")
	require.Equal(t, 0, provider.issueCalls, "must NOT call AFIP again — no duplicate factura")

	var count int64
	require.NoError(t, db.Model(&database.FiscalReceipt{}).Where("bill_id = ?", 1).Count(&count).Error)
	require.Equal(t, int64(1), count, "exactly one issue receipt for the bill")
}

// TestPersistReceiptResult_OrphanedCAE_IncrementsMetricAndGoesTerminal verifies
// that when saveAuthorizedReceiptWithRetry exhausts all inline retries after a
// successful AFIP authorization, the outcome is (a) terminal failed_permanent
// with error code "persist_failed_terminal" and (b) the
// payverge_fiscal_orphaned_cae_total counter is incremented exactly once.
func TestPersistReceiptResult_OrphanedCAE_IncrementsMetricAndGoesTerminal(t *testing.T) {
	before := testutil.ToFloat64(metrics.FiscalOrphanedCAETotal)
	svc, job, jobCtx, result := persistTestFixture(t)

	// Override the save seam so every attempt fails.
	svc.saveReceiptFn = func(_ *database.FiscalReceipt, _ uint, _ *uint) (uint, error) {
		return 0, errors.New("db unavailable")
	}

	out, err := svc.persistReceiptResult(job, jobCtx, ActionIssueReceipt, "factura_b", nil, result, nil)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, database.FiscalStatusFailedPermanent, out.TerminalStatus)
	require.Equal(t, "persist_failed_terminal", out.ErrorCode)
	require.Equal(t, before+1, testutil.ToFloat64(metrics.FiscalOrphanedCAETotal),
		"orphaned-CAE metric must advance exactly once on persist-failed-terminal")
}
