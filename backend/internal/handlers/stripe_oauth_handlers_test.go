package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appconfig "github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins/stripe"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func TestStripeOAuthStartReturnsAuthorizationURL(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)

	client := &stripe.OAuthClient{
		ClientID:     "ca_test_client_id",
		ClientSecret: "sk_test_platform",
		RedirectURI:  "https://api.payverge.io/api/v1/stripe/oauth/callback",
		BaseAuthURL:  "https://connect.stripe.com",
		BaseTokenURL: "https://connect.stripe.com",
	}
	h := NewStripeOAuthHandlers(services.NewPluginService(database.GetDBWrapper()), client)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Set("user_id", "user-1")
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	h.HandleOAuthStart(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	authURL := resp["authorization_url"]
	require.NotEmpty(t, authURL)
	u, err := url.Parse(authURL)
	require.NoError(t, err)
	q := u.Query()
	assert.Equal(t, "ca_test_client_id", q.Get("client_id"))
	assert.Equal(t, "code", q.Get("response_type"))
	assert.Equal(t, "read_write", q.Get("scope"))
	assert.NotEmpty(t, q.Get("state"))
	assert.Empty(t, q.Get("code_challenge"))
	assert.Empty(t, q.Get("code_challenge_method"))

	entry, ok := h.stateStore.Consume(q.Get("state"))
	require.True(t, ok)
	assert.Equal(t, business.ID, entry.businessID)
	_, ok = h.stateStore.Consume(q.Get("state"))
	assert.False(t, ok, "state must be single-use")
}

func TestStripeOAuthStartNotConfigured(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)
	h := NewStripeOAuthHandlers(nil, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	h.HandleOAuthStart(c)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestStripeOAuthCallbackRejectsForgedState(t *testing.T) {
	setupHandlerTestDB(t)

	client := &stripe.OAuthClient{
		ClientID: "ca_id", ClientSecret: "sk_secret",
		RedirectURI: "https://api.payverge.io/api/v1/stripe/oauth/callback",
	}
	h := NewStripeOAuthHandlers(nil, client)
	h.frontendBase = func() string { return "https://app.payverge.io" }

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=abc&state=forged", nil)

	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	loc, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "error", loc.Query().Get("stripe_connect"))
	assert.Equal(t, "state", loc.Query().Get("reason"))
}

func TestStripeOAuthCallbackRejectsMissingCode(t *testing.T) {
	client := &stripe.OAuthClient{ClientID: "ca", ClientSecret: "sk"}
	h := NewStripeOAuthHandlers(nil, client)
	h.frontendBase = func() string { return "https://app.payverge.io" }

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?state=x", nil)
	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	loc, _ := url.Parse(w.Header().Get("Location"))
	assert.Equal(t, "error", loc.Query().Get("stripe_connect"))
	assert.Equal(t, "missing", loc.Query().Get("reason"))
}

func TestStripeOAuthCallbackHappyPath(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "stripe",
		DisplayName: "Stripe",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/oauth/token") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"token_type":             "bearer",
				"scope":                  "read_write",
				"livemode":               true,
				"stripe_user_id":         "acct_oauth_happy_1",
				"stripe_publishable_key": "pk_live_oauth_pub",
				"access_token":           "sk_live_oauth_access",
				"refresh_token":          "rt_oauth_refresh",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer tokenServer.Close()

	client := &stripe.OAuthClient{
		ClientID:     "ca_id",
		ClientSecret: "sk_platform",
		RedirectURI:  "https://api.payverge.io/api/v1/stripe/oauth/callback",
		BaseTokenURL: tokenServer.URL,
		HTTPClient:   tokenServer.Client(),
	}
	h := NewStripeOAuthHandlers(services.NewPluginService(database.GetDBWrapper()), client)
	h.frontendBase = func() string { return "https://app.payverge.io" }

	state := "state-happy-1"
	h.stateStore.Put(state, stripeOAuthState{
		businessID: business.ID,
		userID:     "user-1",
		expiresAt:  time.Now().Add(5 * time.Minute),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=auth-code&state="+state, nil)

	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	loc, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "success", loc.Query().Get("stripe_connect"))

	cfg, enabled, exists, err := database.GetBusinessPluginConfigState(business.ID, "stripe")
	require.NoError(t, err)
	require.True(t, exists)
	require.True(t, enabled)
	assert.Equal(t, "oauth", cfg["connection_mode"])
	assert.Equal(t, "acct_oauth_happy_1", cfg["stripe_user_id"])
	assert.Equal(t, "connected", cfg["oauth_status"])
	assert.Equal(t, true, cfg["live_mode"])
	assert.Equal(t, "pk_live_oauth_pub", cfg["publishable_key"])
	assert.Equal(t, "sk_live_oauth_access", cfg["access_token"])
	_, hasSecret := cfg["secret_key"]
	assert.False(t, hasSecret, "OAuth mode must not retain merchant secret_key")
}

func TestStripeOAuthCallback_MergesExistingConfig(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name: "stripe", DisplayName: "Stripe",
		Category: database.PluginCategoryPayment, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"secret_key":      "sk_live_manual_old",
		"publishable_key": "pk_live_manual_old",
		"webhook_secret":  "whsec_keep_me",
		"auto_capture":    true,
	}))

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"token_type":             "bearer",
			"scope":                  "read_write",
			"livemode":               true,
			"stripe_user_id":         "acct_merged_99",
			"stripe_publishable_key": "pk_live_from_oauth",
			"access_token":           "sk_live_from_oauth",
		})
	}))
	defer tokenServer.Close()

	client := &stripe.OAuthClient{
		ClientID: "ca", ClientSecret: "sk",
		BaseTokenURL: tokenServer.URL, HTTPClient: tokenServer.Client(),
	}
	h := NewStripeOAuthHandlers(services.NewPluginService(database.GetDBWrapper()), client)
	h.frontendBase = func() string { return "https://app.payverge.io" }
	state := "state-merge"
	h.stateStore.Put(state, stripeOAuthState{
		businessID: business.ID, expiresAt: time.Now().Add(time.Minute),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=c&state="+state, nil)
	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	cfg, _, _, err := database.GetBusinessPluginConfigState(business.ID, "stripe")
	require.NoError(t, err)
	assert.Equal(t, "oauth", cfg["connection_mode"])
	assert.Equal(t, "acct_merged_99", cfg["stripe_user_id"])
	// Non-credential operator settings survive merge…
	assert.Equal(t, true, cfg["auto_capture"])
	// …but manual secrets must NOT: leftover webhook_secret would win
	// verification and 401 Connect-signed platform events.
	_, hasWH := cfg["webhook_secret"]
	assert.False(t, hasWH, "manual webhook_secret must be cleared on OAuth connect")
	_, hasSecret := cfg["secret_key"]
	assert.False(t, hasSecret)
	assert.Equal(t, "pk_live_from_oauth", cfg["publishable_key"])
}

func TestStripeOAuthCallback_RejectsSandboxInProduction(t *testing.T) {
	appconfig.SetProductionModeOverride(true)
	t.Cleanup(func() { appconfig.SetProductionModeOverride(false) })

	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name: "stripe", DisplayName: "Stripe",
		Category: database.PluginCategoryPayment, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"token_type":     "bearer",
			"livemode":       false,
			"stripe_user_id": "acct_test_mode",
		})
	}))
	defer tokenServer.Close()

	client := &stripe.OAuthClient{
		ClientID: "ca", ClientSecret: "sk",
		BaseTokenURL: tokenServer.URL, HTTPClient: tokenServer.Client(),
	}
	h := NewStripeOAuthHandlers(services.NewPluginService(database.GetDBWrapper()), client)
	h.frontendBase = func() string { return "https://app.payverge.io" }
	state := "state-sandbox"
	h.stateStore.Put(state, stripeOAuthState{
		businessID: business.ID, expiresAt: time.Now().Add(time.Minute),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=c&state="+state, nil)
	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	loc := w.Header().Get("Location")
	require.Contains(t, loc, "stripe_connect=error")
	require.Contains(t, loc, "sandbox_credentials")
	_, _, _, err := database.GetBusinessPluginConfigState(business.ID, "stripe")
	require.Error(t, err)
}

func TestStripeOAuthCallback_ExpiredState(t *testing.T) {
	client := &stripe.OAuthClient{ClientID: "ca", ClientSecret: "sk"}
	h := NewStripeOAuthHandlers(nil, client)
	h.frontendBase = func() string { return "https://app.payverge.io" }
	state := "expired-state"
	h.stateStore.Put(state, stripeOAuthState{
		businessID: 1, expiresAt: time.Now().Add(-time.Minute),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=c&state="+state, nil)
	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	loc, _ := url.Parse(w.Header().Get("Location"))
	assert.Equal(t, "error", loc.Query().Get("stripe_connect"))
	assert.Equal(t, "state", loc.Query().Get("reason"))
}
