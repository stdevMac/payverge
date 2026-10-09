package security

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/config"
)

func TestEncryptDecryptSecretRoundTrip(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")

	encrypted, err := EncryptSecret("td_live_secret")
	if err != nil {
		t.Fatalf("EncryptSecret() error = %v", err)
	}
	if strings.Contains(encrypted, "td_live_secret") {
		t.Fatalf("EncryptSecret() ciphertext contains plaintext: %q", encrypted)
	}
	if !strings.HasPrefix(encrypted, "v1:") {
		t.Fatalf("EncryptSecret() ciphertext prefix = %q, want v1:", encrypted)
	}

	decrypted, err := DecryptSecret(encrypted)
	if err != nil {
		t.Fatalf("DecryptSecret() error = %v", err)
	}
	if decrypted != "td_live_secret" {
		t.Fatalf("DecryptSecret() = %q, want %q", decrypted, "td_live_secret")
	}
}

func TestEncryptDecryptSecretPreservesWhitespace(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")

	plaintext := "  td_live_secret  \n"
	encrypted, err := EncryptSecret(plaintext)
	if err != nil {
		t.Fatalf("EncryptSecret() error = %v", err)
	}

	decrypted, err := DecryptSecret(encrypted)
	if err != nil {
		t.Fatalf("DecryptSecret() error = %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("DecryptSecret() = %q, want exact plaintext %q", decrypted, plaintext)
	}
}

func TestEncryptSecretUsesUniqueNonce(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")

	first, err := EncryptSecret("td_live_secret")
	if err != nil {
		t.Fatalf("EncryptSecret() first error = %v", err)
	}
	second, err := EncryptSecret("td_live_secret")
	if err != nil {
		t.Fatalf("EncryptSecret() second error = %v", err)
	}
	if first == second {
		t.Fatal("EncryptSecret() returned identical ciphertexts for the same plaintext")
	}
}

func TestEncryptSecretRejectsBlankPlaintext(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")

	if _, err := EncryptSecret(" \n\t "); err == nil {
		t.Fatal("EncryptSecret() error = nil, want error")
	}
}

func TestDecryptSecretRejectsMalformedCiphertext(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")

	if _, err := DecryptSecret("plain-text"); err == nil {
		t.Fatal("DecryptSecret() error = nil, want error")
	}
}

func TestDecryptSecretRejectsTamperedCiphertext(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")

	encrypted, err := EncryptSecret("td_live_secret")
	if err != nil {
		t.Fatalf("EncryptSecret() error = %v", err)
	}
	tampered := encrypted[:len(encrypted)-1] + "A"
	if tampered == encrypted {
		tampered = encrypted[:len(encrypted)-1] + "B"
	}

	if _, err := DecryptSecret(tampered); err == nil {
		t.Fatal("DecryptSecret() error = nil, want error")
	}
}

func TestDecryptSecretRejectsMalformedV1Payloads(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")

	tests := []struct {
		name       string
		ciphertext string
	}{
		{name: "invalid base64", ciphertext: "v1:not base64!"},
		{name: "too short decoded payload", ciphertext: "v1:" + base64.RawURLEncoding.EncodeToString([]byte("short"))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecryptSecret(tt.ciphertext); err == nil {
				t.Fatal("DecryptSecret() error = nil, want error")
			}
		})
	}
}

func TestEncryptSecretAcceptsBase64EncodedKey(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	t.Setenv("PLUGIN_SECRET_KEY", base64.StdEncoding.EncodeToString(key))

	encrypted, err := EncryptSecret("td_live_secret")
	if err != nil {
		t.Fatalf("EncryptSecret() error = %v", err)
	}

	decrypted, err := DecryptSecret(encrypted)
	if err != nil {
		t.Fatalf("DecryptSecret() error = %v", err)
	}
	if decrypted != "td_live_secret" {
		t.Fatalf("DecryptSecret() = %q, want %q", decrypted, "td_live_secret")
	}
}

func TestEncryptSecretRejectsInvalidKeyLength(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "too-short")

	if _, err := EncryptSecret("td_live_secret"); err == nil {
		t.Fatal("EncryptSecret() error = nil, want error")
	}
}

func TestEncryptSecretUsesDevFallbackOutsideProduction(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "")
	t.Setenv("ENV", "development")
	t.Setenv("APP_ENV", "")
	config.SetProductionModeOverride(false)

	encrypted, err := EncryptSecret("td_live_secret")
	if err != nil {
		t.Fatalf("EncryptSecret() error = %v", err)
	}
	decrypted, err := DecryptSecret(encrypted)
	if err != nil {
		t.Fatalf("DecryptSecret() error = %v", err)
	}
	if decrypted != "td_live_secret" {
		t.Fatalf("DecryptSecret() = %q, want %q", decrypted, "td_live_secret")
	}
}

func TestEncryptSecretRequiresKeyInProduction(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "")
	t.Setenv("ENV", "")
	t.Setenv("APP_ENV", "")
	config.SetProductionModeOverride(true)
	t.Cleanup(func() {
		config.SetProductionModeOverride(false)
	})

	if _, err := EncryptSecret("td_live_secret"); err == nil {
		t.Fatal("EncryptSecret() error = nil, want error")
	}
}
