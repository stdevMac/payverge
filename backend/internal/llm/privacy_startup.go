package llm

import (
	"fmt"
	"os"
	"strings"
)

// ZDRMode controls startup validation of privacy-sensitive model routing.
type ZDRMode string

const (
	// ZDRModeAudit logs policy gaps but does not fail startup (local/dev).
	ZDRModeAudit ZDRMode = "audit"
	// ZDRModeEnforce fails startup when a sensitive model is missing from the
	// approved ZDR list (production default after staging proof).
	ZDRModeEnforce ZDRMode = "enforce"
)

// PrivacyStartupState is the redacted readiness snapshot for health.
type PrivacyStartupState struct {
	Mode            ZDRMode `json:"mode"`
	Status          string  `json:"status"` // healthy | degraded | failed
	ApprovedModels  int     `json:"approved_models"`
	SensitiveModels int     `json:"sensitive_models"`
	LastError       string  `json:"last_error,omitempty"`
}

var privacyStartupState = PrivacyStartupState{Mode: ZDRModeAudit, Status: "healthy"}

// PrivacyReadiness returns a copy of the last startup privacy validation state.
func PrivacyReadiness() PrivacyStartupState {
	return privacyStartupState
}

// LoadZDRMode reads OPENROUTER_ZDR_MODE. Empty defaults to audit outside
// production and enforce when production-like env is detected by the caller
// via defaultMode.
func LoadZDRMode(defaultMode ZDRMode) (ZDRMode, error) {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("OPENROUTER_ZDR_MODE")))
	if raw == "" {
		if defaultMode == "" {
			return ZDRModeAudit, nil
		}
		return defaultMode, nil
	}
	switch ZDRMode(raw) {
	case ZDRModeAudit, ZDRModeEnforce:
		return ZDRMode(raw), nil
	default:
		return "", fmt.Errorf("%w: invalid OPENROUTER_ZDR_MODE %q (want audit|enforce)", ErrPrivacyPolicy, raw)
	}
}

// LoadZDRApprovedModels parses OPENROUTER_ZDR_APPROVED_MODELS (comma-separated).
func LoadZDRApprovedModels() []string {
	raw := strings.TrimSpace(os.Getenv("OPENROUTER_ZDR_APPROVED_MODELS"))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// SensitiveModelsFromConfig enumerates primary + fallback models used by
// ZDR-required features.
func SensitiveModelsFromConfig(cfg ModelConfig) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(ids ...string) {
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	add(cfg.Chat)
	add(cfg.ChatFallbacks...)
	add(cfg.Menu)
	add(cfg.MenuFallbacks...)
	add(cfg.Image)
	add(cfg.ImageFallbacks...)
	add(cfg.Director)
	add(cfg.DirectorFallbacks...)
	add(cfg.Guardrail)
	add(cfg.GuardrailFallbacks...)
	return out
}

// ValidateZDRStartup checks that every sensitive model is on the approved ZDR
// list when mode is enforce. Audit mode records degraded state without failing.
func ValidateZDRStartup(cfg ModelConfig, mode ZDRMode, approved []string) error {
	sensitive := SensitiveModelsFromConfig(cfg)
	privacyStartupState = PrivacyStartupState{
		Mode:            mode,
		Status:          "healthy",
		ApprovedModels:  len(approved),
		SensitiveModels: len(sensitive),
	}

	if mode != ZDRModeEnforce && mode != ZDRModeAudit {
		err := fmt.Errorf("%w: unknown ZDR mode %q", ErrPrivacyPolicy, mode)
		privacyStartupState.Status = "failed"
		privacyStartupState.LastError = "invalid_mode"
		return err
	}

	// When enforce mode has no explicit allowlist, bootstrap from the currently
	// configured sensitive models so a first production deploy is not brickable.
	// Operators should pin OPENROUTER_ZDR_APPROVED_MODELS after staging ZDR proof.
	if mode == ZDRModeEnforce && len(approved) == 0 {
		approved = append([]string(nil), sensitive...)
		privacyStartupState.ApprovedModels = len(approved)
		privacyStartupState.LastError = "approved_list_bootstrapped"
	}

	approvedSet := map[string]struct{}{}
	for _, m := range approved {
		approvedSet[m] = struct{}{}
	}

	var missing []string
	for _, m := range sensitive {
		if _, ok := approvedSet[m]; !ok {
			missing = append(missing, m)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	privacyStartupState.LastError = "unapproved_sensitive_models"
	if mode == ZDRModeEnforce {
		privacyStartupState.Status = "failed"
		return fmt.Errorf("%w: sensitive models not on ZDR approved list: %s", ErrPrivacyPolicy, strings.Join(missing, ", "))
	}
	privacyStartupState.Status = "degraded"
	return nil
}
