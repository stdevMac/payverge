package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins/mercadopago"
	"github.com/stdevmac/payverge/backend/internal/plugins/paypal"
	"github.com/stdevmac/payverge/backend/internal/plugins/stripe"
)

// seedEnabledPaymentPlugin creates an active payment plugin and enables it for
// the business with the supplied config so GetBusinessPluginConfig returns it.
func seedEnabledPaymentPlugin(t *testing.T, businessID uint, name string, config map[string]interface{}) {
	t.Helper()
	plugin := &database.Plugin{
		Name:        name,
		DisplayName: name,
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(businessID, plugin.ID, config))
}

func callTestPluginConnection(t *testing.T, businessID uint, provider string) (int, map[string]interface{}) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", businessID)},
		{Key: "plugin_id", Value: provider},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	NewPluginHandlers(nil, nil).TestPluginConnection(c)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp
}

func TestTestPluginConnection_StripeSuccess(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))
	business := createTestBusiness(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/account", r.URL.Path)
		assert.NotEmpty(t, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"acct_123"}`))
	}))
	defer srv.Close()
	prev := stripe.StripeAPIBaseURL
	stripe.StripeAPIBaseURL = srv.URL
	defer func() { stripe.StripeAPIBaseURL = prev }()

	seedEnabledPaymentPlugin(t, business.ID, "stripe", map[string]interface{}{
		"secret_key":      "sk_test_validkey1234567890",
		"publishable_key": "pk_test_validkey1234567890",
	})

	code, resp := callTestPluginConnection(t, business.ID, "stripe")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["ok"])
	assert.Equal(t, "stripe", resp["provider"])
}

func TestTestPluginConnection_StripeAuthFailure(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))
	business := createTestBusiness(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid API Key"}}`))
	}))
	defer srv.Close()
	prev := stripe.StripeAPIBaseURL
	stripe.StripeAPIBaseURL = srv.URL
	defer func() { stripe.StripeAPIBaseURL = prev }()

	seedEnabledPaymentPlugin(t, business.ID, "stripe", map[string]interface{}{
		"secret_key":      "sk_test_badkey1234567890",
		"publishable_key": "pk_test_validkey1234567890",
	})

	code, resp := callTestPluginConnection(t, business.ID, "stripe")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, false, resp["ok"])
	assert.Contains(t, resp["message"], "Invalid credentials")
}

func TestTestPluginConnection_MercadoPagoSuccessAndAuthFailure(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))
	business := createTestBusiness(t)

	var status int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/users/me", r.URL.Path)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	prev := mercadopago.MercadoPagoAPIBaseURL
	mercadopago.MercadoPagoAPIBaseURL = srv.URL
	defer func() { mercadopago.MercadoPagoAPIBaseURL = prev }()

	seedEnabledPaymentPlugin(t, business.ID, "mercadopago", map[string]interface{}{
		"access_token": "APP_USR-token-1234567890",
	})

	status = http.StatusOK
	code, resp := callTestPluginConnection(t, business.ID, "mercadopago")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["ok"])

	status = http.StatusUnauthorized
	code, resp = callTestPluginConnection(t, business.ID, "mercadopago")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, false, resp["ok"])
	assert.Contains(t, resp["message"], "Invalid credentials")
}

func TestTestPluginConnection_PayPalSuccessAndAuthFailure(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))
	business := createTestBusiness(t)

	var status int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/oauth2/token", r.URL.Path)
		if status == http.StatusOK {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"access_token":"A21AA","token_type":"Bearer"}`))
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	defer srv.Close()
	prev := paypal.PayPalAPIBaseURLOverride
	paypal.PayPalAPIBaseURLOverride = srv.URL
	defer func() { paypal.PayPalAPIBaseURLOverride = prev }()

	seedEnabledPaymentPlugin(t, business.ID, "paypal", map[string]interface{}{
		"client_id":     "paypal-client-id-1234567890",
		"client_secret": "paypal-client-secret-1234567890",
		"environment":   "sandbox",
	})

	status = http.StatusOK
	code, resp := callTestPluginConnection(t, business.ID, "paypal")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, resp["ok"])

	status = http.StatusUnauthorized
	code, resp = callTestPluginConnection(t, business.ID, "paypal")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, false, resp["ok"])
	assert.Contains(t, resp["message"], "Invalid credentials")
}

func TestTestPluginConnection_UnsupportedProvider(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))
	business := createTestBusiness(t)

	seedEnabledPaymentPlugin(t, business.ID, "trustpilot", map[string]interface{}{
		"enabled": true,
	})

	code, resp := callTestPluginConnection(t, business.ID, "trustpilot")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, false, resp["ok"])
	assert.Contains(t, resp["message"], "not supported")
}

func TestTestPluginConnection_PluginNotEnabled(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))
	business := createTestBusiness(t)

	code, _ := callTestPluginConnection(t, business.ID, "stripe")
	assert.Equal(t, http.StatusBadRequest, code)
}
