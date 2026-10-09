package llmeval

import (
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

func TestRenderReportTable(t *testing.T) {
	rep := SuiteReport{
		Suite:  "smoke_waiter",
		Total:  2,
		Passed: 1,
		Usage:  llm.Usage{PromptTokens: 100, CompletionTokens: 40, TotalTokens: 140},
		Cases: []CaseResult{
			{ID: "a", Pass: true, Usage: llm.Usage{TotalTokens: 70}},
			{ID: "b", Pass: false, Usage: llm.Usage{TotalTokens: 70}, Assertions: []Result{{Type: "language_is", Pass: false, Detail: "detected en want es"}}},
		},
	}
	out := RenderReport(rep)
	for _, want := range []string{"smoke_waiter", "PASS", "FAIL", "1/2", "140", "detected en want es"} {
		if !strings.Contains(out, want) {
			t.Fatalf("report missing %q:\n%s", want, out)
		}
	}
}

func TestReportAllPassed(t *testing.T) {
	if !(SuiteReport{Total: 3, Passed: 3}).AllPassed() {
		t.Fatalf("3/3 should be AllPassed")
	}
	if (SuiteReport{Total: 3, Passed: 2}).AllPassed() {
		t.Fatalf("2/3 should not be AllPassed")
	}
	if (SuiteReport{Total: 0, Passed: 0}).AllPassed() {
		t.Fatalf("empty suite should not be AllPassed (nothing ran)")
	}
}
