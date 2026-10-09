package services

import "testing"

// stubBudgetGate is a test double for the per-business daily USD ceiling.
type stubBudgetGate struct{ over bool }

func (s stubBudgetGate) OverBudget(uint) bool { return s.over }

// TestDirectorBudgetGate locks the fix for the Director Console having NO cost
// gate (unmetered LLM spend on the flagship AI-Pro surface). Director runs a
// multi-iteration tool-calling loop; every other AI lane consults a per-business
// daily dollar ceiling before calling the model, but Director did not. The gate
// must be consultable and default to uncapped only when no gate is installed.
func TestDirectorBudgetGate(t *testing.T) {
	// No gate installed => never gated (backwards-compatible default).
	nogate := &DirectorConsoleService{}
	if nogate.budgetExceeded(42) {
		t.Fatal("no gate installed must mean uncapped (budgetExceeded=false)")
	}

	// Gate installed and under budget => not gated.
	under := (&DirectorConsoleService{}).WithCostGate(stubBudgetGate{over: false})
	if under.budgetExceeded(42) {
		t.Fatal("under-budget business must not be gated")
	}

	// Gate installed and over budget => gated.
	over := (&DirectorConsoleService{}).WithCostGate(stubBudgetGate{over: true})
	if !over.budgetExceeded(42) {
		t.Fatal("over-budget business must be gated so the loop never runs")
	}
}

// TestBuildBudgetExceededResponse ensures the canned over-budget answer is a
// real, non-empty message (the guest/operator sees a graceful cap notice, not
// a blank card).
func TestBuildBudgetExceededResponse(t *testing.T) {
	resp := buildBudgetExceededResponse("en")
	if resp.Summary == "" || resp.Diagnosis == "" {
		t.Fatalf("budget-exceeded response must be non-empty, got %+v", resp)
	}
}
