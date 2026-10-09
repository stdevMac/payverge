package llmeval

import (
	"fmt"
	"strings"
)

// AllPassed is true only when every case passed AND at least one case ran.
func (r SuiteReport) AllPassed() bool { return r.Total > 0 && r.Passed == r.Total }

// RenderReport produces a plain-text pass/fail table + token-cost summary for
// the live cmd/llmeval binary. Token cost is reported as raw token counts;
// dollar pricing is deliberately out of scope here (Lane H1 owns the cost doc).
func RenderReport(r SuiteReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Suite: %s  (%d/%d passed)\n", r.Suite, r.Passed, r.Total)
	b.WriteString(strings.Repeat("-", 60) + "\n")
	for _, c := range r.Cases {
		status := "PASS"
		if !c.Pass {
			status = "FAIL"
		}
		fmt.Fprintf(&b, "%-4s %-36s tokens=%d\n", status, c.ID, c.Usage.TotalTokens)
		if c.Err != "" {
			fmt.Fprintf(&b, "       error: %s\n", c.Err)
		}
		for _, a := range c.Assertions {
			if !a.Pass {
				fmt.Fprintf(&b, "       [%s] %s\n", a.Type, a.Detail)
			}
		}
	}
	b.WriteString(strings.Repeat("-", 60) + "\n")
	fmt.Fprintf(&b, "Tokens: prompt=%d completion=%d total=%d\n",
		r.Usage.PromptTokens, r.Usage.CompletionTokens, r.Usage.TotalTokens)
	return b.String()
}
