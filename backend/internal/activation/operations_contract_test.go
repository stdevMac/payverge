package activation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientEventCannotSelfAssertBusinessTenant(t *testing.T) {
	event := Event{
		Name: RegistrationStarted, SchemaVersion: SchemaVersion,
		FunnelID:       "728a70ef-0e4f-49b7-9978-229b7bc2be59",
		IdempotencyKey: "client:728a70ef-0e4f-49b7-9978-229b7bc2be59:registration_started:global",
		Dimensions: Dimensions{
			BusinessID: "999", Locale: "en",
			DeviceClass: "desktop", ElapsedMS: 10,
		},
		OccurredAt: time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC),
	}

	require.Error(t, Validate(event, ClientOrigin),
		"the unauthenticated client endpoint must not attribute events to a tenant")
}

func TestDesiredDashboardPlanCoversEveryLaunchDecision(t *testing.T) {
	contracts := DashboardContracts()
	byID := make(map[string]DashboardContract, len(contracts))
	for _, contract := range contracts {
		byID[contract.ID] = contract
	}

	for _, required := range []string{
		"acquisition", "setup_client", "setup_server", "product_activation",
		"revenue_activation", "support", "retention",
	} {
		require.Contains(t, byID, required)
		require.Equal(t, DashboardDesiredOnly, byID[required].Status)
	}
	require.Contains(t, byID["acquisition"].Breakdowns, "acquisition_source")
	require.Contains(t, byID["support"].Query, "escalations")
	require.Contains(t, byID["retention"].Query, "business_activation_states")
}
