package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfirmPendingAlternativePayment_KeepsDeclaredTipWithoutSplitShare(t *testing.T) {
	_, bill := setupPendingAlternativePaymentResolutionTest(t)
	payment := AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		ParticipantName: "Guest",
		Amount:          1000,
		TipAmountCents:  500,
		PaymentMethod:   PaymentMethodCash,
		Status:          AltPaymentStatusPending,
	}
	require.NoError(t, db.Create(&payment).Error)

	_, updatedBill, err := ConfirmPendingAlternativePayment(bill.ID, payment.ID, "owner", nil)
	require.NoError(t, err)
	require.Equal(t, int64(500), updatedBill.TipAmount)

	var reloadedBill Bill
	require.NoError(t, db.First(&reloadedBill, bill.ID).Error)
	require.Equal(t, int64(500), reloadedBill.TipAmount)

	var reloaded AlternativePayment
	require.NoError(t, db.First(&reloaded, payment.ID).Error)
	require.Equal(t, int64(500), reloaded.TipAmountCents)
}

func TestConfirmPendingAlternativePayment_KeepsDeclaredTipWhenSplitShareTipIsZero(t *testing.T) {
	_, bill := setupPendingAlternativePaymentResolutionTest(t)
	holdExpiresAt := time.Now().Add(10 * time.Minute)
	share := BillSplitShare{
		BillID:         bill.ID,
		GuestSessionID: "guest-1",
		DisplayName:    "Guest 1",
		Mode:           BillSplitModeCustom,
		AmountCents:    1000,
		TipCents:       0,
		Status:         BillSplitShareStatusHeld,
		HoldExpiresAt:  &holdExpiresAt,
	}
	require.NoError(t, db.Create(&share).Error)
	payment := AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: BillSplitAlternativePaymentParticipantAddr(share.ID),
		ParticipantName: "Guest 1",
		Amount:          share.AmountCents,
		TipAmountCents:  500,
		PaymentMethod:   PaymentMethodCash,
		Status:          AltPaymentStatusPending,
	}
	require.NoError(t, db.Create(&payment).Error)

	_, updatedBill, err := ConfirmPendingAlternativePayment(bill.ID, payment.ID, "owner", nil)
	require.NoError(t, err)
	require.Equal(t, int64(500), updatedBill.TipAmount)

	var reloadedShare BillSplitShare
	require.NoError(t, db.First(&reloadedShare, share.ID).Error)
	require.Equal(t, int64(500), reloadedShare.TipCents)
	require.Equal(t, BillSplitShareStatusSettled, reloadedShare.Status)

	var reloaded AlternativePayment
	require.NoError(t, db.First(&reloaded, payment.ID).Error)
	require.Equal(t, int64(500), reloaded.TipAmountCents)
}
