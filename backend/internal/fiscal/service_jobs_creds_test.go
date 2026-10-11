package fiscal

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// newFiscalServiceWithExpiredCreds returns a Service and a pending FiscalJob
// whose associated BusinessFiscalSettings has a CredentialsExpiresAt set one
// day in the past, so ProcessClaimedJob should short-circuit immediately.
func newFiscalServiceWithExpiredCreds(t *testing.T) (*Service, database.FiscalJob) {
	t.Helper()
	db := newFiscalTestDB(t)

	// Seed the business row that LoadJobContext requires.
	require.NoError(t, db.Create(&database.Business{
		ID:             1,
		BusinessId:     "biz-expired-creds",
		OwnerAddress:   "owner",
		Name:           "Test Biz",
		SettlementAddr: "settle",
		TippingAddr:    "tip",
	}).Error)

	// Seed a paid bill.
	require.NoError(t, db.Create(&database.Bill{
		ID:          1,
		BusinessID:  1,
		BillNumber:  "PV-expired-test",
		Status:      database.BillStatusPaid,
		Items:       "[]",
		TotalAmount: 2400,
		PaidAmount:  2400,
	}).Error)

	// Seed fiscal settings with an expired CredentialsExpiresAt.
	pos := 3
	expiredAt := time.Now().Add(-24 * time.Hour)
	var settings database.BusinessFiscalSettings
	require.NoError(t, db.Create(&database.BusinessFiscalSettings{
		BusinessID:           1,
		Country:              "AR",
		Provider:             "arca",
		Mode:                 database.FiscalModeAutomaticNonBlocking,
		Environment:          "sandbox",
		TaxID:                "20123456789",
		TaxCondition:         "monotributo",
		PointOfSale:          &pos,
		SetupStatus:          "ready",
		CredentialsExpiresAt: &expiredAt,
	}).Error)
	require.NoError(t, db.Where("business_id = ?", 1).First(&settings).Error)

	// Seed a pending fiscal job.
	idempotencyKey := "test:expired:creds:1"
	job := database.FiscalJob{
		BusinessID:     1,
		SettingsID:     settings.ID,
		BillID:         1,
		Action:         ActionIssueReceipt,
		IdempotencyKey: idempotencyKey,
		Status:         database.FiscalStatusPending,
		MaxAttempts:    5,
		CreatedBy:      "system",
	}
	require.NoError(t, db.Create(&job).Error)

	// Wire a stub factory that should NOT be called when creds are expired.
	called := false
	factory := &fakeFactory{
		provider: &stubProvider{
			IssueFn: func(ctx context.Context, in IssueInput) (*ReceiptResult, error) {
				called = true
				return &ReceiptResult{Status: StatusAuthorized}, nil
			},
		},
	}
	t.Cleanup(func() {
		if called {
			t.Error("provider was called despite expired credentials — short-circuit did not fire")
		}
	})

	svc := NewService(db, NewProviderRegistry()).WithProviderFactory(factory)
	return svc, job
}

// TestExpiredCredentialsShortCircuit asserts an expired CredentialsExpiresAt
// yields a terminal failed_permanent outcome with a distinct error_code,
// instead of a retryable provider_unavailable that burns the budget.
func TestExpiredCredentialsShortCircuit(t *testing.T) {
	s, job := newFiscalServiceWithExpiredCreds(t)
	outcome, err := s.ProcessClaimedJob(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Retryable {
		t.Fatal("expired creds must NOT be retryable")
	}
	if outcome.TerminalStatus != database.FiscalStatusFailedPermanent {
		t.Fatalf("want failed_permanent, got %q", outcome.TerminalStatus)
	}
	if outcome.ErrorCode != "credentials_expired" {
		t.Fatalf("want credentials_expired, got %q", outcome.ErrorCode)
	}
	_ = time.Now
}

// TestPermanentBuildFailureIsNotRetryable asserts that when resolveProvider
// returns an error wrapping ErrPermanent (decrypt failure, bad PEM, unsupported
// combo), ProcessClaimedJob classifies it as failed_permanent rather than
// retryable — so a mis-decryptable credential stops burning the 5x retry budget.
func TestPermanentBuildFailureIsNotRetryable(t *testing.T) {
	db := newFiscalTestDB(t)

	// Seed the minimum context rows LoadJobContext requires.
	require.NoError(t, db.Create(&database.Business{
		ID:             2,
		BusinessId:     "biz-perm-build",
		OwnerAddress:   "owner",
		Name:           "Test Biz",
		SettlementAddr: "settle",
		TippingAddr:    "tip",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID:          2,
		BusinessID:  2,
		BillNumber:  "PV-perm-build",
		Status:      database.BillStatusPaid,
		Items:       "[]",
		TotalAmount: 1200,
		PaidAmount:  1200,
	}).Error)

	pos := 1
	var settings database.BusinessFiscalSettings
	require.NoError(t, db.Create(&database.BusinessFiscalSettings{
		BusinessID:   2,
		Country:      "AR",
		Provider:     "arca",
		Mode:         database.FiscalModeAutomaticNonBlocking,
		Environment:  "sandbox",
		TaxID:        "20123456789",
		TaxCondition: "monotributo",
		PointOfSale:  &pos,
		SetupStatus:  "ready",
	}).Error)
	require.NoError(t, db.Where("business_id = ?", 2).First(&settings).Error)

	job := database.FiscalJob{
		BusinessID:     2,
		SettingsID:     settings.ID,
		BillID:         2,
		Action:         ActionIssueReceipt,
		IdempotencyKey: "test:perm:build:2",
		Status:         database.FiscalStatusPending,
		MaxAttempts:    5,
		CreatedBy:      "system",
	}
	require.NoError(t, db.Create(&job).Error)

	// Factory returns a deterministic (decrypt-style) permanent failure.
	permanentErr := fmt.Errorf("fiscal: decrypt credentials: bad ciphertext: %w", ErrPermanent)
	factory := &fakeFactory{err: permanentErr}

	svc := NewService(db, NewProviderRegistry()).WithProviderFactory(factory)
	outcome, err := svc.ProcessClaimedJob(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Retryable {
		t.Fatalf("permanent build failure must NOT be retryable, got Retryable=true (code=%s)", outcome.ErrorCode)
	}
	if outcome.TerminalStatus != database.FiscalStatusFailedPermanent {
		t.Fatalf("want failed_permanent, got %q", outcome.TerminalStatus)
	}
	if !strings.Contains(outcome.ErrorCode, "permanent") {
		t.Fatalf("want error_code containing 'permanent', got %q", outcome.ErrorCode)
	}
}
