package database

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func setupPendingAlternativePaymentResolutionTest(t *testing.T) (*Business, *Bill) {
	t.Helper()
	setupPaymentLedgerTestDB(t)
	require.NoError(t, db.AutoMigrate(&BillSplitShare{}))

	business := &Business{Name: "Resolution Co", IsActive: true}
	require.NoError(t, db.Create(business).Error)
	bill := &Bill{
		BusinessID:  business.ID,
		BillNumber:  "ALT-RESOLUTION",
		Status:      BillStatusOpen,
		Items:       "[]",
		TotalAmount: 10_000,
	}
	require.NoError(t, db.Create(bill).Error)
	return business, bill
}

func TestCancelPendingAlternativePaymentIsDurableIdempotentAndReleasesCapacity(t *testing.T) {
	_, bill := setupPendingAlternativePaymentResolutionTest(t)
	now := time.Now().UTC()
	expiresAt := now.Add(AlternativePaymentRequestTTL)
	payment := AlternativePayment{
		BillID:             bill.ID,
		ParticipantAddr:    "guest",
		ParticipantName:    "Guest",
		Amount:             7_000,
		BillAmountCents:    7_000,
		PaymentMethod:      PaymentMethodCash,
		Status:             AltPaymentStatusPending,
		ExpiresAt:          &expiresAt,
		IdempotencyKeyHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		PayloadHash:        "1111111111111111111111111111111111111111111111111111111111111111",
	}
	require.NoError(t, db.Create(&payment).Error)

	cancelled, err := CancelPendingAlternativePayment(bill.ID, payment.ID, "staff:7", "guest changed tender", now)
	require.NoError(t, err)
	require.Equal(t, AltPaymentStatusCancelled, cancelled.Status)
	require.Equal(t, "staff:7", cancelled.ResolvedBy)
	require.Equal(t, "guest changed tender", cancelled.ResolutionReason)
	require.NotNil(t, cancelled.ResolvedAt)

	replayed, err := CancelPendingAlternativePayment(bill.ID, payment.ID, "staff:7", "guest changed tender", now.Add(time.Second))
	require.NoError(t, err, "same terminal action must be safely replayable")
	require.Equal(t, cancelled.ID, replayed.ID)
	require.Equal(t, cancelled.ResolvedAt, replayed.ResolvedAt, "a replay must not rewrite the audit timestamp")

	service := &AlternativePaymentService{repo: NewRepository[AlternativePayment](db)}
	replacement := &AlternativePayment{
		BillID: bill.ID, ParticipantAddr: "guest", ParticipantName: "Replacement", Amount: 7_000,
		PaymentMethod:      PaymentMethodCard,
		IdempotencyKeyHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		PayloadHash:        "2222222222222222222222222222222222222222222222222222222222222222",
	}
	_, _, err = service.CreatePendingRequest(replacement, now.Add(2*time.Second), "", 0, nil)
	require.NoError(t, err, "a cancelled request must no longer reserve bill capacity")

	_, _, err = ConfirmPendingAlternativePayment(bill.ID, payment.ID, "owner", nil)
	require.ErrorIs(t, err, ErrAlternativePaymentRequestNotPending)
	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	require.Zero(t, reloaded.PaidAmount)
}

func TestRejectPendingAlternativePaymentReleasesLinkedSplitHold(t *testing.T) {
	_, bill := setupPendingAlternativePaymentResolutionTest(t)
	now := time.Now().UTC()
	holdExpiresAt := now.Add(15 * time.Minute)
	share := BillSplitShare{
		BillID:         bill.ID,
		GuestSessionID: "guest-session",
		DisplayName:    "Guest",
		Mode:           BillSplitModeCustom,
		AmountCents:    4_000,
		Status:         BillSplitShareStatusHeld,
		HoldExpiresAt:  &holdExpiresAt,
	}
	require.NoError(t, db.Create(&share).Error)
	expiresAt := now.Add(AlternativePaymentRequestTTL)
	payment := AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: BillSplitAlternativePaymentParticipantAddr(share.ID),
		ParticipantName: "Guest",
		Amount:          share.AmountCents,
		PaymentMethod:   PaymentMethodCash,
		Status:          AltPaymentStatusPending,
		ExpiresAt:       &expiresAt,
	}
	require.NoError(t, db.Create(&payment).Error)

	rejected, err := RejectPendingAlternativePayment(bill.ID, payment.ID, "owner", "cash not received", now)
	require.NoError(t, err)
	require.Equal(t, AltPaymentStatusRejected, rejected.Status)
	require.Equal(t, "cash not received", rejected.ResolutionReason)

	var releasedShare BillSplitShare
	require.NoError(t, db.First(&releasedShare, share.ID).Error)
	require.Equal(t, BillSplitShareStatusReleased, releasedShare.Status)
	require.Nil(t, releasedShare.HoldExpiresAt)
	require.NotNil(t, releasedShare.ReleasedAt)

	_, err = CancelPendingAlternativePayment(bill.ID, payment.ID, "owner", "different action", now.Add(time.Second))
	require.ErrorIs(t, err, ErrAlternativePaymentRequestNotPending)
	_, _, err = ConfirmPendingAlternativePayment(bill.ID, payment.ID, "owner", nil)
	require.ErrorIs(t, err, ErrAlternativePaymentRequestNotPending)
}

func TestResolvePendingAlternativePaymentRejectsWrongBillAndCommitsExpiry(t *testing.T) {
	_, bill := setupPendingAlternativePaymentResolutionTest(t)
	now := time.Now().UTC()
	expiredAt := now.Add(-time.Second)
	payment := AlternativePayment{
		BillID: bill.ID, ParticipantAddr: "guest", Amount: 1_000,
		PaymentMethod: PaymentMethodCash, Status: AltPaymentStatusPending, ExpiresAt: &expiredAt,
	}
	require.NoError(t, db.Create(&payment).Error)

	_, err := RejectPendingAlternativePayment(bill.ID+1, payment.ID, "owner", "", now)
	require.ErrorIs(t, err, ErrAlternativePaymentBillMismatch)

	_, err = RejectPendingAlternativePayment(bill.ID, payment.ID, "owner", "", now)
	require.ErrorIs(t, err, ErrAlternativePaymentRequestExpired)
	require.NoError(t, db.First(&payment, payment.ID).Error)
	require.Equal(t, AltPaymentStatusExpired, payment.Status, "expiry detected during resolution must commit instead of rolling back")

	_, err = RejectPendingAlternativePayment(bill.ID, payment.ID, "owner", "", now)
	require.True(t, errors.Is(err, ErrAlternativePaymentRequestExpired) || errors.Is(err, ErrAlternativePaymentRequestNotPending))
}

// PayableRemainingCents is the cashier remaining: total − paid − active
// pending alternative requests − held split shares. Crypto quotes must use
// this so a reserved cash request cannot be double-committed as USDC (#529).
func PayableRemainingCents(bill Bill, now time.Time) (int64, error) {
	available, err := payableRemainingAfterReservations(bill, 0, now)
	if err != nil {
		return 0, err
	}
	if available < 0 {
		return 0, nil
	}
	return available, nil
}

func TestPayableRemainingCents_SubtractsPendingCashLikeCreatePendingRequest(t *testing.T) {
	_, bill := setupPendingAlternativePaymentResolutionTest(t)
	require.NoError(t, db.Model(bill).Update("total_amount", int64(1750)).Error)
	bill.TotalAmount = 1750
	now := time.Now().UTC()
	expires := now.Add(5 * time.Minute)
	require.NoError(t, db.Create(&AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		ParticipantName: "QA",
		Amount:          1,
		BillAmountCents: 1,
		PaymentMethod:   PaymentMethodCash,
		Status:          AltPaymentStatusPending,
		ExpiresAt:       &expires,
		CreatedAt:       now,
		UpdatedAt:       now,
	}).Error)

	remaining, err := PayableRemainingCents(*bill, now)
	require.NoError(t, err)
	require.Equal(t, int64(1749), remaining)

	exceedsFull, err := CryptoQuoteExceedsRemaining(*bill, 1750, 0, now)
	require.NoError(t, err)
	require.True(t, exceedsFull, "$17.50 quote must be refused while $0.01 cash is reserved")

	exceedsExact, err := CryptoQuoteExceedsRemaining(*bill, 1749, 0, now)
	require.NoError(t, err)
	require.False(t, exceedsExact)
}
