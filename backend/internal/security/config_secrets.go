package security

import (
	"errors"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/config"
)

// IsSecretConfigKey reports whether a plugin-config key holds a credential that
// must be encrypted at rest and never returned to a browser. This is the
// canonical predicate shared by the encrypt-at-rest layer (EncryptConfigSecrets /
// DecryptConfigSecrets) and the handler-side response masking, so the set of
// fields treated as secret can never drift between the two.
//
// Keys ending in the "_encrypted" suffix are deliberately NOT secret keys:
// they already hold v1: ciphertext that the plugin manages itself, and
// re-encrypting them would double-wrap the value.
func IsSecretConfigKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	switch k {
	case "secret_key", "client_secret", "access_token", "refresh_token",
		"webhook_secret", "signing_secret", "bot_token", "api_key",
		"private_key", "encryption_key", "password":
		return true
	}
	return strings.HasSuffix(k, "_secret") ||
		strings.HasSuffix(k, "_secret_previous") ||
		strings.HasSuffix(k, "_token") ||
		strings.HasSuffix(k, "_password") ||
		strings.HasSuffix(k, "_private_key")
}

// SecretsEncryptionAvailable reports whether a usable encryption key is
// configured, WITHOUT erroring (unlike EncryptSecret, which fails hard). When it
// returns false the config encrypt layer is a no-op, so plugin config is stored
// exactly as it is today (plaintext at rest). Enabling encryption is therefore
// purely a matter of provisioning PLUGIN_SECRET_KEY (production) — outside
// production the per-instance development key always makes it available.
func SecretsEncryptionAvailable() bool {
	_, err := pluginSecretKey()
	return err == nil
}

// ErrPlaintextSecretRefused is returned when a config carries a credential that
// would have to be written unencrypted because no usable PLUGIN_SECRET_KEY is
// configured. Production refuses that write rather than persisting the secret in
// the clear.
var ErrPlaintextSecretRefused = errors.New("refusing to store plugin secrets in plaintext: PLUGIN_SECRET_KEY is missing or invalid")

// needsEncryption reports whether config holds at least one secret-bearing value
// that is not already v1: ciphertext, i.e. whether an unavailable key would
// actually cause a credential to hit the row in the clear.
func needsEncryption(config map[string]interface{}) bool {
	for k, v := range config {
		if !IsSecretConfigKey(k) {
			continue
		}
		s, ok := v.(string)
		if !ok || s == "" || strings.HasPrefix(s, secretCiphertextPrefix) {
			continue
		}
		return true
	}
	return false
}

// EncryptConfigSecrets returns a copy of config in which every secret-bearing
// string field (per IsSecretConfigKey) is encrypted with EncryptSecret. Empty
// strings, non-string values, and values already in v1: ciphertext form are left
// untouched, which makes the function idempotent and safe to run repeatedly
// (e.g. a backfill).
//
// When encryption is unavailable the behavior splits by environment. Production
// fails closed with ErrPlaintextSecretRefused if the config actually carries a
// credential — matching encodePlatformSettingValue, so neither secret sink can
// ever write a plaintext credential to a production row. (Startup preflight
// already rejects a missing/placeholder/invalid PLUGIN_SECRET_KEY; this is the
// write-time backstop for anything that gets past it.) Outside production it
// stays a passthrough so local stacks without a key keep working, and configs
// with no credentials in them pass through everywhere. The input map is never
// mutated.
func EncryptConfigSecrets(cfg map[string]interface{}) (map[string]interface{}, error) {
	if cfg == nil {
		return nil, nil
	}
	out := make(map[string]interface{}, len(cfg))
	for k, v := range cfg {
		out[k] = v
	}
	if !SecretsEncryptionAvailable() {
		if config.IsProductionMode(false) && needsEncryption(out) {
			return nil, ErrPlaintextSecretRefused
		}
		return out, nil
	}
	for k, v := range out {
		if !IsSecretConfigKey(k) {
			continue
		}
		s, ok := v.(string)
		if !ok || s == "" || strings.HasPrefix(s, secretCiphertextPrefix) {
			continue
		}
		encrypted, err := EncryptSecret(s)
		if err != nil {
			return nil, err
		}
		out[k] = encrypted
	}
	return out, nil
}

// DecryptConfigSecrets returns a copy of config in which every v1: ciphertext
// value under a secret key is decrypted back to plaintext. Values without the
// v1: prefix pass through unchanged, so rows written before encryption was
// enabled (legacy plaintext) keep working through a migration. A v1: value that
// fails to decrypt (wrong key or tampering) returns an error rather than handing
// a corrupt credential downstream. The input map is never mutated.
func DecryptConfigSecrets(config map[string]interface{}) (map[string]interface{}, error) {
	if config == nil {
		return nil, nil
	}
	out := make(map[string]interface{}, len(config))
	for k, v := range config {
		out[k] = v
	}
	for k, v := range out {
		if !IsSecretConfigKey(k) {
			continue
		}
		s, ok := v.(string)
		if !ok || !strings.HasPrefix(s, secretCiphertextPrefix) {
			continue
		}
		plaintext, err := DecryptSecret(s)
		if err != nil {
			return nil, err
		}
		out[k] = plaintext
	}
	return out, nil
}
