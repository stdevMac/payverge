package aicontract

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// tagGatedScenarioIDs lists scenarios whose production path is compiled out of
// the current build (scenario ID -> reason). Populated by registerWhatsAppRunners
// in runners_nowhatsapp_test.go; empty in `-tags whatsapp` builds.
var tagGatedScenarioIDs = map[string]string{}

// TestHermeticMatrix_AllScenariosRegistered fails closed if any shipped
// scenario lacks a production-path runner. Add runners in the same commit as
// new YAML scenario IDs.
func TestHermeticMatrix_AllScenariosRegistered(t *testing.T) {
	scenarios, err := LoadScenarios(filepath.Join("testdata", "scenarios.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, scenarios)

	var missing []string
	for _, sc := range scenarios {
		if _, gated := tagGatedScenarioIDs[sc.ID]; gated {
			continue
		}
		if _, ok := GetScenarioRunner(sc.ID); !ok {
			missing = append(missing, sc.ID)
		}
	}
	require.Empty(t, missing, "scenarios without runners: %v (registered=%v)", missing, RegisteredScenarioIDs())
}

// TestHermeticMatrix_RunAll executes every registered runner against its YAML
// contract on real production packages.
func TestHermeticMatrix_RunAll(t *testing.T) {
	scenarios, err := LoadScenarios(filepath.Join("testdata", "scenarios.yaml"))
	require.NoError(t, err)

	for _, sc := range scenarios {
		sc := sc
		t.Run(sc.ID, func(t *testing.T) {
			if why, gated := tagGatedScenarioIDs[sc.ID]; gated {
				t.Skip(why)
			}
			fn, ok := GetScenarioRunner(sc.ID)
			require.True(t, ok, "no runner for %s", sc.ID)
			res, err := fn(sc)
			require.NoError(t, err, "runner error for %s", sc.ID)
			require.NoError(t, MatchExpect(sc, res), "expect mismatch for %s: %+v", sc.ID, res)
		})
	}
}

// TestHermeticMatrix_TagGatedScenariosAreReal keeps the build-tag escape hatch
// honest: a gated ID must name a shipped scenario, carry a reason, and have no
// runner in this build (gating must never hide a runner that exists).
func TestHermeticMatrix_TagGatedScenariosAreReal(t *testing.T) {
	scenarios, err := LoadScenarios(filepath.Join("testdata", "scenarios.yaml"))
	require.NoError(t, err)
	shipped := make(map[string]bool, len(scenarios))
	for _, sc := range scenarios {
		shipped[sc.ID] = true
	}
	for id, why := range tagGatedScenarioIDs {
		require.True(t, shipped[id], "tag-gated scenario %q is not in scenarios.yaml", id)
		require.NotEmpty(t, why, "tag-gated scenario %q needs a reason", id)
		_, registered := GetScenarioRunner(id)
		require.False(t, registered, "scenario %q is tag-gated but has a runner in this build", id)
	}
}
