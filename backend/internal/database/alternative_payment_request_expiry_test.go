package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAlternativePaymentRequestExpired_DerivesFromCreatedAt(t *testing.T) {
	now := time.Date(2026, 8, 14, 15, 0, 0, 0, time.UTC)

	stale := AlternativePayment{CreatedAt: now.Add(-24*time.Hour - time.Second)}
	require.True(t, AlternativePaymentRequestExpired(stale, now),
		"pending requests older than 24h must expire even when expires_at is nil")

	fresh := AlternativePayment{CreatedAt: now.Add(-23 * time.Hour)}
	require.False(t, AlternativePaymentRequestExpired(fresh, now),
		"requests younger than 24h with no expires_at remain confirmable")

	stored := now.Add(-time.Minute)
	early := AlternativePayment{
		CreatedAt: now.Add(-2 * time.Hour),
		ExpiresAt: &stored,
	}
	require.True(t, AlternativePaymentRequestExpired(early, now),
		"an earlier stored expires_at still expires the request")
}

func TestConfirmPendingAlternativePayment_RejectsCreatedAtExpiryWithoutExpiresAt(t *testing.T) {
	_, bill := setupPendingAlternativePaymentResolutionTest(t)
	now := time.Now().UTC()
	createdAt := now.Add(-25 * time.Hour)
	payment := AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		ParticipantName: "Stale card",
		Amount:          5_000,
		BillAmountCents: 5_000,
		PaymentMethod:   PaymentMethodCard,
		Status:          AltPaymentStatusPending,
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}
	require.NoError(t, db.Create(&payment).Error)
	require.NoError(t, db.Model(&payment).Updates(map[string]any{
		"created_at": createdAt,
		"updated_at": createdAt,
		"expires_at": nil,
	}).Error)

	_, _, err := ConfirmPendingAlternativePayment(bill.ID, payment.ID, "owner", nil)
	require.ErrorIs(t, err, ErrAlternativePaymentRequestExpired)

	var reloadedBill Bill
	require.NoError(t, db.First(&reloadedBill, bill.ID).Error)
	require.Zero(t, reloadedBill.PaidAmount)
	require.Equal(t, BillStatusOpen, reloadedBill.Status)

	var reloaded AlternativePayment
	require.NoError(t, db.First(&reloaded, payment.ID).Error)
	require.Equal(t, AltPaymentStatusPending, reloaded.Status)
	require.Nil(t, reloaded.ConfirmedAt)
}

func TestConfirmPendingAlternativePayment_AllowsFreshRequestWithoutExpiresAt(t *testing.T) {
	_, bill := setupPendingAlternativePaymentResolutionTest(t)
	now := time.Now().UTC()
	payment := AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		ParticipantName: "Fresh card",
		Amount:          4_000,
		BillAmountCents: 4_000,
		PaymentMethod:   PaymentMethodCard,
		Status:          AltPaymentStatusPending,
		CreatedAt:       now.Add(-23 * time.Hour),
		UpdatedAt:       now.Add(-23 * time.Hour),
	}
	require.NoError(t, db.Create(&payment).Error)
	require.NoError(t, db.Model(&payment).Updates(map[string]any{
		"created_at": now.Add(-23 * time.Hour),
		"expires_at": nil,
	}).Error)

	confirmed, updatedBill, err := ConfirmPendingAlternativePayment(bill.ID, payment.ID, "owner", nil)
	require.NoError(t, err)
	require.Equal(t, AltPaymentStatusConfirmed, confirmed.Status)
	require.Equal(t, int64(4_000), updatedBill.PaidAmount)
}
