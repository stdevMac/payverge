package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClearTestURLConfigClearsAndRestoresInheritedValues(t *testing.T) {
	inherited := map[string]string{
		"FRONTEND_URL":             "https://inherited-frontend.example",
		"BASE_URL":                 "https://inherited-base.example",
		"NEXT_PUBLIC_BASE_URL":     "https://inherited-public.example",
		"APP_BASE_URL":             "https://inherited-api.example",
		"ALLOWED_REDIRECT_DOMAINS": "inherited-redirect.example",
		"APP_DOMAIN":               "inherited-app.example",
	}
	for key, value := range inherited {
		t.Setenv(key, value)
	}

	t.Run("cleared inside hermetic test scope", func(t *testing.T) {
		ClearTestURLConfig(t)
		for key := range inherited {
			assert.Empty(t, os.Getenv(key), key)
		}
	})

	for key, value := range inherited {
		assert.Equal(t, value, os.Getenv(key), key)
	}
}

func TestSetupTestEnvironmentClearsInheritedURLConfig(t *testing.T) {
	sensitive := []string{
		"FRONTEND_URL",
		"BASE_URL",
		"NEXT_PUBLIC_BASE_URL",
		"APP_BASE_URL",
		"ALLOWED_REDIRECT_DOMAINS",
		"APP_DOMAIN",
		"OPENROUTER_API_KEY",
		"OPENROUTER_ZDR_MODE",
		"EMAIL_API_KEY",
		"AWS_ACCESS_KEY",
		"AWS_SECRET_KEY",
		"AWS_PROTECTED_ACCESS_KEY",
		"AWS_PROTECTED_SECRET_KEY",
		"PLUGIN_SECRET_KEY",
		"STRIPE_WEBHOOK_SECRET",
		"PAYPAL_WEBHOOK_SECRET",
		"MERCADOPAGO_WEBHOOK_SECRET",
		"FISCAL_WSAA_URL",
		"FISCAL_WSFE_URL",
	}
	for _, key := range sensitive {
		t.Setenv(key, "https://inherited.example")
	}

	SetupTestEnvironment(t)

	assert.Equal(t, "http://localhost:3000", FrontendBaseURL())
	assert.Equal(t, "http://localhost:8080", APIBaseURL())
	for _, key := range sensitive[6:] {
		assert.Empty(t, os.Getenv(key), key)
	}
}
