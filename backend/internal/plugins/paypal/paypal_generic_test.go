package paypal

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPayPalGenericPaymentMethodsRequireSupportedEntryPoints(t *testing.T) {
	plugin := &PayPalPlugin{}

	orderID, err := plugin.ProcessPayment(1, 1000, "USD", nil)
	require.Error(t, err)
	require.Empty(t, orderID)
	require.Contains(t, err.Error(), "unsupported")

	err = plugin.RefundPayment(1, "ORDER-1", 1000)
	require.Error(t, err)
	require.Contains(t, err.Error(), "plugin service not configured")
}

func TestPayPalValidateConfigRequiresExplicitPublicBaseURL(t *testing.T) {
	plugin := &PayPalPlugin{}

	err := plugin.ValidateConfig(map[string]interface{}{
		"client_id":     "client-id-12345678901234567890",
		"client_secret": "client-secret-12345678901234567890",
		"webhook_id":    "WH123456789",
		"environment":   "sandbox",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "base_url is required")

	require.NoError(t, plugin.ValidateConfig(map[string]interface{}{
		"client_id":     "client-id-12345678901234567890",
		"client_secret": "client-secret-12345678901234567890",
		"webhook_id":    "WH123456789",
		"environment":   "sandbox",
		"base_url":      "https://api.staging.example",
	}))
}

func TestPayPalValidateConfigGuardsApiBaseURLSSRF(t *testing.T) {
	plugin := &PayPalPlugin{}
	base := map[string]interface{}{
		"client_id":     "client-id-12345678901234567890",
		"client_secret": "client-secret-12345678901234567890",
		"webhook_id":    "WH123456789",
		"environment":   "sandbox",
		"base_url":      "https://api.staging.example",
	}
	withAPIBase := func(v string) map[string]interface{} {
		c := map[string]interface{}{}
		for k, val := range base {
			c[k] = val
		}
		c["api_base_url"] = v
		return c
	}

	// Absent api_base_url stays valid (the field is optional).
	require.NoError(t, plugin.ValidateConfig(base))

	// SSRF vectors via api_base_url are rejected.
	for _, bad := range []string{
		"http://169.254.169.254",
		"https://169.254.169.254/latest/meta-data",
		"https://127.0.0.1:8080",
		"https://internal-billing",
		"https://api.paypal.com.evil.com",
	} {
		require.Error(t, plugin.ValidateConfig(withAPIBase(bad)), "expected %q to be rejected", bad)
	}

	// The real provider API endpoints are accepted.
	require.NoError(t, plugin.ValidateConfig(withAPIBase("https://api.sandbox.paypal.com")))
	require.NoError(t, plugin.ValidateConfig(withAPIBase("https://api.paypal.com")))
}
