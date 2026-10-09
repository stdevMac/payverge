package fiscal

import (
	"encoding/json"
	"time"

	"github.com/stdevmac/payverge/backend/internal/fiscal/certutil"
	"github.com/stdevmac/payverge/backend/internal/security"
)

// parsedCertBundle is the result of validateAndParseCertBundle.
type parsedCertBundle struct {
	Fingerprint string
	NotAfter    time.Time
}

// validateAndParseCertBundle decodes PEM cert+key, verifies they match, and
// returns fingerprint + expiry. Returns an error on any mismatch or parse failure.
func validateAndParseCertBundle(certPEM, keyPEM []byte) (*parsedCertBundle, error) {
	b, err := certutil.ParseBundle(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &parsedCertBundle{
		Fingerprint: b.Fingerprint,
		NotAfter:    b.NotAfter,
	}, nil
}

// CredentialBundle holds the PEM-encoded certificate and private key for an
// AFIP/ARCA fiscal credential. It is stored encrypted at rest via EncryptCredentialBundle
// and retrieved via DecryptCredentialBundle. Never log KeyPEM.
type CredentialBundle struct {
	CertPEM string `json:"cert_pem"`
	KeyPEM  string `json:"key_pem"`
}

// EncryptCredentialBundle serialises the bundle as JSON and encrypts it with
// AES-256-GCM via the security package (PLUGIN_SECRET_KEY or local-dev fallback).
// The returned bytes are safe to store in Settings.CredentialsEncrypted.
func EncryptCredentialBundle(b CredentialBundle) ([]byte, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	enc, err := security.EncryptSecret(string(raw))
	if err != nil {
		return nil, err
	}
	return []byte(enc), nil
}

// DecryptCredentialBundle reverses EncryptCredentialBundle.
func DecryptCredentialBundle(enc []byte) (CredentialBundle, error) {
	var out CredentialBundle
	plain, err := security.DecryptSecret(string(enc))
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal([]byte(plain), &out)
}
