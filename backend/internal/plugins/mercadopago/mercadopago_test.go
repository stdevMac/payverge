package mercadopago

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	appconfig "github.com/stdevmac/payverge/backend/internal/config"
)

func TestMercadoPagoPlugin_GetName(t *testing.T) {
	plugin := NewMercadoPagoPlugin(nil)

	expected := "mercadopago"
	actual := plugin.GetName()

	if actual != expected {
		t.Errorf("Expected plugin name %s, got %s", expected, actual)
	}
}

func TestMercadoPagoPlugin_GetDisplayName(t *testing.T) {
	plugin := NewMercadoPagoPlugin(nil)

	expected := "MercadoPago"
	actual := plugin.GetDisplayName()

	if actual != expected {
		t.Errorf("Expected display name %s, got %s", expected, actual)
	}
}

func TestMercadoPagoPlugin_GetCategory(t *testing.T) {
	plugin := NewMercadoPagoPlugin(nil)

	expected := "payment"
	actual := plugin.GetCategory()

	if actual != expected {
		t.Errorf("Expected category %s, got %s", expected, actual)
	}
}

func TestMercadoPagoPlugin_IsActive(t *testing.T) {
	plugin := NewMercadoPagoPlugin(nil)

	if !plugin.IsActive() {
		t.Error("Expected plugin to be active")
	}

	t.Setenv("APP_ENV", "production")
	t.Setenv("PAYMENT_PROVIDER_MERCADOPAGO_ENABLED", "")
	require.True(t, NewMercadoPagoPlugin(nil).IsActive(),
		"Mercado Pago must stay in the catalog in production so demo/card checkout is visible")
}

func TestMercadoPagoPlugin_IsActiveHonorsExplicitSwitch(t *testing.T) {
	for _, appEnv := range []string{"development", "production"} {
		t.Run(appEnv, func(t *testing.T) {
			t.Setenv("APP_ENV", appEnv)
			t.Setenv("PAYMENT_PROVIDER_MERCADOPAGO_ENABLED", "false")
			require.False(t, NewMercadoPagoPlugin(nil).IsActive(), "explicit false must disable Mercado Pago")
			t.Setenv("PAYMENT_PROVIDER_MERCADOPAGO_ENABLED", "0")
			require.False(t, NewMercadoPagoPlugin(nil).IsActive(), "non-true values must disable Mercado Pago")
			t.Setenv("PAYMENT_PROVIDER_MERCADOPAGO_ENABLED", "TRUE")
			require.True(t, NewMercadoPagoPlugin(nil).IsActive())
		})
	}
}

func TestMercadoPagoPlugin_ValidateConfig(t *testing.T) {
	plugin := NewMercadoPagoPlugin(nil)

	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name: "valid config",
			config: map[string]interface{}{
				"access_token":   "APP_USR-1234567890-test-token",
				"public_key":     "APP_USR-1234567890-test-key",
				"webhook_secret": "mp-webhook-secret",
				"base_url":       "https://api.staging.example",
				"country":        "AR",
			},
			wantErr: false,
		},
		{
			name: "missing access_token",
			config: map[string]interface{}{
				"public_key": "APP_USR-1234567890-test-key",
			},
			wantErr: true,
		},
		{
			name: "missing public_key",
			config: map[string]interface{}{
				"access_token": "APP_USR-1234567890-test-token",
			},
			wantErr: true,
		},
		{
			name: "invalid access_token format",
			config: map[string]interface{}{
				"access_token": "invalid-token",
				"public_key":   "APP_USR-1234567890-test-key",
			},
			wantErr: true,
		},
		{
			name: "invalid public_key format",
			config: map[string]interface{}{
				"access_token": "APP_USR-1234567890-test-token",
				"public_key":   "invalid-key",
			},
			wantErr: true,
		},
		{
			name: "invalid country",
			config: map[string]interface{}{
				"access_token": "APP_USR-1234567890-test-token",
				"public_key":   "APP_USR-1234567890-test-key",
				"base_url":     "https://api.staging.example",
				"country":      "US", // Invalid country for MercadoPago
			},
			wantErr: true,
		},
		{
			name: "missing base_url",
			config: map[string]interface{}{
				"access_token":   "APP_USR-1234567890-test-token",
				"public_key":     "APP_USR-1234567890-test-key",
				"webhook_secret": "mp-webhook-secret",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := plugin.ValidateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMercadoPagoPlugin_ValidateConfigGuardsApiBaseURLSSRF(t *testing.T) {
	plugin := NewMercadoPagoPlugin(nil)
	base := map[string]interface{}{
		"access_token":   "APP_USR-1234567890-test-token",
		"public_key":     "APP_USR-1234567890-test-key",
		"webhook_secret": "mp-webhook-secret",
		"base_url":       "https://api.staging.example",
		"country":        "AR",
	}
	withAPIBase := func(v string) map[string]interface{} {
		c := map[string]interface{}{}
		for k, val := range base {
			c[k] = val
		}
		c["api_base_url"] = v
		return c
	}

	require.NoError(t, plugin.ValidateConfig(base))

	for _, bad := range []string{
		"http://169.254.169.254",
		"https://10.0.0.5",
		"https://localhost",
		"https://api.mercadopago.com.evil.com",
	} {
		require.Error(t, plugin.ValidateConfig(withAPIBase(bad)), "expected %q to be rejected", bad)
	}

	require.NoError(t, plugin.ValidateConfig(withAPIBase("https://api.mercadopago.com")))
}

func TestMercadoPagoPlugin_isValidAccessToken(t *testing.T) {
	plugin := NewMercadoPagoPlugin(nil)

	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{
			name:  "valid production token",
			token: "APP_USR-1234567890-production-token",
			want:  true,
		},
		{
			name:  "valid test token",
			token: "APP_USR-1234567890-TEST-token",
			want:  true,
		},
		{
			name:  "invalid token - wrong prefix",
			token: "INVALID-1234567890-token",
			want:  false,
		},
		{
			name:  "invalid token - too short",
			token: "APP_USR-123",
			want:  false,
		},
		{
			name:  "empty token",
			token: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := plugin.isValidAccessToken(tt.token)
			if got != tt.want {
				t.Errorf("isValidAccessToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMercadoPagoPlugin_isValidPublicKey(t *testing.T) {
	plugin := NewMercadoPagoPlugin(nil)

	tests := []struct {
		name string
		key  string
		want bool
	}{
		{
			name: "valid production key",
			key:  "APP_USR-1234567890-production-key",
			want: true,
		},
		{
			name: "valid test key",
			key:  "APP_USR-1234567890-TEST-key",
			want: true,
		},
		{
			name: "invalid key - wrong prefix",
			key:  "INVALID-1234567890-key",
			want: false,
		},
		{
			name: "invalid key - too short",
			key:  "APP_USR-123",
			want: false,
		},
		{
			name: "empty key",
			key:  "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := plugin.isValidPublicKey(tt.key)
			if got != tt.want {
				t.Errorf("isValidPublicKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMercadoPagoWebhookURLRequiresExplicitBaseURL(t *testing.T) {
	prev := callbackBaseURL
	t.Cleanup(func() { callbackBaseURL = prev })
	callbackBaseURL = func() string { return "" }

	_, err := mercadoPagoWebhookURL(&MercadoPagoConfig{}, 42)
	if err == nil {
		t.Fatal("expected missing base URL to fail")
	}
}

func TestMercadoPagoWebhookURLIgnoresTenantBaseURL(t *testing.T) {
	prev := callbackBaseURL
	t.Cleanup(func() { callbackBaseURL = prev })
	callbackBaseURL = func() string { return "" }

	// With no instance origin, a tenant base_url must not be used as a fallback.
	if _, err := mercadoPagoWebhookURL(&MercadoPagoConfig{BaseURL: "https://attacker.example/"}, 42); err == nil {
		t.Fatal("expected the tenant base_url to be ignored when the instance origin is empty")
	}
}

func TestMercadoPagoWebhookURLPrefersRuntimeOrigin(t *testing.T) {
	prev := callbackBaseURL
	t.Cleanup(func() { callbackBaseURL = prev })

	callbackBaseURL = func() string { return "https://api.staging.example" }
	got, err := mercadoPagoWebhookURL(&MercadoPagoConfig{BaseURL: "https://api.payverge.io"}, 42)
	if err != nil {
		t.Fatalf("expected webhook URL, got %v", err)
	}
	expected := "https://api.staging.example/api/v1/webhooks/mercadopago?business_id=42&source_news=webhooks"
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

// Note: ProcessPayment tests require database connection and are better suited for integration tests

func TestMercadoPagoRefundRequiresPluginService(t *testing.T) {
	plugin := &MercadoPagoPlugin{}
	err := plugin.RefundPayment(1, "12345", 1000)
	require.Error(t, err)
	require.Contains(t, err.Error(), "plugin service not configured")
}

func TestMercadoPagoProcessPaymentIsExplicitlyUnsupported(t *testing.T) {
	plugin := &MercadoPagoPlugin{}
	id, err := plugin.ProcessPayment(1, 1000, "ARS", nil)
	require.Error(t, err)
	require.Empty(t, id)
	require.Contains(t, err.Error(), "unsupported")
}

func TestGetConfigDefaultsEnvironmentToProduction(t *testing.T) {
	p := NewMercadoPagoPlugin(nil)
	cfg, err := p.configFromMap(map[string]interface{}{
		"access_token": "APP_USR-1234567890123456789012345",
		"public_key":   "APP_USR-pk-1234567890123456789012",
	})
	require.NoError(t, err)
	require.Equal(t, "production", cfg.Environment)
}

func TestGetConfigDerivesSandboxFromTestToken(t *testing.T) {
	p := NewMercadoPagoPlugin(nil)
	cfg, err := p.configFromMap(map[string]interface{}{
		"access_token": "TEST-1234567890123456789012345",
		"public_key":   "TEST-pk-1234567890123456789012",
	})
	require.NoError(t, err)
	require.Equal(t, "sandbox", cfg.Environment)
}

func TestValidateConfigAcceptsTestPrefixedToken(t *testing.T) {
	// Non-production: TEST- tokens remain valid for local/sandbox setup.
	appconfig.SetProductionModeOverride(false)
	t.Cleanup(func() { appconfig.SetProductionModeOverride(false) })
	t.Setenv("ENV", "")
	t.Setenv("APP_ENV", "")

	p := NewMercadoPagoPlugin(nil)
	err := p.ValidateConfig(map[string]interface{}{
		"access_token":   "TEST-1234567890123456789012345",
		"public_key":     "TEST-pk-1234567890123456789012",
		"webhook_secret": "whsec",
		"base_url":       "https://api.payverge.io",
	})
	require.NoError(t, err)
}

// F2: production must reject TEST- credentials so real bills cannot settle via
// MP public test cards under a sandbox environment.
func TestValidateConfig_RejectsTestTokenInProduction(t *testing.T) {
	appconfig.SetProductionModeOverride(true)
	t.Cleanup(func() { appconfig.SetProductionModeOverride(false) })

	p := NewMercadoPagoPlugin(nil)
	err := p.ValidateConfig(map[string]interface{}{
		"access_token":   "TEST-1234567890123456789012345",
		"public_key":     "APP_USR-pk-1234567890123456789012",
		"webhook_secret": "whsec",
		"base_url":       "https://api.payverge.io",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "TEST-")
	require.Contains(t, strings.ToLower(err.Error()), "production")
}

func TestValidateConfig_RejectsTestPublicKeyInProduction(t *testing.T) {
	appconfig.SetProductionModeOverride(true)
	t.Cleanup(func() { appconfig.SetProductionModeOverride(false) })

	p := NewMercadoPagoPlugin(nil)
	err := p.ValidateConfig(map[string]interface{}{
		"access_token":   "APP_USR-1234567890123456789012345",
		"public_key":     "TEST-pk-1234567890123456789012",
		"webhook_secret": "whsec",
		"base_url":       "https://api.payverge.io",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "TEST-")
}

func TestValidateConfig_RejectsSandboxEnvironmentInProduction(t *testing.T) {
	appconfig.SetProductionModeOverride(true)
	t.Cleanup(func() { appconfig.SetProductionModeOverride(false) })

	p := NewMercadoPagoPlugin(nil)
	// Genuine TEST- credentials still refused in production.
	err := p.ValidateConfig(map[string]interface{}{
		"access_token":   "TEST-1234567890123456789012345",
		"public_key":     "TEST-pk-1234567890123456789012",
		"webhook_secret": "whsec",
		"base_url":       "https://api.payverge.io",
		"environment":    "sandbox",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "TEST-")

	// P6a: APP_USR + mislabeled sandbox is treated as production and allowed.
	require.NoError(t, p.ValidateConfig(map[string]interface{}{
		"access_token":   "APP_USR-1234567890123456789012345",
		"public_key":     "APP_USR-pk-1234567890123456789012",
		"webhook_secret": "whsec",
		"base_url":       "https://api.payverge.io",
		"environment":    "sandbox",
	}))
}

func TestCreateBillPayment_RefusesSandboxInProduction(t *testing.T) {
	appconfig.SetProductionModeOverride(true)
	t.Cleanup(func() { appconfig.SetProductionModeOverride(false) })

	// Guard used by CreateBillPayment (defense in depth after ValidateConfig).
	err := refuseSandboxPaymentInProduction("sandbox")
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "sandbox")

	require.NoError(t, refuseSandboxPaymentInProduction("production"))
	require.NoError(t, refuseSandboxPaymentInProduction(""))

	// Non-production still allows sandbox settlement.
	appconfig.SetProductionModeOverride(false)
	require.NoError(t, refuseSandboxPaymentInProduction("sandbox"))
}

// R1 + P6a: APP_USR with a mislabeled sandbox environment is treated as
// production (token prefix wins). Genuine TEST- refusal is covered below.
func TestConfigFromMap_APPUSRMislabeledSandboxIsProduction(t *testing.T) {
	appconfig.SetProductionModeOverride(true)
	t.Cleanup(func() { appconfig.SetProductionModeOverride(false) })

	p := NewMercadoPagoPlugin(nil)
	cfg, err := p.configFromMap(map[string]interface{}{
		"access_token": "APP_USR-1234567890123456789012345",
		"public_key":   "APP_USR-pk-1234567890123456789012",
		"environment":  "sandbox",
	})
	require.NoError(t, err)
	require.Equal(t, "production", cfg.Environment)
}

func TestConfigFromMap_RefusesTestTokenInProduction(t *testing.T) {
	appconfig.SetProductionModeOverride(true)
	t.Cleanup(func() { appconfig.SetProductionModeOverride(false) })

	p := NewMercadoPagoPlugin(nil)
	// Explicit environment=production must not bypass TEST- token refusal.
	_, err := p.configFromMap(map[string]interface{}{
		"access_token": "TEST-1234567890123456789012345",
		"public_key":   "TEST-pk-1234567890123456789012",
		"environment":  "production",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "TEST-")
}

func TestConfigFromMap_AllowsSandboxOutsideProduction(t *testing.T) {
	appconfig.SetProductionModeOverride(false)
	t.Cleanup(func() { appconfig.SetProductionModeOverride(false) })
	t.Setenv("ENV", "")
	t.Setenv("APP_ENV", "")

	p := NewMercadoPagoPlugin(nil)
	cfg, err := p.configFromMap(map[string]interface{}{
		"access_token": "TEST-1234567890123456789012345",
		"public_key":   "TEST-pk-1234567890123456789012",
		"environment":  "sandbox",
	})
	require.NoError(t, err)
	require.Equal(t, "sandbox", cfg.Environment)
}

func TestConfigFromMap_AllowsProductionCredentialsInProduction(t *testing.T) {
	appconfig.SetProductionModeOverride(true)
	t.Cleanup(func() { appconfig.SetProductionModeOverride(false) })

	p := NewMercadoPagoPlugin(nil)
	cfg, err := p.configFromMap(map[string]interface{}{
		"access_token": "APP_USR-1234567890123456789012345",
		"public_key":   "APP_USR-pk-1234567890123456789012",
		"environment":  "production",
	})
	require.NoError(t, err)
	require.Equal(t, "production", cfg.Environment)
}

func TestResolveMercadoPagoEnvironmentExplicitSandbox(t *testing.T) {
	appconfig.SetProductionModeOverride(false)
	t.Cleanup(func() { appconfig.SetProductionModeOverride(false) })

	// Outside production, APP_USR + sandbox label is a valid test-app setup.
	require.Equal(t, "sandbox", resolveMercadoPagoEnvironment(map[string]interface{}{
		"environment":  "Sandbox",
		"access_token": "APP_USR-1234567890123456789012345",
	}))
	// Genuine TEST- tokens remain sandbox.
	require.Equal(t, "sandbox", resolveMercadoPagoEnvironment(map[string]interface{}{
		"environment":  "production",
		"access_token": "TEST-1234567890123456789012345",
	}))
	// Label alone (no token prefix) still works.
	require.Equal(t, "sandbox", resolveMercadoPagoEnvironment(map[string]interface{}{
		"environment": "Sandbox",
	}))

	// P6a: in production, APP_USR wins over a mislabeled sandbox environment.
	appconfig.SetProductionModeOverride(true)
	require.Equal(t, "production", resolveMercadoPagoEnvironment(map[string]interface{}{
		"environment":  "sandbox",
		"access_token": "APP_USR-1234567890123456789012345",
	}))
}
