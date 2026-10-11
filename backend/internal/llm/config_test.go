package llm

// NOTE: config_test.go hosts pure unit tests for LoadModelConfig and fallbacks.
// Compose / .env.example parity for OPENROUTER_MODEL_* defaults lives in
// openrouter_model_defaults_parity_test.go so FIND-027 cannot regress via
// stale docker-compose defaults while Go defaults stay current.

import (
	"reflect"
	"testing"
	"time"
)

func TestLoadModelConfigDefaults(t *testing.T) {
	t.Setenv("OPENROUTER_MODEL_CHAT", "")
	t.Setenv("OPENROUTER_MODEL_DIRECTOR", "")
	t.Setenv("OPENROUTER_CHAT_FALLBACKS", "")
	cfg := LoadModelConfig()
	if cfg.Chat != "google/gemini-2.5-flash" {
		t.Fatalf("default chat model = %q, want google/gemini-2.5-flash", cfg.Chat)
	}
	if cfg.Director != "google/gemini-2.5-flash" {
		t.Fatalf("default director model = %q, want google/gemini-2.5-flash", cfg.Director)
	}
	if cfg.Image != "google/gemini-2.5-flash-image" {
		t.Fatalf("default image model = %q", cfg.Image)
	}
	if cfg.ChatFallbacks != nil {
		t.Fatalf("expected nil fallbacks, got %v", cfg.ChatFallbacks)
	}
}

func TestLoadModelConfigOverrides(t *testing.T) {
	t.Setenv("OPENROUTER_MODEL_CHAT", "anthropic/claude-haiku-4.5")
	t.Setenv("OPENROUTER_CHAT_FALLBACKS", "google/gemini-2.0-flash-001, openai/gpt-4o-mini ")
	cfg := LoadModelConfig()
	if cfg.Chat != "anthropic/claude-haiku-4.5" {
		t.Fatalf("override chat model = %q", cfg.Chat)
	}
	want := []string{"google/gemini-2.0-flash-001", "openai/gpt-4o-mini"}
	if !reflect.DeepEqual(cfg.ChatFallbacks, want) {
		t.Fatalf("fallbacks = %v, want %v", cfg.ChatFallbacks, want)
	}
}

func TestLoadModelConfigGuardrailDefault(t *testing.T) {
	t.Setenv("OPENROUTER_MODEL_GUARDRAIL", "")
	t.Setenv("OPENROUTER_GUARDRAIL_FALLBACKS", "")
	cfg := LoadModelConfig()
	if cfg.Guardrail != "google/gemini-2.5-flash-lite" {
		t.Fatalf("default guardrail model = %q", cfg.Guardrail)
	}
	if cfg.GuardrailFallbacks != nil {
		t.Fatalf("expected nil guardrail fallbacks, got %v", cfg.GuardrailFallbacks)
	}
}

func TestLoadModelConfigGuardrailOverride(t *testing.T) {
	t.Setenv("OPENROUTER_MODEL_GUARDRAIL", "google/gemini-2.0-flash-lite-001")
	t.Setenv("OPENROUTER_GUARDRAIL_FALLBACKS", "openai/gpt-4o-mini")
	cfg := LoadModelConfig()
	if cfg.Guardrail != "google/gemini-2.0-flash-lite-001" {
		t.Fatalf("override guardrail model = %q", cfg.Guardrail)
	}
	want := []string{"openai/gpt-4o-mini"}
	if !reflect.DeepEqual(cfg.GuardrailFallbacks, want) {
		t.Fatalf("guardrail fallbacks = %v, want %v", cfg.GuardrailFallbacks, want)
	}
}

func TestFeatureTimeouts(t *testing.T) {
	want := map[string]time.Duration{
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
	for feature, exp := range want {
		if got := FeatureTimeout(feature); got != exp {
			t.Fatalf("FeatureTimeout(%q) = %v, want %v", feature, got, exp)
		}
	}
	if got := FeatureTimeout("unknown"); got != DefaultFeatureTimeout {
		t.Fatalf("FeatureTimeout(unknown) = %v, want %v", got, DefaultFeatureTimeout)
	}
}
