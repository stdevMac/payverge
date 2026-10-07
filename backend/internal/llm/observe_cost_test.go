package llm

import (
	"testing"
	"time"
)

func TestAnnotateCostOnCallInfo(t *testing.T) {
	ci := CallInfo{
		Feature:      "waiter",
		Model:        "google/gemini-2.5-flash",
		ServedModel:  "google/gemini-2.5-flash",
		InputTokens:  1_000_000,
		OutputTokens: 200_000,
		Latency:      120 * time.Millisecond,
	}
	AnnotateCost(&ci, 0)
	if ci.EstimatedCostUSD <= 0 {
		t.Fatalf("EstimatedCostUSD not set: %v", ci.EstimatedCostUSD)
	}
	if !approx(ci.EstimatedCostUSD, 0.80) {
		t.Fatalf("EstimatedCostUSD = %v, want 0.80", ci.EstimatedCostUSD)
	}
}

func TestDailyCostRollupAggregates(t *testing.T) {
	r := NewDailyCostRollup()
	r.Add(7, "waiter", 0.10)
	r.Add(7, "waiter", 0.05)
	r.Add(7, "director", 0.20)
	r.Add(9, "waiter", 0.01)
	lines := r.Flush()
	if len(lines) == 0 {
		t.Fatalf("no rollup lines")
	}
	var foundB7 bool
	for _, l := range lines {
		if l.BusinessID == 7 && !approx(l.TotalUSD, 0.35) {
			t.Fatalf("business 7 total = %v, want 0.35", l.TotalUSD)
		}
		if l.BusinessID == 7 {
			foundB7 = true
		}
	}
	if !foundB7 {
		t.Fatalf("business 7 missing from rollup: %+v", lines)
	}
	if len(r.Flush()) != 0 {
		t.Fatalf("Flush did not reset the rollup")
	}
}
