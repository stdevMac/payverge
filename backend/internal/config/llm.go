package config

// LLM provider portability (self-host).
//
//	LLM_BASE_URL        OpenAI-compatible base URL, e.g. http://ollama:11434/v1,
//	                    http://vllm:8000/v1, http://litellm:4000/v1.
//	                    Empty = OpenRouter (https://openrouter.ai/api/v1).
//	LLM_API_KEY         bearer key for LLM_BASE_URL (optional for Ollama/vLLM).
//	OPENROUTER_API_KEY  legacy/OpenRouter key; used when LLM_API_KEY is empty
//	                    AND the base URL is OpenRouter. It is never forwarded
//	                    to a third-party LLM_BASE_URL.
//
// Model ids stay in the existing OPENROUTER_MODEL_* env vars (see llm.LoadModelConfig
// and docs/self-hosting/ai.md).

import (
	"net"
	"net/url"
	"os"
	"strings"
)

// LLMBaseURL returns LLM_BASE_URL without a trailing slash, or "" when the
// default OpenRouter endpoint applies.
func LLMBaseURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("LLM_BASE_URL")), "/")
}

// LLMIsOpenRouter reports whether LLM traffic goes to OpenRouter: LLM_BASE_URL
// is unset, or its host is openrouter.ai (or a subdomain). OpenRouter-only
// behavior — server-side fallbacks, ZDR routing, strict pricing, ranking
// headers — applies only then.
func LLMIsOpenRouter() bool {
	base := LLMBaseURL()
	if base == "" {
		return true
	}
	u, err := url.Parse(base)
	if err != nil {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	return host == "openrouter.ai" || strings.HasSuffix(host, ".openrouter.ai")
}

// localLLMHostSuffixes are DNS suffixes reserved for private networks
// (RFC 6762 .local, RFC 6761 .localhost, ICANN .internal, RFC 8375
// .home.arpa) plus the conventional .lan and Kubernetes .svc names.
var localLLMHostSuffixes = []string{".localhost", ".local", ".internal", ".lan", ".home.arpa", ".svc"}

// LLMBaseURLIsLocal reports whether LLM_BASE_URL points at operator-run
// infrastructure rather than a public third-party host: a loopback,
// private-range or link-local IP, localhost, a single-label hostname (compose
// or Kubernetes service names such as "ollama", "vllm", "litellm"), or a
// private-network suffix (host.docker.internal, *.local, *.svc, ...).
//
// Only local endpoints get $0 accounting for unpriced models and skip ZDR
// silently. A public OpenAI-compatible vendor (api.openai.com, Together, a
// hosted LiteLLM) is not local: its unpriced calls stay fail-closed under the
// budget caps. False when LLM_BASE_URL is unset (OpenRouter) or invalid.
func LLMBaseURLIsLocal() bool {
	base := LLMBaseURL()
	if base == "" || !LLMBaseURLValid() || LLMIsOpenRouter() {
		return false
	}
	u, err := url.Parse(base)
	if err != nil {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	if host == "localhost" || !strings.Contains(host, ".") {
		return true
	}
	for _, suffix := range localLLMHostSuffixes {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

// LLMBaseURLValid reports whether LLM_BASE_URL is unset or an absolute
// http(s) URL with a host and no credentials.
func LLMBaseURLValid() bool {
	base := LLMBaseURL()
	if base == "" {
		return true
	}
	u, err := url.Parse(base)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return (scheme == "http" || scheme == "https") && u.Hostname() != "" && u.User == nil
}

// LLMAPIKey returns the bearer key for the configured LLM endpoint:
// LLM_API_KEY first; OPENROUTER_API_KEY only when the endpoint is OpenRouter.
func LLMAPIKey() string {
	if key := strings.TrimSpace(os.Getenv("LLM_API_KEY")); key != "" {
		return key
	}
	if LLMIsOpenRouter() {
		return strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	}
	return ""
}

// LLMProviderEnvConfigured reports whether the env names an LLM provider,
// i.e. whether startup will try to build the AI service. OpenRouter needs a
// key; a valid non-OpenRouter LLM_BASE_URL is enough on its own (Ollama/vLLM
// take no key).
//
// This is the static, env-only startup input. Runtime gates and client-facing
// flags (/api/v1/instance, ai_waiter_mode, the
// 503 ai_not_configured answer) read AIProviderConfigured instead, which
// server.SetAIService sets only once the AI service was actually built.
func LLMProviderEnvConfigured() bool {
	if LLMIsOpenRouter() {
		return LLMAPIKey() != ""
	}
	return LLMBaseURLValid()
}

// OpenRouterAppURL is the HTTP-Referer ranking header sent to OpenRouter:
// OPENROUTER_APP_URL, else an explicitly configured PUBLIC_URL, else "".
func OpenRouterAppURL() string {
	if v := strings.TrimSpace(os.Getenv("OPENROUTER_APP_URL")); v != "" {
		return v
	}
	if PublicURLConfigured() {
		return PublicURL()
	}
	return ""
}

// OpenRouterAppTitle is the X-Title ranking header sent to OpenRouter:
// OPENROUTER_APP_TITLE, else ProductName.
func OpenRouterAppTitle() string {
	if v := cleanDisplayValue(os.Getenv("OPENROUTER_APP_TITLE"), 80); v != "" {
		return v
	}
	return ProductName()
}
