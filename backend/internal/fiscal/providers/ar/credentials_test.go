package ar

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newSelfSignedPEM generates a self-signed RSA certificate + PKCS1 key in PEM form.
func newSelfSignedPEM(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "generate RSA key")

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err, "create certificate")

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM
}

func TestParseCertificateBundle(t *testing.T) {
	certPEM, keyPEM := newSelfSignedPEM(t)
	b, err := ParseCertificateBundle(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	if b.Fingerprint == "" || b.NotAfter.IsZero() || b.Cert == nil || b.Key == nil {
		t.Fatalf("incomplete bundle: %+v", b)
	}
}

func TestParseCertificateBundle_Mismatch(t *testing.T) {
	certPEM, _ := newSelfSignedPEM(t)
	_, otherKey := newSelfSignedPEM(t)
	if _, err := ParseCertificateBundle(certPEM, otherKey); err == nil {
		t.Fatal("want error when key does not match cert")
	}
}

func TestParseCertificateBundle_InvalidCertPEM(t *testing.T) {
	_, keyPEM := newSelfSignedPEM(t)
	_, err := ParseCertificateBundle([]byte("not a pem"), keyPEM)
	require.Error(t, err)
	require.Contains(t, err.Error(), "certificate PEM")
}

func TestParseCertificateBundle_InvalidKeyPEM(t *testing.T) {
	certPEM, _ := newSelfSignedPEM(t)
	_, err := ParseCertificateBundle(certPEM, []byte("not a pem"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "private key PEM")
}

func TestParseCertificateBundle_FingerprintIsDeterministic(t *testing.T) {
	certPEM, keyPEM := newSelfSignedPEM(t)
	b1, err := ParseCertificateBundle(certPEM, keyPEM)
	require.NoError(t, err)
	b2, err := ParseCertificateBundle(certPEM, keyPEM)
	require.NoError(t, err)
	require.Equal(t, b1.Fingerprint, b2.Fingerprint)
}

func TestParseCertificateBundle_PKCS8Key(t *testing.T) {
	// Generate an RSA key and encode the private key as PKCS8 (PEM block type "PRIVATE KEY").
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "generate RSA key")

	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "pkcs8-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err, "create certificate")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	pkcs8DER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err, "marshal PKCS8 key")
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8DER})

	b, err := ParseCertificateBundle(certPEM, keyPEM)
	require.NoError(t, err)
	require.NotNil(t, b.Cert)
	require.NotNil(t, b.Key)
	require.NotEmpty(t, b.Fingerprint)
}

func TestParseCertificateBundle_RejectsNonRSA(t *testing.T) {
	// Build an RSA cert so the certificate block is valid, but pair it with an ECDSA PKCS8 key.
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "generate RSA key")

	template := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "ec-key-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &rsaKey.PublicKey, rsaKey)
	require.NoError(t, err, "create certificate")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "generate ECDSA key")
	ecDER, err := x509.MarshalPKCS8PrivateKey(ecKey)
	require.NoError(t, err, "marshal ECDSA PKCS8")
	ecKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ecDER})

	_, err = ParseCertificateBundle(certPEM, ecKeyPEM)
	require.Error(t, err)
}
