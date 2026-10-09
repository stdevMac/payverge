package fiscal

import (
	"errors"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// persistTestFixture builds a minimal Service + job + jobCtx + authorized result
// for exercising the persist path of persistReceiptResult.
func persistTestFixture(t *testing.T) (*Service, database.FiscalJob, *JobContext, *ReceiptResult) {
	t.Helper()
	db := newFiscalTestDB(t)
	svc := NewService(db, NewProviderRegistry())
	job := database.FiscalJob{
		BusinessID: 1,
		SettingsID: 1,
		BillID:     1,
	}
	job.ID = 7
	jobCtx := &JobContext{
		Bill:     database.Bill{TotalAmount: 12100, PaidAmount: 12100},
		Settings: database.BusinessFiscalSettings{Country: "AR", Provider: "arca"},
		Business: database.Business{},
	}
	result := &ReceiptResult{
		Status:        database.FiscalStatusAuthorized,
		AuthCode:      "70123456789012",
		ReceiptNumber: "42",
		QRPayload:     "https://www.afip.gob.ar/fe/qr/?p=abc",
	}
	return svc, job, jobCtx, result
}

// A save failure after a successful AFIP authorization is retried inline; if a
// later attempt succeeds the job is authorized (no re-issue, no duplicate).
func TestPersistReceiptResult_SaveRetriesThenSucceeds(t *testing.T) {
	svc, job, jobCtx, result := persistTestFixture(t)

	calls := 0
	svc.saveReceiptFn = func(_ *database.FiscalReceipt, _ uint, _ *uint) (uint, error) {
		calls++
		if calls < maxReceiptSaveAttempts {
			return 0, errors.New("transient db error")
		}
		return 99, nil
	}

	outcome, err := svc.persistReceiptResult(job, jobCtx, ActionIssueReceipt, "factura_b", nil, result, nil)
	require.NoError(t, err)
	require.NotNil(t, outcome)
	require.False(t, outcome.Retryable)
	require.Equal(t, database.FiscalStatusAuthorized, outcome.TerminalStatus)
	require.NotNil(t, outcome.ReceiptID)
	require.Equal(t, uint(99), *outcome.ReceiptID)
	require.Equal(t, maxReceiptSaveAttempts, calls, "must retry the save inline up to the bound")
}

// A numbered attempt is already persisted, so exhausting the inline saves is
// retryable: the next run consults that voucher and adopts it instead of
// allocating a second legal invoice.
func TestPersistReceiptResult_SaveExhaustedWithProviderReceiptIDIsRetryable(t *testing.T) {
	svc, job, jobCtx, result := persistTestFixture(t)
	result.ProviderReceiptID = "11-1-42"

	calls := 0
	svc.saveReceiptFn = func(_ *database.FiscalReceipt, _ uint, _ *uint) (uint, error) {
		calls++
		return 0, errors.New("db unavailable")
	}

	outcome, err := svc.persistReceiptResult(job, jobCtx, ActionIssueReceipt, "factura_b", nil, result, nil)
	require.NoError(t, err)
	require.NotNil(t, outcome)
	require.True(t, outcome.Retryable)
	require.Equal(t, "receipt_persist_failed", outcome.ErrorCode)
	require.Empty(t, outcome.TerminalStatus)
	require.Equal(t, "db unavailable", outcome.ErrorMessage)
	require.Nil(t, outcome.ReceiptID)
	require.Equal(t, maxReceiptSaveAttempts, calls, "the inline retry is bounded")
}

// With no recorded voucher number, a retry would allocate LastAuthorized+1 and
// duplicate the legal invoice. Exhausting the inline saves stays terminal.
func TestPersistReceiptResult_SaveExhaustedWithoutProviderReceiptIDIsTerminal(t *testing.T) {
	svc, job, jobCtx, result := persistTestFixture(t)

	calls := 0
	svc.saveReceiptFn = func(_ *database.FiscalReceipt, _ uint, _ *uint) (uint, error) {
		calls++
		return 0, errors.New("db unavailable")
	}

	outcome, err := svc.persistReceiptResult(job, jobCtx, ActionIssueReceipt, "factura_b", nil, result, nil)
	require.NoError(t, err)
	require.NotNil(t, outcome)
	require.False(t, outcome.Retryable, "must NOT be retryable — re-issue would duplicate the invoice")
	require.Equal(t, database.FiscalStatusFailedPermanent, outcome.TerminalStatus)
	require.Equal(t, "persist_failed_terminal", outcome.ErrorCode)
	require.Nil(t, outcome.ReceiptID)
	require.Equal(t, maxReceiptSaveAttempts, calls, "the inline retry is bounded")
}
