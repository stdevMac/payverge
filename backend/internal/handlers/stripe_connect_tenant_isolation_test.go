package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/plugins/stripe"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// stripeConnectFixture registers the stripe catalog row and returns it.
func stripeConnectPluginRow(t *testing.T) *database.Plugin {
	t.Helper()
	row := &database.Plugin{
		Name: "stripe", DisplayName: "Stripe",
		Category: database.PluginCategoryPayment, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(row).Error)
	return row
}

// TestStripeConnectWebhook_RejectsForgedCrossTenantMetadata is the tenant
// isolation regression for Stripe Connect.
//
// Every OAuth merchant's events land on the SAME platform endpoint signed with
// the SAME STRIPE_CONNECT_WEBHOOK_SECRET, so a valid signature proves only
// "some connected merchant". The attacker here is a legitimately connected
// merchant (acct_attacker) who sends an event for their OWN account but stamps
// the victim's business_id and bill_id into metadata. Routing on metadata would
// settle the victim's bill with the attacker's payment.
//
// Guard-removal proof: delete the stripeTenant.metadataMismatch rejection in
// handlePaymentWebhook. This test MUST fail — the victim's bill settles
// (PaidAmount > 0) because the payload names it and the PAY-1 ownership guard
// compares bill.BusinessID against the same forged metadata business id.
func TestStripeConnectWebhook_RejectsForgedCrossTenantMetadata(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{}, &database.BusinessPlugin{}, &database.WebhookEvent{},
	))

	attacker := createTestBusinessWithName(t, "AttackerMerchant")
	victim := createTestBusinessWithName(t, "VictimMerchant")
	victimBill := createTestBillForBiz(t, victim.ID)

	pluginRow := stripeConnectPluginRow(t)
	require.NoError(t, database.EnableBusinessPlugin(attacker.ID, pluginRow.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  "acct_attacker",
		"oauth_status":    "connected",
		"live_mode":       true,
	}))
	require.NoError(t, database.EnableBusinessPlugin(victim.ID, pluginRow.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  "acct_victim",
		"oauth_status":    "connected",
		"live_mode":       true,
	}))

	t.Setenv("STRIPE_CONNECT_WEBHOOK_SECRET", "whsec_connect_platform_shared")

	// The double drives a real settlement on the victim's bill so the guard is
	// load-bearing: without it, this webhook marks the bill paid.
	testPlugin := &testPaymentPlugin{
		name:            "stripe",
		verifySignature: true,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			PaymentID: "cs_forged_cross_tenant",
			BillID:    victimBill.ID,
			Amount:    1000,
			Currency:  "usd",
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(testPlugin)
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("stripe") })

	payload, err := json.Marshal(map[string]interface{}{
		"id":      "evt_forged_cross_tenant",
		"type":    "checkout.session.completed",
		"account": "acct_attacker",
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"id":             "cs_forged_cross_tenant",
				"payment_intent": "pi_forged_cross_tenant",
				"amount_total":   float64(1000),
				"currency":       "usd",
				"payment_status": "paid",
				"metadata": map[string]interface{}{
					"bill_id":     fmt.Sprintf("%d", victimBill.ID),
					"business_id": fmt.Sprintf("%d", victim.ID),
				},
			},
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
	c.Request.Header.Set("Stripe-Signature", signStripePayload(t, payload, "whsec_connect_platform_shared"))

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "stripe")

	require.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())

	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, victimBill.ID).Error)
	assert.Zero(t, reloaded.PaidAmount, "victim bill must not settle from another account's event")
	assert.NotEqual(t, database.BillStatusPaid, reloaded.Status)

	var payments int64
	require.NoError(t, database.GetDB().Model(&database.Payment{}).
		Where("tx_hash = ?", "plugin_cs_forged_cross_tenant").Count(&payments).Error)
	assert.Zero(t, payments, "no Payment row may be created for a forged cross-tenant event")
}

// TestStripeConnectWebhook_UnknownAccountDoesNotAdoptBillOwner covers the other
// half of the same attack: omit metadata.business_id entirely and let the bill
// backfill pick the tenant. A Connect delivery whose account resolves to no
// business must resolve to no business, full stop — never to whoever owns the
// bill_id the payload names.
func TestStripeConnectWebhook_UnknownAccountDoesNotAdoptBillOwner(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{}, &database.BusinessPlugin{}, &database.WebhookEvent{},
	))

	victim := createTestBusinessWithName(t, "VictimMerchantB")
	victimBill := createTestBillForBiz(t, victim.ID)

	pluginRow := stripeConnectPluginRow(t)
	require.NoError(t, database.EnableBusinessPlugin(victim.ID, pluginRow.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  "acct_victim_b",
		"oauth_status":    "connected",
	}))

	t.Setenv("STRIPE_CONNECT_WEBHOOK_SECRET", "whsec_connect_platform_shared")

	testPlugin := &testPaymentPlugin{
		name:            "stripe",
		verifySignature: true,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			PaymentID: "cs_unknown_account",
			BillID:    victimBill.ID,
			Amount:    1000,
			Currency:  "usd",
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(testPlugin)
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("stripe") })

	payload, err := json.Marshal(map[string]interface{}{
		"id":      "evt_unknown_account",
		"type":    "checkout.session.completed",
		"account": "acct_not_connected_here",
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"id":             "cs_unknown_account",
				"payment_intent": "pi_unknown_account",
				"amount_total":   float64(1000),
				"currency":       "usd",
				"payment_status": "paid",
				"metadata": map[string]interface{}{
					"bill_id": fmt.Sprintf("%d", victimBill.ID),
				},
			},
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
	c.Request.Header.Set("Stripe-Signature", signStripePayload(t, payload, "whsec_connect_platform_shared"))

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "stripe")

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())

	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, victimBill.ID).Error)
	assert.Zero(t, reloaded.PaidAmount, "unknown connected account must not settle any bill")
}

// TestStripeConnectWebhook_RejectsAmbiguousConnectedAccount: if two businesses
// somehow hold the same acct_… (legacy data — the OAuth callback now refuses to
// create this), a money event that cannot name its tenant must be rejected, not
// routed to whichever row the database returned first.
func TestStripeConnectWebhook_RejectsAmbiguousConnectedAccount(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{}, &database.BusinessPlugin{}, &database.WebhookEvent{},
	))

	first := createTestBusinessWithName(t, "SharedAccountOne")
	second := createTestBusinessWithName(t, "SharedAccountTwo")

	pluginRow := stripeConnectPluginRow(t)
	for _, biz := range []*database.Business{first, second} {
		require.NoError(t, database.EnableBusinessPlugin(biz.ID, pluginRow.ID, map[string]interface{}{
			"connection_mode": "oauth",
			"stripe_user_id":  "acct_shared_legacy",
			"oauth_status":    "connected",
		}))
	}

	t.Setenv("STRIPE_CONNECT_WEBHOOK_SECRET", "whsec_connect_platform_shared")
	plugins.GlobalRegistry.RegisterPlugin(&testPaymentPlugin{
		name:            "stripe",
		verifySignature: true,
		webhookResponse: &plugins.WebhookResponse{Success: true, Status: "completed", PaymentID: "cs_ambiguous"},
	})
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("stripe") })

	payload, err := json.Marshal(map[string]interface{}{
		"id":      "evt_ambiguous_account",
		"type":    "checkout.session.completed",
		"account": "acct_shared_legacy",
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"id":             "cs_ambiguous",
				"amount_total":   float64(500),
				"currency":       "usd",
				"payment_status": "paid",
			},
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
	c.Request.Header.Set("Stripe-Signature", signStripePayload(t, payload, "whsec_connect_platform_shared"))

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "stripe")

	assert.Equal(t, http.StatusConflict, w.Code, "body=%s", w.Body.String())
}

// TestStripeConnectDeauth_HidesGuestPaymentOption: a revoked OAuth grant must
// stop the guest checkout from offering "Pay with card". The row stays enabled
// so the operator still sees the reconnect CTA, but the guest-facing config
// flag flips off — otherwise every diner gets a checkout that cannot be created.
func TestStripeConnectDeauth_HidesGuestPaymentOption(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{}, &database.BusinessPlugin{}, &database.WebhookEvent{},
	))

	business := createTestBusinessWithName(t, "DeauthGuestVisibility")
	pluginRow := stripeConnectPluginRow(t)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRow.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  "acct_guest_visibility",
		"oauth_status":    "connected",
		"access_token":    "sk_live_oauth_access",
	}))

	// Guests can see the card option while the grant is live.
	cfgBefore, err := database.GetBusinessPluginConfig(business.ID, "stripe")
	require.NoError(t, err)
	require.True(t, evalPaymentPluginConfigEnabled("stripe", cfgBefore))

	t.Setenv("STRIPE_CONNECT_WEBHOOK_SECRET", "whsec_connect_guest_visibility")
	plugins.GlobalRegistry.RegisterPlugin(stripe.NewStripePlugin(services.NewPluginService(database.GetDBWrapper())))
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("stripe") })

	payload, err := json.Marshal(map[string]interface{}{
		"id":      "evt_deauth_guest_visibility",
		"type":    "account.application.deauthorized",
		"account": "acct_guest_visibility",
		"data":    map[string]interface{}{"object": map[string]interface{}{"id": "ca_app"}},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
	c.Request.Header.Set("Stripe-Signature", signStripePayload(t, payload, "whsec_connect_guest_visibility"))

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "stripe")
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	cfgAfter, err := database.GetBusinessPluginConfig(business.ID, "stripe")
	require.NoError(t, err)
	assert.Equal(t, "reauth_required", cfgAfter["oauth_status"])
	assert.False(t, evalPaymentPluginConfigEnabled("stripe", cfgAfter),
		"deauthorized Stripe must not be offered to guests")

	// The operator-facing row is untouched so the reconnect CTA still renders.
	var row database.BusinessPlugin
	require.NoError(t, database.GetDB().
		Where("business_id = ? AND plugin_id = ?", business.ID, pluginRow.ID).
		First(&row).Error)
	assert.True(t, row.IsEnabled, "operator row must stay enabled for the reconnect CTA")
}

// TestStripeConnectDeauth_ReplayDoesNotDisconnectReconnectedMerchant: the
// deauth mutation is destructive and not idempotent in effect, so it must go
// through the webhook_events dedup like every other event. Stripe retries a
// delivery for up to three days; a retry arriving after the merchant
// reconnected would otherwise silently disconnect them again.
func TestStripeConnectDeauth_ReplayDoesNotDisconnectReconnectedMerchant(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{}, &database.BusinessPlugin{}, &database.WebhookEvent{},
	))

	business := createTestBusinessWithName(t, "ReconnectedMerchant")
	pluginRow := stripeConnectPluginRow(t)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRow.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  "acct_replay_target",
		"oauth_status":    "connected",
		"access_token":    "sk_live_first_grant",
	}))

	t.Setenv("STRIPE_CONNECT_WEBHOOK_SECRET", "whsec_connect_replay")
	plugins.GlobalRegistry.RegisterPlugin(stripe.NewStripePlugin(services.NewPluginService(database.GetDBWrapper())))
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("stripe") })

	payload, err := json.Marshal(map[string]interface{}{
		"id":      "evt_deauth_replayed",
		"type":    "account.application.deauthorized",
		"account": "acct_replay_target",
		"data":    map[string]interface{}{"object": map[string]interface{}{"id": "ca_app"}},
	})
	require.NoError(t, err)

	deliver := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
		c.Request.Header.Set("Stripe-Signature", signStripePayload(t, payload, "whsec_connect_replay"))
		NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "stripe")
		return w
	}

	require.Equal(t, http.StatusOK, deliver().Code)
	cfg, err := database.GetBusinessPluginConfig(business.ID, "stripe")
	require.NoError(t, err)
	require.Equal(t, "reauth_required", cfg["oauth_status"])

	// Merchant reconnects (same account, fresh grant).
	cfg["oauth_status"] = "connected"
	cfg["access_token"] = "sk_live_second_grant"
	cfg["enabled"] = true
	require.NoError(t, database.UpdateBusinessPluginConfig(business.ID, "stripe", cfg))

	// Stripe retries the original delivery.
	require.Equal(t, http.StatusOK, deliver().Code)

	after, err := database.GetBusinessPluginConfig(business.ID, "stripe")
	require.NoError(t, err)
	assert.Equal(t, "connected", after["oauth_status"], "replayed deauth must not re-disconnect")
	assert.Equal(t, "sk_live_second_grant", after["access_token"], "replayed deauth must not clear the new grant")
	assert.True(t, evalPaymentPluginConfigEnabled("stripe", after))
}
