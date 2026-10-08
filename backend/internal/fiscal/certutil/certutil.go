// Package certutil provides shared PEM certificate/private-key parsing for the
// fiscal subsystem. It has no imports outside the Go standard library, which
// prevents import cycles between the fiscal and fiscal/providers/ar packages.
package certutil

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"time"
)

// Bundle is the result of a successful ParseBundle call. Private key material
// must never be logged.
type Bundle struct {
	Cert        *x509.Certificate
	Key         *rsa.PrivateKey
	Fingerprint string    // SHA-256 hex digest of the DER-encoded certificate
	NotAfter    time.Time // certificate expiry
}

// ParseBundle decodes PEM-encoded certificate and private key bytes, verifies
// that the RSA key matches the certificate's public key, and returns a Bundle.
// The key is accepted in PKCS1 ("RSA PRIVATE KEY") or PKCS8 ("PRIVATE KEY")
// PEM form. Returns an error if either block is absent, the certificate or key
// cannot be parsed, the key is not RSA, or the key does not match the
// certificate.
func ParseBundle(certPEM, keyPEM []byte) (*Bundle, error) {
	cblock, _ := pem.Decode(certPEM)
	if cblock == nil {
		return nil, errors.New("invalid certificate PEM")
	}
	cert, err := x509.ParseCertificate(cblock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	kblock, _ := pem.Decode(keyPEM)
	if kblock == nil {
		return nil, errors.New("invalid private key PEM")
	}
	key, err := parseRSAKey(kblock.Bytes)
	if err != nil {
		return nil, err
	}

	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok || pub.N.Cmp(key.N) != 0 {
		return nil, errors.New("private key does not match certificate")
	}

	sum := sha256.Sum256(cert.Raw)
	return &Bundle{
		Cert:        cert,
		Key:         key,
		Fingerprint: hex.EncodeToString(sum[:]),
		NotAfter:    cert.NotAfter,
	}, nil
}

// parseRSAKey attempts PKCS1 then PKCS8 DER parsing. Returns an error if
// neither format succeeds or the PKCS8 key is not RSA.
func parseRSAKey(der []byte) (*rsa.PrivateKey, error) {
	if k, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return k, nil
	}
	k8, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	rsaKey, ok := k8.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}
	return rsaKey, nil
}
