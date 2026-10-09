package llm

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type modelPrice struct {
	InputPerMTok  float64
	OutputPerMTok float64
}

const cacheReadFactor = 0.25

// defaultPrices is the authoritative checked-in pricing table. Every configured
// primary and fallback model must appear here (or via OPENROUTER_PRICES) or
// startup / reserve fails closed (see ValidateConfiguredModelsPriced).
var defaultPrices = map[string]modelPrice{
	"google/gemini-2.0-flash-001":   {InputPerMTok: 0.10, OutputPerMTok: 0.40},
	"google/gemini-2.5-flash":       {InputPerMTok: 0.30, OutputPerMTok: 2.50},
	"google/gemini-2.5-flash-image": {InputPerMTok: 0.30, OutputPerMTok: 2.50},
	// Default guardrail primary (config.go); must stay priced so production
	// ValidateConfiguredModelsPriced does not hard-fail on stock config.
	"google/gemini-2.5-flash-lite": {InputPerMTok: 0.10, OutputPerMTok: 0.40},
}

var (
	priceMu sync.RWMutex
	prices  = clonePrices(defaultPrices)
)

func clonePrices(in map[string]modelPrice) map[string]modelPrice {
	out := make(map[string]modelPrice, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func loadPriceOverrides() {
	merged := clonePrices(defaultPrices)
	for _, entry := range strings.Split(os.Getenv("OPENROUTER_PRICES"), ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		model, rate, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		in, out, ok := strings.Cut(rate, "/")
		if !ok {
			continue
		}
		inF, e1 := strconv.ParseFloat(strings.TrimSpace(in), 64)
		outF, e2 := strconv.ParseFloat(strings.TrimSpace(out), 64)
		if e1 != nil || e2 != nil {
			continue
		}
		merged[strings.TrimSpace(model)] = modelPrice{InputPerMTok: inF, OutputPerMTok: outF}
	}
	priceMu.Lock()
	prices = merged
	priceMu.Unlock()
}

func init() { loadPriceOverrides() }

// ModelPriced reports whether the static price table has an entry for model.
// Callers use this to distinguish a genuine $0 cost from an UNPRICED model
// whose spend is therefore invisible (e.g. an OpenRouter failover target).
func ModelPriced(model string) bool {
	priceMu.RLock()
	_, ok := prices[model]
	priceMu.RUnlock()
	return ok
}

// unpricedModelsFree is the self-host escape hatch for local OpenAI-compatible
// endpoints the operator runs (Ollama, vLLM, LiteLLM, ...). There is no
// per-token price list for those, so instead of failing closed the budget
// ledger books unpriced models at $0 ("cost unknown"). ModelPriced stays
// honest so telemetry keeps reporting cost_priced=false for them.
var unpricedModelsFree atomic.Bool

// SetUnpricedModelsFree toggles $0 budget accounting for unpriced models.
// Startup enables it only when LLM_BASE_URL is a local endpoint
// (config.LLMBaseURLIsLocal); OpenRouter and hosted third-party endpoints keep
// the fail-closed pricing contract.
func SetUnpricedModelsFree(enabled bool) { unpricedModelsFree.Store(enabled) }

// budgetPriceKnown reports whether the budget ledger can account for model:
// it is in the price table, or unpriced models are explicitly free.
func budgetPriceKnown(model string) bool {
	return ModelPriced(model) || unpricedModelsFree.Load()
}

func EstimateCostUSD(model string, inputTokens, cachedInput, outputTokens int) float64 {
	priceMu.RLock()
	p, ok := prices[model]
	priceMu.RUnlock()
	if !ok {
		return 0
	}
	if cachedInput > inputTokens {
		cachedInput = inputTokens
	}
	uncached := inputTokens - cachedInput
	const perMTok = 1_000_000.0
	inCost := (float64(uncached)/perMTok)*p.InputPerMTok +
		(float64(cachedInput)/perMTok)*p.InputPerMTok*cacheReadFactor
	outCost := (float64(outputTokens) / perMTok) * p.OutputPerMTok
	return inCost + outCost
}
