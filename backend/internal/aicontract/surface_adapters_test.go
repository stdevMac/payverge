package aicontract

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/agents/ops_guides"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/pii"
	"github.com/stretchr/testify/require"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// These adapters exercise real production packages (privacy policy, pii redactor,
// ops guide catalog mutation gate, DB permanent delete) with a scripted provider
// boundary — not live models.

func TestAdapter_DirectorHostile_PrivacyClassRequiresZDR(t *testing.T) {
	scenarios, err := LoadScenarios(filepath.Join("testdata", "scenarios.yaml"))
	require.NoError(t, err)
	var sc *Scenario
	for i := range scenarios {
		if scenarios[i].ID == "director-hostile-blocked-before-context" {
			sc = &scenarios[i]
			break
		}
	}
	require.NotNil(t, sc)
	require.True(t, sc.Expect.ZDR != nil && *sc.Expect.ZDR)
	// Real privacy policy: director traffic always requires ZDR.
	req, err := llm.NewGenerateRequest("director")
	require.NoError(t, err)
	require.True(t, req.RequiresZDR())
	require.Equal(t, llm.PrivacyBusinessConfidential, req.PrivacyClass)
	// Hostile scenarios must not allow provider spend (expect.effects provider_calls=0).
	require.Equal(t, 0, sc.Expect.Effects["provider_calls"])
	require.Equal(t, 0, sc.Expect.Effects["context_load"])
}

func TestAdapter_ConciergePIIRedaction_RealRedactor(t *testing.T) {
	// Real pii.Redact path used when transcripts are stored.
	out := pii.Redact("email me at owner@example.com or phone +15551234567")
	require.Contains(t, out, "[redacted-email]")
	require.NotContains(t, out, "owner@example.com")
	require.Contains(t, out, "[redacted-phone]")
}

func TestAdapter_OpsNoMutationCatalogGate(t *testing.T) {
	// Real catalog package: destinations trusted; copy never claims restaurant mutations.
	c := ops_guides.NewDefaultCatalog()
	require.NoError(t, c.ValidateCatalog())
	// The hermetic scenario pins no_operational_mutation.
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

func TestAdapter_WaiterInvalidTool_ScriptedProviderAndPrivacy(t *testing.T) {
	scenarios, err := LoadScenarios(filepath.Join("testdata", "scenarios.yaml"))
	require.NoError(t, err)
	var sc *Scenario
	for i := range scenarios {
		if scenarios[i].ID == "waiter-invalid-item-tool-dropped-everywhere" {
			sc = &scenarios[i]
			break
		}
	}
	require.NotNil(t, sc)
	p := NewScriptedProvider(sc.ProviderScript)
	resp, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "waiter",
		Model:    "google/gemini-2.5-flash",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "add secret"}},
		Tools:    []llm.Tool{{Name: "add_to_cart"}},
	})
	require.NoError(t, err)
	require.Len(t, resp.ToolCalls, 1)
	require.Equal(t, "add_to_cart", resp.ToolCalls[0].Name)
	require.True(t, p.LastRequests[0].RequiresZDR())
	require.Equal(t, llm.PrivacyCustomerSensitive, p.LastRequests[0].PrivacyClass)
	// Production contract: invalid IDs must not become cart effects.
	require.Equal(t, 0, sc.Expect.Effects["cart_add"])
}

func TestAdapter_PermanentDelete_RealDBPath(t *testing.T) {
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(
		&database.OpsAssistantThread{},
		&database.OpsAssistantMessage{},
		&database.OpsAssistantToolCall{},
		&database.OpsAssistantRequest{},
		&database.DirectorConsoleThread{},
		&database.DirectorConsoleMessage{},
		&database.DirectorToolCall{},
		&database.DirectorProposedAction{},
		&database.DirectorActionAudit{},
	))
	database.SetTestDB(gormDB)

	dir, err := database.CreateDirectorConsoleThread(7, "strategy", "en")
	require.NoError(t, err)
	require.NoError(t, database.PermanentlyDeleteDirectorConsoleThread(7, dir.ID))
	_, err = database.GetDirectorConsoleThreadByID(7, dir.ID)
	require.Error(t, err)
}
