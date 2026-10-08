package activation

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDashboardContractsSeparateConsentQualifiedAndServerDenominators(t *testing.T) {
	contracts := DashboardContracts()
	require.Len(t, contracts, 7)
	for _, contract := range contracts {
		require.NotEmpty(t, contract.ID)
		require.NotEmpty(t, contract.Title)
		require.Equal(t, DashboardDesiredOnly, contract.Status)
		require.NotEmpty(t, contract.Provider)
		for _, event := range contract.Events {
			definition := MustDefinition(event)
			require.Equal(t, contract.Denominator, definition.Denominator,
				"dashboard %s must not mix consent-qualified client events with server denominators", contract.ID)
		}
	}
	require.Equal(t, ConsentQualifiedClient, contracts[1].Denominator)
	require.Equal(t, ServerAllEligible, contracts[2].Denominator)
	require.Equal(t, DashboardProviderPostHog, contracts[0].Provider)
	require.Contains(t, contracts[0].Breakdowns, "acquisition_source")
	require.Equal(t, DashboardProviderPostgres, contracts[5].Provider)
	require.NotEmpty(t, contracts[5].Query)
	require.Contains(t, contracts[6].Query, "COUNT(DISTINCT activity.business_id)")
}

func TestExportDashboardPlanIsDesiredOnly(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, ExportDashboardPlan(&output))

	var plan DashboardPlan
	require.NoError(t, json.Unmarshal(output.Bytes(), &plan))
	require.Equal(t, 1, plan.SchemaVersion)
	require.Equal(t, DashboardDesiredOnly, plan.Status)
	require.Len(t, plan.Contracts, 7)
	for _, contract := range plan.Contracts {
		require.Equal(t, DashboardDesiredOnly, contract.Status)
	}
}
