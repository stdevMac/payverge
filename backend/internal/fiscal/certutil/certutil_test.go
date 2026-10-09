package certutil

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

func newSelfSignedPEM(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "certutil-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM
}

func TestParseBundle_ValidBundle(t *testing.T) {
	certPEM, keyPEM := newSelfSignedPEM(t)
	b, err := ParseBundle(certPEM, keyPEM)
	require.NoError(t, err)
	require.NotNil(t, b.Cert)
	require.NotNil(t, b.Key)
	require.NotEmpty(t, b.Fingerprint)
	require.False(t, b.NotAfter.IsZero())
}

func TestParseBundle_KeyCertMismatch(t *testing.T) {
	certPEM, _ := newSelfSignedPEM(t)
	_, otherKey := newSelfSignedPEM(t)
	_, err := ParseBundle(certPEM, otherKey)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match")
}

func TestParseBundle_PKCS8Key(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "pkcs8-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	pkcs8DER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8DER})

	b, err := ParseBundle(certPEM, keyPEM)
	require.NoError(t, err)
	require.NotNil(t, b.Cert)
	require.NotNil(t, b.Key)
	require.NotEmpty(t, b.Fingerprint)
}

func TestParseBundle_NonRSAKeyRejected(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "ec-reject-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &rsaKey.PublicKey, rsaKey)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ecDER, err := x509.MarshalPKCS8PrivateKey(ecKey)
	require.NoError(t, err)
	ecKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ecDER})

	_, err = ParseBundle(certPEM, ecKeyPEM)
	require.Error(t, err)
}

func TestParseBundle_InvalidCertPEM(t *testing.T) {
	_, keyPEM := newSelfSignedPEM(t)
	_, err := ParseBundle([]byte("not a pem"), keyPEM)
	require.Error(t, err)
	require.Contains(t, err.Error(), "certificate PEM")
}

func TestParseBundle_InvalidKeyPEM(t *testing.T) {
	certPEM, _ := newSelfSignedPEM(t)
	_, err := ParseBundle(certPEM, []byte("not a pem"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "private key PEM")
}
