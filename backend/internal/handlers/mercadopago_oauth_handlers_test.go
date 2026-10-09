package handlers

import (
	"context"
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
	"github.com/stdevmac/payverge/backend/internal/plugins/mercadopago"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func TestMercadoPagoOAuthStartReturnsAuthorizationURL(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)

	client := &mercadopago.OAuthClient{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURI:  "https://api.payverge.io/api/v1/mercadopago/oauth/callback",
		BaseAuthURL:  "https://auth.mercadopago.com",
		BaseAPIURL:   "https://api.mercadopago.com",
	}
	h := NewMercadoPagoOAuthHandlers(services.NewPluginService(database.GetDBWrapper()), client)

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
	assert.Equal(t, "test-client-id", q.Get("client_id"))
	assert.Equal(t, "code", q.Get("response_type"))
	assert.Equal(t, "mp", q.Get("platform_id"))
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	assert.NotEmpty(t, q.Get("state"))
	assert.NotEmpty(t, q.Get("code_challenge"))

	entry, ok := h.stateStore.Consume(q.Get("state"))
	require.True(t, ok)
	assert.Equal(t, business.ID, entry.businessID)
	assert.NotEmpty(t, entry.codeVerifier)
	_, ok = h.stateStore.Consume(q.Get("state"))
	assert.False(t, ok)
}

func TestMercadoPagoOAuthCallbackRejectsForgedState(t *testing.T) {
	setupHandlerTestDB(t)

	client := &mercadopago.OAuthClient{
		ClientID:     "id",
		ClientSecret: "secret",
		RedirectURI:  "https://api.payverge.io/api/v1/mercadopago/oauth/callback",
		BaseAuthURL:  "https://auth.mercadopago.com",
		BaseAPIURL:   "https://api.mercadopago.com",
	}
	h := NewMercadoPagoOAuthHandlers(nil, client)
	h.frontendBase = func() string { return "https://app.payverge.io" }

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=abc&state=forged", nil)

	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	loc, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "error", loc.Query().Get("mp_connect"))
	assert.Equal(t, "state", loc.Query().Get("reason"))
}

func TestMercadoPagoOAuthCallbackHappyPath(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "mercadopago",
		DisplayName: "MercadoPago",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/oauth/token") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token":  "APP_USR-oauth-access-token-1234567890",
				"refresh_token": "TG-refresh-token-12345678901234567890",
				"public_key":    "APP_USR-oauth-public-key-1234567890",
				"expires_in":    21600,
				"user_id":       999001,
				"live_mode":     true,
				"scope":         "offline_access read write",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer tokenServer.Close()

	client := &mercadopago.OAuthClient{
		ClientID:     "id",
		ClientSecret: "secret",
		RedirectURI:  "https://api.payverge.io/api/v1/mercadopago/oauth/callback",
		BaseAuthURL:  "https://auth.mercadopago.com",
		BaseAPIURL:   tokenServer.URL,
		HTTPClient:   tokenServer.Client(),
	}
	h := NewMercadoPagoOAuthHandlers(services.NewPluginService(database.GetDBWrapper()), client)
	h.frontendBase = func() string { return "https://app.payverge.io" }
	h.usersMeFn = func(ctx context.Context, accessToken, apiBase string) (string, int64, error) {
		return "MLA", 999001, nil
	}

	state := "state-happy-1"
	h.stateStore.Put(state, mpOAuthState{
		businessID:   business.ID,
		userID:       "user-1",
		codeVerifier: "verifier-12345678901234567890123456789012",
		expiresAt:    time.Now().Add(5 * time.Minute),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=auth-code&state="+state, nil)

	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	loc, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "success", loc.Query().Get("mp_connect"))

	cfg, enabled, exists, err := database.GetBusinessPluginConfigState(business.ID, "mercadopago")
	require.NoError(t, err)
	require.True(t, exists)
	require.True(t, enabled)
	assert.Equal(t, "oauth", cfg["connection_mode"])
	assert.Equal(t, "APP_USR-oauth-access-token-1234567890", cfg["access_token"])
	assert.Equal(t, "TG-refresh-token-12345678901234567890", cfg["refresh_token"])
	assert.Equal(t, "999001", cfg["mp_user_id"])
	assert.Equal(t, "AR", cfg["country"])
	assert.Equal(t, "production", cfg["environment"])
	assert.NotEmpty(t, cfg["token_expires_at"])
}

func TestMercadoPagoOAuthCallbackReplacesTokens(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "mercadopago",
		DisplayName: "MercadoPago",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"access_token":    "APP_USR-old-access-token-1234567890",
		"refresh_token":   "TG-old-refresh-token-123456789012",
		"public_key":      "APP_USR-old-public-key-1234567890",
	}))

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "APP_USR-new-access-token-1234567890",
			"refresh_token": "TG-new-refresh-token-123456789012",
			"public_key":    "APP_USR-new-public-key-1234567890",
			"expires_in":    3600,
			"user_id":       42,
			"live_mode":     false,
		})
	}))
	defer tokenServer.Close()

	client := &mercadopago.OAuthClient{
		ClientID: "id", ClientSecret: "secret",
		RedirectURI: "https://api.payverge.io/api/v1/mercadopago/oauth/callback",
		BaseAPIURL:  tokenServer.URL, HTTPClient: tokenServer.Client(),
	}
	h := NewMercadoPagoOAuthHandlers(services.NewPluginService(database.GetDBWrapper()), client)
	h.frontendBase = func() string { return "https://app.payverge.io" }
	h.usersMeFn = func(ctx context.Context, accessToken, apiBase string) (string, int64, error) {
		return "MLA", 42, nil
	}
	state := "state-replace"
	h.stateStore.Put(state, mpOAuthState{
		businessID: business.ID, codeVerifier: "v", expiresAt: time.Now().Add(time.Minute),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=c&state="+state, nil)
	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	cfg, _, _, err := database.GetBusinessPluginConfigState(business.ID, "mercadopago")
	require.NoError(t, err)
	assert.Equal(t, "APP_USR-new-access-token-1234567890", cfg["access_token"])
	assert.Equal(t, "TG-new-refresh-token-123456789012", cfg["refresh_token"])
}

// R3: OAuth connect on a disabled plugin row must preserve non-OAuth keys
// (installments, webhook_secret) instead of wholesale wipe.
func TestMercadoPagoOAuthCallback_PreservesConfigWhenPluginDisabled(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "mercadopago",
		DisplayName: "MercadoPago",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"access_token":   "APP_USR-manual-access-token-1234567890",
		"public_key":     "APP_USR-manual-public-key-1234567890",
		"webhook_secret": "whsec-keep-while-disabled",
		"installments":   float64(6),
		"auto_return":    "approved",
		"base_url":       "https://api.example.com",
		"country":        "AR",
		"environment":    "production",
	}))
	require.NoError(t, database.DisableBusinessPlugin(business.ID, plugin.ID))

	// Confirm GetBusinessPluginConfig (enabled-only) cannot see the row.
	_, err := database.GetBusinessPluginConfig(business.ID, "mercadopago")
	require.Error(t, err)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "APP_USR-oauth-access-token-1234567890",
			"refresh_token": "TG-oauth-refresh-token-123456789012",
			"public_key":    "APP_USR-oauth-public-key-1234567890",
			"expires_in":    3600,
			"user_id":       55,
			"live_mode":     true,
		})
	}))
	defer tokenServer.Close()

	client := &mercadopago.OAuthClient{
		ClientID: "id", ClientSecret: "secret",
		RedirectURI: "https://api.payverge.io/api/v1/mercadopago/oauth/callback",
		BaseAPIURL:  tokenServer.URL, HTTPClient: tokenServer.Client(),
	}
	h := NewMercadoPagoOAuthHandlers(services.NewPluginService(database.GetDBWrapper()), client)
	h.frontendBase = func() string { return "https://app.payverge.io" }
	h.usersMeFn = func(ctx context.Context, accessToken, apiBase string) (string, int64, error) {
		return "MLA", 55, nil
	}
	state := "state-disabled-merge"
	h.stateStore.Put(state, mpOAuthState{
		businessID: business.ID, codeVerifier: "v", expiresAt: time.Now().Add(time.Minute),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=c&state="+state, nil)
	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	loc := w.Header().Get("Location")
	require.Contains(t, loc, "mp_connect=success")

	cfg, enabled, _, err := database.GetBusinessPluginConfigState(business.ID, "mercadopago")
	require.NoError(t, err)
	require.True(t, enabled, "OAuth connect must re-enable the plugin")
	assert.Equal(t, "whsec-keep-while-disabled", cfg["webhook_secret"])
	switch v := cfg["installments"].(type) {
	case float64:
		assert.Equal(t, float64(6), v)
	case int:
		assert.Equal(t, 6, v)
	case int64:
		assert.Equal(t, int64(6), v)
	default:
		t.Fatalf("installments type %T value %#v", cfg["installments"], cfg["installments"])
	}
	assert.Equal(t, "APP_USR-oauth-access-token-1234567890", cfg["access_token"])
	assert.Equal(t, "oauth", cfg["connection_mode"])
}

// R3: omitted public_key on token response must clear any stored public_key.
func TestMercadoPagoOAuthCallback_ClearsPublicKeyWhenOmitted(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "mercadopago",
		DisplayName: "MercadoPago",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"access_token": "APP_USR-old-access-token-1234567890",
		"public_key":   "APP_USR-stale-other-account-key-1234",
		"environment":  "production",
	}))

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No public_key in response.
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "APP_USR-new-access-token-1234567890",
			"refresh_token": "TG-new-refresh-token-123456789012",
			"expires_in":    3600,
			"user_id":       88,
			"live_mode":     true,
		})
	}))
	defer tokenServer.Close()

	client := &mercadopago.OAuthClient{
		ClientID: "id", ClientSecret: "secret",
		RedirectURI: "https://api.payverge.io/api/v1/mercadopago/oauth/callback",
		BaseAPIURL:  tokenServer.URL, HTTPClient: tokenServer.Client(),
	}
	h := NewMercadoPagoOAuthHandlers(services.NewPluginService(database.GetDBWrapper()), client)
	h.frontendBase = func() string { return "https://app.payverge.io" }
	h.usersMeFn = func(ctx context.Context, accessToken, apiBase string) (string, int64, error) {
		return "MLA", 88, nil
	}
	state := "state-clear-pk"
	h.stateStore.Put(state, mpOAuthState{
		businessID: business.ID, codeVerifier: "v", expiresAt: time.Now().Add(time.Minute),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=c&state="+state, nil)
	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	cfg, _, _, err := database.GetBusinessPluginConfigState(business.ID, "mercadopago")
	require.NoError(t, err)
	_, hasPK := cfg["public_key"]
	assert.False(t, hasPK, "stale public_key must be cleared when token omits it; got %#v", cfg["public_key"])
	assert.Equal(t, "APP_USR-new-access-token-1234567890", cfg["access_token"])
}

// R1: production must reject OAuth connect when the token is not live
// (live_mode=false → sandbox credentials).
func TestMercadoPagoOAuthCallback_RejectsSandboxLiveModeInProduction(t *testing.T) {
	appconfig.SetProductionModeOverride(true)
	t.Cleanup(func() { appconfig.SetProductionModeOverride(false) })

	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "mercadopago",
		DisplayName: "MercadoPago",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "APP_USR-sandbox-access-token-1234567890",
			"refresh_token": "TG-sandbox-refresh-token-123456789012",
			"public_key":    "APP_USR-sandbox-public-key-1234567890",
			"expires_in":    3600,
			"user_id":       77,
			"live_mode":     false,
		})
	}))
	defer tokenServer.Close()

	client := &mercadopago.OAuthClient{
		ClientID: "id", ClientSecret: "secret",
		RedirectURI: "https://api.payverge.io/api/v1/mercadopago/oauth/callback",
		BaseAPIURL:  tokenServer.URL, HTTPClient: tokenServer.Client(),
	}
	h := NewMercadoPagoOAuthHandlers(services.NewPluginService(database.GetDBWrapper()), client)
	h.frontendBase = func() string { return "https://app.payverge.io" }
	h.usersMeFn = func(ctx context.Context, accessToken, apiBase string) (string, int64, error) {
		return "MLA", 77, nil
	}
	state := "state-sandbox-prod"
	h.stateStore.Put(state, mpOAuthState{
		businessID: business.ID, codeVerifier: "v", expiresAt: time.Now().Add(time.Minute),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=c&state="+state, nil)
	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	loc := w.Header().Get("Location")
	require.Contains(t, loc, "mp_connect=error")
	require.Contains(t, loc, "sandbox_credentials")
	// Config must not have been persisted.
	_, _, _, err := database.GetBusinessPluginConfigState(business.ID, "mercadopago")
	require.Error(t, err)
}

// F6: OAuth connect must merge into existing manual config, preserving
// webhook_secret, installments, and other non-token fields.
func TestMercadoPagoOAuthCallback_MergesExistingConfig(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "mercadopago",
		DisplayName: "MercadoPago",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"access_token":   "APP_USR-manual-access-token-1234567890",
		"public_key":     "APP_USR-manual-public-key-1234567890",
		"webhook_secret": "whsec-keep-me-secret",
		"installments":   float64(3),
		"auto_return":    "approved",
		"base_url":       "https://api.example.com",
		"country":        "AR",
		"environment":    "production",
	}))

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "APP_USR-oauth-access-token-1234567890",
			"refresh_token": "TG-oauth-refresh-token-123456789012",
			"public_key":    "APP_USR-oauth-public-key-1234567890",
			"expires_in":    3600,
			"user_id":       99,
			"live_mode":     true,
		})
	}))
	defer tokenServer.Close()

	client := &mercadopago.OAuthClient{
		ClientID: "id", ClientSecret: "secret",
		RedirectURI: "https://api.payverge.io/api/v1/mercadopago/oauth/callback",
		BaseAPIURL:  tokenServer.URL, HTTPClient: tokenServer.Client(),
	}
	h := NewMercadoPagoOAuthHandlers(services.NewPluginService(database.GetDBWrapper()), client)
	h.frontendBase = func() string { return "https://app.payverge.io" }
	h.usersMeFn = func(ctx context.Context, accessToken, apiBase string) (string, int64, error) {
		return "MLC", 99, nil // Chile — overwrites country when present
	}
	state := "state-merge"
	h.stateStore.Put(state, mpOAuthState{
		businessID: business.ID, codeVerifier: "v", expiresAt: time.Now().Add(time.Minute),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=c&state="+state, nil)
	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	cfg, _, _, err := database.GetBusinessPluginConfigState(business.ID, "mercadopago")
	require.NoError(t, err)

	// OAuth fields overwrite.
	assert.Equal(t, "oauth", cfg["connection_mode"])
	assert.Equal(t, "APP_USR-oauth-access-token-1234567890", cfg["access_token"])
	assert.Equal(t, "TG-oauth-refresh-token-123456789012", cfg["refresh_token"])
	assert.Equal(t, "connected", cfg["oauth_status"])
	assert.Equal(t, "CL", cfg["country"]) // from site_id MLC

	// Pre-existing non-token fields must survive.
	assert.Equal(t, "whsec-keep-me-secret", cfg["webhook_secret"])
	assert.Equal(t, "https://api.example.com", cfg["base_url"])
	assert.Equal(t, "approved", cfg["auto_return"])
	// installments may be float64 or int after JSON round-trip
	switch v := cfg["installments"].(type) {
	case float64:
		assert.Equal(t, float64(3), v)
	case int:
		assert.Equal(t, 3, v)
	case int64:
		assert.Equal(t, int64(3), v)
	default:
		t.Fatalf("installments type %T value %#v", cfg["installments"], cfg["installments"])
	}
}

// P3: decrypt failure (PLUGIN_SECRET_KEY rotation) must abort without persist.
func TestMercadoPagoOAuthCallback_DecryptFailureAbortsWithoutPersist(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name: "mercadopago", DisplayName: "MercadoPago",
		Category: database.PluginCategoryPayment, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"access_token":       "APP_USR-old-access-token-1234567890",
		"refresh_token":      "TG-old-refresh-token-123456789012",
		"mp_user_id":         "111",
		"mp_store_id":        "store-old",
		"mp_external_pos_id": "pos-old",
		"webhook_secret":     "whsec-old",
	}))

	// Rotate key — stored ciphertext becomes undecryptable.
	t.Setenv("PLUGIN_SECRET_KEY", "fedcba9876543210fedcba9876543210")

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "APP_USR-new-access-token-should-not-persist",
			"refresh_token": "TG-new-refresh-should-not-persist-xxxxx",
			"public_key":    "APP_USR-new-public-key-should-not",
			"expires_in":    3600,
			"user_id":       222,
			"live_mode":     true,
		})
	}))
	defer tokenServer.Close()

	client := &mercadopago.OAuthClient{
		ClientID: "id", ClientSecret: "secret",
		RedirectURI: "https://api.payverge.io/api/v1/mercadopago/oauth/callback",
		BaseAPIURL:  tokenServer.URL, HTTPClient: tokenServer.Client(),
	}
	h := NewMercadoPagoOAuthHandlers(services.NewPluginService(database.GetDBWrapper()), client)
	h.frontendBase = func() string { return "https://app.payverge.io" }
	h.usersMeFn = func(ctx context.Context, accessToken, apiBase string) (string, int64, error) {
		return "MLA", 222, nil
	}
	state := "state-decrypt-fail"
	h.stateStore.Put(state, mpOAuthState{
		businessID: business.ID, codeVerifier: "v", expiresAt: time.Now().Add(time.Minute),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=c&state="+state, nil)
	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	loc, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "error", loc.Query().Get("mp_connect"))
	assert.Equal(t, "config_load", loc.Query().Get("reason"))

	// New tokens must not have been written. Load raw config (decrypt may still fail).
	// Rotate back to original key to inspect the preserved row.
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")
	cfg, _, _, err := database.GetBusinessPluginConfigState(business.ID, "mercadopago")
	require.NoError(t, err)
	assert.Equal(t, "APP_USR-old-access-token-1234567890", cfg["access_token"])
	assert.NotEqual(t, "APP_USR-new-access-token-should-not-persist", cfg["access_token"])
	assert.Equal(t, "111", cfg["mp_user_id"])
	assert.Equal(t, "store-old", cfg["mp_store_id"])
}

// P3: different mp_user_id clears store/POS so provisioning re-runs.
func TestMercadoPagoOAuthCallback_DifferentUserIDClearsStorePOS(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name: "mercadopago", DisplayName: "MercadoPago",
		Category: database.PluginCategoryPayment, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"access_token":       "APP_USR-old-access-token-1234567890",
		"mp_user_id":         "111",
		"mp_store_id":        "store-old",
		"mp_external_pos_id": "pos-old",
		"webhook_secret":     "whsec-keep",
	}))

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "APP_USR-new-access-token-1234567890",
			"refresh_token": "TG-new-refresh-token-123456789012",
			"expires_in":    3600,
			"user_id":       999,
			"live_mode":     true,
		})
	}))
	defer tokenServer.Close()

	client := &mercadopago.OAuthClient{
		ClientID: "id", ClientSecret: "secret",
		RedirectURI: "https://api.payverge.io/api/v1/mercadopago/oauth/callback",
		BaseAPIURL:  tokenServer.URL, HTTPClient: tokenServer.Client(),
	}
	h := NewMercadoPagoOAuthHandlers(services.NewPluginService(database.GetDBWrapper()), client)
	h.frontendBase = func() string { return "https://app.payverge.io" }
	h.usersMeFn = func(ctx context.Context, accessToken, apiBase string) (string, int64, error) {
		return "MLA", 999, nil
	}
	state := "state-user-change"
	h.stateStore.Put(state, mpOAuthState{
		businessID: business.ID, codeVerifier: "v", expiresAt: time.Now().Add(time.Minute),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=c&state="+state, nil)
	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	cfg, _, _, err := database.GetBusinessPluginConfigState(business.ID, "mercadopago")
	require.NoError(t, err)
	assert.Equal(t, "999", cfg["mp_user_id"])
	_, hasStore := cfg["mp_store_id"]
	_, hasPOS := cfg["mp_external_pos_id"]
	assert.False(t, hasStore, "store id must clear on account change")
	assert.False(t, hasPOS, "external pos id must clear on account change")
	assert.Equal(t, "whsec-keep", cfg["webhook_secret"], "non-account fields preserved")
}

// P3: same mp_user_id reconnect preserves store/POS.
func TestMercadoPagoOAuthCallback_SameUserIDPreservesStorePOS(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name: "mercadopago", DisplayName: "MercadoPago",
		Category: database.PluginCategoryPayment, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"access_token":       "APP_USR-old-access-token-1234567890",
		"mp_user_id":         "555",
		"mp_store_id":        "store-keep",
		"mp_external_pos_id": "pos-keep",
	}))

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "APP_USR-new-access-token-1234567890",
			"refresh_token": "TG-new-refresh-token-123456789012",
			"expires_in":    3600,
			"user_id":       555,
			"live_mode":     true,
		})
	}))
	defer tokenServer.Close()

	client := &mercadopago.OAuthClient{
		ClientID: "id", ClientSecret: "secret",
		RedirectURI: "https://api.payverge.io/api/v1/mercadopago/oauth/callback",
		BaseAPIURL:  tokenServer.URL, HTTPClient: tokenServer.Client(),
	}
	h := NewMercadoPagoOAuthHandlers(services.NewPluginService(database.GetDBWrapper()), client)
	h.frontendBase = func() string { return "https://app.payverge.io" }
	h.usersMeFn = func(ctx context.Context, accessToken, apiBase string) (string, int64, error) {
		return "MLA", 555, nil
	}
	state := "state-same-user"
	h.stateStore.Put(state, mpOAuthState{
		businessID: business.ID, codeVerifier: "v", expiresAt: time.Now().Add(time.Minute),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=c&state="+state, nil)
	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	cfg, _, _, err := database.GetBusinessPluginConfigState(business.ID, "mercadopago")
	require.NoError(t, err)
	assert.Equal(t, "555", cfg["mp_user_id"])
	assert.Equal(t, "store-keep", cfg["mp_store_id"])
	assert.Equal(t, "pos-keep", cfg["mp_external_pos_id"])
}

// Q1: /users/me failure must not poison mp_user_id to "0" and must not wipe
// store/POS on a same-account reconnect that still has a token-seeded user id.
func TestMercadoPagoOAuthCallback_UsersMeFailureKeepsTokenUserID(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name: "mercadopago", DisplayName: "MercadoPago",
		Category: database.PluginCategoryPayment, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"access_token":       "APP_USR-old-access-token-1234567890",
		"mp_user_id":         "777001",
		"mp_store_id":        "store-keep-usersme-fail",
		"mp_external_pos_id": "pos-keep-usersme-fail",
	}))

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "APP_USR-new-access-token-usersme-fail",
			"refresh_token": "TG-new-refresh-token-usersme-fail",
			"expires_in":    3600,
			"user_id":       777001,
			"live_mode":     true,
		})
	}))
	defer tokenServer.Close()

	client := &mercadopago.OAuthClient{
		ClientID: "id", ClientSecret: "secret",
		RedirectURI: "https://api.payverge.io/api/v1/mercadopago/oauth/callback",
		BaseAPIURL:  tokenServer.URL, HTTPClient: tokenServer.Client(),
	}
	h := NewMercadoPagoOAuthHandlers(services.NewPluginService(database.GetDBWrapper()), client)
	h.frontendBase = func() string { return "https://app.payverge.io" }
	h.usersMeFn = func(ctx context.Context, accessToken, apiBase string) (string, int64, error) {
		// Production fetch returns (0, err) on all error paths — must not overwrite.
		return "", 0, fmt.Errorf("users/me unavailable: temporary outage")
	}

	runCallback := func(state string) {
		h.stateStore.Put(state, mpOAuthState{
			businessID: business.ID, codeVerifier: "v", expiresAt: time.Now().Add(time.Minute),
		})
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=c&state="+state, nil)
		h.HandleOAuthCallback(c)
		require.Equal(t, http.StatusFound, w.Code, w.Body.String())
		loc, err := url.Parse(w.Header().Get("Location"))
		require.NoError(t, err)
		assert.Equal(t, "success", loc.Query().Get("mp_connect"))
	}

	runCallback("state-usersme-fail-1")
	cfg, _, _, err := database.GetBusinessPluginConfigState(business.ID, "mercadopago")
	require.NoError(t, err)
	assert.NotEqual(t, "0", cfg["mp_user_id"], "must never persist poisoned mp_user_id=0")
	assert.Equal(t, "777001", cfg["mp_user_id"], "must keep token-seeded user id when /users/me fails")
	assert.Equal(t, "store-keep-usersme-fail", cfg["mp_store_id"])
	assert.Equal(t, "pos-keep-usersme-fail", cfg["mp_external_pos_id"])

	// Same-account reconnect with another /users/me failure must still preserve store/POS.
	runCallback("state-usersme-fail-2")
	cfg, _, _, err = database.GetBusinessPluginConfigState(business.ID, "mercadopago")
	require.NoError(t, err)
	assert.NotEqual(t, "0", cfg["mp_user_id"])
	assert.Equal(t, "777001", cfg["mp_user_id"])
	assert.Equal(t, "store-keep-usersme-fail", cfg["mp_store_id"], "same-account reconnect must not wipe store")
	assert.Equal(t, "pos-keep-usersme-fail", cfg["mp_external_pos_id"], "same-account reconnect must not wipe POS")
}

// Q1: when both token and /users/me yield no positive user id, leave mp_user_id
// unset (do not write "0") so a later re-resolve can fill it.
func TestMercadoPagoOAuthCallback_UsersMeFailureZeroTokenLeavesUserIDUnset(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name: "mercadopago", DisplayName: "MercadoPago",
		Category: database.PluginCategoryPayment, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "APP_USR-fresh-access-token-no-userid",
			"refresh_token": "TG-fresh-refresh-token-no-userid",
			"expires_in":    3600,
			// omit user_id → 0
			"live_mode": true,
		})
	}))
	defer tokenServer.Close()

	client := &mercadopago.OAuthClient{
		ClientID: "id", ClientSecret: "secret",
		RedirectURI: "https://api.payverge.io/api/v1/mercadopago/oauth/callback",
		BaseAPIURL:  tokenServer.URL, HTTPClient: tokenServer.Client(),
	}
	h := NewMercadoPagoOAuthHandlers(services.NewPluginService(database.GetDBWrapper()), client)
	h.frontendBase = func() string { return "https://app.payverge.io" }
	h.usersMeFn = func(ctx context.Context, accessToken, apiBase string) (string, int64, error) {
		return "", 0, fmt.Errorf("users/me failed")
	}
	state := "state-no-userid"
	h.stateStore.Put(state, mpOAuthState{
		businessID: business.ID, codeVerifier: "v", expiresAt: time.Now().Add(time.Minute),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=c&state="+state, nil)
	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	cfg, _, _, err := database.GetBusinessPluginConfigState(business.ID, "mercadopago")
	require.NoError(t, err)
	assert.NotEqual(t, "0", cfg["mp_user_id"])
	_, has := cfg["mp_user_id"]
	assert.False(t, has, "mp_user_id must stay unset when no positive id is known")
}

// TestMercadoPagoOAuthCallback_LandsOnThePluginsTab: /business/plugins is not a
// route in this app, so every completed connection used to end on a 404 and the
// mp_connect toast — which renders inside the dashboard's plugin manager — never
// fired. Same bug the Stripe callback carried.
func TestMercadoPagoOAuthCallback_LandsOnThePluginsTab(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name: "mercadopago", DisplayName: "MercadoPago",
		Category: database.PluginCategoryPayment, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "APP_USR-landing-access-token-1234567890",
			"refresh_token": "TG-landing-refresh-token-1234567890123",
			"public_key":    "APP_USR-landing-public-key-1234567890",
			"expires_in":    21600,
			"user_id":       999777,
			"live_mode":     true,
			"scope":         "offline_access read write",
		})
	}))
	defer tokenServer.Close()

	client := &mercadopago.OAuthClient{
		ClientID: "id", ClientSecret: "secret",
		BaseAPIURL: tokenServer.URL, HTTPClient: tokenServer.Client(),
	}
	h := NewMercadoPagoOAuthHandlers(services.NewPluginService(database.GetDBWrapper()), client)
	h.frontendBase = func() string { return "https://app.payverge.io" }
	h.usersMeFn = func(ctx context.Context, accessToken, apiBase string) (string, int64, error) {
		return "MLA", 999777, nil
	}

	state := "state-mp-landing"
	h.stateStore.Put(state, mpOAuthState{
		businessID: business.ID, codeVerifier: "v", expiresAt: time.Now().Add(time.Minute),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/callback?code=c&state="+state, nil)
	h.HandleOAuthCallback(c)

	require.Equal(t, http.StatusFound, w.Code)
	loc, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("/business/%d/dashboard", business.ID), loc.Path)
	assert.Equal(t, "plugins", loc.Query().Get("tab"))
	assert.Equal(t, "success", loc.Query().Get("mp_connect"))
}

// TestMercadoPagoOAuthCallback_UnknownBusinessRedirectsHome: with no consumable
// state there is no business to route to, so the fallback must still be a real
// route.
func TestMercadoPagoOAuthCallback_UnknownBusinessRedirectsHome(t *testing.T) {
	client := &mercadopago.OAuthClient{ClientID: "id", ClientSecret: "secret"}
	h := NewMercadoPagoOAuthHandlers(nil, client)
	h.frontendBase = func() string { return "https://app.payverge.io" }

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
