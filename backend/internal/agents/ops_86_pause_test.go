package agents

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/agents/ops_guides"
	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
)

// #874(a): the expo asking in Spanish what is 86'd resolved no guide at all,
// because the ES/es-AR inventory guide never carried the 86 vocabulary the EN
// guide has. The assistant then had nothing deterministic to answer with.
func TestResolveOpsIntentsSpanish86QuestionsResolveInventoryGuide(t *testing.T) {
	catalog := ops_guides.NewDefaultCatalog()
	tests := []struct {
		name   string
		query  string
		locale string
	}{
		{name: "es/que esta 86", query: "¿qué está 86?", locale: "es"},
		{name: "es/86 sin acentos", query: "que esta 86", locale: "es"},
		{name: "es/platos agotados", query: "¿qué platos están agotados?", locale: "es"},
		{name: "es/86 un plato", query: "86 el bife de chorizo", locale: "es"},
		{name: "es-AR/que esta 86", query: "¿qué está 86?", locale: "es-AR"},
		{name: "es-AR/platos agotados", query: "¿qué platos están agotados?", locale: "es-AR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits := ResolveOpsIntents(catalog, tt.query, tt.locale, "")
			require.NotEmpty(t, hits, "Spanish 86 question resolved no guide")
			assert.Equal(t, "inventory-stock", hits[0].Guide.ID)
		})
	}
}

// The EN guide already answered these; ES parity must not regress EN.
func TestResolveOpsIntentsEnglish86QuestionsStillResolveInventoryGuide(t *testing.T) {
	catalog := ops_guides.NewDefaultCatalog()
	for _, query := range []string{"what is 86'd?", "86 the ribeye", "what is out of stock?"} {
		t.Run(query, func(t *testing.T) {
			hits := ResolveOpsIntents(catalog, query, "en", "")
			require.NotEmpty(t, hits)
			assert.Equal(t, "inventory-stock", hits[0].Guide.ID)
		})
	}
}

// #874(b): "pause the AI" fell through to ai-waiter-configure — the setup
// guide — because expandOpsIntentAliases routes any mention of the AI Waiter
// to the configure alias regardless of what the operator wants to do with it,
// and no guide covered the shipped pause/takeover path at all.
func TestResolveOpsIntentsPauseRoutesToTakeoverNotSetup(t *testing.T) {
	catalog := ops_guides.NewDefaultCatalog()
	tests := []struct {
		name   string
		query  string
		locale string
	}{
		{name: "en/bare pause", query: "pause", locale: "en"},
		{name: "en/pause the ai", query: "pause the AI", locale: "en"},
		{name: "en/pause the ai waiter", query: "pause the ai waiter", locale: "en"},
		{name: "en/pause the assistant", query: "how do I pause the assistant?", locale: "en"},
		{name: "en/take over the chat", query: "I want to take over the chat", locale: "en"},
		{name: "en/resume", query: "resume the ai", locale: "en"},
		{name: "es/pausar la ia", query: "pausar la IA", locale: "es"},
		{name: "es/pausar camarero", query: "¿cómo pauso el camarero IA?", locale: "es"},
		{name: "es/intervenir el chat", query: "quiero intervenir el chat", locale: "es"},
		{name: "es-AR/pausar el mozo", query: "¿cómo pauso el mozo IA?", locale: "es-AR"},
		{name: "es-AR/pausar la ia", query: "pausar la IA", locale: "es-AR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits := ResolveOpsIntents(catalog, tt.query, tt.locale, "")
			require.NotEmpty(t, hits, "pause intent resolved no guide")
			assert.Equal(t, "ai-waiter-takeover", hits[0].Guide.ID,
				"pause intent must not open the AI Waiter setup guide")
		})
	}
}

// The takeover alias must not swallow unrelated "pause" questions: pausing a
// subscription or plan is not an AI Waiter takeover.
func TestResolveOpsIntentsPauseSubscriptionIsNotATakeover(t *testing.T) {
	catalog := ops_guides.NewDefaultCatalog()
	tests := []struct {
		query  string
		locale string
	}{
		{query: "can I pause my subscription?", locale: "en"},
		{query: "¿puedo pausar mi suscripción?", locale: "es"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			for _, hit := range ResolveOpsIntents(catalog, tt.query, tt.locale, "") {
				assert.NotEqual(t, "ai-waiter-takeover", hit.Guide.ID)
				assert.NotEqual(t, "ai-waiter-configure", hit.Guide.ID)
			}
		})
	}
}

// Asking how to configure the AI Waiter must still land on the setup guide.
func TestResolveOpsIntentsConfigureStillResolvesSetupGuide(t *testing.T) {
	catalog := ops_guides.NewDefaultCatalog()
	tests := []struct {
		query  string
		locale string
	}{
		{query: "how do I configure the ai waiter?", locale: "en"},
		{query: "¿cómo configuro el camarero IA?", locale: "es"},
		{query: "¿cómo configuro el mozo IA?", locale: "es-AR"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			hits := ResolveOpsIntents(catalog, tt.query, tt.locale, "")
			require.NotEmpty(t, hits)
			assert.Equal(t, "ai-waiter-configure", hits[0].Guide.ID)
		})
	}
}

// The takeover guide must exist in every locale with the permission the
// shipped pause endpoint actually enforces (ai_waiter:reply).
func TestOpsTakeoverGuideShipsInEveryLocale(t *testing.T) {
	catalog := ops_guides.NewDefaultCatalog()
	require.NoError(t, catalog.ValidateCatalog())
	for _, locale := range []string{"en", "es", "es-AR"} {
		t.Run(locale, func(t *testing.T) {
			guide, ok := catalog.Get(locale, "ai-waiter-takeover")
			require.True(t, ok)
			assert.Equal(t, "ai-waiter", guide.Tab)
			assert.Equal(t, "tab:ai-waiter", guide.Destination)
			assert.Equal(t, "ai_waiter:reply", guide.RequiredPermission)
			assert.NotEmpty(t, guide.Steps)
		})
	}
	enGuide, _ := catalog.Get("en", "ai-waiter-takeover")
	esGuide, _ := catalog.Get("es", "ai-waiter-takeover")
	arGuide, _ := catalog.Get("es-AR", "ai-waiter-takeover")
	assert.NotEqual(t, enGuide.Answer, esGuide.Answer, "ES must not fall back to English copy")
	assert.NotEqual(t, esGuide.Answer, arGuide.Answer, "es-AR must carry its own voseo copy")
	// es-AR is the Rioplatense layer: it says "Mozo IA", never "Camarero IA".
	assert.Contains(t, arGuide.Answer, "Mozo IA")
	assert.NotContains(t, arGuide.Answer, "Camarero IA")
}

// #874(b): the single-guide answer was emitted twice — once as answer.content
// and again as the one section — so the widget printed the same paragraph
// twice and ToLegacy flattened it into a duplicated V1 answer.
func TestFinalizeOpsV2SingleGuideAnswerIsNotDuplicated(t *testing.T) {
	match := requireOpsGuideMatch(t, "en", "ai-waiter-takeover")
	response, err := FinalizeOpsV2(OpsFinalizeInput{
		ResponseID: "ops-874-single",
		Locale:     "en",
		BusinessID: 142,
		Model:      StructuredResponse{Answer: "I paused the AI for you."},
		Matches:    []ops_guides.GuideMatch{match},
		Access:     allowOpsGuides(match),
	})
	require.NoError(t, err)
	require.NoError(t, assistantcontract.Validate(response))

	// The one guide is the answer; wrapping a copy of it in a section is what
	// made it render twice.
	assert.Empty(t, response.Sections)
	assert.Equal(t, match.Guide.Answer, response.Answer.Content)
	assert.Equal(t, match.Guide.Steps, response.Steps)
	// The navigation button and its provenance survive at response level.
	require.Len(t, response.Actions, 1)
	assert.Equal(t, "navigate:ai-waiter-takeover", response.Actions[0].ID)
	require.Len(t, response.Sources, 1)
	assert.Equal(t, "guide:ai-waiter-takeover", response.Sources[0].ID)
	assert.NotContains(t, response.Answer.Content, "I paused the AI for you.")

	legacy, err := assistantcontract.ToLegacy(response)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(legacy.Answer, match.Guide.Answer),
		"V1 answer repeated the guide text")
	assert.Equal(t, match.Guide.Steps, legacy.Steps)
}

// Two or more guides still render as titled sections.
func TestFinalizeOpsV2MultiGuideKeepsSections(t *testing.T) {
	matches := requireOpsGuideMatches(t, "en", "ai-waiter-takeover", "inventory-stock")
	response, err := FinalizeOpsV2(OpsFinalizeInput{
		ResponseID: "ops-874-multi",
		Locale:     "en",
		BusinessID: 142,
		Matches:    matches,
		Access:     allowOpsGuides(matches...),
	})
	require.NoError(t, err)
	require.NoError(t, assistantcontract.Validate(response))
	require.Len(t, response.Sections, 2)
	assert.Empty(t, response.Steps, "multi-guide steps stay inside their own section")
	assert.NotEqual(t, response.Sections[0].Answer, response.Answer.Content)
}
