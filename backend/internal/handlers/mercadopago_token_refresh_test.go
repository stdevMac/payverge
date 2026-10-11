package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins/mercadopago"
)

func seedMercadoPagoPlugin(t *testing.T) *database.Plugin {
	t.Helper()
	plugin := &database.Plugin{
		Name:        "mercadopago",
		DisplayName: "MercadoPago",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	return plugin
}

func TestRefreshMercadoPagoTokens_RefreshesNearExpiryAndSkipsFarExpiry(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	plugin := seedMercadoPagoPlugin(t)
	nearBiz := createTestBusiness(t)
	farBiz := createTestBusiness(t)
	manualBiz := createTestBusiness(t)

	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	nearExpiry := now.Add(10 * 24 * time.Hour).Format(time.RFC3339)
	farExpiry := now.Add(90 * 24 * time.Hour).Format(time.RFC3339)

	require.NoError(t, database.EnableBusinessPlugin(nearBiz.ID, plugin.ID, map[string]interface{}{
		"connection_mode":  "oauth",
		"access_token":     "APP_USR-old-access-token-near-1234567890",
		"refresh_token":    "TG-old-refresh-near-1234567890123456",
		"token_expires_at": nearExpiry,
		"oauth_status":     "connected",
		"public_key":       "APP_USR-old-public-key-near",
	}))
	require.NoError(t, database.EnableBusinessPlugin(farBiz.ID, plugin.ID, map[string]interface{}{
		"connection_mode":  "oauth",
		"access_token":     "APP_USR-old-access-token-far-1234567890",
		"refresh_token":    "TG-old-refresh-far-12345678901234567",
		"token_expires_at": farExpiry,
		"oauth_status":     "connected",
	}))
	require.NoError(t, database.EnableBusinessPlugin(manualBiz.ID, plugin.ID, map[string]interface{}{
		"connection_mode": "manual",
		"access_token":    "TEST-manual-access-token-1234567890",
	}))

	var seenRefreshToken string
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.True(t, strings.HasSuffix(r.URL.Path, "/oauth/token"))
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "refresh_token", body["grant_type"])
		seenRefreshToken = body["refresh_token"]
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "APP_USR-new-access-token-near-1234567890",
			"refresh_token": "TG-new-refresh-near-1234567890123456",
			"public_key":    "APP_USR-new-public-key-near",
			"expires_in":    21600,
			"user_id":       42,
			"live_mode":     true,
		})
	}))
	defer tokenServer.Close()

	client := &mercadopago.OAuthClient{
		ClientID:     "id",
		ClientSecret: "secret",
		BaseAPIURL:   tokenServer.URL,
		HTTPClient:   tokenServer.Client(),
	}

	stats := RefreshMercadoPagoTokens(database.GetDBWrapper(), client, now)
	assert.Equal(t, 1, stats.Refreshed)
	assert.Equal(t, 2, stats.Skipped) // far oauth + manual
	assert.Equal(t, 0, stats.Failed)
	assert.Equal(t, "TG-old-refresh-near-1234567890123456", seenRefreshToken)

	nearCfg, err := database.GetBusinessPluginConfig(nearBiz.ID, "mercadopago")
	require.NoError(t, err)
	assert.Equal(t, "APP_USR-new-access-token-near-1234567890", nearCfg["access_token"])
	// Single-use refresh tokens: new refresh_token must replace the old one.
	assert.Equal(t, "TG-new-refresh-near-1234567890123456", nearCfg["refresh_token"])
	assert.Equal(t, "connected", nearCfg["oauth_status"])
	assert.Equal(t, float64(0), nearCfg["oauth_refresh_failures"])
	assert.Equal(t, "APP_USR-new-public-key-near", nearCfg["public_key"])
	newExpiry, err := time.Parse(time.RFC3339, nearCfg["token_expires_at"].(string))
	require.NoError(t, err)
	assert.Equal(t, now.Add(21600*time.Second), newExpiry)

	farCfg, err := database.GetBusinessPluginConfig(farBiz.ID, "mercadopago")
	require.NoError(t, err)
	assert.Equal(t, "APP_USR-old-access-token-far-1234567890", farCfg["access_token"])
	assert.Equal(t, "TG-old-refresh-far-12345678901234567", farCfg["refresh_token"])
	assert.Equal(t, farExpiry, farCfg["token_expires_at"])
}

func TestRefreshMercadoPagoTokens_FailureIncrementsAndFlagsReauthAt3(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	plugin := seedMercadoPagoPlugin(t)
	biz := createTestBusiness(t)
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)

	require.NoError(t, database.EnableBusinessPlugin(biz.ID, plugin.ID, map[string]interface{}{
		"connection_mode":  "oauth",
		"access_token":     "APP_USR-old-access-token-fail-1234567890",
		"refresh_token":    "TG-old-refresh-fail-1234567890123456",
		"token_expires_at": now.Add(5 * 24 * time.Hour).Format(time.RFC3339),
		"oauth_status":     "connected",
	}))

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","message":"Error validating grant"}`))
	}))
	defer tokenServer.Close()

	client := &mercadopago.OAuthClient{
		ClientID: "id", ClientSecret: "secret",
		BaseAPIURL: tokenServer.URL, HTTPClient: tokenServer.Client(),
	}

	// Failures 1 and 2: counter only.
	for i := 1; i <= 2; i++ {
		stats := RefreshMercadoPagoTokens(database.GetDBWrapper(), client, now)
		assert.Equal(t, 0, stats.Refreshed)
		assert.Equal(t, 1, stats.Failed)
		cfg, err := database.GetBusinessPluginConfig(biz.ID, "mercadopago")
		require.NoError(t, err)
		assert.Equal(t, float64(i), cfg["oauth_refresh_failures"])
		assert.Equal(t, "connected", cfg["oauth_status"])
		// Secrets untouched on failure.
		assert.Equal(t, "APP_USR-old-access-token-fail-1234567890", cfg["access_token"])
		assert.Equal(t, "TG-old-refresh-fail-1234567890123456", cfg["refresh_token"])
	}

	// Failure 3: reauth_required.
	stats := RefreshMercadoPagoTokens(database.GetDBWrapper(), client, now)
	assert.Equal(t, 1, stats.Failed)
	cfg, err := database.GetBusinessPluginConfig(biz.ID, "mercadopago")
	require.NoError(t, err)
	assert.Equal(t, float64(3), cfg["oauth_refresh_failures"])
	assert.Equal(t, "reauth_required", cfg["oauth_status"])
}

func TestRefreshMercadoPagoTokens_NilClientNoOp(t *testing.T) {
	setupHandlerTestDB(t)
	stats := RefreshMercadoPagoTokens(database.GetDBWrapper(), nil, time.Now())
	assert.Equal(t, RefreshStats{}, stats)
}
