package demo

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/fiscal"

	"github.com/stretchr/testify/require"
)

// TestDemoFiscalSeederUsesProductionActionConstant (L6-19 / B-8) fails if the
// demo day generator writes a free-string fiscal action (e.g. "invoice") that
// constant-keyed guards (issuable picker, delivery enqueue, credit-note
// eligibility, outbox uniqueness) never match. The seeder must use
// fiscal.ActionIssueReceipt so production and demo stay aligned.
func TestDemoFiscalSeederUsesProductionActionConstant(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	srcPath := filepath.Join(filepath.Dir(thisFile), "day_generator.go")
	raw, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	src := string(raw)

	// Production constant value must appear via the fiscal package symbol —
	// not a hardcoded "invoice" (or any other free string).
	require.Contains(t, src, "fiscal.ActionIssueReceipt",
		"createFiscalAndPrint must set Action from fiscal.ActionIssueReceipt")
	require.NotContains(t, src, `Action: "invoice"`,
		"demo seeder must not write Action: \"invoice\" (defeats constant-keyed guards)")
	require.NotContains(t, src, `Action: "issue_receipt"`,
		"prefer fiscal.ActionIssueReceipt over a string literal so renames cannot drift")

	// Sanity: production constant is still the value guards key on.
	require.Equal(t, "issue_receipt", fiscal.ActionIssueReceipt)
}
