package config

import (
	"sync/atomic"
	"testing"
)

// aiProviderConfigured records whether an LLM provider was wired at startup.
// It is set by whoever constructs the AI service (server.SetAIService) and is
// the single input for "is AI available on this instance" questions:
//
//   - LLM-only routes (AI menu tools, menu wizard, marketing image generation,
//     ops assistant) answer 503 {"error":"ai_not_configured"} when false.
//   - GET /api/v1/instance reports it as features.ai.
//   - Guest projections expose ai_waiter_mode ("llm" vs "basic"); the guest
//     AI waiter keeps its deterministic menu-snapshot fallback when false.
//
// It deliberately does not read env vars: provider selection (OPENROUTER_API_KEY,
// LLM_BASE_URL/LLM_API_KEY, ...) is owned by the startup wiring, which knows
// whether a client was actually constructed. LLMProviderEnvConfigured is the
// env-only input that wiring uses to decide whether to try.
var aiProviderConfigured atomic.Bool

// SetAIProviderConfigured records whether an LLM provider client exists.
func SetAIProviderConfigured(configured bool) {
	aiProviderConfigured.Store(configured)
}

// AIProviderConfigured reports whether an LLM provider was configured at
// startup. False on a fresh self-host box with no AI key.
func AIProviderConfigured() bool {
	return aiProviderConfigured.Load()
}

// SetAIProviderConfiguredForTesting pins the flag for one test and restores
// the previous value on cleanup. Not for production use.
func SetAIProviderConfiguredForTesting(tb testing.TB, configured bool) {
	tb.Helper()
	prev := aiProviderConfigured.Load()
	aiProviderConfigured.Store(configured)
	tb.Cleanup(func() { aiProviderConfigured.Store(prev) })
}
