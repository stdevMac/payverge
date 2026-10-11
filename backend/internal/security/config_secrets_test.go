package security

import (
	"errors"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/config"
)

func TestIsSecretConfigKey(t *testing.T) {
	secret := []string{
		"secret_key", "client_secret", "access_token", "refresh_token",
		"webhook_secret", "signing_secret", "bot_token", "api_key",
		"private_key", "encryption_key", "password",
		"stripe_secret", "some_token", "db_password", "service_private_key",
		"webhook_secret_previous",
		"SECRET_KEY", " Access_Token ",
	}
	for _, k := range secret {
		if !IsSecretConfigKey(k) {
			t.Errorf("IsSecretConfigKey(%q) = false, want true", k)
		}
	}

	// Non-secret keys — must stay plaintext/queryable. *_encrypted fields
	// already hold plugin-managed ciphertext and MUST NOT be re-touched by
	// this layer.
	notSecret := []string{
		"publishable_key", "client_id", "public_key", "enabled", "webhook_id",
		"environment", "base_url", "fee_mode", "api_key_encrypted",
		"webhook_secret_encrypted", "chat_id", "is_connected",
	}
	for _, k := range notSecret {
		if IsSecretConfigKey(k) {
			t.Errorf("IsSecretConfigKey(%q) = true, want false", k)
		}
	}
}

func TestEncryptConfigSecrets_EncryptsOnlySecretFields(t *testing.T) {
	in := map[string]interface{}{
		"secret_key":      "sk_live_abc123",
		"webhook_secret":  "whsec_xyz",
		"publishable_key": "pk_live_public",
		"enabled":         true,
	}
	out, err := EncryptConfigSecrets(in)
	if err != nil {
		t.Fatalf("EncryptConfigSecrets() error = %v", err)
	}

	for _, k := range []string{"secret_key", "webhook_secret"} {
		v, _ := out[k].(string)
		if !strings.HasPrefix(v, secretCiphertextPrefix) {
			t.Errorf("%s not encrypted: %q", k, v)
		}
		if strings.Contains(v, in[k].(string)) {
			t.Errorf("%s ciphertext leaks plaintext: %q", k, v)
		}
	}
	if out["publishable_key"] != "pk_live_public" {
		t.Errorf("publishable_key was modified: %v", out["publishable_key"])
	}
	if out["enabled"] != true {
		t.Errorf("enabled was modified: %v", out["enabled"])
	}
}

func TestEncryptConfigSecrets_DoesNotMutateInput(t *testing.T) {
	in := map[string]interface{}{"secret_key": "sk_live_abc123"}
	if _, err := EncryptConfigSecrets(in); err != nil {
		t.Fatalf("EncryptConfigSecrets() error = %v", err)
	}
	if in["secret_key"] != "sk_live_abc123" {
		t.Fatalf("input map was mutated: %v", in["secret_key"])
	}
}

func TestEncryptDecryptConfigSecrets_RoundTrip(t *testing.T) {
	in := map[string]interface{}{
		"secret_key":     "sk_live_abc123",
		"access_token":   "APP_USR-token-456",
		"client_secret":  "ps_secret_789",
		"public_key":     "pub_key",
		"enabled":        true,
		"webhook_secret": "",
	}
	enc, err := EncryptConfigSecrets(in)
	if err != nil {
		t.Fatalf("EncryptConfigSecrets() error = %v", err)
	}
	dec, err := DecryptConfigSecrets(enc)
	if err != nil {
		t.Fatalf("DecryptConfigSecrets() error = %v", err)
	}
	for k, want := range in {
		if dec[k] != want {
			t.Errorf("round-trip %s = %v, want %v", k, dec[k], want)
		}
	}
}

func TestEncryptConfigSecrets_Idempotent(t *testing.T) {
	in := map[string]interface{}{"secret_key": "sk_live_abc123"}
	enc1, err := EncryptConfigSecrets(in)
	if err != nil {
		t.Fatalf("first encrypt error = %v", err)
	}
	enc2, err := EncryptConfigSecrets(enc1)
	if err != nil {
		t.Fatalf("second encrypt error = %v", err)
	}
	// A second pass must NOT re-encrypt an already-v1: value (double encryption
	// would make the value undecryptable in one DecryptSecret pass).
	if enc2["secret_key"] != enc1["secret_key"] {
		t.Fatalf("encrypt not idempotent: %v != %v", enc2["secret_key"], enc1["secret_key"])
	}
	dec, err := DecryptConfigSecrets(enc2)
	if err != nil {
		t.Fatalf("decrypt error = %v", err)
	}
	if dec["secret_key"] != "sk_live_abc123" {
		t.Fatalf("idempotent round-trip = %v, want original", dec["secret_key"])
	}
}

func TestDecryptConfigSecrets_LegacyPlaintextPassthrough(t *testing.T) {
	// Rows written before encryption was enabled hold plaintext secrets with no
	// v1: prefix. They must read back unchanged so nothing breaks mid-migration.
	in := map[string]interface{}{
		"secret_key": "sk_live_legacy_plaintext",
		"enabled":    true,
	}
	out, err := DecryptConfigSecrets(in)
	if err != nil {
		t.Fatalf("DecryptConfigSecrets() error = %v", err)
	}
	if out["secret_key"] != "sk_live_legacy_plaintext" {
		t.Fatalf("legacy plaintext not preserved: %v", out["secret_key"])
	}
}

func TestEncryptConfigSecrets_SkipsEmptyAndNonString(t *testing.T) {
	in := map[string]interface{}{
		"secret_key":    "",         // empty — nothing to encrypt
		"fail_count":    3,          // numeric under non-secret key
		"signing_token": float64(7), // numeric under a *_token key
	}
	out, err := EncryptConfigSecrets(in)
	if err != nil {
		t.Fatalf("EncryptConfigSecrets() error = %v", err)
	}
	if out["secret_key"] != "" {
		t.Errorf("empty secret was modified: %v", out["secret_key"])
	}
	if out["fail_count"] != 3 {
		t.Errorf("numeric field modified: %v", out["fail_count"])
	}
	if out["signing_token"] != float64(7) {
		t.Errorf("non-string secret-keyed field modified: %v", out["signing_token"])
	}
}

func TestDecryptConfigSecrets_TamperedCiphertextErrors(t *testing.T) {
	enc, err := EncryptConfigSecrets(map[string]interface{}{"secret_key": "sk_live_abc123"})
	if err != nil {
		t.Fatalf("encrypt error = %v", err)
	}
	ct := enc["secret_key"].(string)
	tampered := ct[:len(ct)-2] + flipLast2(ct)
	if tampered == ct {
		tampered = ct + "AA"
	}
	if _, err := DecryptConfigSecrets(map[string]interface{}{"secret_key": tampered}); err == nil {
		t.Fatal("DecryptConfigSecrets() error = nil for tampered ciphertext, want error")
	}
}

func flipLast2(s string) string {
	if len(s) < 2 {
		return s + "ZZ"
	}
	b := []byte(s[len(s)-2:])
	for i := range b {
		if b[i] == 'A' {
			b[i] = 'B'
		} else {
			b[i] = 'A'
		}
	}
	return string(b)
}

func TestEncryptConfigSecrets_FailsClosedInProduction(t *testing.T) {
	// Production with no usable key: writing the credential would put it in the
	// clear on the row (and in every pg_dump of it). Refuse the write instead,
	// matching encodePlatformSettingValue's fail-closed contract.
	t.Setenv("PLUGIN_SECRET_KEY", "")
	t.Setenv("ENV", "")
	t.Setenv("APP_ENV", "")
	config.SetProductionModeOverride(true)
	t.Cleanup(func() { config.SetProductionModeOverride(false) })

	if SecretsEncryptionAvailable() {
		t.Fatal("SecretsEncryptionAvailable() = true with no key in production")
	}
	in := map[string]interface{}{"secret_key": "sk_live_abc123"}
	out, err := EncryptConfigSecrets(in)
	if !errors.Is(err, ErrPlaintextSecretRefused) {
		t.Fatalf("EncryptConfigSecrets() error = %v, want ErrPlaintextSecretRefused", err)
	}
	if out != nil {
		t.Fatalf("EncryptConfigSecrets() returned a config alongside the refusal: %v", out)
	}
}

func TestEncryptConfigSecrets_ProductionAllowsSecretlessConfig(t *testing.T) {
	// No credential in the map means nothing can leak, so an unavailable key must
	// not block ordinary plugin settings saves.
	t.Setenv("PLUGIN_SECRET_KEY", "")
	t.Setenv("ENV", "")
	t.Setenv("APP_ENV", "")
	config.SetProductionModeOverride(true)
	t.Cleanup(func() { config.SetProductionModeOverride(false) })

	in := map[string]interface{}{"enabled": true, "currency": "usd", "secret_key": ""}
	out, err := EncryptConfigSecrets(in)
	if err != nil {
		t.Fatalf("EncryptConfigSecrets() error = %v, want nil for a credential-free config", err)
	}
	if out["currency"] != "usd" || out["enabled"] != true {
		t.Fatalf("non-secret fields altered: %v", out)
	}
}

func TestEncryptConfigSecrets_NoOpOutsideProduction(t *testing.T) {
	// A set-but-invalid key outside production (the local .env placeholder case)
	// stays a passthrough so local stacks keep working.
	t.Setenv("PLUGIN_SECRET_KEY", "not-a-valid-32-byte-key")
	t.Setenv("ENV", "development")
	t.Setenv("APP_ENV", "")
	config.SetProductionModeOverride(false)

	if SecretsEncryptionAvailable() {
		t.Fatal("SecretsEncryptionAvailable() = true for an invalid key")
	}
	in := map[string]interface{}{"secret_key": "sk_test_abc123"}
	out, err := EncryptConfigSecrets(in)
	if err != nil {
		t.Fatalf("EncryptConfigSecrets() error = %v (must be a no-op outside production)", err)
	}
	if out["secret_key"] != "sk_test_abc123" {
		t.Fatalf("expected plaintext passthrough when unavailable, got %v", out["secret_key"])
	}
}

func TestSecretsEncryptionAvailable_DevFallback(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "")
	t.Setenv("ENV", "development")
	t.Setenv("APP_ENV", "")
	config.SetProductionModeOverride(false)
	if !SecretsEncryptionAvailable() {
		t.Fatal("SecretsEncryptionAvailable() = false in development (dev fallback expected)")
	}
}

func TestEncryptDecryptConfigSecrets_NilSafe(t *testing.T) {
	if out, err := EncryptConfigSecrets(nil); err != nil || out != nil {
		t.Fatalf("EncryptConfigSecrets(nil) = (%v, %v), want (nil, nil)", out, err)
	}
	if out, err := DecryptConfigSecrets(nil); err != nil || out != nil {
		t.Fatalf("DecryptConfigSecrets(nil) = (%v, %v), want (nil, nil)", out, err)
	}
}

func benchConfig() map[string]interface{} {
	return map[string]interface{}{
		"secret_key":      "sk_live_51AbCdEfGhIjKlMnOpQrStUvWxYz",
		"webhook_secret":  "whsec_AbCdEfGhIjKlMnOpQrStUvWxYz0123456789",
		"publishable_key": "pk_live_51AbCdEfGhIjKlMnOpQrStUvWxYz",
		"environment":     "live",
		"enabled":         true,
	}
}

// BenchmarkDecryptConfigSecrets measures the per-read cost added at the DB read
// boundary for a stripe-like config with two encrypted secret fields. This runs
// on every plugin-config load (payment paths included), so it is the relevant
// hot-path metric for the encryption-at-rest change.
func BenchmarkDecryptConfigSecrets(b *testing.B) {
	enc, err := EncryptConfigSecrets(benchConfig())
	if err != nil {
		b.Fatalf("setup encrypt error = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := DecryptConfigSecrets(enc); err != nil {
			b.Fatalf("decrypt error = %v", err)
		}
	}
}

// BenchmarkDecryptConfigSecrets_NoSecrets measures the cost on configs with no
// secret fields (e.g. telegram connection state) — should be near-free.
func BenchmarkDecryptConfigSecrets_NoSecrets(b *testing.B) {
	cfg := map[string]interface{}{"chat_id": "12345", "is_connected": true, "enabled": true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := DecryptConfigSecrets(cfg); err != nil {
			b.Fatalf("decrypt error = %v", err)
		}
	}
}

func BenchmarkEncryptConfigSecrets(b *testing.B) {
	cfg := benchConfig()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := EncryptConfigSecrets(cfg); err != nil {
			b.Fatalf("encrypt error = %v", err)
		}
	}
}
