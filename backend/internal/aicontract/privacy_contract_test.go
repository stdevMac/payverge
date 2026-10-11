package aicontract

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stretchr/testify/require"
)

// TestHermeticScenarios_PrivacyAndScriptedProvider runs the privacy-sensitive
// scenario through the scripted provider, asserting ZDR policy application.
func TestHermeticScenarios_PrivacyAndScriptedProvider(t *testing.T) {
	scenarios, err := LoadScenarios(filepath.Join("testdata", "scenarios.yaml"))
	require.NoError(t, err)

	var privacy *Scenario
	for i := range scenarios {
		if scenarios[i].ID == "privacy-sensitive-requires-zdr" {
			privacy = &scenarios[i]
			break
		}
	}
	require.NotNil(t, privacy)

	p := NewScriptedProvider(privacy.ProviderScript)
	resp, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "waiter",
		Model:    "google/gemini-2.5-flash",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "hello"}},
	})
	require.NoError(t, err)
	require.Equal(t, privacy.Expect.ServedModel, resp.Model)
	require.Equal(t, privacy.Expect.ProviderCalls, p.CallCount())
	require.Equal(t, 0, p.Remaining())
	require.Equal(t, llm.PrivacyCustomerSensitive, p.LastRequests[0].PrivacyClass)
	require.True(t, p.LastRequests[0].RequiresZDR())
}

func TestHermeticScenarios_OpsGuidanceOnlyMarker(t *testing.T) {
	scenarios, err := LoadScenarios(filepath.Join("testdata", "scenarios.yaml"))
	require.NoError(t, err)
	found := false
	for _, s := range scenarios {
		if s.ID == "ops-no-operational-mutation-capability" {
			found = true
			require.True(t, s.Expect.NoOperationalMutation)
			require.Equal(t, 0, s.Expect.Effects["menu_mutation"])
			require.Equal(t, 0, s.Expect.Effects["director_apply"])
		}
	}
	require.True(t, found)
}
