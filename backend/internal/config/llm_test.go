package config

import "testing"

func clearLLMEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"LLM_BASE_URL", "LLM_API_KEY", "OPENROUTER_API_KEY", "OPENROUTER_APP_URL", "OPENROUTER_APP_TITLE", "PUBLIC_URL", "FRONTEND_URL", "BASE_URL", "NEXT_PUBLIC_BASE_URL", "APP_BASE_URL", "PRODUCT_NAME"} {
		t.Setenv(k, "")
	}
}

func TestLLMDefaultsToOpenRouterWithLegacyKey(t *testing.T) {
	clearLLMEnv(t)
	if !LLMIsOpenRouter() {
		t.Fatal("empty LLM_BASE_URL must mean OpenRouter")
	}
	if LLMProviderEnvConfigured() {
		t.Fatal("OpenRouter without a key must not count as configured")
	}
	t.Setenv("OPENROUTER_API_KEY", "sk-or-legacy")
	if got := LLMAPIKey(); got != "sk-or-legacy" {
		t.Fatalf("LLMAPIKey = %q", got)
	}
	if !LLMProviderEnvConfigured() {
		t.Fatal("legacy OPENROUTER_API_KEY must configure AI")
	}
	t.Setenv("LLM_API_KEY", "sk-or-new")
	if got := LLMAPIKey(); got != "sk-or-new" {
		t.Fatalf("LLM_API_KEY must win, got %q", got)
	}
}

func TestLLMBaseURLOpenRouterHostDetection(t *testing.T) {
	clearLLMEnv(t)
	cases := map[string]bool{
		"https://openrouter.ai/api/v1":            true,
		"https://OpenRouter.ai/api/v1/":           true,
		"https://eu.openrouter.ai/api/v1":         true,
		"http://ollama:11434/v1":                  false,
		"http://localhost:8000/v1":                false,
		"https://openrouter.ai.evil.example/v1":   false,
		"https://notopenrouter.ai/v1":             false,
		"https://litellm.internal.example.com/v1": false,
	}
	for base, want := range cases {
		t.Setenv("LLM_BASE_URL", base)
		if got := LLMIsOpenRouter(); got != want {
			t.Errorf("LLMIsOpenRouter(%q) = %v, want %v", base, got, want)
		}
	}
}

func TestLLMBaseURLTrimsTrailingSlash(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("LLM_BASE_URL", " http://ollama:11434/v1/ ")
	if got := LLMBaseURL(); got != "http://ollama:11434/v1" {
		t.Fatalf("LLMBaseURL = %q", got)
	}
}

func TestLLMSelfHostedNeedsNoKeyAndNeverGetsOpenRouterKey(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("LLM_BASE_URL", "http://ollama:11434/v1")
	t.Setenv("OPENROUTER_API_KEY", "sk-or-secret")
	if got := LLMAPIKey(); got != "" {
		t.Fatalf("OpenRouter key must not be forwarded to a third-party base URL, got %q", got)
	}
	if !LLMProviderEnvConfigured() {
		t.Fatal("a valid non-OpenRouter LLM_BASE_URL must configure AI without a key")
	}
	t.Setenv("LLM_API_KEY", "litellm-key")
	if got := LLMAPIKey(); got != "litellm-key" {
		t.Fatalf("LLMAPIKey = %q", got)
	}
}

func TestLLMInvalidBaseURLIsNotConfigured(t *testing.T) {
	clearLLMEnv(t)
	for _, bad := range []string{"ollama:11434", "ftp://ollama/v1", "http://user:pw@ollama/v1", "not a url"} {
		t.Setenv("LLM_BASE_URL", bad)
		if LLMBaseURLValid() {
			t.Errorf("LLMBaseURLValid(%q) = true", bad)
		}
		if LLMProviderEnvConfigured() {
			t.Errorf("LLMProviderEnvConfigured with invalid base %q", bad)
		}
	}
}

func TestOpenRouterRankingHeadersDefaultToInstanceIdentity(t *testing.T) {
	clearLLMEnv(t)
	if got := OpenRouterAppURL(); got != "" {
		t.Fatalf("unconfigured instance must not send a referer, got %q", got)
	}
	if got := OpenRouterAppTitle(); got != DefaultProductName {
		t.Fatalf("title = %q", got)
	}
	t.Setenv("PUBLIC_URL", "https://pos.example.com")
	t.Setenv("PRODUCT_NAME", "Bistro OS")
	if got := OpenRouterAppURL(); got != "https://pos.example.com" {
		t.Fatalf("referer = %q", got)
	}
	if got := OpenRouterAppTitle(); got != "Bistro OS" {
		t.Fatalf("title = %q", got)
	}
	t.Setenv("OPENROUTER_APP_URL", "https://override.example")
	t.Setenv("OPENROUTER_APP_TITLE", "Override")
	if OpenRouterAppURL() != "https://override.example" || OpenRouterAppTitle() != "Override" {
		t.Fatal("explicit OPENROUTER_APP_* must win")
	}
}

func TestLLMBaseURLIsLocal(t *testing.T) {
	clearLLMEnv(t)
	if LLMBaseURLIsLocal() {
		t.Fatal("unset LLM_BASE_URL is OpenRouter, not local")
	}
	cases := map[string]bool{
		// Operator-run infrastructure.
		"http://ollama:11434/v1":                      true,
		"http://vllm:8000/v1":                         true,
		"http://litellm:4000/v1":                      true,
		"http://localhost:11434/v1":                   true,
		"http://127.0.0.1:8000/v1":                    true,
		"http://[::1]:8000/v1":                        true,
		"http://10.0.0.5:8000/v1":                     true,
		"http://192.168.1.20:11434/v1":                true,
		"http://172.16.4.2:8000/v1":                   true,
		"http://[fd00::12]:8000/v1":                   true,
		"http://host.docker.internal:11434/v1":        true,
		"http://gpu-box.local:8000/v1":                true,
		"http://ollama.ai.svc.cluster.local:11434/v1": true,
		"http://vllm.ml.svc:8000/v1":                  true,
		"http://llm.home.arpa/v1":                     true,
		"http://ollama.localhost/v1":                  true,
		// Public third-party vendors, OpenRouter and invalid values.
		"https://api.openai.com/v1":                    false,
		"https://api.together.xyz/v1":                  false,
		"https://litellm.example.com/v1":               false,
		"https://8.8.8.8/v1":                           false,
		"https://openrouter.ai/api/v1":                 false,
		"ollama:11434":                                 false,
		"http://user:pass@ollama:11434/v1":             false,
		"https://api.openai.com.evil.example/local/v1": false,
	}
	for base, want := range cases {
		t.Setenv("LLM_BASE_URL", base)
		if got := LLMBaseURLIsLocal(); got != want {
			t.Errorf("LLMBaseURLIsLocal(%q) = %v, want %v", base, got, want)
		}
	}
}
