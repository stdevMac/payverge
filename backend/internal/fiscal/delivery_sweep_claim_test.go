package fiscal

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newReceiptRepoWithUndelivered seeds one authorized, undelivered receipt and
// returns the repo and receipt ID. Mirrors the existing fiscal repo test setup.
func newReceiptRepoWithUndelivered(t *testing.T) (*Repository, uint) {
	t.Helper()
	db := newFiscalTestDB(t)
	repo := NewRepository(db)
	return repo, seedUndeliveredReceipt(t, db)
}

func seedUndeliveredReceipt(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	issued := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	r := &database.FiscalReceipt{
		BusinessID:       1,
		SettingsID:       1,
		BillID:           1,
		Country:          "AR",
		Provider:         "arca",
		Action:           ActionIssueReceipt,
		ReceiptType:      "factura_c",
		TotalAmountCents: 12100,
		Currency:         "ARS",
		Status:           database.FiscalStatusAuthorized,
		IssuedAt:         &issued,
		// DeliveredAt intentionally nil — undelivered
	}
	require.NoError(t, db.Create(r).Error)
	return r.ID
}

// TestClaimReceiptDeliveryIsExclusive asserts two workers cannot both claim the
// same undelivered receipt within the lock TTL.
func TestClaimReceiptDeliveryIsExclusive(t *testing.T) {
	repo, receiptID := newReceiptRepoWithUndelivered(t)
	staleBefore := time.Now().Add(-10 * time.Minute)

	won1, err := repo.ClaimReceiptDelivery(receiptID, "worker-A", staleBefore)
	if err != nil {
		t.Fatal(err)
	}
	won2, err := repo.ClaimReceiptDelivery(receiptID, "worker-B", staleBefore)
	if err != nil {
		t.Fatal(err)
	}
	if !won1 || won2 {
		t.Fatalf("expected exactly one winner, got won1=%v won2=%v", won1, won2)
	}
}

// TestClaimReceiptDeliveryStaleReclaim asserts a stale lock (older than staleBefore)
// can be reclaimed by a new worker, so a crashed worker doesn't strand a receipt.
func TestClaimReceiptDeliveryStaleReclaim(t *testing.T) {
	repo, receiptID := newReceiptRepoWithUndelivered(t)

	// Worker A claims with a very-recent lock (not yet stale).
	recentStaleBefore := time.Now().Add(-10 * time.Minute)
	won1, err := repo.ClaimReceiptDelivery(receiptID, "worker-A", recentStaleBefore)
	require.NoError(t, err)
	require.True(t, won1, "worker-A must win the initial claim")

	// Worker B uses a future staleBefore (everything older than "now+1h" is stale),
	// so worker A's lock is now considered stale and worker B reclaims it.
	futureStaleBefore := time.Now().Add(time.Hour)
	won2, err := repo.ClaimReceiptDelivery(receiptID, "worker-B", futureStaleBefore)
	require.NoError(t, err)
	require.True(t, won2, "worker-B must reclaim the stale lock")
}

// TestClaimReceiptDeliveryAlreadyDeliveredIsNoOp asserts that a receipt with
// delivered_at already set cannot be claimed (the delivery idempotency guard).
func TestClaimReceiptDeliveryAlreadyDeliveredIsNoOp(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)

	issued := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	delivered := issued.Add(time.Hour)
	r := &database.FiscalReceipt{
		BusinessID:       1,
		SettingsID:       1,
		BillID:           2,
		Country:          "AR",
		Provider:         "arca",
		Action:           ActionIssueReceipt,
		ReceiptType:      "factura_c",
		TotalAmountCents: 12100,
		Currency:         "ARS",
		Status:           database.FiscalStatusAuthorized,
		IssuedAt:         &issued,
		DeliveredAt:      &delivered, // already delivered
	}
	require.NoError(t, db.Create(r).Error)

	staleBefore := time.Now().Add(-10 * time.Minute)
	won, err := repo.ClaimReceiptDelivery(r.ID, "worker-A", staleBefore)
	require.NoError(t, err)
	require.False(t, won, "already-delivered receipt must not be claimable")
}
