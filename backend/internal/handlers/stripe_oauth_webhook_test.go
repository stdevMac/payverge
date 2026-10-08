package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/plugins/stripe"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func TestStripeConnectDeauth_MarksOAuthBusinessReauthRequired(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{}, &database.BusinessPlugin{}, &database.WebhookEvent{},
	))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name: "stripe", DisplayName: "Stripe",
		Category: database.PluginCategoryPayment, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  "acct_deauth_target",
		"oauth_status":    "connected",
		"access_token":    "sk_live_oauth_access",
		"live_mode":       true,
	}))

	// Manual business with same plugin name but different account — must not be touched.
	manualBiz := createTestBusiness(t)
	require.NoError(t, database.EnableBusinessPlugin(manualBiz.ID, plugin.ID, map[string]interface{}{
		"secret_key":      "sk_live_manual_only_key_long",
		"publishable_key": "pk_live_manual_only_key_long",
		"webhook_secret":  "whsec_manual_biz",
		"connection_mode": "manual",
	}))

	t.Setenv("STRIPE_CONNECT_WEBHOOK_SECRET", "whsec_connect_platform")
	plugins.GlobalRegistry.RegisterPlugin(stripe.NewStripePlugin(services.NewPluginService(database.GetDBWrapper())))
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("stripe") })

	payload, err := json.Marshal(map[string]interface{}{
		"id":      "evt_deauth_1",
		"type":    "account.application.deauthorized",
		"account": "acct_deauth_target",
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"id":   "ca_app",
				"name": "Payverge",
			},
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
	c.Request.Header.Set("Stripe-Signature", signStripePayload(t, payload, "whsec_connect_platform"))

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "stripe")

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	cfg, _, _, err := database.GetBusinessPluginConfigState(business.ID, "stripe")
	require.NoError(t, err)
	assert.Equal(t, "reauth_required", cfg["oauth_status"])
	assert.Equal(t, "acct_deauth_target", cfg["stripe_user_id"])
	_, hasAT := cfg["access_token"]
	assert.False(t, hasAT, "access_token should be cleared on deauth")

	// Manual business config unchanged.
	mCfg, err := database.GetBusinessPluginConfig(manualBiz.ID, "stripe")
	require.NoError(t, err)
	assert.Equal(t, "sk_live_manual_only_key_long", mCfg["secret_key"])
	assert.NotEqual(t, "reauth_required", mCfg["oauth_status"])
}

func TestStripeManualWebhook_StillVerifiesWithBusinessSecret(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{}, &database.BusinessPlugin{}, &database.WebhookEvent{},
		&database.Bill{},
	))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name: "stripe", DisplayName: "Stripe",
		Category: database.PluginCategoryPayment, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"secret_key":      "sk_test_manual_webhook_key_ok",
		"publishable_key": "pk_test_manual_webhook_key_ok",
		"webhook_secret":  "whsec_manual_only_secret",
	}))

	// Platform connect secret set — must NOT be required for manual path when
	// business secret verifies.
	t.Setenv("STRIPE_CONNECT_WEBHOOK_SECRET", "whsec_should_not_be_needed")
	plugins.GlobalRegistry.RegisterPlugin(stripe.NewStripePlugin(services.NewPluginService(database.GetDBWrapper())))
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("stripe") })

	bill := &database.Bill{
		BusinessID:     business.ID,
		BillNumber:     fmt.Sprintf("STRIPE-OAUTH-TEST-%d", business.ID),
		Status:         "open",
		TotalAmount:    1000,
		SettlementAddr: "0x0",
		TippingAddr:    "0x0",
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	payload, err := json.Marshal(map[string]interface{}{
		"id":   "evt_manual_checkout_1",
		"type": "checkout.session.completed",
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"id":             "cs_manual_1",
				"payment_intent": "pi_manual_1",
				"amount_total":   float64(1000),
				"currency":       "usd",
				"payment_status": "paid",
				"metadata": map[string]interface{}{
					"bill_id":     fmt.Sprintf("%d", bill.ID),
					"business_id": fmt.Sprintf("%d", business.ID),
				},
			},
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
	// Sign with business secret (not connect platform secret).
	c.Request.Header.Set("Stripe-Signature", signStripePayload(t, payload, "whsec_manual_only_secret"))

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "stripe")

	// Manual path must accept the business-signed event (200 or settlement
	// success — not 401 invalid signature).
	require.NotEqual(t, http.StatusUnauthorized, w.Code, "body=%s", w.Body.String())
	require.NotEqual(t, http.StatusServiceUnavailable, w.Code, "body=%s", w.Body.String())
	// Signature verified; settlement may succeed or fail on bill state — either
	// is fine as long as we did not reject the signature.
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest || w.Code == http.StatusInternalServerError,
		"unexpected status %d body=%s", w.Code, w.Body.String())
}

func TestStripeManualWebhook_RejectsWrongSecret(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{}, &database.BusinessPlugin{},
	))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name: "stripe", DisplayName: "Stripe",
		Category: database.PluginCategoryPayment, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"secret_key":      "sk_test_manual_webhook_key_ok",
		"publishable_key": "pk_test_manual_webhook_key_ok",
		"webhook_secret":  "whsec_manual_correct",
	}))

	plugins.GlobalRegistry.RegisterPlugin(stripe.NewStripePlugin(nil))
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("stripe") })

	payload, err := json.Marshal(map[string]interface{}{
		"id":   "evt_bad_sig",
		"type": "checkout.session.completed",
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"id": "cs_x", "payment_intent": "pi_x",
				"amount_total": float64(100), "currency": "usd",
				"payment_status": "paid",
				"metadata": map[string]interface{}{
					"business_id": fmt.Sprintf("%d", business.ID),
					"bill_id":     "1",
				},
			},
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
	c.Request.Header.Set("Stripe-Signature", signStripePayload(t, payload, "whsec_wrong_secret"))

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "stripe")
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

// Full shipped path: manual config → real HandleOAuthCallback → Connect-signed
// deauth with STRIPE_CONNECT_WEBHOOK_SECRET (the primary migration topology).
func TestStripeOAuthCallback_ThenConnectDeauth_PlatformSecret(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{}, &database.BusinessPlugin{}, &database.WebhookEvent{},
	))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name: "stripe", DisplayName: "Stripe",
		Category: database.PluginCategoryPayment, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"secret_key":      "sk_live_manual_pre_oauth_key",
		"publishable_key": "pk_live_manual_pre_oauth_key",
		"webhook_secret":  "whsec_manual_pre_oauth_stale",
		"auto_capture":    true,
	}))

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"token_type":     "bearer",
			"livemode":       true,
			"stripe_user_id": "acct_e2e_manual_to_oauth",
			"access_token":   "sk_live_oauth_e2e",
		})
	}))
	defer tokenServer.Close()

	client := &stripe.OAuthClient{
		ClientID: "ca", ClientSecret: "sk_platform",
		BaseTokenURL: tokenServer.URL, HTTPClient: tokenServer.Client(),
	}
	h := NewStripeOAuthHandlers(services.NewPluginService(database.GetDBWrapper()), client)
	h.frontendBase = func() string { return "https://app.payverge.io" }
	state := "state-e2e-migrate"
	h.stateStore.Put(state, stripeOAuthState{
		businessID: business.ID, expiresAt: time.Now().Add(time.Minute),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=c&state="+state, nil)
	h.HandleOAuthCallback(c)
	require.Equal(t, http.StatusFound, w.Code)
	require.Contains(t, w.Header().Get("Location"), "stripe_connect=success")

	cfg, err := database.GetBusinessPluginConfig(business.ID, "stripe")
	require.NoError(t, err)
	require.Equal(t, "oauth", cfg["connection_mode"])
	_, hasWH := cfg["webhook_secret"]
	require.False(t, hasWH, "real OAuth callback must clear manual webhook_secret")

	t.Setenv("STRIPE_CONNECT_WEBHOOK_SECRET", "whsec_e2e_connect_platform")
	plugins.GlobalRegistry.RegisterPlugin(stripe.NewStripePlugin(services.NewPluginService(database.GetDBWrapper())))
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("stripe") })

	payload, err := json.Marshal(map[string]interface{}{
		"id":      "evt_e2e_deauth",
		"type":    "account.application.deauthorized",
		"account": "acct_e2e_manual_to_oauth",
		"data":    map[string]interface{}{"object": map[string]interface{}{"id": "ca"}},
	})
	require.NoError(t, err)

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
	c2.Request.Header.Set("Stripe-Signature", signStripePayload(t, payload, "whsec_e2e_connect_platform"))
	NewPluginHandlers(nil, nil).handlePaymentWebhook(c2, "stripe")
	require.Equal(t, http.StatusOK, w2.Code, "body=%s", w2.Body.String())

	cfg, err = database.GetBusinessPluginConfig(business.ID, "stripe")
	require.NoError(t, err)
	assert.Equal(t, "reauth_required", cfg["oauth_status"])
}

// Manual→OAuth migration path: business previously had a per-account
// webhook_secret. After OAuth connect that secret is cleared, and Connect
// events signed with STRIPE_CONNECT_WEBHOOK_SECRET must still verify.
func TestStripeConnectDeauth_AfterManualToOAuthMerge_UsesPlatformSecret(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{}, &database.BusinessPlugin{}, &database.WebhookEvent{},
	))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name: "stripe", DisplayName: "Stripe",
		Category: database.PluginCategoryPayment, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)

	// Start as manual with a per-business webhook secret.
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"secret_key":      "sk_live_manual_before_oauth_xx",
		"publishable_key": "pk_live_manual_before_oauth_xx",
		"webhook_secret":  "whsec_stale_manual_must_not_win",
		"auto_capture":    true,
	}))

	// Simulate OAuth callback merge: clear manual secrets, set oauth fields.
	// (Same outcome as HandleOAuthCallback after a real connect.)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  "acct_migrated_from_manual",
		"oauth_status":    "connected",
		"live_mode":       true,
		"auto_capture":    true,
		// Explicitly no secret_key / webhook_secret — matches fixed callback.
	}))

	cfg, err := database.GetBusinessPluginConfig(business.ID, "stripe")
	require.NoError(t, err)
	_, hasStale := cfg["webhook_secret"]
	require.False(t, hasStale, "fixture must not retain manual webhook_secret")

	t.Setenv("STRIPE_CONNECT_WEBHOOK_SECRET", "whsec_connect_after_migrate")
	// Poison other env secrets so only Connect secret is the valid path.
	t.Setenv("STRIPE_PLUGIN_WEBHOOK_SECRET", "")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "")

	plugins.GlobalRegistry.RegisterPlugin(stripe.NewStripePlugin(services.NewPluginService(database.GetDBWrapper())))
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("stripe") })

	payload, err := json.Marshal(map[string]interface{}{
		"id":      "evt_deauth_migrated",
		"type":    "account.application.deauthorized",
		"account": "acct_migrated_from_manual",
		"data": map[string]interface{}{
			"object": map[string]interface{}{"id": "ca_app", "name": "Payverge"},
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
	// Sign with platform Connect secret — must verify for OAuth businesses.
	c.Request.Header.Set("Stripe-Signature", signStripePayload(t, payload, "whsec_connect_after_migrate"))

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "stripe")

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	cfg, err = database.GetBusinessPluginConfig(business.ID, "stripe")
	require.NoError(t, err)
	assert.Equal(t, "reauth_required", cfg["oauth_status"])
}

// Even if a stale per-business webhook_secret somehow remains on an OAuth
// config, Connect-signed events must still verify via platform secret.
func TestStripeOAuthWebhook_ConnectSecretWinsOverStaleBusinessSecret(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{}, &database.BusinessPlugin{}, &database.WebhookEvent{},
	))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name: "stripe", DisplayName: "Stripe",
		Category: database.PluginCategoryPayment, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	// Intentionally leave a stale manual webhook_secret on an OAuth row
	// (pre-fix world or partial write).
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  "acct_stale_whsec",
		"oauth_status":    "connected",
		"webhook_secret":  "whsec_stale_manual_leftover",
	}))

	t.Setenv("STRIPE_CONNECT_WEBHOOK_SECRET", "whsec_platform_connect_ok")
	plugins.GlobalRegistry.RegisterPlugin(stripe.NewStripePlugin(services.NewPluginService(database.GetDBWrapper())))
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("stripe") })

	payload, err := json.Marshal(map[string]interface{}{
		"id":      "evt_deauth_stale",
		"type":    "account.application.deauthorized",
		"account": "acct_stale_whsec",
		"data":    map[string]interface{}{"object": map[string]interface{}{"id": "ca"}},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
	// Sign with Connect platform secret, NOT the stale business secret.
	c.Request.Header.Set("Stripe-Signature", signStripePayload(t, payload, "whsec_platform_connect_ok"))

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "stripe")
	require.Equal(t, http.StatusOK, w.Code, "body=%s — Connect secret must verify even with stale business webhook_secret", w.Body.String())
}

func TestExtractPluginWebhookBusinessID_StripeAccountLookup(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name: "stripe", DisplayName: "Stripe",
		Category: database.PluginCategoryPayment, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  "acct_lookup_99",
		"oauth_status":    "connected",
	}))

	payload := map[string]interface{}{
		"type":    "checkout.session.completed",
		"account": "acct_lookup_99",
		"data":    map[string]interface{}{"object": map[string]interface{}{}},
	}
	got := extractPluginWebhookBusinessID("stripe", payload, nil)
	assert.Equal(t, business.ID, got)
}
