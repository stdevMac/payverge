package server

// DollarBudgetGate is the per-business daily USD ceiling abstraction the AI
// lanes consult before a model call. Satisfied by *llm.AICostGate in prod
// (durable BudgetStore-backed) and by a stub in tests. A nil gate is a no-op
// (never over budget). Hard enforcement also wraps the OpenRouter call path
// (reserve → call → finalize/release); this gate is the fast pre-check.
type DollarBudgetGate interface {
	OverBudget(businessID uint) bool
}

var aiCostGate DollarBudgetGate

// SetAICostGate installs the process-wide dollar budget gate (wired in main.go
// from the durable BudgetStore). nil disables enforcement.
func SetAICostGate(g DollarBudgetGate) { aiCostGate = g }

// aiOverDollarBudget reports whether the business hit its daily USD ceiling.
// No gate installed => false (uncapped).
func aiOverDollarBudget(businessID uint) bool {
	if aiCostGate == nil {
		return false
	}
	return aiCostGate.OverBudget(businessID)
}

// guestAICostGate is the per-business daily USD ceiling for guest-facing AI
// (the AI waiter on web and WhatsApp). It is a separate ledger scope from the
// owner gate so anonymous guests can never exhaust the owner's own AI tools.
var guestAICostGate DollarBudgetGate

// SetGuestAICostGate installs the guest-scope dollar gate. nil disables it.
func SetGuestAICostGate(g DollarBudgetGate) { guestAICostGate = g }

// guestAIOverDollarBudget reports whether the business's guest-facing AI hit
// its daily USD ceiling (or the instance-wide ceiling). No gate => false.
func guestAIOverDollarBudget(businessID uint) bool {
	if guestAICostGate == nil {
		return false
	}
	return guestAICostGate.OverBudget(businessID)
}
