package aicontract

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateScenarios_RejectsDuplicateIDs(t *testing.T) {
	zdr := true
	err := ValidateScenarios([]Scenario{
		{ID: "a", Surface: "waiter", Locale: "en", Expect: ExpectedOutcome{Code: "ok", ZDR: &zdr}},
		{ID: "a", Surface: "waiter", Locale: "en", Expect: ExpectedOutcome{Code: "ok", ZDR: &zdr}},
	})
	require.Error(t, err)
}

func TestValidateScenarios_SensitiveRequiresZDR(t *testing.T) {
	err := ValidateScenarios([]Scenario{
		{ID: "w1", Surface: "waiter", Locale: "en", Expect: ExpectedOutcome{Code: "ok"}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "zdr")
}

func TestValidateScenarios_RejectsRawJID(t *testing.T) {
	zdr := true
	err := ValidateScenarios([]Scenario{
		{
			ID: "wa1", Surface: "waiter_whatsapp", Locale: "en",
			Input:  map[string]any{"from": "5491112345678@s.whatsapp.net"},
			Expect: ExpectedOutcome{Code: "ok", ZDR: &zdr},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "JID")
}

func TestLoadScenarios_Testdata(t *testing.T) {
	path := filepath.Join("testdata", "scenarios.yaml")
	scenarios, err := LoadScenarios(path)
	require.NoError(t, err)
	require.NotEmpty(t, scenarios)
	ids := map[string]struct{}{}
	for _, s := range scenarios {
		_, dup := ids[s.ID]
		require.False(t, dup, s.ID)
		ids[s.ID] = struct{}{}
	}
	require.Contains(t, ids, "ops-no-operational-mutation-capability")
	require.Contains(t, ids, "privacy-sensitive-requires-zdr")
}
