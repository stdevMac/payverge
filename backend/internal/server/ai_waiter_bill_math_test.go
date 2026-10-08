package server

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func billMathTestFacts() services.WaiterVisitFacts {
	return services.WaiterVisitFacts{
		HasOpenBill: true,
		BillSummary: "- 1x Ojo de bife (ARS 39500.00)\nTotal: ARS 39500.00",
	}
}

func billMathTestMoney() waiterBillMoney {
	return waiterBillMoney{TotalCents: 3950000, Currency: "ARS"}
}

// Issue 943: "la cuenta / dividí entre 2 / 10% de propina" answered with the
// clarification stub ("No encontré eso en el menú") or a summary with no math.
// Split and tip asks must be recognized and computed from the server-owned
// bill total.
func TestWaiterBillMathAsk_ParsesSplitAndTip(t *testing.T) {
	cases := []struct {
		locale  string
		message string
		split   int
		tip     float64
	}{
		{"es", "Dividí la cuenta entre 2", 2, 0},
		{"es", "Agregá 10% de propina", 0, 10},
		{"es", "¿Podés dividir la cuenta entre 3 y sumar 10% de propina?", 3, 10},
		{"es-AR", "la cuenta a medias por favor", 2, 0},
		{"en", "Can we split the bill between 2 and add a 10% tip?", 2, 10},
		{"en", "split the check 3 ways", 3, 0},
		{"fr", "split the bill between 4 please", 4, 0},
		{"es", "¿Cuánto cuesta el ojo de bife?", 0, 0},
		{"en", "Add a Sparkling Water", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.message, func(t *testing.T) {
			ask := waiterBillMathAsk(tc.locale, tc.message)
			assert.Equal(t, tc.split, ask.SplitCount, "split count")
			assert.InDelta(t, tc.tip, ask.TipPercent, 0.001, "tip percent")
		})
	}
}

func TestFinalizeWaiterV2_SplitsAndTipsTheOpenBill(t *testing.T) {
	snapshot := waiterV2TestSnapshot(t, "es")

	t.Run("split only divides the current total", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-bill-split", Locale: "es", Mode: "ordering",
			UserMessage: "Dividí la cuenta entre 2", Snapshot: snapshot,
			Visit: billMathTestFacts(), Bill: billMathTestMoney(),
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.Equal(t, assistantcontract.StatusComplete, response.Status)
		assert.Contains(t, response.Answer.Content, "ARS 19750.00")
		assert.Empty(t, response.Actions)
	})

	t.Run("tip ask is answered with the tip and the new total", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-bill-tip", Locale: "es", Mode: "ordering",
			UserMessage: "Agregá 10% de propina", Snapshot: snapshot,
			Visit: billMathTestFacts(), Bill: billMathTestMoney(),
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.NotContains(t, response.Answer.Content, "No encontré eso en el menú")
		assert.Contains(t, response.Answer.Content, "ARS 3950.00")
		assert.Contains(t, response.Answer.Content, "ARS 43450.00")
		assert.Equal(t, assistantcontract.StatusComplete, response.Status)
	})

	t.Run("split plus tip splits the tipped total", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-bill-both", Locale: "en", Mode: "ordering",
			UserMessage: "Can we split the bill between 2 and add a 10% tip?", Snapshot: snapshot,
			Visit: billMathTestFacts(), Bill: billMathTestMoney(),
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.Contains(t, response.Answer.Content, "ARS 43450.00")
		assert.Contains(t, response.Answer.Content, "ARS 21725.00")
	})
}

// Honesty guard: with no open bill (or no server-side total) the waiter states
// the bill situation and never invents an amount to split or tip.
func TestFinalizeWaiterV2_BillMathWithoutATotalStatesNoNumbers(t *testing.T) {
	snapshot := waiterV2TestSnapshot(t, "es")

	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-bill-empty", Locale: "es", Mode: "ordering",
		UserMessage: "Dividí la cuenta entre 2 y agregá 10% de propina", Snapshot: snapshot,
		Visit: services.WaiterVisitFacts{},
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	assert.NotContains(t, response.Answer.Content, "No encontré eso en el menú")
	assert.NotContains(t, response.Answer.Content, "0.00")
	assert.Empty(t, response.Actions)
}

// A split of an odd total must not silently drop or invent a cent.
func TestWaiterBillSplitSharesCoverTheWholeTotal(t *testing.T) {
	share, exact := waiterSplitShareCents(3950000, 3)
	assert.False(t, exact)
	assert.Equal(t, int64(1316667), share, "the per-guest share is rounded up so the shares cover the bill")

	share, exact = waiterSplitShareCents(3950000, 2)
	assert.True(t, exact)
	assert.Equal(t, int64(1975000), share)
}
