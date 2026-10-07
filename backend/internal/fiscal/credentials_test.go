package fiscal

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncryptDecryptCredentialBundle(t *testing.T) {
	in := CredentialBundle{CertPEM: "C", KeyPEM: "K"}
	enc, err := EncryptCredentialBundle(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecryptCredentialBundle(enc)
	if err != nil {
		t.Fatal(err)
	}
	if out.CertPEM != "C" || out.KeyPEM != "K" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

func TestEncryptCredentialBundle_EmptyPEMsRoundTrip(t *testing.T) {
	enc, err := EncryptCredentialBundle(CredentialBundle{CertPEM: "", KeyPEM: ""})
	require.NoError(t, err)
	out, err := DecryptCredentialBundle(enc)
	require.NoError(t, err)
	require.Equal(t, "", out.CertPEM)
	require.Equal(t, "", out.KeyPEM)
}

func TestDecryptCredentialBundle_InvalidCiphertextErrors(t *testing.T) {
	_, err := DecryptCredentialBundle([]byte("not-a-valid-ciphertext"))
	require.Error(t, err)
}

func TestEncryptDecryptCredentialBundle_RoundTripPreservesAllFields(t *testing.T) {
	in := CredentialBundle{
		CertPEM: "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----",
		KeyPEM:  "-----BEGIN RSA PRIVATE KEY-----\nMIIE...\n-----END RSA PRIVATE KEY-----",
	}
	enc, err := EncryptCredentialBundle(in)
	require.NoError(t, err)
	require.NotEmpty(t, enc)

	out, err := DecryptCredentialBundle(enc)
	require.NoError(t, err)
	require.Equal(t, in.CertPEM, out.CertPEM)
	require.Equal(t, in.KeyPEM, out.KeyPEM)
}
