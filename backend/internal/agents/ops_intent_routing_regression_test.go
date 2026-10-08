package agents

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/agents/ops_guides"
)

// #874: the takeover alias and the new ES inventory phrases were too greedy and
// pulled eleven unrelated questions away from the guide they resolved to before
// the branch. Every row below is a measured base-vs-branch delta, so this table
// pins the pre-874 route as the contract. An empty want means the query must
// resolve no guide at all and fall through to the model, exactly as it did
// before 874.
func TestResolveOpsIntentsDoesNotRegressPre874Routes(t *testing.T) {
	catalog := ops_guides.NewDefaultCatalog()
	tests := []struct {
		name   string
		query  string
		locale string
		want   string
	}{
		// A stop verb next to a plan/billing noun is not an AI Waiter takeover,
		// even when "AI" qualifies the noun; with no billing guide it resolves
		// nothing and falls through to the model.
		{name: "pause AI plan", query: "how do I pause my AI plan?", locale: "en", want: ""},
		{name: "pause AI subscription", query: "can I pause my AI subscription?", locale: "en", want: ""},
		// "AI" as an adjective on another surface's noun is that surface's question.
		{name: "turn off AI recommendations in inventory", query: "how do I turn off AI recommendations in inventory?", locale: "en", want: "inventory-stock"},
		{name: "stop chat notifications", query: "how do I stop chat notifications for my team?", locale: "en", want: ""},
		{name: "stop a waiter clocking in", query: "how do I stop a waiter from clocking in?", locale: "en", want: ""},
		// "qué falta" is a generic setup/progress ask, not an inventory one.
		{name: "que falta para publicar", query: "¿qué falta para publicar mi página de negocio?", locale: "es", want: ""},
		{name: "que falta para terminar el setup", query: "¿qué falta para terminar la configuración inicial?", locale: "es", want: "overview-get-started"},
		// 86 as a count, a table number, an amount of money, or part of a
		// larger number is not the sold-out slang.
		{name: "86 covers sold", query: "vendimos 86 cubiertos hoy, ¿dónde veo las analíticas?", locale: "es", want: "analytics-read"},
		{name: "table 86", query: "la mesa 86 pidió la cuenta", locale: "es", want: ""},
		{name: "$86 in cash", query: "¿cómo cobro $86 en efectivo?", locale: "es", want: ""},
		{name: "order 186", query: "el pedido 186 no salió", locale: "es", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits := ResolveOpsIntents(catalog, tt.query, tt.locale, "")
			if tt.want == "" {
				assert.Empty(t, hits, "query must resolve no guide")
				return
			}
			require.NotEmpty(t, hits, "query resolved no guide")
			assert.Equal(t, tt.want, hits[0].Guide.ID)
		})
	}
}

// The counterpart of the table above: the routes 874 was written to add must
// survive the tightening.
func TestResolveOpsIntents86AndTakeoverRoutesSurviveTheTightening(t *testing.T) {
	catalog := ops_guides.NewDefaultCatalog()
	tests := []struct {
		query  string
		locale string
		want   string
	}{
		{query: "¿qué está 86?", locale: "es", want: "inventory-stock"},
		{query: "que esta 86", locale: "es", want: "inventory-stock"},
		{query: "86 el bife de chorizo", locale: "es", want: "inventory-stock"},
		{query: "¿qué platos están agotados?", locale: "es", want: "inventory-stock"},
		{query: "¿qué está 86?", locale: "es-AR", want: "inventory-stock"},
		{query: "what is 86'd?", locale: "en", want: "inventory-stock"},
		{query: "86 the ribeye", locale: "en", want: "inventory-stock"},
		{query: "what is out of stock?", locale: "en", want: "inventory-stock"},
		{query: "pause the AI", locale: "en", want: "ai-waiter-takeover"},
		{query: "pause the ai waiter", locale: "en", want: "ai-waiter-takeover"},
		{query: "I want to take over the chat", locale: "en", want: "ai-waiter-takeover"},
		{query: "pausar la IA", locale: "es", want: "ai-waiter-takeover"},
		{query: "quiero intervenir el chat", locale: "es", want: "ai-waiter-takeover"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			hits := ResolveOpsIntents(catalog, tt.query, tt.locale, "")
			require.NotEmpty(t, hits)
			assert.Equal(t, tt.want, hits[0].Guide.ID)
		})
	}
}
