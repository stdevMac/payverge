package llm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateZDRStartup_EnforceRejectsUnapproved(t *testing.T) {
	cfg := ModelConfig{
		Chat:      "google/gemini-2.5-flash",
		Director:  "google/gemini-2.5-flash",
		Menu:      "google/gemini-2.5-flash",
		Image:     "google/gemini-2.5-flash-image",
		Guardrail: "google/gemini-2.5-flash-lite",
	}
	// Explicit allowlist that omits image + guardrail models.
	err := ValidateZDRStartup(cfg, ZDRModeEnforce, []string{"google/gemini-2.5-flash"})
	require.ErrorIs(t, err, ErrPrivacyPolicy)
	st := PrivacyReadiness()
	require.Equal(t, "failed", st.Status)
}

func TestValidateZDRStartup_EnforceBootstrapsEmptyAllowlist(t *testing.T) {
	cfg := ModelConfig{
		Chat: "m1", Director: "m1", Menu: "m1", Image: "m1", Guardrail: "m1",
	}
	require.NoError(t, ValidateZDRStartup(cfg, ZDRModeEnforce, nil))
	st := PrivacyReadiness()
	require.Equal(t, "healthy", st.Status)
	require.Equal(t, "approved_list_bootstrapped", st.LastError)
}

func TestValidateZDRStartup_EnforceAcceptsFullList(t *testing.T) {
	cfg := ModelConfig{
		Chat:          "google/gemini-2.5-flash",
		Director:      "google/gemini-2.5-flash",
		Menu:          "google/gemini-2.5-flash",
		Image:         "google/gemini-2.5-flash-image",
		Guardrail:     "google/gemini-2.5-flash-lite",
		ChatFallbacks: []string{"openai/gpt-4o-mini"},
	}
	approved := []string{
		"google/gemini-2.5-flash",
		"google/gemini-2.5-flash-image",
		"google/gemini-2.5-flash-lite",
		"openai/gpt-4o-mini",
	}
	require.NoError(t, ValidateZDRStartup(cfg, ZDRModeEnforce, approved))
	st := PrivacyReadiness()
	require.Equal(t, "healthy", st.Status)
	require.Equal(t, ZDRModeEnforce, st.Mode)
}

func TestValidateZDRStartup_AuditDegradesWithoutFailing(t *testing.T) {
	cfg := ModelConfig{Chat: "vendor/secret-model", Director: "vendor/secret-model", Menu: "vendor/secret-model", Image: "vendor/secret-model", Guardrail: "vendor/secret-model"}
	err := ValidateZDRStartup(cfg, ZDRModeAudit, nil)
	require.NoError(t, err)
	st := PrivacyReadiness()
	require.Equal(t, "degraded", st.Status)
}

func TestLoadZDRMode_Invalid(t *testing.T) {
	t.Setenv("OPENROUTER_ZDR_MODE", "off")
	_, err := LoadZDRMode(ZDRModeAudit)
	require.ErrorIs(t, err, ErrPrivacyPolicy)
}
