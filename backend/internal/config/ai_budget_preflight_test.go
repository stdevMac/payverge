package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// L3 (round-2 review): the optional instance-wide AI caps were never checked
// by preflight, so a typo fell back to the default with only a log line.
func TestValidateProduction_OptionalAIBudgetCaps(t *testing.T) {
	cases := []struct {
		name         string
		global, pool string
		wantCode     string
		wantWarning  string
	}{
		{name: "both empty are derived", global: "", pool: ""},
		{name: "valid explicit caps", global: "60", pool: "20"},
		{name: "pool alone is fine", pool: "15"},
		{name: "global unparseable", global: "twenty", wantCode: "ai_budget.global.invalid"},
		{name: "global zero", global: "0", wantCode: "ai_budget.global.invalid"},
		{name: "global negative", global: "-5", wantCode: "ai_budget.global.invalid"},
		{name: "global infinite", global: "Inf", wantCode: "ai_budget.global.invalid"},
		{name: "pool NaN", pool: "NaN", wantCode: "ai_budget.guest_pool.invalid"},
		{name: "pool zero", global: "20", pool: "0", wantCode: "ai_budget.guest_pool.invalid"},
		{name: "pool above global", global: "20", pool: "25", wantCode: "ai_budget.guest_pool.above_global"},
		{name: "pool below global leaves an owner reserve", global: "20", pool: "15"},
		{name: "pool equal to global warns", global: "20", pool: "20", wantWarning: "ai_budget.owner_reserve.empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validProductionInputs()
			in.AIBudgetGlobalUSD = tc.global
			in.AIBudgetGuestPoolUSD = tc.pool
			report := ValidateProduction(in)
			if tc.wantCode == "" {
				require.True(t, report.OK(), "unexpected issues: %v", report.Codes())
			} else {
				require.False(t, report.OK())
				assert.Contains(t, report.Codes(), tc.wantCode)
			}
			var warnings []string
			for _, w := range report.Warnings() {
				warnings = append(warnings, w.Code)
			}
			if tc.wantWarning != "" {
				assert.Contains(t, warnings, tc.wantWarning)
			} else {
				assert.NotContains(t, warnings, "ai_budget.owner_reserve.empty")
			}
			for _, issue := range append(report.Issues(), report.Warnings()...) {
				if tc.global != "" {
					assert.NotContains(t, issue.Message, "="+tc.global, "messages must not echo raw values")
				}
			}
		})
	}
}
