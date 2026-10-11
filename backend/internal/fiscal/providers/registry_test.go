package providers_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal"
	"github.com/stdevmac/payverge/backend/internal/fiscal/providers"
	"github.com/stdevmac/payverge/backend/internal/security"

	"github.com/stretchr/testify/require"
)

// selfSignedPEMForRegistry generates a minimal RSA self-signed cert+key pair
// for use in registry tests.
func selfSignedPEMForRegistry(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "registry-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return
}

// encryptBundleForTest encrypts a CertPEM/KeyPEM pair as a fiscal.CredentialBundle
// using the security package (uses the local-dev fallback when PLUGIN_SECRET_KEY is unset).
func encryptBundleForTest(t *testing.T, certPEM, keyPEM []byte) []byte {
	t.Helper()
	raw, err := json.Marshal(fiscal.CredentialBundle{
		CertPEM: string(certPEM),
		KeyPEM:  string(keyPEM),
	})
	require.NoError(t, err)
	enc, err := security.EncryptSecret(string(raw))
	require.NoError(t, err)
	return []byte(enc)
}

func TestCredentialAwareFactory_BuildAR(t *testing.T) {
	certPEM, keyPEM := selfSignedPEMForRegistry(t)
	encrypted := encryptBundleForTest(t, certPEM, keyPEM)

	taxID := "20-12345678-9"
	settings := &database.BusinessFiscalSettings{
		Country:              "AR",
		Provider:             "arca",
		Environment:          "sandbox",
		TaxID:                taxID,
		CredentialsEncrypted: encrypted,
	}

	f := providers.NewCredentialAwareFactory()
	p, err := f.Build(context.Background(), settings)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.Equal(t, "AR", p.Country())
	require.Equal(t, "arca", p.Name())
}

func TestCredentialAwareFactory_BuildAR_CaseInsensitive(t *testing.T) {
	certPEM, keyPEM := selfSignedPEMForRegistry(t)
	encrypted := encryptBundleForTest(t, certPEM, keyPEM)

	settings := &database.BusinessFiscalSettings{
		Country:              "ar",
		Provider:             "ARCA",
		Environment:          "sandbox",
		TaxID:                "20123456789",
		CredentialsEncrypted: encrypted,
	}

	f := providers.NewCredentialAwareFactory()
	p, err := f.Build(context.Background(), settings)
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestCredentialAwareFactory_BuildAR_MissingCredentials(t *testing.T) {
	settings := &database.BusinessFiscalSettings{
		Country:              "AR",
		Provider:             "arca",
		Environment:          "sandbox",
		TaxID:                "20123456789",
		CredentialsEncrypted: nil,
	}

	f := providers.NewCredentialAwareFactory()
	_, err := f.Build(context.Background(), settings)
	require.Error(t, err)
}

func TestCredentialAwareFactory_BuildAR_EmptyCredentials(t *testing.T) {
	settings := &database.BusinessFiscalSettings{
		Country:              "AR",
		Provider:             "arca",
		Environment:          "sandbox",
		TaxID:                "20123456789",
		CredentialsEncrypted: []byte{},
	}

	f := providers.NewCredentialAwareFactory()
	_, err := f.Build(context.Background(), settings)
	require.Error(t, err)
}

func TestCredentialAwareFactory_BuildUnknownCountry(t *testing.T) {
	// Use well-formed (but throwaway) credentials so the decrypt step succeeds
	// and we reach the country dispatch, which should return an unsupported-combo error.
	certPEM, keyPEM := selfSignedPEMForRegistry(t)
	encrypted := encryptBundleForTest(t, certPEM, keyPEM)

	settings := &database.BusinessFiscalSettings{
		Country:              "BR",
		Provider:             "nfse",
		Environment:          "sandbox",
		TaxID:                "12345678901234",
		CredentialsEncrypted: encrypted,
	}

	f := providers.NewCredentialAwareFactory()
	_, err := f.Build(context.Background(), settings)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "BR") || strings.Contains(err.Error(), "unsupported") || strings.Contains(err.Error(), "unknown"),
		"error should mention the unsupported country/provider: %s", err)
}

func TestCredentialAwareFactory_ImplementsProviderFactory(t *testing.T) {
	// Compile-time check: *CredentialAwareFactory must satisfy fiscal.ProviderFactory.
	var _ fiscal.ProviderFactory = providers.NewCredentialAwareFactory()
}
