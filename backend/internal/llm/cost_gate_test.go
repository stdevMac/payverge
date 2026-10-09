package llm

import "testing"

func TestAICostGateOverBudget(t *testing.T) {
	r := NewDailyCostRollup()
	g := NewAICostGate(r, 1.00) // $1.00/business/day

	if g.OverBudget(5) {
		t.Fatal("fresh business should not be over budget")
	}
	r.Add(5, "waiter", 0.99)
	if g.OverBudget(5) {
		t.Fatal("$0.99 < $1.00 cap should still be under budget")
	}
	r.Add(5, "director", 0.02) // now $1.01
	if !g.OverBudget(5) {
		t.Fatal("$1.01 >= $1.00 cap should be over budget")
	}
	// Another business is independent.
	if g.OverBudget(6) {
		t.Fatal("untouched business 6 should be under budget")
	}
}

func TestAICostGateDisabledWhenCapZero(t *testing.T) {
	r := NewDailyCostRollup()
	g := NewAICostGate(r, 0) // 0 => disabled (unlimited)
	r.Add(5, "waiter", 9999)
	if g.OverBudget(5) {
		t.Fatal("cap=0 must disable the gate (never over budget)")
	}
}

// A feature-scoped gate must count only its feature's spend, ignoring other
// features that share the same (synthetic) business bucket.
func TestFeatureCostGateScopesToFeature(t *testing.T) {
	r := NewDailyCostRollup()
	g := NewFeatureCostGate(r, 1.00, "concierge") // $1.00/day for concierge only

	// Unrelated spend on the same business-0 bucket (e.g. guardrail classifier)
	// must not push the concierge gate over budget.
	r.Add(0, "guardrail", 5.00)
	if g.OverBudget(0) {
		t.Fatal("guardrail spend must not trip the concierge feature ceiling")
	}

	r.Add(0, "concierge", 0.99)
	if g.OverBudget(0) {
		t.Fatal("$0.99 concierge < $1.00 cap should be under budget")
	}
	r.Add(0, "concierge", 0.02) // now $1.01 concierge
	if !g.OverBudget(0) {
		t.Fatal("$1.01 concierge >= $1.00 cap should be over budget")
	}
}

func TestAICostGateNilSafe(t *testing.T) {
	var g *AICostGate
	if g.OverBudget(1) {
		t.Fatal("nil gate must be a no-op (never over budget)")
	}
}
