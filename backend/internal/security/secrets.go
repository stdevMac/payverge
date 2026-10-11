package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/config"
)

const secretCiphertextPrefix = "v1:"

func EncryptSecret(plaintext string) (string, error) {
	if strings.TrimSpace(plaintext) == "" {
		return "", errors.New("secret plaintext is required")
	}

	gcm, err := secretCipher()
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate secret nonce: %w", err)
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return secretCiphertextPrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func DecryptSecret(ciphertext string) (string, error) {
	ciphertext = strings.TrimSpace(ciphertext)
	if !strings.HasPrefix(ciphertext, secretCiphertextPrefix) {
		return "", errors.New("secret ciphertext must use v1 format")
	}

	encoded := strings.TrimPrefix(ciphertext, secretCiphertextPrefix)
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode secret ciphertext: %w", err)
	}

	gcm, err := secretCipher()
	if err != nil {
		return "", err
	}
	if len(sealed) <= gcm.NonceSize() {
		return "", errors.New("secret ciphertext is too short")
	}

	nonce := sealed[:gcm.NonceSize()]
	ciphertextBytes := sealed[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt secret ciphertext: %w", err)
	}

	return string(plaintext), nil
}

func secretCipher() (cipher.AEAD, error) {
	key, err := pluginSecretKey()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create secret cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create secret gcm: %w", err)
	}

	return gcm, nil
}

// pluginSecretKey returns the AES-256 key for plugin and fiscal credentials.
// PLUGIN_SECRET_KEY always wins. Production has no fallback. Development uses
// the per-instance key from EnsureDevPluginSecretKey (or an ephemeral
// per-process key) — never a constant, since a constant compiled into a public
// repository would decrypt every credential an instance stored with it.
func pluginSecretKey() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("PLUGIN_SECRET_KEY"))
	if raw == "" {
		if config.IsProductionMode(false) {
			return nil, errors.New("PLUGIN_SECRET_KEY is required in production")
		}
		return developmentPluginKey()
	}
	return decodePluginKey(raw)
}
