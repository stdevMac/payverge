package aicontract

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Known production AI surfaces for scenario gating.
var knownSurfaces = map[string]struct{}{
	"waiter":          {},
	"waiter_whatsapp": {},
	"ops_assistant":   {},
	"director":        {},
	"menu":            {},
	"privacy":         {},
}

// Scenario is one hermetic production regression case.
type Scenario struct {
	ID             string          `yaml:"id"`
	Surface        string          `yaml:"surface"`
	Locale         string          `yaml:"locale"`
	Input          map[string]any  `yaml:"input"`
	ProviderScript []ProviderStep  `yaml:"provider_script"`
	Expect         ExpectedOutcome `yaml:"expect"`
}

// ProviderStep is one scripted model call expectation and response.
type ProviderStep struct {
	Feature      string         `yaml:"feature"`
	PrivacyClass string         `yaml:"privacy_class"`
	RequireZDR   *bool          `yaml:"require_zdr"`
	Model        string         `yaml:"model"`
	Fallbacks    []string       `yaml:"fallbacks"`
	HasTools     *bool          `yaml:"has_tools"`
	HasSchema    *bool          `yaml:"has_schema"`
	PromptMarks  []string       `yaml:"prompt_marks"`
	ResponseText string         `yaml:"response_text"`
	ServedModel  string         `yaml:"served_model"`
	Error        string         `yaml:"error"`
	ToolCalls    []ScriptedTool `yaml:"tool_calls"`
}

// ScriptedTool is a tool call returned by the fake provider.
type ScriptedTool struct {
	Name string         `yaml:"name"`
	Args map[string]any `yaml:"args"`
}

// ExpectedOutcome is the deterministic terminal contract for a scenario.
type ExpectedOutcome struct {
	HTTPStatus    int            `yaml:"http_status"`
	Code          string         `yaml:"code"`
	TextContains  []string       `yaml:"text_contains"`
	TextExcludes  []string       `yaml:"text_excludes"`
	ActionKinds   []string       `yaml:"action_kinds"`
	Effects       map[string]int `yaml:"effects"`
	ProviderCalls int            `yaml:"provider_calls"`
	ServedModel   string         `yaml:"served_model"`
	PrivacyClass  string         `yaml:"privacy_class"`
	ZDR           *bool          `yaml:"zdr"`
	// NoOperationalMutation asserts Ops remains guidance-only (Wave 3/6).
	NoOperationalMutation bool `yaml:"no_operational_mutation"`
	// AssistantV2 enables the production-shaped V2 invariant gate for this
	// scenario. The allowlists are exact server-owned identifiers, not model
	// supplied labels or URLs.
	AssistantV2 *AssistantV2Expected `yaml:"assistant_v2"`
}

// AssistantV2Expected describes trusted inputs against which a real V2
// finalizer result is checked. Empty, non-nil allowlists explicitly require
// the corresponding response collection to be empty.
type AssistantV2Expected struct {
	RolloutEnabled              bool     `yaml:"rollout_enabled"`
	AllowedActionTargets        []string `yaml:"allowed_action_targets"`
	TrustedSources              []string `yaml:"trusted_sources"`
	CanonicalEntities           []string `yaml:"canonical_entities"`
	LocaleContains              []string `yaml:"locale_contains"`
	LocaleExcludes              []string `yaml:"locale_excludes"`
	ExactAnswer                 string   `yaml:"exact_answer"`
	DeterministicLocaleFallback bool     `yaml:"deterministic_locale_fallback"`
	RequireReadableV1           bool     `yaml:"require_readable_v1"`
	NoUnavailableWaiterAction   bool     `yaml:"no_unavailable_waiter_action"`
	RequiredZeroEffects         []string `yaml:"required_zero_effects"`
	ExactProviderCalls          *int     `yaml:"exact_provider_calls"`
}

// ScenarioFile is the on-disk YAML envelope.
type ScenarioFile struct {
	Scenarios []Scenario `yaml:"scenarios"`
}

var (
	emailLike = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	phoneLike = regexp.MustCompile(`\+?\d[\d\-\s()]{8,}\d`)
	jidLike   = regexp.MustCompile(`\d{8,}@s\.whatsapp\.net`)
)

// LoadScenarios reads and validates a YAML scenario file.
func LoadScenarios(path string) ([]Scenario, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file ScenarioFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := ValidateScenarios(file.Scenarios); err != nil {
		return nil, err
	}
	return file.Scenarios, nil
}

// ValidateScenarios enforces the hermetic scenario contract.
func ValidateScenarios(scenarios []Scenario) error {
	seen := map[string]struct{}{}
	for i, s := range scenarios {
		if strings.TrimSpace(s.ID) == "" {
			return fmt.Errorf("scenario[%d]: id is required", i)
		}
		if _, dup := seen[s.ID]; dup {
			return fmt.Errorf("duplicate scenario id %q", s.ID)
		}
		seen[s.ID] = struct{}{}
		if _, ok := knownSurfaces[s.Surface]; !ok {
			return fmt.Errorf("scenario %q: unknown surface %q", s.ID, s.Surface)
		}
		if strings.TrimSpace(s.Locale) == "" {
			return fmt.Errorf("scenario %q: locale is required", s.ID)
		}
		if err := validateExpect(s); err != nil {
			return err
		}
		if err := rejectRealPII(s); err != nil {
			return err
		}
		if requiresZDRSurface(s.Surface) {
			if s.Expect.ZDR == nil || !*s.Expect.ZDR {
				return fmt.Errorf("scenario %q: sensitive surface %q requires expect.zdr=true", s.ID, s.Surface)
			}
			for j, step := range s.ProviderScript {
				if step.RequireZDR != nil && !*step.RequireZDR {
					return fmt.Errorf("scenario %q step %d: cannot disable ZDR for sensitive surface", s.ID, j)
				}
			}
		}
	}
	return nil
}

func validateExpect(s Scenario) error {
	e := s.Expect
	hasTerminal := e.HTTPStatus != 0 || e.Code != "" || len(e.TextContains) > 0 ||
		len(e.Effects) > 0 || e.NoOperationalMutation || e.ProviderCalls > 0
	if !hasTerminal {
		return fmt.Errorf("scenario %q: missing terminal expectation", s.ID)
	}
	if v2 := e.AssistantV2; v2 != nil {
		if !v2.RolloutEnabled {
			return fmt.Errorf("scenario %q: assistant_v2 rollout_enabled must be true", s.ID)
		}
		if v2.AllowedActionTargets == nil || v2.TrustedSources == nil || v2.CanonicalEntities == nil {
			return fmt.Errorf("scenario %q: assistant_v2 trust allowlists must be explicit arrays", s.ID)
		}
		if !v2.RequireReadableV1 {
			return fmt.Errorf("scenario %q: assistant_v2 require_readable_v1 must be true", s.ID)
		}
		if len(v2.LocaleContains) == 0 && !v2.DeterministicLocaleFallback {
			return fmt.Errorf("scenario %q: assistant_v2 locale expectation is required", s.ID)
		}
		if v2.DeterministicLocaleFallback && strings.TrimSpace(v2.ExactAnswer) == "" {
			return fmt.Errorf("scenario %q: assistant_v2 deterministic fallback requires exact_answer", s.ID)
		}
		for _, key := range v2.RequiredZeroEffects {
			if strings.TrimSpace(key) == "" {
				return fmt.Errorf("scenario %q: assistant_v2 required_zero_effects cannot contain blanks", s.ID)
			}
		}
	}
	return nil
}

func requiresZDRSurface(surface string) bool {
	switch surface {
	case "waiter", "waiter_whatsapp", "ops_assistant", "director", "menu":
		return true
	default:
		return false
	}
}

func rejectRealPII(s Scenario) error {
	blob := fmt.Sprintf("%v %v %v", s.Input, s.ProviderScript, s.Expect)
	// JIDs first — the local part can look email-like to a naive regex.
	if jidLike.MatchString(blob) {
		return fmt.Errorf("scenario %q: must not embed raw WhatsApp JIDs", s.ID)
	}
	if m := emailLike.FindString(blob); m != "" {
		if strings.Contains(m, "example.com") || strings.Contains(m, "example.") ||
			strings.Contains(m, "fixture.test") || strings.Contains(m, "[redacted-email]") {
			return nil
		}
		return fmt.Errorf("scenario %q: appears to embed a real email %q", s.ID, m)
	}
	_ = phoneLike // phones in fixtures may look real; prefer placeholders in docs
	return nil
}
