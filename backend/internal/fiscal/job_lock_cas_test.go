package fiscal

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// onAttemptProvider mimics the AR provider's ordering: OnAttempt must succeed
// before the (counted) CAE request is sent.
type onAttemptProvider struct {
	fakeProvider
	requestCalls int
}

func (p *onAttemptProvider) IssueReceipt(_ context.Context, in IssueInput) (*ReceiptResult, error) {
	if err := in.OnAttempt("11-1-43"); err != nil {
		return nil, err
	}
	p.requestCalls++
	return &ReceiptResult{Status: database.FiscalStatusAuthorized, AuthCode: "70123456789012", ReceiptNumber: "43"}, nil
}

func lockedFiscalJob(t *testing.T, owner string) (*Repository, database.FiscalJob) {
	t.Helper()
	db := newFiscalTestDB(t)
	at := time.Now()
	job := database.FiscalJob{
		BusinessID: 1, SettingsID: 1, BillID: 1, Action: ActionIssueReceipt,
		IdempotencyKey: "cas-" + t.Name(), Status: database.FiscalStatusPending,
		MaxAttempts: 5, CreatedBy: "system", LockedAt: &at, LockedBy: &owner,
	}
	require.NoError(t, db.Create(&job).Error)
	return NewRepository(db), job
}

func TestRecordJobAttemptIsLockOwnerScoped(t *testing.T) {
	repo, job := lockedFiscalJob(t, "worker-A")

	require.Error(t, repo.RecordJobAttempt(job.ID, "worker-B", "11-1-43"), "foreign locked_by must error")
	require.Error(t, repo.RecordJobAttempt(job.ID, "", "11-1-43"), "an unlocked job must error")
	var reload database.FiscalJob
	require.NoError(t, repo.db.First(&reload, job.ID).Error)
	require.Nil(t, reload.AttemptedProviderReceiptID, "column must be unchanged")

	require.NoError(t, repo.RecordJobAttempt(job.ID, "worker-A", "11-1-43"))
	require.NoError(t, repo.db.First(&reload, job.ID).Error)
	require.NotNil(t, reload.AttemptedProviderReceiptID)
	require.Equal(t, "11-1-43", *reload.AttemptedProviderReceiptID)
}

func TestProcessIssueJob_LostLockSkipsCAERequest(t *testing.T) {
	repo, job := lockedFiscalJob(t, "worker-A")
	svc := NewService(repo.db, NewProviderRegistry())
	stale := "worker-B" // this worker's lock was reclaimed; A now owns the row
	job.LockedBy = &stale
	jobCtx := &JobContext{
		Bill:     database.Bill{TotalAmount: 12100, PaidAmount: 12100},
		Settings: database.BusinessFiscalSettings{Country: "AR", Provider: "arca"},
	}
	provider := &onAttemptProvider{}
	out, err := svc.processIssueJob(context.Background(), job, jobCtx, provider)
	require.NoError(t, err)
	require.True(t, out.Retryable, "lost lock must be retryable, not terminal")
	require.Equal(t, 0, provider.requestCalls, "RequestCAE must not run after the lock was lost")

	var reload database.FiscalJob
	require.NoError(t, repo.db.First(&reload, job.ID).Error)
	require.Nil(t, reload.AttemptedProviderReceiptID)
}

func TestProcessCreditNoteJob_RefusesCreditNoteAsOriginal(t *testing.T) {
	db := newFiscalTestDB(t)
	creditOfCredit := database.FiscalReceipt{
		BusinessID: 1, SettingsID: 1, BillID: 1, Action: ActionCreditNote,
		ReceiptType: "nota_de_credito_b", TotalAmountCents: 1000,
		Status: database.FiscalStatusAuthorized,
	}
	require.NoError(t, db.Create(&creditOfCredit).Error)
	job := database.FiscalJob{
		BusinessID: 1, SettingsID: 1, BillID: 1, Action: ActionCreditNote,
		ReceiptID: &creditOfCredit.ID, IdempotencyKey: "cn-of-cn",
		Status: database.FiscalStatusPending, MaxAttempts: 5, CreatedBy: "system",
	}
	require.NoError(t, db.Create(&job).Error)

	calls := 0
	prov := &stubProvider{CreditFn: func(context.Context, CreditNoteInput) (*ReceiptResult, error) {
		calls++
		return &ReceiptResult{Status: StatusAuthorized, AuthCode: "1", ReceiptNumber: "1"}, nil
	}}
	svc := NewService(db, NewProviderRegistry())
	jobCtx := &JobContext{Settings: database.BusinessFiscalSettings{Country: "AR", Provider: "arca"}}
	out, err := svc.processCreditNoteJob(context.Background(), job, jobCtx, prov)
	require.NoError(t, err)
	require.Equal(t, database.FiscalStatusFailedPermanent, out.TerminalStatus)
	require.Equal(t, "credit_note_original_is_credit_note", out.ErrorCode)
	require.Equal(t, 0, calls)
}

// Job A recorded voucher 43, lost the response and released the series lock.
// Job B then read LastAuthorized=42 and recorded the same 43. B's record must
// supersede A's stale claim, so A's retry cannot consult 43 and adopt B's CAE
// before B has saved its receipt.
func TestRecordJobAttempt_SupersedesStaleSameNumberClaimOfOtherLiveJob(t *testing.T) {
	repo, a := lockedFiscalJob(t, "worker-A")
	bOwner := "worker-B"
	at := time.Now()
	b := database.FiscalJob{
		BusinessID: 1, SettingsID: 1, BillID: 2, Action: ActionIssueReceipt,
		IdempotencyKey: "cas-b", Status: database.FiscalStatusPending,
		MaxAttempts: 5, CreatedBy: "system", LockedAt: &at, LockedBy: &bOwner,
	}
	require.NoError(t, repo.db.Create(&b).Error)
	other := database.FiscalJob{
		BusinessID: 1, SettingsID: 2, BillID: 3, Action: ActionIssueReceipt,
		IdempotencyKey: "cas-other-series", Status: database.FiscalStatusPending,
		MaxAttempts: 5, CreatedBy: "system", LockedAt: &at, LockedBy: &bOwner,
	}
	require.NoError(t, repo.db.Create(&other).Error)

	require.NoError(t, repo.RecordJobAttempt(a.ID, "worker-A", "11-1-43"))
	require.NoError(t, repo.RecordJobAttempt(other.ID, "worker-B", "11-1-43"))
	require.NoError(t, repo.RecordJobAttempt(b.ID, "worker-B", "11-1-43"))

	var reload database.FiscalJob
	require.NoError(t, repo.db.First(&reload, a.ID).Error)
	require.Nil(t, reload.AttemptedProviderReceiptID, "A's stale claim on 43 must be cleared")
	reload = database.FiscalJob{}
	require.NoError(t, repo.db.First(&reload, b.ID).Error)
	require.NotNil(t, reload.AttemptedProviderReceiptID)
	require.Equal(t, "11-1-43", *reload.AttemptedProviderReceiptID)
	reload = database.FiscalJob{}
	require.NoError(t, repo.db.First(&reload, other.ID).Error)
	require.NotNil(t, reload.AttemptedProviderReceiptID, "a different settings series is untouched")
}
