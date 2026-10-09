package activation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCatalogIsExactVersionedAndClassified(t *testing.T) {
	want := []Name{
		RegistrationStarted, RegistrationCompleted, WorkspaceCreated,
		OnboardingStepViewed, OnboardingStepClicked, MenuItemCreated,
		TableCreated, QRPreviewed, PaymentConfigured, StaffInvited,
		SetupCompleted, TestOrderCompleted, ActivationAchieved,
		FirstPaidBill,
	}
	require.Equal(t, 1, SchemaVersion)
	require.Equal(t, want, Names())
	require.Len(t, Names(), 14)
	for _, name := range want {
		def, ok := DefinitionFor(name)
		require.True(t, ok, name)
		require.NotEmpty(t, def.Denominator)
	}
	require.True(t, MustDefinition(FirstPaidBill).ServerAuthoritative)
	require.False(t, MustDefinition(OnboardingStepClicked).ServerAuthoritative)
}

func TestValidateRejectsUnknownVersionPIIAndPaymentData(t *testing.T) {
	valid := Event{
		Name: RegistrationStarted, SchemaVersion: SchemaVersion,
		FunnelID:       "728a70ef-0e4f-49b7-9978-229b7bc2be59",
		IdempotencyKey: "client:728a70ef-0e4f-49b7-9978-229b7bc2be59:registration_started:global",
		Dimensions:     Dimensions{Locale: "es-AR", DeviceClass: "mobile", AcquisitionSource: "organic", AcquisitionCampaign: "launch_2026", ElapsedMS: 1250},
	}
	require.NoError(t, Validate(valid, ClientOrigin))

	cases := []Event{
		{Name: RegistrationStarted, SchemaVersion: 99, FunnelID: valid.FunnelID, IdempotencyKey: valid.IdempotencyKey, Dimensions: valid.Dimensions},
		{Name: Name("made_up"), SchemaVersion: SchemaVersion, FunnelID: valid.FunnelID, IdempotencyKey: valid.IdempotencyKey, Dimensions: valid.Dimensions},
		{Name: RegistrationStarted, SchemaVersion: SchemaVersion, FunnelID: valid.FunnelID, IdempotencyKey: valid.IdempotencyKey, Dimensions: valid.Dimensions, Extra: map[string]any{"email": "owner@example.test"}},
		{Name: RegistrationStarted, SchemaVersion: SchemaVersion, FunnelID: valid.FunnelID, IdempotencyKey: valid.IdempotencyKey, Dimensions: valid.Dimensions, Extra: map[string]any{"card_last4": "4242"}},
		{Name: RegistrationStarted, SchemaVersion: SchemaVersion, FunnelID: valid.FunnelID, IdempotencyKey: valid.IdempotencyKey, Dimensions: Dimensions{Locale: "owner@example.test", DeviceClass: "mobile"}},
		{Name: RegistrationStarted, SchemaVersion: SchemaVersion, FunnelID: valid.FunnelID, IdempotencyKey: "client:tampered", Dimensions: valid.Dimensions},
	}
	for _, event := range cases {
		require.Error(t, Validate(event, ClientOrigin))
	}
	require.Error(t, Validate(Event{Name: FirstPaidBill, SchemaVersion: SchemaVersion, BusinessID: 7, FunnelID: valid.FunnelID, Dimensions: valid.Dimensions}, ClientOrigin))
}
