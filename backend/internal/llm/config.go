// Package llm config: per-call-site model IDs, fallback lists, and per-feature
// request timeouts, loaded from the environment. Defaults keep the current
// Gemini models, addressed by their OpenRouter IDs.
//
// Model environment variables (all optional; defaults in parentheses):
//
// OPENROUTER_MODEL_CHAT       AI waiter + WhatsApp        (google/gemini-2.5-flash)
// OPENROUTER_MODEL_MENU       extraction, wizard, menu-gen (google/gemini-2.5-flash)
// OPENROUTER_MODEL_IMAGE      image generation            (google/gemini-2.5-flash-image)
// OPENROUTER_MODEL_DIRECTOR   director console loop        (google/gemini-2.5-flash)
//
//	OPENROUTER_MODEL_GUARDRAIL  input classifier (Lane E)    (google/gemini-2.5-flash-lite)
//
// Fallback lists (comma-separated OpenRouter IDs, tried after the primary):
//
//	OPENROUTER_CHAT_FALLBACKS, OPENROUTER_MENU_FALLBACKS, OPENROUTER_IMAGE_FALLBACKS,
//	OPENROUTER_DIRECTOR_FALLBACKS, OPENROUTER_GUARDRAIL_FALLBACKS
//
// Per-feature timeouts: callers wrap their context with FeatureTimeout(feature)
// before calling Provider.Generate so latency budgets are consistent across the
// codebase. Feature keys match GenerateRequest.Feature / CallInfo.Feature.
package llm

import (
	"os"
	"strings"
	"time"
)

// ModelConfig holds the per-call-site model IDs and optional fallback lists,
// loaded from environment variables. Defaults keep the current Gemini models,
// addressed by their OpenRouter IDs.
type ModelConfig struct {
	Chat      string // AI waiter + WhatsApp
	Menu      string // extraction, wizard, menu-gen
	Image     string // image generation
	Director  string // director console loop
	Guardrail string // input classifier (Lane E)

	ChatFallbacks      []string
	MenuFallbacks      []string
	ImageFallbacks     []string
	DirectorFallbacks  []string
	GuardrailFallbacks []string
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envList(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
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

// LoadModelConfig reads model IDs and fallbacks from the environment.
func LoadModelConfig() ModelConfig {
	return ModelConfig{
		Chat:      envOr("OPENROUTER_MODEL_CHAT", "google/gemini-2.5-flash"),
		Menu:      envOr("OPENROUTER_MODEL_MENU", "google/gemini-2.5-flash"),
		Image:     envOr("OPENROUTER_MODEL_IMAGE", "google/gemini-2.5-flash-image"),
		Director:  envOr("OPENROUTER_MODEL_DIRECTOR", "google/gemini-2.5-flash"),
		Guardrail: envOr("OPENROUTER_MODEL_GUARDRAIL", "google/gemini-2.5-flash-lite"),

		ChatFallbacks:      envList("OPENROUTER_CHAT_FALLBACKS"),
		MenuFallbacks:      envList("OPENROUTER_MENU_FALLBACKS"),
		ImageFallbacks:     envList("OPENROUTER_IMAGE_FALLBACKS"),
		DirectorFallbacks:  envList("OPENROUTER_DIRECTOR_FALLBACKS"),
		GuardrailFallbacks: envList("OPENROUTER_GUARDRAIL_FALLBACKS"),
	}
}

// DefaultFeatureTimeout bounds any feature not explicitly listed in featureTimeouts.
const DefaultFeatureTimeout = 60 * time.Second

// featureTimeouts is the per-feature context ceiling caller lanes apply via
// context.WithTimeout(ctx, FeatureTimeout(feature)). Keys match
// GenerateRequest.Feature. Guardrail (2s) is the outer ceiling; Lane E's
// classifier also applies its own internal 2s ctx timeout (Contract C2).
var featureTimeouts = map[string]time.Duration{
	"waiter":          20 * time.Second,
	"waiter_whatsapp": 20 * time.Second,
	"director":        35 * time.Second,
	"wizard":          30 * time.Second,
	"extraction":      60 * time.Second,
	"image":           60 * time.Second,
	"guardrail":       2 * time.Second,
	"ops_assistant":   30 * time.Second,
	"marketing":       30 * time.Second,
}

// FeatureTimeout returns the request timeout for a feature, or
// DefaultFeatureTimeout for unknown features.
func FeatureTimeout(feature string) time.Duration {
	if d, ok := featureTimeouts[feature]; ok {
		return d
	}
	return DefaultFeatureTimeout
}
