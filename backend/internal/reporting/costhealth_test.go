package reporting

import (
	"encoding/json"
	"math"
	"testing"
)

// TestCostHealth_SharedDenominator is the Seam-1 gate for findings 7–9:
// food%, labor%, and prime% must all divide by RecognizedRevenue. Recipe-mapped
// sales are reported as coverage, never as the food-cost denominator.
//
// Today (broken): food $200 / recipe-mapped $400 = 50%, labor $300 / $1000 = 30%,
// prime = 80% — arithmetic nonsense. Want: 20% / 30% / 50% + coverage 40%.
func TestCostHealth_SharedDenominator(t *testing.T) {
	basis, err := NewRevenueBasis(RevenueComponents{
		RecognizedRevenue: 1000,
		RecipeMappedSales: 400,
	})
	if err != nil {
		t.Fatalf("NewRevenueBasis: %v", err)
	}

	rep := Compute(basis, 300 /* labor */, 200 /* cogs */)

	if rep.Status != StatusOK {
		t.Fatalf("status = %q, want %q", rep.Status, StatusOK)
	}
	if math.Abs(rep.FoodCostPct-0.20) > 1e-9 {
		t.Fatalf("food_cost_pct = %v, want 0.20 (200/1000, NOT 200/400)", rep.FoodCostPct)
	}
	if math.Abs(rep.LaborCostPct-0.30) > 1e-9 {
		t.Fatalf("labor_cost_pct = %v, want 0.30 (300/1000)", rep.LaborCostPct)
	}
	if rep.PrimeCostPct == nil {
		t.Fatal("prime_cost_pct must be present when status is ok")
	}
	if math.Abs(*rep.PrimeCostPct-0.50) > 1e-9 {
		t.Fatalf("prime_cost_pct = %v, want 0.50 (shared denom)", *rep.PrimeCostPct)
	}
	if math.Abs(rep.RecipeCoveragePct-0.40) > 1e-9 {
		t.Fatalf("recipe_coverage_pct = %v, want 0.40 (400/1000)", rep.RecipeCoveragePct)
	}
	// The broken sum (50%+30%=80%) must not appear.
	if rep.PrimeCostPct != nil && math.Abs(*rep.PrimeCostPct-0.80) < 1e-9 {
		t.Fatal("prime_cost_pct must not be the mixed-denominator 80%")
	}
}

// TestCostHealth_ImplausibleLaborExceedsRevenue is the gate for finding 10:
// a ratio above 1.0 is a data problem, not a metric. Never emit prime_cost_pct
// next to an impossible labor%, and never clamp.
func TestCostHealth_ImplausibleLaborExceedsRevenue(t *testing.T) {
	basis, err := NewRevenueBasis(RevenueComponents{
		RecognizedRevenue: 5600,
		RecipeMappedSales: 4100,
	})
	if err != nil {
		t.Fatalf("NewRevenueBasis: %v", err)
	}

	rep := Compute(basis, 19440 /* labor */, 0 /* cogs */)

	if rep.Status != StatusImplausible {
		t.Fatalf("status = %q, want %q", rep.Status, StatusImplausible)
	}
	wantLabor := 19440.0 / 5600.0 // ≈ 3.4714…
	if math.Abs(rep.LaborCostPct-wantLabor) > 1e-9 {
		t.Fatalf("labor_cost_pct = %v, want %v (never clamp)", rep.LaborCostPct, wantLabor)
	}
	// Rounded display of 3.47 is what the defect rendered as "347%".
	if math.Round(rep.LaborCostPct*100)/100 != 3.47 {
		t.Fatalf("labor_cost_pct rounded to 2dp = %.2f, want 3.47", rep.LaborCostPct)
	}
	if rep.Reason != ReasonLaborExceedsRevenue {
		t.Fatalf("reason = %q, want %q", rep.Reason, ReasonLaborExceedsRevenue)
	}
	if rep.PrimeCostPct != nil {
		t.Fatalf("prime_cost_pct must not be emitted when implausible, got %v", *rep.PrimeCostPct)
	}

	// JSON contract: status/implausible reason present; prime_cost_pct absent.
	raw, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if m["status"] != string(StatusImplausible) {
		t.Fatalf("json status = %v, want %q", m["status"], StatusImplausible)
	}
	if m["reason"] != ReasonLaborExceedsRevenue {
		t.Fatalf("json reason = %v, want %q", m["reason"], ReasonLaborExceedsRevenue)
	}
	if _, ok := m["prime_cost_pct"]; ok {
		t.Fatalf("json must omit prime_cost_pct when implausible, got %v", m["prime_cost_pct"])
	}
	if lp, ok := m["labor_cost_pct"].(float64); !ok || math.Abs(lp-wantLabor) > 1e-9 {
		t.Fatalf("json labor_cost_pct = %v, want %v", m["labor_cost_pct"], wantLabor)
	}
}

// TestCostHealth_InsufficientNoSales covers the zero-revenue path: ratios are
// undefined, so the report is insufficient_data rather than a green 0%.
func TestCostHealth_InsufficientNoSales(t *testing.T) {
	basis, err := NewRevenueBasis(RevenueComponents{
		RecognizedRevenue: 0,
		RecipeMappedSales: 0,
	})
	if err != nil {
		t.Fatalf("NewRevenueBasis: %v", err)
	}

	rep := Compute(basis, 500, 100)
	if rep.Status != StatusInsufficient {
		t.Fatalf("status = %q, want %q", rep.Status, StatusInsufficient)
	}
	if rep.PrimeCostPct != nil {
		t.Fatalf("prime_cost_pct must be absent when insufficient, got %v", *rep.PrimeCostPct)
	}
}

// TestCostHealth_FoodExceedsRevenue surfaces food/cogs > revenue as implausible
// with a distinct reason (never clamp, never invent a prime%).
func TestCostHealth_FoodExceedsRevenue(t *testing.T) {
	basis, err := NewRevenueBasis(RevenueComponents{
		RecognizedRevenue: 1000,
		RecipeMappedSales: 1000,
	})
	if err != nil {
		t.Fatalf("NewRevenueBasis: %v", err)
	}

	rep := Compute(basis, 100, 1500)
	if rep.Status != StatusImplausible {
		t.Fatalf("status = %q, want %q", rep.Status, StatusImplausible)
	}
	if rep.Reason != ReasonFoodExceedsRevenue {
		t.Fatalf("reason = %q, want %q", rep.Reason, ReasonFoodExceedsRevenue)
	}
	if rep.PrimeCostPct != nil {
		t.Fatal("prime_cost_pct must be absent when food exceeds revenue")
	}
}
