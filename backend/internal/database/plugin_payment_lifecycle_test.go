package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResolvePendingPluginPaymentReleasesSplitHoldAndPreservesConfirmedTracker(t *testing.T) {
	_, bill := setupPendingAlternativePaymentResolutionTest(t)
	now := time.Now().UTC()
	holdExpiresAt := now.Add(15 * time.Minute)
	share := BillSplitShare{
		BillID: bill.ID, GuestSessionID: "plugin-guest", DisplayName: "Plugin Guest",
		Mode: BillSplitModeCustom, AmountCents: 4_000,
		Status: BillSplitShareStatusHeld, HoldExpiresAt: &holdExpiresAt,
	}
	require.NoError(t, db.Create(&share).Error)

	pending := AlternativePayment{
		BillID: bill.ID, ParticipantAddr: "mp_pending", ParticipantName: fmt.Sprintf("mercadopago|split_share_id=%d", share.ID),
		Amount: 4_000, BillAmountCents: 4_000, PaymentMethod: AlternativePaymentMethod("mercadopago"), Status: AltPaymentStatusPending,
	}
	require.NoError(t, db.Create(&pending).Error)

	resolved, err := ResolvePendingPluginPayment(bill.ID, "mercadopago", "mp_pending", AltPaymentStatusCancelled, now)
	require.NoError(t, err)
	require.True(t, resolved)
	require.NoError(t, db.First(&pending, pending.ID).Error)
	require.Equal(t, AltPaymentStatusCancelled, pending.Status)
	require.Equal(t, "plugin_webhook", pending.ResolvedBy)

	var releasedShare BillSplitShare
	require.NoError(t, db.First(&releasedShare, share.ID).Error)
	require.Equal(t, BillSplitShareStatusReleased, releasedShare.Status)
	require.Nil(t, releasedShare.HoldExpiresAt)
	require.NotNil(t, releasedShare.ReleasedAt)

	resolved, err = ResolvePendingPluginPayment(bill.ID, "mercadopago", "mp_pending", AltPaymentStatusCancelled, now.Add(time.Second))
	require.NoError(t, err)
	require.False(t, resolved, "replaying the same terminal event must be an idempotent no-op")

	confirmed := AlternativePayment{
		BillID: bill.ID, ParticipantAddr: "mp_confirmed", ParticipantName: "mercadopago",
		Amount: 1_000, BillAmountCents: 1_000, PaymentMethod: AlternativePaymentMethod("mercadopago"), Status: AltPaymentStatusConfirmed,
	}
	require.NoError(t, db.Create(&confirmed).Error)
	resolved, err = ResolvePendingPluginPayment(bill.ID, "mercadopago", "mp_confirmed", AltPaymentStatusExpired, now)
	require.NoError(t, err)
	require.False(t, resolved)
	require.NoError(t, db.First(&confirmed, confirmed.ID).Error)
	require.Equal(t, AltPaymentStatusConfirmed, confirmed.Status)
}
