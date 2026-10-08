package database

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupPluginSecretsTestDB gives each test its own in-memory SQLite DB (named by
// the test) with just the plugin tables migrated, so plugin-name unique indexes
// don't collide across tests sharing the process cache.
func setupPluginSecretsTestDB(t *testing.T) {
	t.Helper()
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &Plugin{}, &BusinessPlugin{}))
	SetTestDB(gormDB)
}

func paymentPlugin(t *testing.T, name string) *Plugin {
	t.Helper()
	plugin, err := CreatePlugin(Plugin{
		Name:        name,
		DisplayName: name,
		Category:    PluginCategoryPayment,
		IsActive:    true,
	})
	require.NoError(t, err)
	return plugin
}

func rawConfigColumn(t *testing.T, businessID, pluginID uint) string {
	t.Helper()
	var raw string
	require.NoError(t, GetDB().
		Raw("SELECT config FROM business_plugins WHERE business_id = ? AND plugin_id = ?", businessID, pluginID).
		Scan(&raw).Error)
	return raw
}

// TestEnableBusinessPlugin_EncryptsSecretsAtRest is the core B1 regression: after
// EnableBusinessPlugin, the stored config column must NOT contain the plaintext
// payment-provider secrets (only v1: ciphertext), while non-secret fields stay
// plaintext, and GetBusinessPluginConfig transparently returns plaintext so
// callers (the payment plugins) are unaffected.
func TestEnableBusinessPlugin_EncryptsSecretsAtRest(t *testing.T) {
	setupPluginSecretsTestDB(t)
	plugin := paymentPlugin(t, "stripe")

	const (
		secret  = "sk_live_super_secret_value_0123456789"
		signing = "whsec_signing_secret_value"
		pub     = "pk_live_public_value"
	)
	const bizID = uint(1)
	require.NoError(t, EnableBusinessPlugin(bizID, plugin.ID, map[string]interface{}{
		"enabled":         true,
		"secret_key":      secret,
		"webhook_secret":  signing,
		"publishable_key": pub,
	}))

	raw := rawConfigColumn(t, bizID, plugin.ID)
	require.NotContains(t, raw, secret, "plaintext secret_key must not be stored at rest")
	require.NotContains(t, raw, signing, "plaintext webhook_secret must not be stored at rest")
	require.Contains(t, raw, "v1:", "secret fields should be stored as v1: ciphertext")
	require.Contains(t, raw, pub, "non-secret publishable_key should remain plaintext/queryable")

	cfg, err := GetBusinessPluginConfig(bizID, "stripe")
	require.NoError(t, err)
	require.Equal(t, secret, cfg["secret_key"], "GetBusinessPluginConfig must transparently decrypt")
	require.Equal(t, signing, cfg["webhook_secret"])
	require.Equal(t, pub, cfg["publishable_key"])
	require.Equal(t, true, cfg["enabled"])
}

// TestUpdateBusinessPluginConfig_EncryptsSecretsAtRest covers the second write
// choke point (the raw UPDATE path).
func TestUpdateBusinessPluginConfig_EncryptsSecretsAtRest(t *testing.T) {
	setupPluginSecretsTestDB(t)
	plugin := paymentPlugin(t, "mercadopago")
	const bizID = uint(2)
	require.NoError(t, EnableBusinessPlugin(bizID, plugin.ID, map[string]interface{}{"enabled": true}))

	const token = "APP_USR-1234567890-secret-access-token"
	require.NoError(t, UpdateBusinessPluginConfig(bizID, "mercadopago", map[string]interface{}{
		"enabled":      true,
		"access_token": token,
		"public_key":   "APP_USR-public",
	}))

	raw := rawConfigColumn(t, bizID, plugin.ID)
	require.NotContains(t, raw, token, "plaintext access_token must not be stored at rest after update")
	require.Contains(t, raw, "v1:", "access_token should be stored as v1: ciphertext")

	cfg, err := GetBusinessPluginConfig(bizID, "mercadopago")
	require.NoError(t, err)
	require.Equal(t, token, cfg["access_token"], "update path must round-trip plaintext on read")
	require.Equal(t, "APP_USR-public", cfg["public_key"])
}

// TestGetBusinessPluginConfig_LegacyPlaintextStillReadable proves rows written
// before encryption-at-rest (plaintext, no v1: prefix) continue to read back
// unchanged — so a deploy + key provisioning doesn't strand existing configs.
func TestGetBusinessPluginConfig_LegacyPlaintextStillReadable(t *testing.T) {
	setupPluginSecretsTestDB(t)
	plugin := paymentPlugin(t, "paypal")
	const bizID = uint(3)

	// Insert a legacy plaintext row directly, bypassing EnableBusinessPlugin's
	// encryption — this is exactly the shape of pre-migration production rows.
	require.NoError(t, GetDB().Create(&BusinessPlugin{
		BusinessID: bizID,
		PluginID:   plugin.ID,
		IsEnabled:  true,
		Config:     `{"enabled":true,"client_secret":"legacy_plaintext_secret","client_id":"legacy_id"}`,
	}).Error)

	cfg, err := GetBusinessPluginConfig(bizID, "paypal")
	require.NoError(t, err)
	require.Equal(t, "legacy_plaintext_secret", cfg["client_secret"], "legacy plaintext secret must pass through")
	require.Equal(t, "legacy_id", cfg["client_id"])
}
