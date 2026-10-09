package llm

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestEstimateCostKnownModel(t *testing.T) {
	got := EstimateCostUSD("google/gemini-2.5-flash", 1_000_000, 0, 200_000)
	if !approx(got, 0.80) {
		t.Fatalf("cost = %v, want 0.80", got)
	}
}

func TestEstimateCostCacheDiscount(t *testing.T) {
	got := EstimateCostUSD("google/gemini-2.5-flash", 1_000_000, 800_000, 0)
	if !approx(got, 0.12) {
		t.Fatalf("cost = %v, want 0.12", got)
	}
}

func TestEstimateCostUnknownModelIsZero(t *testing.T) {
	if got := EstimateCostUSD("totally/unknown-model", 1000, 0, 1000); got != 0 {
		t.Fatalf("unknown model cost = %v, want 0", got)
	}
}

func TestEstimateCostEnvOverride(t *testing.T) {
	t.Setenv("OPENROUTER_PRICES", "totally/unknown-model=1.00/2.00")
	loadPriceOverrides()
	defer loadPriceOverrides()
	got := EstimateCostUSD("totally/unknown-model", 1_000_000, 0, 1_000_000)
	if !approx(got, 3.00) {
		t.Fatalf("override cost = %v, want 3.00", got)
	}
}
