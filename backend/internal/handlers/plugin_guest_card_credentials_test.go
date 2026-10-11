package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestGuestCardPluginHasCredentialShape(t *testing.T) {
	t.Parallel()

	assert.False(t, guestCardPluginHasCredentialShape("mercadopago", map[string]interface{}{
		"demo": true, "enabled": true, "environment": "sandbox",
	}), "demo MP without TEST-/APP_USR- token is not guest-visible")

	assert.True(t, guestCardPluginHasCredentialShape("mercadopago", map[string]interface{}{
		"access_token": "TEST-123456789012345678901",
	}))

	assert.True(t, guestCardPluginHasCredentialShape("mercadopago", map[string]interface{}{
		"access_token": "v1:ciphertext-from-enable-at-rest",
	}), "listing maps keep access_token as v1: ciphertext")

	assert.False(t, guestCardPluginHasCredentialShape("mercadopago", map[string]interface{}{
		"access_token": "v1:",
	}), "empty v1: prefix is not a credential")

	assert.True(t, guestCardPluginHasCredentialShape("paypal", map[string]interface{}{
		"client_secret_encrypted": "v1:abc",
	}), "*_encrypted charge secrets already hold ciphertext")

	assert.True(t, guestCardPluginHasCredentialShape("stripe", map[string]interface{}{
		"secret_key": "v1:sealed-stripe-secret",
	}))

	assert.False(t, guestCardPluginHasCredentialShape("stripe", map[string]interface{}{
		"webhook_secret": "v1:signing-only",
	}), "signing secrets must not make a card rail guest-visible")

	assert.False(t, evalPaymentPluginConfigEnabled("mercadopago", map[string]interface{}{
		"enabled": true, "demo": true,
	}))

	assert.True(t, evalPaymentPluginConfigEnabled("mercadopago", map[string]interface{}{
		"enabled":      true,
		"access_token": "APP_USR-123456789012345678901",
	}))

	assert.True(t, evalPaymentPluginConfigEnabled("mercadopago", map[string]interface{}{
		"enabled":      true,
		"access_token": "v1:ciphertext-from-enable-at-rest",
	}))

	assert.True(t, evalPaymentPluginConfigEnabled("usdc_payment", map[string]interface{}{
		"enabled": true,
	}), "crypto rails are not gated on MP token shape")

	assert.True(t, evalPaymentPluginConfigEnabled("stripe", map[string]interface{}{
		"access_token": "sk_live_oauth_access",
	}))
}

func TestGetBusinessPaymentPlugins_EncryptedAtRestCardRailStaysGuestVisible(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")
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

	const plaintext = "TEST-123456789012345678901"
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"enabled":      true,
		"access_token": plaintext,
	}))

	var raw string
	require.NoError(t, database.GetDB().
		Raw("SELECT config FROM business_plugins WHERE business_id = ? AND plugin_id = ?", business.ID, plugin.ID).
		Scan(&raw).Error)
	require.Contains(t, raw, "v1:", "EnableBusinessPlugin must persist ciphertext")
	require.NotContains(t, raw, plaintext, "plaintext token must not remain at rest")

	configs, err := database.GetBusinessPaymentPluginConfigs(business.ID, []string{"mercadopago"})
	require.NoError(t, err)
	require.Contains(t, configs, "mercadopago")
	listedToken, _ := configs["mercadopago"]["access_token"].(string)
	require.True(t, strings.HasPrefix(listedToken, "v1:"), "batched listing must not decrypt secrets")
	require.True(t, evalPaymentPluginConfigEnabled("mercadopago", configs["mercadopago"]),
		"encrypted-at-rest card rail must stay guest-visible")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	NewPluginHandlers(nil, nil).GetBusinessPaymentPlugins(c)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Plugins []map[string]interface{} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Plugins, 1)
	assert.Equal(t, "mercadopago", resp.Plugins[0]["name"])
	assert.Equal(t, true, resp.Plugins[0]["is_enabled"])
	_, hasConfig := resp.Plugins[0]["config"]
	assert.False(t, hasConfig)
	assert.NotContains(t, w.Body.String(), plaintext)
}
