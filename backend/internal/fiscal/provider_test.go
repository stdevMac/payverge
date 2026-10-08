package fiscal

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeProvider struct {
	country string
	name    string
}

func (f fakeProvider) Country() string { return f.country }
func (f fakeProvider) Name() string    { return f.name }
func (f fakeProvider) ValidateSettings(context.Context, Settings) error {
	return nil
}
func (f fakeProvider) IssueReceipt(context.Context, IssueInput) (*ReceiptResult, error) {
	return &ReceiptResult{Status: StatusAuthorized}, nil
}
func (f fakeProvider) IssueCreditNote(context.Context, CreditNoteInput) (*ReceiptResult, error) {
	return &ReceiptResult{Status: StatusAuthorized}, nil
}
func (f fakeProvider) GetStatus(context.Context, string) (*ReceiptStatus, error) {
	return &ReceiptStatus{Status: StatusAuthorized}, nil
}

func TestProviderRegistryLookup(t *testing.T) {
	reg := NewProviderRegistry()
	reg.Register(fakeProvider{country: "AR", name: "arca"})

	got, ok := reg.Get(" ar ", " ARCA ")
	require.True(t, ok)
	require.Equal(t, "AR", got.Country())
	require.Equal(t, "arca", got.Name())
}

func TestProviderRegistryZeroValueRegisterAndGet(t *testing.T) {
	var reg ProviderRegistry

	reg.Register(fakeProvider{country: "AR", name: "arca"})

	got, ok := reg.Get("ar", "arca")
	require.True(t, ok)
	require.Equal(t, "AR", got.Country())
	require.Equal(t, "arca", got.Name())
}

func TestProviderRegistryNilProviderRegistration(t *testing.T) {
	var reg ProviderRegistry

	require.NotPanics(t, func() {
		reg.Register(nil)
	})

	got, ok := reg.Get("AR", "arca")
	require.False(t, ok)
	require.Nil(t, got)
}

func TestProviderRegistryMissingLookup(t *testing.T) {
	reg := NewProviderRegistry()

	got, ok := reg.Get("AR", "arca")
	require.False(t, ok)
	require.Nil(t, got)
}

func TestSettingsDoesNotSerializeRawCredentials(t *testing.T) {
	settings := Settings{
		CredentialsEncrypted: []byte("secret"),
	}

	payload, err := json.Marshal(settings)
	require.NoError(t, err)

	require.NotContains(t, string(payload), "CredentialsEncrypted")
	require.NotContains(t, strings.ToLower(string(payload)), "secret")
}
