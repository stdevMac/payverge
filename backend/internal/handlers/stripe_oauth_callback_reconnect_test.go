package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins/stripe"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// stripeOAuthCallbackFixture wires a callback handler against a fake Stripe
// token endpoint that hands back the given account.
func stripeOAuthCallbackFixture(t *testing.T, account string, liveMode bool) (*StripeOAuthHandlers, *database.Plugin) {
	t.Helper()

	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))
	plugin := &database.Plugin{
		Name: "stripe", DisplayName: "Stripe",
		Category: database.PluginCategoryPayment, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"token_type":             "bearer",
			"scope":                  "read_write",
			"livemode":               liveMode,
			"stripe_user_id":         account,
			"stripe_publishable_key": "pk_live_" + account,
			"access_token":           "sk_live_" + account,
			"refresh_token":          "rt_" + account,
		})
	}))
	t.Cleanup(tokenServer.Close)

	client := &stripe.OAuthClient{
		ClientID: "ca", ClientSecret: "sk",
		BaseTokenURL: tokenServer.URL, HTTPClient: tokenServer.Client(),
	}
	h := NewStripeOAuthHandlers(services.NewPluginService(database.GetDBWrapper()), client)
	h.frontendBase = func() string { return "https://app.payverge.io" }
	return h, plugin
}

// runStripeOAuthCallback drives one callback for businessID and returns the
// parsed redirect target.
func runStripeOAuthCallback(t *testing.T, h *StripeOAuthHandlers, businessID uint) *url.URL {
	t.Helper()

	state := fmt.Sprintf("state-%s-%d", t.Name(), businessID)
	h.stateStore.Put(state, stripeOAuthState{
		businessID: businessID, expiresAt: time.Now().Add(time.Minute),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=c&state="+state, nil)
	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	loc, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	return loc
}

// TestStripeOAuthCallback_LandsOnThePluginsTab: /business/plugins is not a route
// in this app. Every callback used to redirect there, so a merchant who finished
// the Connect flow landed on a 404 and never saw the success toast — which only
// renders inside the dashboard's plugin manager.
func TestStripeOAuthCallback_LandsOnThePluginsTab(t *testing.T) {
	setupHandlerTestDB(t)
	h, _ := stripeOAuthCallbackFixture(t, "acct_landing_1", true)
	business := createTestBusiness(t)

	loc := runStripeOAuthCallback(t, h, business.ID)

	assert.Equal(t, fmt.Sprintf("/business/%d/dashboard", business.ID), loc.Path,
		"callback must land on a route that exists")
	assert.Equal(t, "plugins", loc.Query().Get("tab"))
	assert.Equal(t, "success", loc.Query().Get("stripe_connect"))
}

// TestStripeOAuthCallback_UnknownBusinessRedirectsHome: when the state cannot be
// consumed there is no business to route to, so the fallback must still be a
// real route rather than the dead plugins path.
func TestStripeOAuthCallback_UnknownBusinessRedirectsHome(t *testing.T) {
	setupHandlerTestDB(t)
	h, _ := stripeOAuthCallbackFixture(t, "acct_unknown", true)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=c&state=forged", nil)
	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	loc, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/", loc.Path)
	assert.Empty(t, loc.Query().Get("tab"))
	assert.Equal(t, "state", loc.Query().Get("reason"))
}

// TestStripeOAuthCallback_ReconnectKeepsAccountWebhookSecret: clearing manual
// credentials is a manual→OAuth migration step, not something to redo on every
// callback. An already-connected merchant reconnecting (reauth, scope change)
// must keep the endpoint secret they configured for this very account —
// deleting it silently breaks signature verification for their events.
func TestStripeOAuthCallback_ReconnectKeepsAccountWebhookSecret(t *testing.T) {
	setupHandlerTestDB(t)
	h, plugin := stripeOAuthCallbackFixture(t, "acct_reconnect_same", true)
	business := createTestBusiness(t)

	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  "acct_reconnect_same",
		"oauth_status":    "reauth_required",
		"webhook_secret":  "whsec_account_endpoint_value",
		"enabled":         false,
		"auto_capture":    true,
	}))

	loc := runStripeOAuthCallback(t, h, business.ID)
	assert.Equal(t, "success", loc.Query().Get("stripe_connect"))

	cfg, _, _, err := database.GetBusinessPluginConfigState(business.ID, "stripe")
	require.NoError(t, err)
	assert.Equal(t, "whsec_account_endpoint_value", cfg["webhook_secret"],
		"reconnecting the same account must not wipe its endpoint secret")
	assert.Equal(t, "connected", cfg["oauth_status"])
	assert.Equal(t, true, cfg["auto_capture"])
	// Deauthorization hid the card option from guests; the new grant restores it.
	assert.Equal(t, true, cfg["enabled"],
		"a reconnect must make the payment option visible to guests again")
}

// TestStripeOAuthCallback_AccountSwitchClearsStaleSecret: the flip side — the
// secret belonged to the OLD account, so moving the business to a different
// acct_ must drop it.
func TestStripeOAuthCallback_AccountSwitchClearsStaleSecret(t *testing.T) {
	setupHandlerTestDB(t)
	h, plugin := stripeOAuthCallbackFixture(t, "acct_new_owner", true)
	business := createTestBusiness(t)

	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  "acct_old_owner",
		"oauth_status":    "connected",
		"webhook_secret":  "whsec_old_account_value",
	}))

	loc := runStripeOAuthCallback(t, h, business.ID)
	assert.Equal(t, "success", loc.Query().Get("stripe_connect"))

	cfg, _, _, err := database.GetBusinessPluginConfigState(business.ID, "stripe")
	require.NoError(t, err)
	assert.Equal(t, "acct_new_owner", cfg["stripe_user_id"])
	_, hasWH := cfg["webhook_secret"]
	assert.False(t, hasWH, "a secret scoped to the previous account must not survive the switch")
}

// TestStripeOAuthCallback_RejectsAccountOwnedByAnotherBusiness: Connect delivers
// every event for an account to one platform endpoint, so two businesses on the
// same acct_ is an unresolvable webhook ambiguity (the webhook path 409s). Refuse
// the grant at connect time instead of half-connecting the second merchant.
func TestStripeOAuthCallback_RejectsAccountOwnedByAnotherBusiness(t *testing.T) {
	setupHandlerTestDB(t)
	h, plugin := stripeOAuthCallbackFixture(t, "acct_shared_claim", true)

	owner := createTestBusiness(t)
	require.NoError(t, database.EnableBusinessPlugin(owner.ID, plugin.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  "acct_shared_claim",
		"oauth_status":    "connected",
	}))

	claimant := createTestBusiness(t)
	loc := runStripeOAuthCallback(t, h, claimant.ID)

	assert.Equal(t, "error", loc.Query().Get("stripe_connect"))
	assert.Equal(t, "account_in_use", loc.Query().Get("reason"))
	assert.Equal(t, fmt.Sprintf("/business/%d/dashboard", claimant.ID), loc.Path,
		"the operator still needs to land somewhere real to read the error")

	_, _, exists, err := database.GetBusinessPluginConfigState(claimant.ID, "stripe")
	if err == nil {
		assert.False(t, exists, "a refused grant must not enable the plugin")
	}

	ids, err := database.FindBusinessIDsByStripeUserID("acct_shared_claim")
	require.NoError(t, err)
	assert.Equal(t, []uint{owner.ID}, ids, "the original owner keeps the account")
}

// TestStripeOAuthCallback_SameBusinessReconnectIsNotSelfBlocked guards the
// exclusion clause: the ownership check must ignore the business doing the
// reconnect, or every reauth would fail with account_in_use.
func TestStripeOAuthCallback_SameBusinessReconnectIsNotSelfBlocked(t *testing.T) {
	setupHandlerTestDB(t)
	h, plugin := stripeOAuthCallbackFixture(t, "acct_self_reconnect", true)
	business := createTestBusiness(t)

	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  "acct_self_reconnect",
		"oauth_status":    "connected",
	}))

	loc := runStripeOAuthCallback(t, h, business.ID)
	assert.Equal(t, "success", loc.Query().Get("stripe_connect"))
}
