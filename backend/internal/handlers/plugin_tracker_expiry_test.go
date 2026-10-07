package handlers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// P4/Q3: MercadoPago trackers get ExpiresAt; non-MP keep NULL; expired pending
// ORD neither blocks a new charge nor resolves for settlement via webhook lookup.

func TestStorePluginPaymentRecord_SetsExpiresAt(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.AlternativePayment{}))

	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)

	before := time.Now().UTC()
	tracker, err := NewPluginHandlers(nil, nil).storePluginPaymentRecord(
		bill.ID, business.ID, "mercadopago", "ORD01EXPIRYTEST1234567890AB", 1000, "ARS", 1000, 0, nil,
	)
	require.NoError(t, err)
	require.NotNil(t, tracker.ExpiresAt)
	assert.True(t, tracker.ExpiresAt.After(before.Add(23*time.Hour)), "ORD TTL should be ~24h")
	assert.True(t, tracker.ExpiresAt.Before(before.Add(25*time.Hour)))

	guest, err := NewPluginHandlers(nil, nil).storePluginPaymentRecord(
		bill.ID, business.ID, "mercadopago", "mp_tracker_guest_checkout_abc", 1000, "ARS", 1000, 0, nil,
	)
	require.NoError(t, err)
	require.NotNil(t, guest.ExpiresAt)
	assert.True(t, guest.ExpiresAt.After(before.Add(6*24*time.Hour)), "guest Checkout Pro TTL should be ~7d")
	assert.True(t, guest.ExpiresAt.Before(before.Add(8*24*time.Hour)))
}

// Q3: Stripe (and other non-MP) trackers must keep NULL ExpiresAt so a 24h
// default does not silently re-open split-shares after expiry.
func TestStorePluginPaymentRecord_StripeExpiresAtNil(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.AlternativePayment{}))

	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)

	stripe, err := NewPluginHandlers(nil, nil).storePluginPaymentRecord(
		bill.ID, business.ID, "stripe", "pi_stripe_123_no_ttl", 1500, "USD", 1500, 0, nil,
	)
	require.NoError(t, err)
	assert.Nil(t, stripe.ExpiresAt, "stripe trackers must not auto-expire")

	paypal, err := NewPluginHandlers(nil, nil).storePluginPaymentRecord(
		bill.ID, business.ID, "paypal", "PAYID-TEST-NO-TTL", 2000, "USD", 2000, 0, nil,
	)
	require.NoError(t, err)
	assert.Nil(t, paypal.ExpiresAt, "paypal trackers must not auto-expire")
}

func TestFindPendingMercadoPagoTracker_IgnoresExpired(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.AlternativePayment{}))

	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)

	past := time.Now().UTC().Add(-time.Hour)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "ORD01EXPIREDTRACKER123456789",
		ParticipantName: "mercadopago",
		Amount:          1000,
		PaymentMethod:   "mercadopago",
		Status:          database.AltPaymentStatusPending,
		ExpiresAt:       &past,
		IdempotencyKey:  "plugin:mercadopago:ORD01EXPIREDTRACKER123456789",
	}).Error)

	found, ok, err := findPendingMercadoPagoTracker(bill.ID)
	require.NoError(t, err)
	assert.False(t, ok, "expired ORD must not block a new charge")
	assert.Nil(t, found)

	// Sweep should have marked it expired.
	var row database.AlternativePayment
	require.NoError(t, database.GetDB().Where("participant_addr = ?", "ORD01EXPIREDTRACKER123456789").First(&row).Error)
	assert.Equal(t, database.AltPaymentStatusExpired, row.Status)
}

func TestGetBusinessIDByPluginPaymentTracker_ExpiredPendingDoesNotSettle(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.AlternativePayment{}))

	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)

	past := time.Now().UTC().Add(-2 * time.Hour)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "ORD01NOSETTLEEXPIRED12345678",
		ParticipantName: "mercadopago",
		Amount:          5000,
		PaymentMethod:   "mercadopago",
		Status:          database.AltPaymentStatusPending,
		ExpiresAt:       &past,
		IdempotencyKey:  fmt.Sprintf("plugin:mercadopago:ORD01NOSETTLE-%d", time.Now().UnixNano()),
	}).Error)

	_, err := database.GetBusinessIDByPluginPaymentTracker("ORD01NOSETTLEEXPIRED12345678", "mercadopago")
	require.Error(t, err, "expired pending tracker must not resolve business for settlement")

	// Active pending still resolves.
	future := time.Now().UTC().Add(time.Hour)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "ORD01ACTIVESETTLE1234567890AB",
		ParticipantName: "mercadopago",
		Amount:          5000,
		PaymentMethod:   "mercadopago",
		Status:          database.AltPaymentStatusPending,
		ExpiresAt:       &future,
		IdempotencyKey:  fmt.Sprintf("plugin:mercadopago:ORD01ACTIVE-%d", time.Now().UnixNano()),
	}).Error)
	bizID, err := database.GetBusinessIDByPluginPaymentTracker("ORD01ACTIVESETTLE1234567890AB", "mercadopago")
	require.NoError(t, err)
	assert.Equal(t, business.ID, bizID)
}

func TestPluginTrackerExpiresAt_Policy(t *testing.T) {
	now := time.Now().UTC()
	ord := pluginTrackerExpiresAt("mercadopago", "ORD01ABC")
	guest := pluginTrackerExpiresAt("mercadopago", "mp_tracker_xyz")
	mpOther := pluginTrackerExpiresAt("mercadopago", "123456789")
	stripe := pluginTrackerExpiresAt("stripe", "pi_stripe_123")
	paypal := pluginTrackerExpiresAt("paypal", "PAYID-X")

	require.NotNil(t, ord)
	require.NotNil(t, guest)
	require.NotNil(t, mpOther)
	assert.InDelta(t, pluginTrackerORDExpiry.Seconds(), ord.Sub(now).Seconds(), 2)
	assert.InDelta(t, pluginTrackerGuestCheckoutExpiry.Seconds(), guest.Sub(now).Seconds(), 2)
	assert.InDelta(t, pluginTrackerDefaultExpiry.Seconds(), mpOther.Sub(now).Seconds(), 2)
	assert.Nil(t, stripe, "non-MP must not get ExpiresAt")
	assert.Nil(t, paypal, "non-MP must not get ExpiresAt")
}

// Q3: recently-expired ORD trackers are still reconciled (self-heal after sweeper).
func TestReconcilePendingPluginPayments_RecentlyExpiredORDSettles(t *testing.T) {
	setupHandlerTestDB(t)
	migrateSplitPluginTables(t)

	business := createTestBusiness(t)
	bill := &database.Bill{
		BusinessID: business.ID, BillNumber: fmt.Sprintf("B-recon-ord-exp-%d", time.Now().UnixNano()),
		Status: database.BillStatusOpen, Items: "[]", Subtotal: 2500, TotalAmount: 2500,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	// Wall-clock Now() (not UTC) so SQLite datetime compare matches the reconciler.
	expiredAt := time.Now().Add(-2 * time.Hour) // within 48h lookback
	createdAt := expiredAt.Add(-24 * time.Hour)
	orderID := "ORD01RECONEXPIRED1234567890AB"
	tracker := &database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: orderID,
		ParticipantName: "mercadopago",
		Amount:          2500,
		BillAmountCents: 2500,
		PaymentMethod:   "mercadopago",
		Status:          database.AltPaymentStatusExpired,
		ExpiresAt:       &expiredAt,
		IdempotencyKey:  fmt.Sprintf("plugin:mercadopago:%s", orderID),
	}
	require.NoError(t, database.GetDB().Create(tracker).Error)
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).
		Where("id = ?", tracker.ID).
		UpdateColumn("created_at", createdAt).Error)

	registerReconPlugin(t, "mercadopago", "completed")

	NewPluginHandlers(nil, nil).ReconcilePendingPluginPayments(context.Background(), 50)

	settled, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2500), settled.PaidAmount, "recently-expired ORD must self-heal via reconciler")
	_, err = database.GetPaymentByTxHash("plugin_" + orderID)
	assert.NoError(t, err)

	var row database.AlternativePayment
	require.NoError(t, database.GetDB().First(&row, tracker.ID).Error)
	assert.Equal(t, database.AltPaymentStatusConfirmed, row.Status)
}
