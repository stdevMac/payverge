package database

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecretPlatformSettingIsEncryptedAtRest(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")
	db := setupPlatformSettingsTestDB(t)

	require.NoError(t, db.SetPlatformSetting("test_secret_key", "sk_live_supersecret", CategoryGeneral, true))

	// Raw row must be ciphertext.
	var raw PlatformSettings
	require.NoError(t, db.conn.Where("key = ?", "test_secret_key").First(&raw).Error)
	require.True(t, strings.HasPrefix(raw.Value, "v1:"), "stored value must carry the ciphertext prefix, got %q", raw.Value)
	require.NotContains(t, raw.Value, "sk_live_supersecret")

	// Read path must transparently decrypt.
	value, err := db.GetPlatformSettingValue("test_secret_key")
	require.NoError(t, err)
	require.Equal(t, "sk_live_supersecret", value)
}

func TestLegacyPlaintextSecretStillReads(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")
	db := setupPlatformSettingsTestDB(t)

	// A pre-encryption row written directly, bypassing SetPlatformSetting.
	require.NoError(t, db.conn.Create(&PlatformSettings{
		Key: "test_webhook_secret", Value: "whsec_legacy", Category: CategoryGeneral, IsSecret: true,
	}).Error)

	value, err := db.GetPlatformSettingValue("test_webhook_secret")
	require.NoError(t, err)
	require.Equal(t, "whsec_legacy", value, "legacy plaintext secrets must keep reading until re-saved")
}

func TestNonSecretSettingStaysPlaintext(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")
	db := setupPlatformSettingsTestDB(t)

	require.NoError(t, db.SetPlatformSetting("test_public_key", "pk_live_public", CategoryGeneral, false))

	var raw PlatformSettings
	require.NoError(t, db.conn.Where("key = ?", "test_public_key").First(&raw).Error)
	require.Equal(t, "pk_live_public", raw.Value)
}
