package ar

import (
	"crypto/rsa"
	"crypto/x509"
	"time"

	"github.com/stdevmac/payverge/backend/internal/fiscal/certutil"
)

// CertificateBundle holds a parsed X.509 certificate and its matching RSA private key,
// along with a SHA-256 fingerprint and expiry date derived from the certificate.
// Private key material must never be logged.
type CertificateBundle struct {
	Cert        *x509.Certificate
	Key         *rsa.PrivateKey
	Fingerprint string
	NotAfter    time.Time
}

// ParseCertificateBundle decodes PEM-encoded certificate and private key bytes,
// verifies that the key matches the certificate's public key, and returns a
// CertificateBundle. Returns an error if either PEM block is invalid, the key
// does not match the certificate, or the key is not RSA.
func ParseCertificateBundle(certPEM, keyPEM []byte) (*CertificateBundle, error) {
	b, err := certutil.ParseBundle(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &CertificateBundle{
		Cert:        b.Cert,
		Key:         b.Key,
		Fingerprint: b.Fingerprint,
		NotAfter:    b.NotAfter,
	}, nil
}
