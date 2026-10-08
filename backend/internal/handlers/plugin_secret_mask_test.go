package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// SEC-1 (2026-06-14 audit): plugin payment secrets must never be returned to a
// browser for any plugin that does not implement PublicConfigProvider. These
// pure-helper tests pin the fail-closed mask + the save-path preserve logic.

func TestMaskSecretConfig_MasksKnownSecretsKeepsPublic(t *testing.T) {
	in := map[string]interface{}{
		"secret_key":              "sk_live_abc",
		"webhook_secret":          "whsec_xyz",
		"webhook_secret_previous": "whsec_previous",
		"client_secret":           "cs_123",
		"access_token":            "APP_USR-tok",
		"bot_token":               "12345:abc",
		"signing_secret":          "sig_1",
		"publishable_key":         "pk_live_pub",
		"client_id":               "client-id-1",
		"webhook_id":              "wh-1",
		"base_url":                "https://api.example.com",
		"enabled":                 true,
	}
	out := maskSecretConfig(in)

	for _, k := range []string{"secret_key", "webhook_secret", "webhook_secret_previous", "client_secret", "access_token", "bot_token", "signing_secret"} {
		assert.Equal(t, maskedSecretValue, out[k], "secret key %q must be masked", k)
	}
	// Public / display-safe fields pass through untouched.
	assert.Equal(t, "pk_live_pub", out["publishable_key"])
	assert.Equal(t, "client-id-1", out["client_id"])
	assert.Equal(t, "wh-1", out["webhook_id"])
	assert.Equal(t, "https://api.example.com", out["base_url"])
	assert.Equal(t, true, out["enabled"])
	// The input map must not be mutated.
	assert.Equal(t, "sk_live_abc", in["secret_key"])
	assert.Equal(t, "whsec_previous", in["webhook_secret_previous"])
}

func TestMaskSecretConfig_EmptySecretStaysEmpty(t *testing.T) {
	out := maskSecretConfig(map[string]interface{}{"secret_key": ""})
	assert.Equal(t, "", out["secret_key"], "empty secret should stay empty, not become the sentinel")
}

func TestMaskSecretConfig_NilSafe(t *testing.T) {
	assert.Nil(t, maskSecretConfig(nil))
}

func TestRestoreMaskedSecrets_PreservesOnSentinelEmptyMissing(t *testing.T) {
	existing := map[string]interface{}{
		"secret_key":              "sk_live_REAL",
		"webhook_secret":          "whsec_REAL",
		"webhook_secret_previous": "whsec_PREV_REAL",
		"publishable_key":         "pk_live_x",
	}
	incoming := map[string]interface{}{
		"secret_key":              maskedSecretValue, // round-tripped mask -> must restore
		"webhook_secret_previous": maskedSecretValue,
		"publishable_key":         "pk_live_x",
		// webhook_secret omitted entirely -> must restore
	}
	out := restoreMaskedSecrets(incoming, existing)
	assert.Equal(t, "sk_live_REAL", out["secret_key"])
	assert.Equal(t, "whsec_REAL", out["webhook_secret"])
	assert.Equal(t, "whsec_PREV_REAL", out["webhook_secret_previous"])
	assert.Equal(t, "pk_live_x", out["publishable_key"])
}

func TestRestoreMaskedSecrets_AllowsGenuineNewSecret(t *testing.T) {
	existing := map[string]interface{}{"secret_key": "sk_live_OLD"}
	incoming := map[string]interface{}{"secret_key": "sk_live_NEW"}
	out := restoreMaskedSecrets(incoming, existing)
	assert.Equal(t, "sk_live_NEW", out["secret_key"], "a genuinely changed secret must be saved")
}

func TestRestoreMaskedSecrets_EmptyStringPreservesStored(t *testing.T) {
	existing := map[string]interface{}{"secret_key": "sk_live_OLD"}
	incoming := map[string]interface{}{"secret_key": ""}
	out := restoreMaskedSecrets(incoming, existing)
	assert.Equal(t, "sk_live_OLD", out["secret_key"], "blank incoming secret must preserve the stored value")
}
