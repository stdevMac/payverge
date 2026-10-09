package main

import (
	"fmt"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/llm/openrouter"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

// llmDisabledReason explains why startup will not build the AI service, or
// returns "" when a provider is configured. The guest AI waiter keeps its
// no-key fallback either way.
func llmDisabledReason() string {
	if !config.LLMBaseURLValid() {
		return "LLM_BASE_URL is not an absolute http(s) URL; AI features disabled"
	}
	if !config.LLMProviderEnvConfigured() {
		return "no LLM provider configured (set OPENROUTER_API_KEY, or LLM_BASE_URL for Ollama/vLLM/LiteLLM); AI features disabled"
	}
	return ""
}

// llmProviderConfig builds the provider config from LLM_BASE_URL /
// LLM_API_KEY / OPENROUTER_API_KEY. OpenRouter ranking headers default to the
// instance identity and are only sent to OpenRouter.
func llmProviderConfig() openrouter.Config {
	if !config.LLMIsOpenRouter() {
		return openrouter.Config{
			APIKey:           config.LLMAPIKey(),
			BaseURL:          config.LLMBaseURL(),
			OpenAICompatible: true,
		}
	}
	return openrouter.Config{
		APIKey:  config.LLMAPIKey(),
		BaseURL: config.LLMBaseURL(),
		Referer: config.OpenRouterAppURL(),
		Title:   config.OpenRouterAppTitle(),
	}
}

// llmEndpoint classifies where LLM traffic goes; pricing and privacy rules
// differ per class.
type llmEndpoint int

const (
	// llmEndpointOpenRouter: LLM_BASE_URL unset or an openrouter.ai host.
	llmEndpointOpenRouter llmEndpoint = iota
	// llmEndpointLocal: operator-run infrastructure (Ollama, vLLM, a LiteLLM
	// proxy on the compose network, ...) — see config.LLMBaseURLIsLocal.
	llmEndpointLocal
	// llmEndpointHosted: any other public OpenAI-compatible vendor.
	llmEndpointHosted
)

// currentLLMEndpoint classifies the configured LLM_BASE_URL.
func currentLLMEndpoint() llmEndpoint {
	switch {
	case config.LLMIsOpenRouter():
		return llmEndpointOpenRouter
	case config.LLMBaseURLIsLocal():
		return llmEndpointLocal
	default:
		return llmEndpointHosted
	}
}

// llmStartupReport is the outcome of the model pricing + ZDR startup checks.
type llmStartupReport struct {
	Fatal    []string
	Warnings []string
	Info     []string
	// UnpricedFree is true when unpriced models must be booked at $0
	// (local endpoint without an OPENROUTER_PRICES entry).
	UnpricedFree bool
}

// llmStartupChecks validates the configured models.
//
// OpenRouter: unchanged contract — every primary/fallback must be priced
// (fatal in production) and ZDR routing is validated (OPENROUTER_ZDR_MODE,
// enforce by default in production).
//
// Local endpoint (Ollama, vLLM, LiteLLM on the operator's network): missing
// prices are a warning and the budget ledger records unknown cost as $0;
// OPENROUTER_ZDR_MODE is ignored because prompts stay on the operator's own
// hardware.
//
// Hosted third-party endpoint (api.openai.com, Together, ...): missing prices
// are a warning, but unknown cost is NOT booked at $0 — budget-capped calls to
// unpriced models stay fail-closed so USD caps keep meaning something. ZDR
// cannot be routed or verified there; a ZDR mode of enforce (explicit, or the
// production default) is reported loudly instead of being silently ignored.
func llmStartupChecks(models llm.ModelConfig, productionMode bool, endpoint llmEndpoint) llmStartupReport {
	var r llmStartupReport
	switch endpoint {
	case llmEndpointLocal:
		if err := llm.ValidateConfiguredModelsPriced(models); err != nil {
			r.UnpricedFree = true
			r.Warnings = append(r.Warnings, fmt.Sprintf(
				"AI model pricing incomplete for the local LLM_BASE_URL (%v); unpriced calls are recorded at $0 and AI budget caps cannot trip on them — set OPENROUTER_PRICES=model=in/out to enforce caps", err))
		}
		r.Info = append(r.Info, "AI privacy ZDR checks skipped: LLM_BASE_URL is a local endpoint (OPENROUTER_ZDR_MODE ignored)")
		return r
	case llmEndpointHosted:
		if err := llm.ValidateConfiguredModelsPriced(models); err != nil {
			r.Warnings = append(r.Warnings, fmt.Sprintf(
				"AI model pricing incomplete for the hosted LLM_BASE_URL (%v); budget-capped AI calls to unpriced models are refused (fail closed) — set OPENROUTER_PRICES=model=in/out for every configured model", err))
		}
		zdrDefault := llm.ZDRModeAudit
		if productionMode {
			zdrDefault = llm.ZDRModeEnforce
		}
		zdrMode, zerr := llm.LoadZDRMode(zdrDefault)
		switch {
		case zerr != nil:
			r.Warnings = append(r.Warnings, fmt.Sprintf("AI privacy ZDR mode invalid and not applicable to a hosted LLM_BASE_URL: %v", zerr))
		case zdrMode == llm.ZDRModeEnforce:
			r.Warnings = append(r.Warnings,
				"AI privacy: OPENROUTER_ZDR_MODE=enforce cannot be honored — LLM_BASE_URL is a hosted third-party endpoint outside OpenRouter zero-data-retention routing, so prompts (which can include guest and order details) follow that provider's retention policy. Use OpenRouter or a local endpoint for ZDR, or set OPENROUTER_ZDR_MODE=audit to acknowledge")
		default:
			r.Info = append(r.Info, "AI privacy ZDR not applicable: LLM_BASE_URL is a hosted third-party endpoint; its own retention policy applies")
		}
		return r
	}

	if err := llm.ValidateConfiguredModelsPriced(models); err != nil {
		if productionMode {
			r.Fatal = append(r.Fatal, fmt.Sprintf("AI model pricing validation failed: %v", err))
		} else {
			r.Warnings = append(r.Warnings, fmt.Sprintf("AI model pricing validation failed (non-production): %v", err))
		}
	}
	// Zero-data-retention routing validation (Wave 5). Production defaults to
	// enforce when OPENROUTER_ZDR_MODE is unset; local/dev defaults audit.
	zdrDefault := llm.ZDRModeAudit
	if productionMode {
		zdrDefault = llm.ZDRModeEnforce
	}
	zdrMode, zerr := llm.LoadZDRMode(zdrDefault)
	if zerr != nil {
		if productionMode {
			r.Fatal = append(r.Fatal, fmt.Sprintf("AI privacy ZDR mode invalid: %v", zerr))
			return r
		}
		r.Warnings = append(r.Warnings, fmt.Sprintf("AI privacy ZDR mode invalid (non-production): %v", zerr))
		zdrMode = llm.ZDRModeAudit
	}
	if zerr := llm.ValidateZDRStartup(models, zdrMode, llm.LoadZDRApprovedModels()); zerr != nil {
		if productionMode {
			r.Fatal = append(r.Fatal, fmt.Sprintf("AI privacy ZDR validation failed: %v", zerr))
		} else {
			r.Warnings = append(r.Warnings, fmt.Sprintf("AI privacy ZDR validation failed (non-production): %v", zerr))
		}
	} else {
		st := llm.PrivacyReadiness()
		r.Info = append(r.Info, fmt.Sprintf("AI privacy ZDR readiness mode=%s status=%s sensitive_models=%d approved=%d",
			st.Mode, st.Status, st.SensitiveModels, st.ApprovedModels))
	}
	return r
}

// runLLMStartupChecks applies llmStartupChecks to the process: logs, enables
// $0 accounting for unpriced local models, and exits on fatal issues.
func runLLMStartupChecks(models llm.ModelConfig, productionMode bool) {
	report := llmStartupChecks(models, productionMode, currentLLMEndpoint())
	llm.SetUnpricedModelsFree(report.UnpricedFree)
	for _, w := range report.Warnings {
		logger.Logger.Warn(w)
	}
	for _, i := range report.Info {
		logger.Logger.Info(i)
	}
	for _, f := range report.Fatal {
		logger.Logger.Fatal(f)
	}
}

// llmPreflightOpenRouterZDRMode feeds the production preflight: the
// OPENROUTER_ZDR_MODE rule only applies when traffic actually goes to
// OpenRouter.
func llmPreflightOpenRouterZDRMode(raw string) string {
	if !config.LLMIsOpenRouter() {
		return ""
	}
	return raw
}
