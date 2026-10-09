package fiscal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// A voucher adopted on retry carries the authority's issue date; the stored
// receipt must use it instead of the retry's clock.
func TestBuildAuthorizedReceipt_UsesAdoptedIssueDate(t *testing.T) {
	retryClock := time.Date(2026, 4, 1, 3, 10, 0, 0, time.UTC)
	s := &Service{nowFn: func() time.Time { return retryClock }}
	jobCtx := &JobContext{Bill: database.Bill{TotalAmount: 5000}}
	adopted := time.Date(2026, 3, 31, 3, 0, 0, 0, time.UTC)

	receipt := s.buildAuthorizedReceipt(database.FiscalJob{}, jobCtx, ActionIssueReceipt, "factura_b", nil,
		&ReceiptResult{ReceiptNumber: "42", AuthCode: "70123456789012", IssuedAt: &adopted})
	require.NotNil(t, receipt.IssuedAt)
	require.True(t, receipt.IssuedAt.Equal(adopted))

	fresh := s.buildAuthorizedReceipt(database.FiscalJob{}, jobCtx, ActionIssueReceipt, "factura_b", nil,
		&ReceiptResult{ReceiptNumber: "43", AuthCode: "70123456789013"})
	require.True(t, fresh.IssuedAt.Equal(retryClock))
}
