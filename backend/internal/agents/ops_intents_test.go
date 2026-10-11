package agents

import (
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/agents/ops_guides"

	"github.com/stretchr/testify/require"
)

const productionSpanishFiveTopicPrompt = "Necesito ayuda para agregar un producto al menú, crear una mesa y descargar su código QR, conectar una integración de pagos, revisar el inventario y configurar el asistente."

func TestResolveOpsIntentsProductionSpanishPrompt(t *testing.T) {
	got := ResolveOpsIntents(defaultOpsGuideCatalog, productionSpanishFiveTopicPrompt, "es", "overview")
	require.Equal(t, []string{
		"menu-add-item",
		"tables-create-qr",
		"plugins-connect",
		"inventory-stock",
		"ai-waiter-configure",
	}, guideIDs(got))
}

func TestResolveOpsIntentsGuestOrderDescribesDinerUI(t *testing.T) {
	got := ResolveOpsIntents(defaultOpsGuideCatalog, "How do guests order?", "en", "overview")
	require.Equal(t, []string{"guest-order-experience"}, guideIDs(got))
	require.Contains(t, strings.ToLower(got[0].Guide.Answer), "scan")
	require.Contains(t, strings.ToLower(got[0].Guide.Answer), "digital menu")
	require.NotContains(t, strings.ToLower(got[0].Guide.Answer), "setup progress")
}

func TestResolveOpsIntentsTurnOnAIWaiterDescribesDinerUINotSetup(t *testing.T) {
	queries := []struct {
		locale string
		query  string
	}{
		{locale: "en", query: "How do I turn on the AI Waiter… what will diners see?"},
		{locale: "en", query: "How do I turn on the AI Waiter? What will diners see?"},
		{locale: "es", query: "¿Cómo activo el Camarero IA y qué ven los comensales?"},
		{locale: "es-AR", query: "¿Cómo activo el Mozo IA y qué ven los comensales?"},
	}
	for _, tt := range queries {
		t.Run(tt.locale+"/"+tt.query, func(t *testing.T) {
			got := ResolveOpsIntents(defaultOpsGuideCatalog, tt.query, tt.locale, "overview")
			require.Equal(t, []string{"guest-order-experience"}, guideIDs(got), tt.query)
			lower := strings.ToLower(got[0].Guide.Answer)
			require.Contains(t, lower, "qr")
			require.NotContains(t, lower, "setup progress")
			require.NotContains(t, lower, "incomplete setup")
			require.NotContains(t, lower, "set name and priority")
			require.NotContains(t, lower, "nombre y la prioridad")
		})
	}
}

func TestResolveOpsIntentsStrongSingleTopicStaysSingle(t *testing.T) {
	got := ResolveOpsIntents(defaultOpsGuideCatalog, "¿Cómo agrego platos al menú?", "es", "bills")
	require.Equal(t, []string{"menu-add-item"}, guideIDs(got))
}

func TestResolveOpsIntentsStrongWholePrinterQuestionStaysSingle(t *testing.T) {
	got := ResolveOpsIntents(
		defaultOpsGuideCatalog,
		"How do I print a bill and a receipt?",
		"en",
		"overview",
	)
	require.Equal(t, []string{"printers-connect"}, guideIDs(got))
}

func TestResolveOpsIntentsStrongWholeCheckDoesNotSuppressRealMultiIntent(t *testing.T) {
	tests := []struct {
		query string
		want  []string
	}{
		{
			query: "How do I print a bill and create a table QR code?",
			want:  []string{"printers-connect", "tables-create-qr"},
		},
		{
			query: "How do I print a bill and review inventory?",
			want:  []string{"printers-connect", "inventory-stock"},
		},
		{
			query: "How do I print a bill and connect a payment plugin?",
			want:  []string{"printers-connect", "plugins-connect"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := ResolveOpsIntents(defaultOpsGuideCatalog, tt.query, "en", "overview")
			require.Equal(t, tt.want, guideIDs(got))
		})
	}
}

func TestResolveOpsIntentsLowConfidenceNoiseReturnsNone(t *testing.T) {
	for _, tt := range []struct {
		locale string
		query  string
	}{
		{locale: "es", query: "¿Qué clima hará mañana en Córdoba?"},
		{locale: "en", query: "Tell me a joke about a blue umbrella."},
		{locale: "en", query: "Tell me a story about a magenta umbrella."},
		{locale: "en", query: "Describe a human astronaut walking on Mars."},
		{locale: "en", query: "A magenta umbrella and a human astronaut."},
		{locale: "en", query: "invent a story about a table in the moonlight"},
		{locale: "es-AR", query: "Inventá un cuento sobre un menú bajo la luna."},
	} {
		require.Empty(t, ResolveOpsIntents(defaultOpsGuideCatalog, tt.query, tt.locale, "overview"), tt.query)
	}
}

func TestResolveOpsIntentsRecognizesEveryLocalizedSupportCatalogPhrase(t *testing.T) {
	catalog := ops_guides.NewDefaultCatalog()
	for _, locale := range []string{"en", "es", "es-AR"} {
		guide, ok := catalog.Get(locale, "support-contact")
		require.True(t, ok)
		for _, phrase := range guide.Phrases {
			t.Run(locale+"/"+phrase, func(t *testing.T) {
				got := ResolveOpsIntents(catalog, phrase, locale, "overview")
				require.Equal(t, []string{"support-contact"}, guideIDs(got))
			})
		}
	}
}

func TestResolveOpsIntentsRecognizesNaturalSupportAgentRequests(t *testing.T) {
	tests := []struct {
		locale string
		query  string
	}{
		{locale: "en", query: "I need to speak to an agent"},
		{locale: "es", query: "Necesito conectarme con un agente"},
		{locale: "es-AR", query: "Necesito ayuda de una persona"},
	}
	for _, tt := range tests {
		t.Run(tt.locale+"/"+tt.query, func(t *testing.T) {
			got := ResolveOpsIntents(defaultOpsGuideCatalog, tt.query, tt.locale, "overview")
			require.Equal(t, []string{"support-contact"}, guideIDs(got))
		})
	}
}

func TestResolveOpsIntentsOverlappingPhrasesDoNotDuplicateGuide(t *testing.T) {
	got := ResolveOpsIntents(
		defaultOpsGuideCatalog,
		"Agregar platos al menú, añadir producto y crear un elemento del menú.",
		"es",
		"overview",
	)
	require.Equal(t, []string{"menu-add-item"}, guideIDs(got))
}

func TestResolveOpsIntentsDuplicateTopHitDoesNotHideAnotherIntent(t *testing.T) {
	got := ResolveOpsIntents(
		defaultOpsGuideCatalog,
		"inventory; inventory stock alongside add menu items",
		"en",
		"overview",
	)
	require.Equal(t, []string{"inventory-stock", "menu-add-item"}, guideIDs(got))
}

func TestResolveOpsIntentsLocaleConjunctionAndPunctuationMatrix(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		locale string
		want   []string
	}{
		{
			name:   "english semicolons and conjunction",
			query:  "add menu items; create table and QR code; payment plugin; inventory; AI Waiter",
			locale: "en",
			want:   []string{"menu-add-item", "tables-create-qr", "plugins-connect", "inventory-stock", "ai-waiter-configure"},
		},
		{
			name:   "spanish argentina mozo alias and bullets",
			query:  "agregar producto al menú • crear mesa • integración de pagos • inventario • Mozo IA",
			locale: "es-AR",
			want:   []string{"menu-add-item", "tables-create-qr", "plugins-connect", "inventory-stock", "ai-waiter-configure"},
		},
		{
			name:   "spanish camarero alias and newlines",
			query:  "agregar producto al menú\ncrear mesa\nintegración de pagos\ninventario\nCamarero IA",
			locale: "es",
			want:   []string{"menu-add-item", "tables-create-qr", "plugins-connect", "inventory-stock", "ai-waiter-configure"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveOpsIntents(defaultOpsGuideCatalog, tt.query, tt.locale, "overview")
			require.Equal(t, tt.want, guideIDs(got))
		})
	}
}

func TestResolveOpsIntentsMultiWordConnectorsPreserveOrder(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		locale string
		want   []string
	}{
		{
			name:   "english as well as",
			query:  "review inventory as well as add menu items",
			locale: "en",
			want:   []string{"inventory-stock", "menu-add-item"},
		},
		{
			name:   "spanish junto con",
			query:  "revisar inventario junto con agregar un producto al menú",
			locale: "es",
			want:   []string{"inventory-stock", "menu-add-item"},
		},
		{
			name:   "spanish argentina mas",
			query:  "revisar inventario más configurar Mozo IA",
			locale: "es-AR",
			want:   []string{"inventory-stock", "ai-waiter-configure"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveOpsIntents(defaultOpsGuideCatalog, tt.query, tt.locale, "overview")
			require.Equal(t, tt.want, guideIDs(got))
		})
	}
}

func TestResolveOpsIntentsInlineNumberedEnumerations(t *testing.T) {
	want := []string{
		"menu-add-item",
		"tables-create-qr",
		"plugins-connect",
		"inventory-stock",
		"ai-waiter-configure",
	}
	tests := []struct {
		name   string
		query  string
		locale string
	}{
		{
			name:   "parentheses inline es-AR",
			query:  "1) agregar producto al menú 2) crear mesa 3) integración de pagos 4) inventario 5) Mozo IA",
			locale: "es-AR",
		},
		{
			name:   "periods inline es",
			query:  "1. agregar producto al menú 2. crear mesa 3. integración de pagos 4. inventario 5. Camarero IA",
			locale: "es",
		},
		{
			name:   "numbers newlines and bullets",
			query:  "1) agregar producto al menú\n2) crear mesa\n• integración de pagos\n• inventario\n• Mozo IA",
			locale: "es-AR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveOpsIntents(defaultOpsGuideCatalog, tt.query, tt.locale, "overview")
			require.Equal(t, want, guideIDs(got))
		})
	}
}

func TestSplitOpsIntentClausesDoesNotSplitDigitsInsideWords(t *testing.T) {
	tests := []string{
		"revisar impresora modelo2) con recibos",
		"revisar impresora v2.0 con recibos",
		"connect a 600-pair printer",
	}
	for _, query := range tests {
		require.Equal(t, []string{query}, splitOpsIntentClauses(query), query)
	}
}

func TestResolveOpsIntentsDoesNotSplitDashboardRouteSlash(t *testing.T) {
	query := "Open settings/printers."
	require.Equal(t, []string{query}, splitOpsIntentClauses(query))
	require.Equal(t, []string{"printers-connect"}, guideIDs(
		ResolveOpsIntents(defaultOpsGuideCatalog, query, "en", "overview"),
	))
}

func TestResolveOpsIntentsPreservesOrderAndCapsAtFive(t *testing.T) {
	got := ResolveOpsIntents(
		defaultOpsGuideCatalog,
		"inventario, agregar producto al menú, crear mesa, integración de pagos, Camarero IA, impresora térmica",
		"es",
		"overview",
	)
	require.Equal(t, []string{
		"inventory-stock",
		"menu-add-item",
		"tables-create-qr",
		"plugins-connect",
		"ai-waiter-configure",
	}, guideIDs(got))
}

func guideIDs(matches []ops_guides.GuideMatch) []string {
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		ids = append(ids, match.Guide.ID)
	}
	return ids
}
