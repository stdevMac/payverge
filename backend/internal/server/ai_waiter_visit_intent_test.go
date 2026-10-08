package server

import (
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func visitIntentTestFacts() services.WaiterVisitFacts {
	return services.WaiterVisitFacts{
		HoursKnown: true, TodayOpen: "11:00", TodayClose: "23:00",
		HasOpenBill: true, BillSummary: "- 1x Harvest Bowl ($14.00)\nTotal: $14.00",
	}
}

func strayWaterCartCall() []llm.ToolCall {
	return []llm.ToolCall{{Name: "add_to_cart", Args: map[string]any{"menu_item_id": "7", "quantity": float64(1)}}}
}

// Issue 867: "What time do you close Sunday? Can I order at 10:30pm?" trips the
// cart vocabulary ("order"), so the model was consulted and answered a visit
// question by putting Sparkling Water on the guest's check. The visit answer
// must win and no cart action may survive.
func TestFinalizeWaiterV2_VisitQuestionBeatsStrayCartCall(t *testing.T) {
	snapshot := waiterV2TestSnapshot(t, "en")

	cases := []struct {
		name    string
		message string
		expect  string
	}{
		{"hours", "What time do you close on Sunday? Can I order at 10:30pm?", "23:00"},
		{"bill", "Can I get the bill and order the check please?", "14.00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response, err := FinalizeWaiterV2(WaiterFinalizeInput{
				ResponseID: "waiter-visit-867", Locale: "en", Mode: "ordering",
				UserMessage: tc.message, ModelText: "Adding a water for you.",
				ValidatedCalls: strayWaterCartCall(), Snapshot: snapshot, Visit: visitIntentTestFacts(),
			})
			require.NoError(t, err)
			requireValidWaiterV2(t, response)
			assert.Empty(t, response.Actions, "a visit question must never authorize a cart action")
			assert.NotContains(t, response.Answer.Content, "Sparkling Water")
			assert.Contains(t, response.Answer.Content, tc.expect)
			assert.Equal(t, assistantcontract.StatusComplete, response.Status)
		})
	}
}

// The model is only worth calling when the turn can actually move the cart. A
// pure visit question answered from snapshot facts must skip the LLM entirely,
// which is what removes the stray tool call at the source (issue 867).
func TestWaiterNeedsModel_SkipsModelForVisitQuestions(t *testing.T) {
	snapshot := waiterV2TestSnapshot(t, "en")

	assert.False(t, waiterNeedsModel("en", "What time do you close on Sunday? Can I order at 10:30pm?", snapshot))
	assert.False(t, waiterNeedsModel("es", "¿A qué hora cierran? ¿Puedo pedir a las 22:30?", snapshot))
	assert.True(t, waiterNeedsModel("en", "Please add a Harvest Bowl", snapshot),
		"a real cart request still needs the model's item resolution")
}

// Guardrail: naming a dish alongside cart vocabulary is still a cart turn, so
// the 867 fix must not swallow legitimate ordering.
func TestFinalizeWaiterV2_NamedItemCartRequestStillOrders(t *testing.T) {
	snapshot := waiterV2TestSnapshot(t, "en")

	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-visit-867-cart", Locale: "en", Mode: "ordering",
		UserMessage:    "Add a Sparkling Water to the order please",
		ValidatedCalls: strayWaterCartCall(), Snapshot: snapshot, Visit: visitIntentTestFacts(),
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	require.Len(t, response.Actions, 1)
	assert.Equal(t, "add_cart_item", response.Actions[0].Type)
	assert.True(t, strings.Contains(response.Answer.Content, "Sparkling Water"))
}
