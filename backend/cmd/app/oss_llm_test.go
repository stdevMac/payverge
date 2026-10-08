package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

func clearOSSLLMEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"LLM_BASE_URL", "LLM_API_KEY", "OPENROUTER_API_KEY", "OPENROUTER_APP_URL", "OPENROUTER_APP_TITLE", "OPENROUTER_ZDR_MODE", "OPENROUTER_ZDR_APPROVED_MODELS", "PUBLIC_URL", "FRONTEND_URL", "BASE_URL", "NEXT_PUBLIC_BASE_URL", "APP_BASE_URL", "PRODUCT_NAME"} {
		t.Setenv(k, "")
	}
}

func TestLLMProviderConfigOpenRouterDefault(t *testing.T) {
	clearOSSLLMEnv(t)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	t.Setenv("PUBLIC_URL", "https://pos.example.com")
	cfg := llmProviderConfig()
	if cfg.OpenAICompatible || cfg.BaseURL != "" || cfg.APIKey != "sk-or-test" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.Referer != "https://pos.example.com" || cfg.Title != "Payverge" {
		t.Fatalf("ranking headers = %q / %q", cfg.Referer, cfg.Title)
	}
	if llmDisabledReason() != "" {
		t.Fatalf("configured OpenRouter reported disabled: %s", llmDisabledReason())
	}
}

func TestLLMProviderConfigSelfHostedBaseURL(t *testing.T) {
	clearOSSLLMEnv(t)
	t.Setenv("LLM_BASE_URL", "http://ollama:11434/v1/")
	t.Setenv("OPENROUTER_API_KEY", "sk-or-must-not-leak")
	cfg := llmProviderConfig()
	if !cfg.OpenAICompatible || cfg.BaseURL != "http://ollama:11434/v1" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.APIKey != "" || cfg.Referer != "" || cfg.Title != "" {
		t.Fatalf("self-hosted cfg leaked OpenRouter settings: %+v", cfg)
	}
	if llmDisabledReason() != "" {
		t.Fatalf("keyless Ollama reported disabled: %s", llmDisabledReason())
	}
}

func TestLLMDisabledReason(t *testing.T) {
	clearOSSLLMEnv(t)
	if r := llmDisabledReason(); !strings.Contains(r, "AI features disabled") {
		t.Fatalf("reason = %q", r)
	}
	t.Setenv("LLM_BASE_URL", "ollama:11434")
	if r := llmDisabledReason(); !strings.Contains(r, "LLM_BASE_URL") {
		t.Fatalf("reason = %q", r)
	}
}

func TestLLMStartupChecksSelfHostedUnpricedIsWarningNotFatal(t *testing.T) {
	clearOSSLLMEnv(t)
	t.Setenv("OPENROUTER_ZDR_MODE", "audit") // ignored off OpenRouter
	models := llm.ModelConfig{Chat: "llama3.1:8b", Menu: "llama3.1:8b", Guardrail: "llama3.2:3b"}
	r := llmStartupChecks(models, true, llmEndpointLocal)
	if len(r.Fatal) != 0 {
		t.Fatalf("self-hosted unpriced must not be fatal in production: %v", r.Fatal)
	}
	if !r.UnpricedFree || len(r.Warnings) == 0 || !strings.Contains(r.Warnings[0], "OPENROUTER_PRICES") {
		t.Fatalf("report = %+v", r)
	}
	if len(r.Info) == 0 || !strings.Contains(r.Info[0], "ZDR checks skipped") {
		t.Fatalf("info = %v", r.Info)
	}
}

func TestLLMStartupChecksSelfHostedPricedDoesNotEnableFreeMode(t *testing.T) {
	clearOSSLLMEnv(t)
	models := llm.ModelConfig{Chat: "google/gemini-2.5-flash"}
	r := llmStartupChecks(models, true, llmEndpointLocal)
	if r.UnpricedFree || len(r.Fatal) != 0 || len(r.Warnings) != 0 {
		t.Fatalf("report = %+v", r)
	}
}

func TestLLMStartupChecksOpenRouterStaysFailClosedInProduction(t *testing.T) {
	clearOSSLLMEnv(t)
	models := llm.ModelConfig{Chat: "vendor/unpriced-model"}
	r := llmStartupChecks(models, true, llmEndpointOpenRouter)
	if r.UnpricedFree {
		t.Fatal("OpenRouter must never enable $0 accounting")
	}
	if len(r.Fatal) == 0 || !strings.Contains(strings.Join(r.Fatal, "\n"), "pricing validation failed") {
		t.Fatalf("fatal = %v", r.Fatal)
	}

	r = llmStartupChecks(models, false, llmEndpointOpenRouter)
	if len(r.Fatal) != 0 || len(r.Warnings) == 0 {
		t.Fatalf("non-production OpenRouter must warn: %+v", r)
	}

	t.Setenv("OPENROUTER_ZDR_MODE", "bogus")
	r = llmStartupChecks(llm.ModelConfig{Chat: "google/gemini-2.5-flash"}, true, llmEndpointOpenRouter)
	if len(r.Fatal) == 0 || !strings.Contains(r.Fatal[0], "ZDR mode invalid") {
		t.Fatalf("invalid ZDR mode must be fatal on OpenRouter in production: %v", r.Fatal)
	}
}

func TestCurrentLLMEndpointClassification(t *testing.T) {
	cases := map[string]llmEndpoint{
		"":                                     llmEndpointOpenRouter,
		"https://openrouter.ai/api/v1":         llmEndpointOpenRouter,
		"http://ollama:11434/v1":               llmEndpointLocal,
		"http://host.docker.internal:11434/v1": llmEndpointLocal,
		"http://192.168.1.20:8000/v1":          llmEndpointLocal,
		"https://api.openai.com/v1":            llmEndpointHosted,
		"https://litellm.example.com/v1":       llmEndpointHosted,
	}
	for base, want := range cases {
		clearOSSLLMEnv(t)
		t.Setenv("LLM_BASE_URL", base)
		if got := currentLLMEndpoint(); got != want {
			t.Errorf("currentLLMEndpoint(%q) = %d, want %d", base, got, want)
		}
	}
}

// A public third-party vendor must not get $0 accounting: USD caps stay
// fail-closed for unpriced models, and enforce-mode ZDR is reported loudly.
func TestLLMStartupChecksHostedUnpricedStaysFailClosed(t *testing.T) {
	clearOSSLLMEnv(t)
	models := llm.ModelConfig{Chat: "vendor/unpriced-model"}
	r := llmStartupChecks(models, true, llmEndpointHosted)
	if r.UnpricedFree {
		t.Fatal("hosted endpoints must never enable $0 accounting")
	}
	if len(r.Fatal) != 0 {
		t.Fatalf("hosted unpriced is a warning, not fatal: %v", r.Fatal)
	}
	warn := strings.Join(r.Warnings, "\n")
	if !strings.Contains(warn, "OPENROUTER_PRICES") || !strings.Contains(warn, "fail closed") {
		t.Fatalf("pricing warning missing: %v", r.Warnings)
	}
	// Production defaults ZDR to enforce, which a hosted vendor cannot honor.
	if !strings.Contains(warn, "cannot be honored") {
		t.Fatalf("enforce-mode ZDR must be reported, warnings = %v", r.Warnings)
	}

	t.Setenv("OPENROUTER_ZDR_MODE", "audit")
	r = llmStartupChecks(llm.ModelConfig{Chat: "google/gemini-2.5-flash"}, true, llmEndpointHosted)
	if len(r.Warnings) != 0 || len(r.Fatal) != 0 || r.UnpricedFree {
		t.Fatalf("priced hosted model in audit mode: %+v", r)
	}
	if len(r.Info) == 0 || !strings.Contains(r.Info[0], "retention policy") {
		t.Fatalf("info = %v", r.Info)
	}

	t.Setenv("OPENROUTER_ZDR_MODE", "bogus")
	r = llmStartupChecks(llm.ModelConfig{Chat: "google/gemini-2.5-flash"}, true, llmEndpointHosted)
	if len(r.Fatal) != 0 || len(r.Warnings) == 0 || !strings.Contains(r.Warnings[0], "ZDR mode invalid") {
		t.Fatalf("invalid ZDR mode off OpenRouter is a warning: %+v", r)
	}
}

func TestLLMPreflightZDRModeIgnoredOffOpenRouter(t *testing.T) {
	clearOSSLLMEnv(t)
	if got := llmPreflightOpenRouterZDRMode("audit"); got != "audit" {
		t.Fatalf("OpenRouter preflight value = %q", got)
	}
	t.Setenv("LLM_BASE_URL", "http://vllm:8000/v1")
	if got := llmPreflightOpenRouterZDRMode("audit"); got != "" {
		t.Fatalf("non-OpenRouter preflight value = %q, want empty", got)
	}
}

func TestMainWiresLLMPortabilityHelpers(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, want := range []string{
		"llmDisabledReason()",
		"openrouter.New(llmProviderConfig()",
		"runLLMStartupChecks(models, productionMode)",
		"OpenRouterAPIKey:     config.LLMAPIKey()",
		"llmPreflightOpenRouterZDRMode(",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("main.go missing %q", want)
		}
	}
	if strings.Contains(s, `os.Getenv("OPENROUTER_API_KEY")`) {
		t.Error("main.go must resolve the LLM key through config.LLMAPIKey()")
	}
}
