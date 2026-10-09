package config

import (
	"math"
	"strconv"
	"strings"
)

// validateOptionalAIBudgetCaps checks the optional instance-wide AI spend caps.
// AI_BUDGET_GLOBAL_USD_DAY and AI_BUDGET_GUEST_POOL_USD_DAY may be left empty
// (startup derives them from AI_DAILY_BUDGET_USD), but a value that is set
// must be a positive decimal: startup would otherwise fall back to the
// default with only a log warning, so a typo silently changes the caps. A
// guest pool above the global ceiling is refused too (startup would clamp it),
// and a pool equal to the global ceiling, which leaves owners no reserve, is a
// warning.
func validateOptionalAIBudgetCaps(r *Report, in ProductionInputs) {
	global, globalSet, globalOK := optionalPositiveDecimal(in.AIBudgetGlobalUSD)
	pool, poolSet, poolOK := optionalPositiveDecimal(in.AIBudgetGuestPoolUSD)
	if globalSet && !globalOK {
		r.add("ai_budget.global.invalid", "ai_budget", "AI_BUDGET_GLOBAL_USD_DAY must be empty (derived) or a positive decimal in production")
	}
	if poolSet && !poolOK {
		r.add("ai_budget.guest_pool.invalid", "ai_budget", "AI_BUDGET_GUEST_POOL_USD_DAY must be empty (derived) or a positive decimal in production")
	}
	if !globalSet || !globalOK || !poolSet || !poolOK {
		return
	}
	if pool > global {
		r.add("ai_budget.guest_pool.above_global", "ai_budget", "AI_BUDGET_GUEST_POOL_USD_DAY must not exceed AI_BUDGET_GLOBAL_USD_DAY")
		return
	}
	if pool >= global {
		r.addWarning("ai_budget.owner_reserve.empty", "ai_budget", "AI_BUDGET_GUEST_POOL_USD_DAY leaves no owner reserve under AI_BUDGET_GLOBAL_USD_DAY; guests can deny owner AI")
	}
}

// optionalPositiveDecimal parses an optional decimal: set reports a non-empty
// value, ok reports a finite value above zero.
func optionalPositiveDecimal(raw string) (value float64, set, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false, false
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, true, false
	}
	return f, true, true
}
