package ar

import (
	"context"
	"errors"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/fiscal"

	"github.com/stretchr/testify/require"
)

func validSettings() fiscal.Settings {
	point := 1
	return fiscal.Settings{
		TaxID:       "20123456789",
		PointOfSale: &point,
	}
}

func TestProviderValidateSettings_Succeeds(t *testing.T) {
	client := &fakeWSFEClient{}
	provider := NewProvider(client, 20111111112)

	err := provider.ValidateSettings(context.Background(), validSettings())
	require.NoError(t, err)
	require.Equal(t, 1, client.dummyHit, "FEDummy must be called once")
	require.Equal(t, 1, client.authHit, "WSAA authenticate must be called once")
}

func TestProviderValidateSettings_AcceptsFormattedCUIT(t *testing.T) {
	client := &fakeWSFEClient{}
	provider := NewProvider(client, 20111111112)
	s := validSettings()
	s.TaxID = "20-12345678-9" // dashes stripped to 11 digits

	err := provider.ValidateSettings(context.Background(), s)
	require.NoError(t, err)
}

func TestProviderValidateSettings_RejectsMissingCUIT(t *testing.T) {
	client := &fakeWSFEClient{}
	provider := NewProvider(client, 20111111112)
	s := validSettings()
	s.TaxID = ""

	err := provider.ValidateSettings(context.Background(), s)
	require.Error(t, err)
	require.Contains(t, err.Error(), "CUIT")
	require.Zero(t, client.dummyHit, "must not call the network on a static validation failure")
}

func TestProviderValidateSettings_RejectsWrongLengthCUIT(t *testing.T) {
	client := &fakeWSFEClient{}
	provider := NewProvider(client, 20111111112)
	s := validSettings()
	s.TaxID = "12345" // not 11 digits

	err := provider.ValidateSettings(context.Background(), s)
	require.Error(t, err)
	require.Contains(t, err.Error(), "CUIT")
	require.Zero(t, client.dummyHit)
}

func TestProviderValidateSettings_RejectsNilPointOfSale(t *testing.T) {
	client := &fakeWSFEClient{}
	provider := NewProvider(client, 20111111112)
	s := validSettings()
	s.PointOfSale = nil

	err := provider.ValidateSettings(context.Background(), s)
	require.Error(t, err)
	require.Contains(t, err.Error(), "point of sale")
	require.Zero(t, client.dummyHit)
}

func TestProviderValidateSettings_RejectsNonPositivePointOfSale(t *testing.T) {
	client := &fakeWSFEClient{}
	provider := NewProvider(client, 20111111112)
	s := validSettings()
	zero := 0
	s.PointOfSale = &zero

	err := provider.ValidateSettings(context.Background(), s)
	require.Error(t, err)
	require.Contains(t, err.Error(), "point of sale")
	require.Zero(t, client.dummyHit)
}

func TestProviderValidateSettings_NilClientErrors(t *testing.T) {
	provider := NewProvider(nil, 20111111112)

	err := provider.ValidateSettings(context.Background(), validSettings())
	require.Error(t, err)
	require.Contains(t, err.Error(), "wsfe client")
}

func TestProviderValidateSettings_DummyErrorPropagates(t *testing.T) {
	expected := errors.New("AppServer down")
	client := &fakeWSFEClient{dummyErr: expected}
	provider := NewProvider(client, 20111111112)

	err := provider.ValidateSettings(context.Background(), validSettings())
	require.ErrorIs(t, err, expected)
	require.Zero(t, client.authHit, "must not authenticate after a failed FEDummy")
}

func TestProviderValidateSettings_AuthErrorPropagates(t *testing.T) {
	expected := errors.New("wsaa login failed")
	client := &fakeWSFEClient{authErr: expected}
	provider := NewProvider(client, 20111111112)

	err := provider.ValidateSettings(context.Background(), validSettings())
	require.ErrorIs(t, err, expected)
	require.Equal(t, 1, client.dummyHit, "FEDummy must run before the auth round-trip")
}
