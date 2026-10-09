package llm

import (
	"errors"
	"testing"
	"time"
)

func TestObserverContractShape(t *testing.T) {
	var got CallInfo
	var obs Observer = func(ci CallInfo) { got = ci }
	obs(CallInfo{
		Feature:      "waiter",
		Model:        "google/gemini-2.0-flash-001",
		ServedModel:  "openai/gpt-4o-mini",
		InputTokens:  120,
		OutputTokens: 45,
		Latency:      30 * time.Millisecond,
		Err:          errors.New("x"),
	})
	if got.Feature != "waiter" || got.Model != "google/gemini-2.0-flash-001" {
		t.Fatalf("feature/model = %q/%q", got.Feature, got.Model)
	}
	if got.ServedModel != "openai/gpt-4o-mini" {
		t.Fatalf("served model = %q", got.ServedModel)
	}
	if got.InputTokens != 120 || got.OutputTokens != 45 {
		t.Fatalf("tokens = %d/%d", got.InputTokens, got.OutputTokens)
	}
	if got.Latency != 30*time.Millisecond || got.Err == nil {
		t.Fatalf("latency/err = %v/%v", got.Latency, got.Err)
	}
}

func TestAnnotateCost_FlagsUnpricedServedModel(t *testing.T) {
	ci := CallInfo{ServedModel: "some/unlisted-fallback", InputTokens: 100, OutputTokens: 50}
	AnnotateCost(&ci, 0)
	if ci.EstimatedCostUSD != 0 {
		t.Fatalf("EstimatedCostUSD = %v, want 0 for unpriced model", ci.EstimatedCostUSD)
	}
	if ci.CostPriced {
		// $0 here means UNPRICED (no price-table entry), not a genuinely free model.
		t.Fatalf("CostPriced = true, want false for a model absent from the price table")
	}
}

func TestAnnotateCost_FlagsPricedServedModel(t *testing.T) {
	ci := CallInfo{ServedModel: "google/gemini-2.0-flash-001", InputTokens: 1_000_000, OutputTokens: 0}
	AnnotateCost(&ci, 0)
	if ci.EstimatedCostUSD <= 0 {
		t.Fatalf("EstimatedCostUSD = %v, want > 0 for a priced model", ci.EstimatedCostUSD)
	}
	if !ci.CostPriced {
		t.Fatalf("CostPriced = false, want true for a model present in the price table")
	}
}
