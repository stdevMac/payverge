package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestGuestFeedbackClaimIsAtomic asserts only one of two sequential claims of
// the same bill wins, proving the send is gated by an atomic claim rather than
// a read-then-write. (Sequential rather than concurrent to keep the test
// deterministic with SQLite; the schedclaim primitive is tested concurrently
// in its own package.)
func TestGuestFeedbackClaimIsAtomic(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	biz := createTestHospitalityBusiness(t, db, "feedback-claim")

	// Create a paid bill with feedback_email_sent_at = NULL
	closedAt := time.Now().Add(-25 * time.Hour)
	customerID := uint(0)
	{
		cust := &database.Customer{
			Email: "claim-test-guest@example.com",
			Name:  "Claim Test Guest",
		}
		require.NoError(t, db.Create(cust).Error)
		customerID = cust.ID
	}
	bill := &database.Bill{
		BusinessID:     biz.ID,
		Status:         database.BillStatusPaid,
		ClosedAt:       &closedAt,
		CRMCustomerID:  &customerID,
		SettlementAddr: "0xsettlement",
		TippingAddr:    "0xtipping",
		// FeedbackEmailSentAt intentionally nil
	}
	require.NoError(t, db.Create(bill).Error)

	gfs := NewGuestFeedbackScheduler(database.GetDBWrapper())

	first := gfs.claimFeedback(context.Background(), bill.ID)
	second := gfs.claimFeedback(context.Background(), bill.ID)

	if first == second {
		t.Fatalf("expected exactly one winning claim, got first=%v second=%v", first, second)
	}
	if !first {
		t.Fatalf("expected the first claim to win")
	}
}

// TestGuestFeedbackClaimRelease verifies that releasing a claim allows a
// subsequent caller to win it — mirroring the transient-failure retry path.
func TestGuestFeedbackClaimRelease(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	biz := createTestHospitalityBusiness(t, db, "feedback-release")

	closedAt := time.Now().Add(-25 * time.Hour)
	customerID := uint(0)
	{
		cust := &database.Customer{
			Email: "release-test-guest@example.com",
			Name:  "Release Test Guest",
		}
		require.NoError(t, db.Create(cust).Error)
		customerID = cust.ID
	}
	bill := &database.Bill{
		BusinessID:     biz.ID,
		Status:         database.BillStatusPaid,
		ClosedAt:       &closedAt,
		CRMCustomerID:  &customerID,
		SettlementAddr: "0xsettlement",
		TippingAddr:    "0xtipping",
	}
	require.NoError(t, db.Create(bill).Error)

	gfs := NewGuestFeedbackScheduler(database.GetDBWrapper())

	won := gfs.claimFeedback(context.Background(), bill.ID)
	require.True(t, won, "first claim must win")

	lost := gfs.claimFeedback(context.Background(), bill.ID)
	require.False(t, lost, "second claim must lose")

	// Simulate transient send failure: release so next tick can retry.
	gfs.releaseFeedbackClaim(bill.ID)

	reclaimed := gfs.claimFeedback(context.Background(), bill.ID)
	require.True(t, reclaimed, "released claim must be claimable again")
}
