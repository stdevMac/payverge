package handlers

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins"

	"github.com/stretchr/testify/require"
)

// isPartialPluginRefund must honor the plugin-declared RefundedAmountCents when
// present, regardless of the captured Amount, so partial-vs-full is a property
// the plugin declares — not one the handler infers from per-provider Amount
// semantics.
func TestIsPartialPluginRefund_HonorsDeclaredRefundedAmount(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBillForBiz(t, biz.ID)
	_, txHash := settleBillWithPluginPayment(t, bill, "PAY_DECLARED", 1000)

	ph := &PluginHandlers{}
	partial := int64(300)
	full := int64(1000)

	cases := []struct {
		name    string
		resp    *plugins.WebhookResponse
		partial bool
	}{
		// Declared partial wins even though Amount==full capture (the MP-shaped trap).
		{"declared-partial-amount-is-full", &plugins.WebhookResponse{Status: "refunded", Amount: 1000, RefundedAmountCents: &partial}, true},
		// Declared full is full even if Amount is something else.
		{"declared-full", &plugins.WebhookResponse{Status: "refunded", Amount: 1000, RefundedAmountCents: &full}, false},
		// No declaration → fall back to the legacy Amount heuristic.
		{"legacy-amount-partial", &plugins.WebhookResponse{Status: "refunded", Amount: 300}, true},
		{"legacy-amount-full", &plugins.WebhookResponse{Status: "refunded", Amount: 1000}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.partial, ph.isPartialPluginRefund(tc.resp, txHash))
		})
	}
	_ = database.GetDB()
}
