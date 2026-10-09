package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func waiterV2TestSnapshot(t *testing.T, locale string) WaiterMenuSnapshot {
	t.Helper()
	business := &database.Business{
		ID: 47, DefaultLanguage: "en", IsActive: true, KitchenEnabled: true, OrdersEnabled: true,
	}
	categories := []database.MenuCategory{
		{
			ID: "mains", Name: "Mains", SortOrder: 1,
			Items: []database.MenuItem{
				{ID: "harvest-bowl", Name: "Harvest Bowl", Description: "Roasted vegetables and grains", Price: 14, Currency: "USD", IsAvailable: true, Allergens: []string{"sesame"}, DietaryTags: []string{"vegetarian", "vegan"}, SortOrder: 1},
				{ID: "classic-burger", Name: "Classic Burger", Description: "Beef patty on a bun", Price: 16, Currency: "USD", IsAvailable: true, Allergens: []string{"gluten"}, SortOrder: 2},
				{ID: "pasta-primavera", Name: "Pasta Primavera", Description: "Pasta with seasonal vegetables", Price: 15, Currency: "USD", IsAvailable: true, Allergens: []string{"gluten"}, DietaryTags: []string{"vegetarian"}, SortOrder: 3},
			},
		},
		{
			ID: "drinks-and-extras", Name: "Drinks and extras", SortOrder: 2,
			Items: []database.MenuItem{
				{ID: "7", Name: "Sparkling Water", Description: "Chilled sparkling water", Price: 4, Currency: "USD", IsAvailable: true, SortOrder: 1},
				{ID: "seasonal-soup", Name: "Seasonal Soup", Description: "Ask staff about today's preparation", Price: 9, Currency: "USD", IsAvailable: false, Allergens: []string{"milk"}, SortOrder: 2},
			},
		},
	}
	bundles := []database.Bundle{{
		ID: 7, BusinessID: business.ID, Name: "Lunch Combo", Price: 18, Currency: "USD", IsActive: true,
		Items: `[{"menu_item_id":"harvest-bowl","name":"stale name","quantity":1}]`,
	}}
	snapshot, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
		Business: business, Locale: locale, Mode: "ordering", BusinessOpen: true,
		Categories: categories, SourceCategories: categories, Bundles: bundles, HiddenItemIDs: map[string]bool{},
	})
	require.NoError(t, err)
	return snapshot
}

func waiterV2SnapshotWithItems(t *testing.T, locale string, items []database.MenuItem, categoryName string) WaiterMenuSnapshot {
	t.Helper()
	business := &database.Business{ID: 47, DefaultLanguage: "en", IsActive: true, KitchenEnabled: true, OrdersEnabled: true}
	snapshot, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
		Business: business, Locale: locale, Mode: "ordering", BusinessOpen: true,
		Categories:    []database.MenuCategory{{ID: "test-category", Name: categoryName, Items: items}},
		HiddenItemIDs: map[string]bool{},
	})
	require.NoError(t, err)
	return snapshot
}

func waiterV2TestSnapshotForMode(t *testing.T, locale, mode string) WaiterMenuSnapshot {
	t.Helper()
	snapshot := waiterV2TestSnapshot(t, locale)
	snapshot.Mode = mode
	if mode != "ordering" {
		snapshot.OrderingOpen = false
		snapshot.OrderableByKey = map[WaiterMenuEntityKey]WaiterMenuEntity{}
	}
	return snapshot
}

func requireValidWaiterV2(t *testing.T, response assistantcontract.Response) {
	t.Helper()
	require.NoError(t, assistantcontract.Validate(response))
	require.Equal(t, 2, response.Version)
	require.NotNil(t, response.Sections)
	require.NotNil(t, response.Actions)
	require.NotNil(t, response.Sources)
	require.NotNil(t, response.Entities)
}

func responseJSONLower(t *testing.T, response assistantcontract.Response) string {
	t.Helper()
	raw, err := json.Marshal(response)
	require.NoError(t, err)
	return strings.ToLower(string(raw))
}

func TestFinalizeWaiterV2_OffMenuRecommendationCannotCreateEntitiesOrFacts(t *testing.T) {
	snapshot := waiterV2TestSnapshot(t, "en")
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-off-menu", Locale: "en", Mode: "ordering",
		UserMessage: "What do you recommend?",
		ModelText:   "Try our house salad with fries, then coffee and a digestif. [See it](https://evil.example/menu)",
		Snapshot:    snapshot,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)

	serialized := responseJSONLower(t, response)
	for _, unsupported := range []string{"house salad", "fries", "coffee", "digestif", "evil.example"} {
		assert.NotContains(t, serialized, unsupported)
	}
	// The waiter now answers the recommendation (#577) — but only ever from the
	// snapshot, so every named dish is an on-menu, evidence-backed entity.
	assert.Equal(t, assistantcontract.StatusComplete, response.Status)
	require.NotEmpty(t, response.Entities)
	for _, entity := range response.Entities {
		_, ok := snapshot.ByKey[WaiterMenuEntityKey{Type: entity.Type, ID: strings.TrimPrefix(entity.ID, entity.Type+":")}]
		assert.True(t, ok, "entity %s must resolve to a snapshot key", entity.ID)
	}
	assert.Empty(t, response.Actions)
}

func TestFinalizeWaiterV2_GroundsMenuIntentMatrix(t *testing.T) {
	snapshot := waiterV2TestSnapshot(t, "en")

	t.Run("stable id evidence grounds a recommendation without authorizing a cart action", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-recommend", Locale: "en", Mode: "ordering",
			UserMessage: "What do you recommend?", ModelText: "The Harvest Bowl is famous.", Snapshot: snapshot,
			ValidatedCalls: []llm.ToolCall{{Name: "add_to_cart", Args: map[string]any{"menu_item_id": "harvest-bowl", "quantity": float64(1)}}},
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		require.Len(t, response.Entities, 1)
		assert.Equal(t, "menu_item:harvest-bowl", response.Entities[0].ID)
		require.Len(t, response.Sources, 1)
		assert.Empty(t, response.Actions)
	})

	t.Run("recommendation selection is the server's, not the model's", func(t *testing.T) {
		// Same question, opposite model prose. The server must return the same
		// deterministic snapshot-ordered picks either way — prose is never the
		// selector, even when it names a real on-menu item.
		var picks [][]string
		for _, modelText := range []string{"The Harvest Bowl is famous.", "Only order the Pasta Primavera, skip everything else."} {
			response, err := FinalizeWaiterV2(WaiterFinalizeInput{
				ResponseID: "waiter-recommend-prose-only", Locale: "en", Mode: "ordering",
				UserMessage: "What do you recommend?", ModelText: modelText, Snapshot: snapshot,
			})
			require.NoError(t, err)
			requireValidWaiterV2(t, response)
			assert.Equal(t, assistantcontract.StatusComplete, response.Status)
			ids := make([]string, 0, len(response.Entities))
			for _, entity := range response.Entities {
				ids = append(ids, entity.ID)
			}
			picks = append(picks, ids)
		}
		assert.Equal(t, []string{"menu_item:harvest-bowl", "menu_item:classic-burger", "menu_item:pasta-primavera"}, picks[0])
		assert.Equal(t, picks[0], picks[1])
	})

	t.Run("recommendation names available snapshot items and never the unavailable one", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-recommend-open", Locale: "en", Mode: "ordering",
			UserMessage: "What's good today?", ModelText: "", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.Equal(t, assistantcontract.StatusComplete, response.Status)
		require.NotEmpty(t, response.Entities)
		serialized := responseJSONLower(t, response)
		assert.Contains(t, serialized, "harvest bowl")
		assert.NotContains(t, serialized, "seasonal soup")
		for _, entity := range response.Entities {
			assert.Equal(t, "available", entity.Availability)
		}
	})

	t.Run("veggie options is a grounded dietary recommendation, not an allergen refusal (#596)", func(t *testing.T) {
		// "egg" hiding inside "veggie" used to short-circuit this question into
		// the allergen path before the dietary classifier could run.
		intent, _ := classifyWaiterV2Intent("en", "do you have veggie options ?", snapshot)
		assert.Equal(t, waiterIntentRecommendation, intent)

		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-veggie", Locale: "en", Mode: "ordering",
			UserMessage: "do you have veggie options ?", ModelText: "", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.Equal(t, assistantcontract.StatusComplete, response.Status)
		require.NotEmpty(t, response.Entities)
		ids := make([]string, 0, len(response.Entities))
		for _, entity := range response.Entities {
			ids = append(ids, entity.ID)
		}
		assert.Contains(t, ids, "menu_item:harvest-bowl") // vegetarian-tagged
		serialized := responseJSONLower(t, response)
		assert.NotContains(t, serialized, strings.ToLower(services.AllergenRefusal("en")))
		assert.NotContains(t, serialized, "confirm with our staff before ordering")
	})

	t.Run("pairing names a companion item from another category", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-pairing", Locale: "en", Mode: "ordering",
			UserMessage: "What pairs with the Harvest Bowl?", ModelText: "A rare vintage wine.", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		require.GreaterOrEqual(t, len(response.Entities), 2)
		assert.Equal(t, "menu_item:harvest-bowl", response.Entities[0].ID)
		serialized := responseJSONLower(t, response)
		assert.Contains(t, serialized, "sparkling water")
		assert.NotContains(t, serialized, "vintage wine")
	})

	t.Run("alternative for an unavailable item names available items instead", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-alternative", Locale: "en", Mode: "ordering",
			UserMessage: "The Seasonal Soup is unavailable — what's a good alternative?",
			ModelText:   "Try the imaginary gazpacho.", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		require.GreaterOrEqual(t, len(response.Entities), 2)
		assert.Equal(t, "menu_item:seasonal-soup", response.Entities[0].ID)
		assert.Equal(t, "unavailable", response.Entities[0].Availability)
		for _, entity := range response.Entities[1:] {
			assert.Equal(t, "available", entity.Availability)
		}
		assert.NotContains(t, responseJSONLower(t, response), "gazpacho")
	})

	t.Run("specific item named by the user resolves from the snapshot", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-specific-item", Locale: "en", Mode: "ordering",
			UserMessage: "Tell me about the Harvest Bowl", ModelText: "It contains invented truffles.", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		require.Len(t, response.Entities, 1)
		assert.Equal(t, "menu_item:harvest-bowl", response.Entities[0].ID)
		assert.Equal(t, "Harvest Bowl", response.Entities[0].DisplayName)
		assert.Empty(t, response.Actions)
		assert.NotContains(t, responseJSONLower(t, response), "truffles")
	})

	t.Run("known unavailable item is explainable but not actionable", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-unavailable", Locale: "en", Mode: "ordering",
			UserMessage: "Can I order the Seasonal Soup?", ModelText: "Yes, I added it.", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		require.Len(t, response.Entities, 1)
		assert.Equal(t, "menu_item:seasonal-soup", response.Entities[0].ID)
		assert.Equal(t, "unavailable", response.Entities[0].Availability)
		assert.Empty(t, response.Actions)
		assert.NotContains(t, strings.ToLower(response.Answer.Content), "added")
	})

	t.Run("full menu is deterministic and grouped by snapshot category", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-full-menu", Locale: "en", Mode: "ordering",
			UserMessage: "Show me the full menu", ModelText: "Our secret fries are the bestseller.", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		require.Len(t, response.Sections, 2)
		assert.Equal(t, []string{"Mains", "Drinks and extras"}, []string{response.Sections[0].Title, response.Sections[1].Title})
		serialized := responseJSONLower(t, response)
		assert.Contains(t, serialized, "harvest bowl")
		assert.Contains(t, serialized, "seasonal soup")
		assert.NotContains(t, serialized, "secret fries")
	})

	t.Run("unsupported named menu request asks for clarification", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-unknown", Locale: "es-AR", Mode: "ordering",
			UserMessage: "¿Tienen ensalada de la casa?", ModelText: "Sí, es nuestra especialidad.", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.Equal(t, assistantcontract.StatusNeedsClarification, response.Status)
		assert.Equal(t, services.ClarifyItemMessage("es-AR"), response.Answer.Content)
		assert.Empty(t, response.Entities)
	})
}

func TestFinalizeWaiterV2_NamespacesCollidingMenuAndBundleIDsDeterministically(t *testing.T) {
	forward := WaiterFinalizeInput{
		ResponseID: "waiter-colliding-ids", Locale: "en", Mode: "ordering",
		UserMessage: "Add both to my cart", Snapshot: waiterV2TestSnapshot(t, "en"),
		ValidatedCalls: []llm.ToolCall{
			{Name: "add_to_cart", Args: map[string]any{"menu_item_id": "7", "quantity": float64(1)}},
			{Name: "add_to_cart", Args: map[string]any{"bundle_id": "7", "quantity": float64(1)}},
		},
	}
	reverse := forward
	reverse.ValidatedCalls = []llm.ToolCall{forward.ValidatedCalls[1], forward.ValidatedCalls[0]}
	first, err := FinalizeWaiterV2(forward)
	require.NoError(t, err)
	second, err := FinalizeWaiterV2(reverse)
	require.NoError(t, err)
	for index := range first.Sources {
		_, err := time.Parse(time.RFC3339, first.Sources[index].RetrievedAt)
		require.NoError(t, err)
		first.Sources[index].RetrievedAt = ""
		second.Sources[index].RetrievedAt = ""
	}
	require.Equal(t, first, second, "trusted V2 ordering must not depend on model call order")
	require.Equal(t, []string{"menu_item:7", "bundle:7"}, []string{first.Entities[0].ID, first.Entities[1].ID})
	require.Equal(t, []string{"menu:menu_item:7", "menu:bundle:7"}, []string{first.Sources[0].ID, first.Sources[1].ID})
	require.Equal(t, []string{"menu_item", "bundle"}, []string{first.Actions[0].Target.Kind, first.Actions[1].Target.Kind})
	require.Equal(t, []string{"7", "7"}, []string{first.Actions[0].Target.ID, first.Actions[1].Target.ID})
}

func TestFinalizeWaiterV2_RequiresMatchingModeAndNonNegatedExplicitCartIntent(t *testing.T) {
	snapshot := waiterV2TestSnapshot(t, "en")
	call := llm.ToolCall{Name: "add_to_cart", Args: map[string]any{"menu_item_id": "harvest-bowl", "quantity": float64(1)}}

	for _, mode := range []string{"concierge", "invalid"} {
		_, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-mode-" + mode, Locale: "en", Mode: mode,
			UserMessage: "Add the Harvest Bowl", Snapshot: snapshot, ValidatedCalls: []llm.ToolCall{call},
		})
		require.Error(t, err)
	}

	for _, text := range []string{
		"Add the Harvest Bowl",
		"Put the Harvest Bowl in my cart",
		"Get me the Harvest Bowl",
		"Order the Harvest Bowl",
		"I'd like the Harvest Bowl",
		"I would like the Harvest Bowl",
	} {
		t.Run("affirmative "+text, func(t *testing.T) {
			response, err := FinalizeWaiterV2(WaiterFinalizeInput{
				ResponseID: "waiter-affirmative-cart", Locale: "en", Mode: "ordering",
				UserMessage: text, Snapshot: snapshot, ValidatedCalls: []llm.ToolCall{call},
			})
			require.NoError(t, err)
			require.Len(t, response.Actions, 1)
		})
	}

	for _, test := range []struct {
		name   string
		locale string
		text   string
	}{
		{name: "en don't add", locale: "en", text: "Don't add the Harvest Bowl"},
		{name: "en do not put", locale: "en", text: "Do not put the Harvest Bowl in my cart"},
		{name: "en don't get", locale: "en", text: "Don't get me the Harvest Bowl"},
		{name: "en don't want add", locale: "en", text: "I don't want you to add the Harvest Bowl"},
		{name: "en can you not add", locale: "en", text: "Can you not add the Harvest Bowl?"},
		{name: "en never order", locale: "en", text: "Never order the Harvest Bowl"},
		{name: "en without adding", locale: "en", text: "Tell me about it without adding the Harvest Bowl"},
		{name: "en cancel", locale: "en", text: "Cancel my Harvest Bowl order"},
		{name: "en ready status", locale: "en", text: "Is my Harvest Bowl order ready?"},
		{name: "en order status", locale: "en", text: "What is the status of my Harvest Bowl order?"},
		{name: "es no agregar", locale: "es", text: "No agregues el Harvest Bowl"},
		{name: "es no poner", locale: "es", text: "No pongas el Harvest Bowl en mi carrito"},
		{name: "es no traer", locale: "es", text: "No me traigas el Harvest Bowl"},
		{name: "es no quiero agregar", locale: "es", text: "No quiero agregar el Harvest Bowl"},
		{name: "es no quiero que agregues", locale: "es", text: "No quiero que agregues el Harvest Bowl"},
		{name: "es nunca pedir", locale: "es", text: "Nunca pidas el Harvest Bowl"},
		{name: "es sin añadir", locale: "es", text: "Explícalo sin añadir el Harvest Bowl"},
		{name: "es cancel", locale: "es", text: "Cancela mi pedido de Harvest Bowl"},
		{name: "es ready status", locale: "es", text: "¿Está listo mi pedido de Harvest Bowl?"},
		{name: "es order status", locale: "es", text: "¿Cuál es el estado de mi pedido de Harvest Bowl?"},
		{name: "es-AR no agregar", locale: "es-AR", text: "No agregues el Harvest Bowl"},
		{name: "es-AR no poner", locale: "es-AR", text: "No pongas el Harvest Bowl en mi carrito"},
		{name: "es-AR no traer", locale: "es-AR", text: "No me traigas el Harvest Bowl"},
		{name: "es-AR no quiero agregar", locale: "es-AR", text: "No quiero agregar el Harvest Bowl"},
		{name: "es-AR no quiero que agregues", locale: "es-AR", text: "No quiero que agregues el Harvest Bowl"},
		{name: "es-AR nunca pedir", locale: "es-AR", text: "Nunca pidas el Harvest Bowl"},
		{name: "es-AR sin sumar", locale: "es-AR", text: "Explicalo sin sumar el Harvest Bowl"},
		{name: "es-AR cancel", locale: "es-AR", text: "Cancelá mi pedido de Harvest Bowl"},
		{name: "es-AR ready status", locale: "es-AR", text: "¿Está listo mi pedido de Harvest Bowl?"},
		{name: "es-AR order status", locale: "es-AR", text: "¿Cuál es el estado de mi pedido de Harvest Bowl?"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := FinalizeWaiterV2(WaiterFinalizeInput{
				ResponseID: "waiter-negated-" + test.locale, Locale: test.locale, Mode: "ordering",
				UserMessage: test.text, Snapshot: snapshot, ValidatedCalls: []llm.ToolCall{call},
			})
			require.NoError(t, err)
			requireValidWaiterV2(t, response)
			assert.Empty(t, response.Actions)
		})
	}
}

func TestFinalizeWaiterV2_AffirmativeCartIntentWithNoGroundedSelectionClarifies(t *testing.T) {
	snapshot := waiterV2TestSnapshot(t, "en")
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-rejected-cart-with-non-cart", Locale: "en", Mode: "ordering",
		UserMessage: "Add it to my cart", ModelText: "Done.", Snapshot: snapshot,
		// The invalid add_to_cart call has already been removed by Task 3's
		// validator; unrelated non-cart calls intentionally pass through it.
		ValidatedCalls: []llm.ToolCall{{Name: "navigate", Args: map[string]any{"href": "https://evil.example"}}},
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	assert.Equal(t, assistantcontract.StatusNeedsClarification, response.Status)
	assert.Equal(t, services.ClarifyItemMessage("en"), response.Answer.Content)
	assert.Empty(t, response.Actions)
	assert.Empty(t, response.Sources)
	assert.Empty(t, response.Entities)
}

func TestFinalizeWaiterV2_GenericPreferenceWithoutCartContextIsNotCartIntent(t *testing.T) {
	snapshot := waiterV2TestSnapshotForMode(t, "en", "concierge")
	for _, text := range []string{
		"I'd like a joke",
		"I would like to know your hours",
	} {
		t.Run(text, func(t *testing.T) {
			response, err := FinalizeWaiterV2(WaiterFinalizeInput{
				ResponseID: "waiter-generic-preference", Locale: "en", Mode: "concierge",
				UserMessage: text, ModelText: "Untrusted factual prose.", Snapshot: snapshot,
			})
			require.NoError(t, err)
			requireValidWaiterV2(t, response)
			assert.NotEqual(t, assistantcontract.StatusBlocked, response.Status)
			assert.NotEqual(t, services.OrderingPausedMessage("en"), response.Answer.Content)
			assert.NotEqual(t, services.ClarifyItemMessage("en"), response.Answer.Content)
			assert.Empty(t, response.Actions)
		})
	}
}

func TestFinalizeWaiterV2_RejectsMalformedStructuredCartNotes(t *testing.T) {
	for _, test := range []struct {
		name  string
		notes any
	}{
		{name: "non-string", notes: 42},
		{name: "unicode bidi format", notes: "no onions \u202Eevil"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := FinalizeWaiterV2(WaiterFinalizeInput{
				ResponseID: "waiter-malformed-notes", Locale: "en", Mode: "ordering",
				UserMessage: "Add the Harvest Bowl", Snapshot: waiterV2TestSnapshot(t, "en"),
				ValidatedCalls: []llm.ToolCall{{Name: "add_to_cart", Args: map[string]any{
					"menu_item_id": "harvest-bowl", "quantity": float64(1), "notes": test.notes,
				}}},
			})
			require.NoError(t, err)
			requireValidWaiterV2(t, response)
			assert.Equal(t, assistantcontract.StatusNeedsClarification, response.Status)
			assert.Empty(t, response.Actions)
		})
	}
}

func TestFinalizeWaiterV2_DoesNotMutateSnapshotPresentationSlices(t *testing.T) {
	snapshot := waiterV2SnapshotWithItems(t, "en", []database.MenuItem{{
		ID: "allergen-item", Name: "Allergen Item", Description: "Safe", IsAvailable: true,
		Allergens: []string{"*milk*", "[nuts](https://evil.example)"}, DietaryTags: []string{"_vegan_"},
	}}, "Mains")
	key := WaiterMenuEntityKey{Type: waiterMenuEntityTypeMenuItem, ID: "allergen-item"}
	beforeEntitiesAllergens := append([]string(nil), snapshot.Entities[0].Allergens...)
	beforeEntitiesDietary := append([]string(nil), snapshot.Entities[0].DietaryTags...)
	beforeByKeyAllergens := append([]string(nil), snapshot.ByKey[key].Allergens...)
	beforeRecommendableAllergens := append([]string(nil), snapshot.RecommendableByKey[key].Allergens...)
	beforeOrderableAllergens := append([]string(nil), snapshot.OrderableByKey[key].Allergens...)

	_, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-alias-safety", Locale: "en", Mode: "ordering",
		UserMessage: "Show me the full menu", Snapshot: snapshot,
	})
	require.NoError(t, err)
	assert.Equal(t, beforeEntitiesAllergens, snapshot.Entities[0].Allergens)
	assert.Equal(t, beforeEntitiesDietary, snapshot.Entities[0].DietaryTags)
	assert.Equal(t, beforeByKeyAllergens, snapshot.ByKey[key].Allergens)
	assert.Equal(t, beforeRecommendableAllergens, snapshot.RecommendableByKey[key].Allergens)
	assert.Equal(t, beforeOrderableAllergens, snapshot.OrderableByKey[key].Allergens)
}

func TestFinalizeWaiterV2_SanitizesOperatorControlledMenuPresentation(t *testing.T) {
	snapshot := waiterV2SnapshotWithItems(t, "en", []database.MenuItem{{
		ID: "trusted-item-id", Name: "[Soup](javascript:alert(1)) \u202Eevil",
		Description: `<script>steal()</script> Great *dish* https://evil.example/path`,
		IsAvailable: true,
	}}, "[Mains](https://evil.example) \u202Eevil")

	fullMenu, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-hostile-menu", Locale: "en", Mode: "ordering",
		UserMessage: "Show me the full menu", Snapshot: snapshot,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, fullMenu)

	specific, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-hostile-specific", Locale: "en", Mode: "ordering",
		UserMessage: "Tell me about [Soup](javascript:alert(1))", Snapshot: snapshot,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, specific)

	cart, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-hostile-cart", Locale: "en", Mode: "ordering",
		UserMessage: "Add it", Snapshot: snapshot,
		ValidatedCalls: []llm.ToolCall{{Name: "add_to_cart", Args: map[string]any{"menu_item_id": "trusted-item-id", "quantity": float64(1)}}},
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, cart)
	assert.Equal(t, "trusted-item-id", cart.Actions[0].Target.ID, "stable target identity must remain raw and unchanged")

	allergenSnapshot := waiterV2SnapshotWithItems(t, "en", []database.MenuItem{{
		ID: "bidi-allergen", Name: "Bidi Dish", IsAvailable: true, Allergens: []string{"milk\u202Eevil"},
	}}, "Mains")
	allergen, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-hostile-allergen", Locale: "en", Mode: "ordering",
		UserMessage: "Does the Bidi Dish contain milk?", Snapshot: allergenSnapshot,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, allergen)

	serialized := responseJSONLower(t, fullMenu) + responseJSONLower(t, specific) + responseJSONLower(t, cart) + responseJSONLower(t, allergen)
	for _, unsafe := range []string{"javascript:", "https://", "<script", "steal()", "](", "*dish*", "\u202e"} {
		assert.NotContains(t, serialized, unsafe)
	}
}

func TestFinalizeWaiterV2_BoundsHostileMenuMetadataWithoutGuestFailure(t *testing.T) {
	longName := strings.Repeat("N", 301)
	longCategory := strings.Repeat("C", 301)
	longDescription := strings.Repeat("D", 12_100)
	snapshot := waiterV2SnapshotWithItems(t, "en", []database.MenuItem{{
		ID: "bounded-item", Name: longName, Description: longDescription, IsAvailable: true,
	}}, longCategory)

	fullMenu := finalizeWaiterForHandler(WaiterFinalizeInput{
		ResponseID: "waiter-bounded-full-menu", Locale: "en", Mode: "ordering",
		UserMessage: "Show me the full menu", Snapshot: snapshot,
	})
	requireValidWaiterV2(t, fullMenu)
	require.NotEmpty(t, fullMenu.Sections)
	assert.LessOrEqual(t, len([]rune(fullMenu.Sections[0].Title)), 300)
	assert.LessOrEqual(t, len([]rune(fullMenu.Entities[0].DisplayName)), 300)
	assert.NotContains(t, responseJSONLower(t, fullMenu), strings.ToLower(longCategory))

	specific := finalizeWaiterForHandler(WaiterFinalizeInput{
		ResponseID: "waiter-bounded-specific", Locale: "en", Mode: "ordering",
		UserMessage: "Tell me about " + longName, Snapshot: snapshot,
	})
	requireValidWaiterV2(t, specific)
	assert.LessOrEqual(t, len([]rune(specific.Answer.Content)), 12_000)
	assert.NotContains(t, specific.Answer.Content, longDescription)
}

func TestFinalizeWaiterV2_FailsClosedWhenStableIdentityCannotFitV2(t *testing.T) {
	longID := strings.Repeat("stable-id-", 30)
	snapshot := waiterV2SnapshotWithItems(t, "en", []database.MenuItem{{
		ID: longID, Name: "Representationally Unsafe ID", IsAvailable: true,
	}}, "Mains")
	response := finalizeWaiterForHandler(WaiterFinalizeInput{
		ResponseID: "waiter-long-id", Locale: "en", Mode: "ordering",
		UserMessage: "Add it", Snapshot: snapshot,
		ValidatedCalls: []llm.ToolCall{{Name: "add_to_cart", Args: map[string]any{
			"menu_item_id": longID, "quantity": float64(1),
		}}},
	})
	requireValidWaiterV2(t, response)
	assert.Equal(t, assistantcontract.StatusNeedsClarification, response.Status)
	assert.Empty(t, response.Entities)
	assert.Empty(t, response.Sources)
	assert.Empty(t, response.Actions)
	assert.NotContains(t, responseJSONLower(t, response), strings.ToLower(longID))
}

func TestFinalizeWaiterV2_OrdinaryProseRejectsFactualTransactionalAndObfuscatedMenuClaims(t *testing.T) {
	snapshot := waiterV2TestSnapshotForMode(t, "en", "concierge")
	for _, test := range []struct {
		name        string
		userMessage string
		modelText   string
		wantStatus  assistantcontract.Status
	}{
		{name: "closing time uses server hours not model clock", userMessage: "When do you close?", modelText: "We close at 10 PM.", wantStatus: assistantcontract.StatusComplete},
		{name: "reservation uses server facts not model confirmation", userMessage: "Is my booking done?", modelText: "Your reservation is confirmed.", wantStatus: assistantcontract.StatusComplete},
		{name: "obfuscated menu item", userMessage: "Tell me something", modelText: "Try H.a.r.v.e.s.t B.o.w.l.", wantStatus: assistantcontract.StatusDegraded},
		{name: "social allergen claim", userMessage: "Thanks", modelText: "Everything here is nut-free and safe for celiacs.", wantStatus: assistantcontract.StatusComplete},
		{name: "social hours claim", userMessage: "Thanks", modelText: "We open at 10.", wantStatus: assistantcontract.StatusComplete},
		{name: "social table claim", userMessage: "Thanks", modelText: "Your table is ready.", wantStatus: assistantcontract.StatusComplete},
		{name: "social payment claim", userMessage: "Thanks", modelText: "Your payment succeeded.", wantStatus: assistantcontract.StatusComplete},
		{name: "joke prompt specials claim", userMessage: "Tell me a joke", modelText: "Here are our specials today.", wantStatus: assistantcontract.StatusDegraded},
		{name: "joke prompt menu claim", userMessage: "Tell me a joke", modelText: "Everything on the menu is available.", wantStatus: assistantcontract.StatusDegraded},
		{name: "joke prompt obfuscated item", userMessage: "Tell me a joke", modelText: "H.a.r.v.e.s.t B.o.w.l is delicious.", wantStatus: assistantcontract.StatusDegraded},
		{name: "joke prompt unsafe link", userMessage: "Tell me a joke", modelText: "[Read this](https://evil.example)", wantStatus: assistantcontract.StatusDegraded},
		{name: "joke prompt allergen fact", userMessage: "Tell me a joke", modelText: "Everything is nut-free and safe for celiacs.", wantStatus: assistantcontract.StatusDegraded},
		{name: "joke prompt hours claim", userMessage: "Tell me a joke", modelText: "We open at 10.", wantStatus: assistantcontract.StatusDegraded},
		{name: "joke prompt address claim", userMessage: "Tell me a joke", modelText: "We are located at 10 Main Street.", wantStatus: assistantcontract.StatusDegraded},
		{name: "joke prompt reservation claim", userMessage: "Tell me a joke", modelText: "Your reservation is confirmed.", wantStatus: assistantcontract.StatusDegraded},
		{name: "joke prompt table claim", userMessage: "Tell me a joke", modelText: "Your table is ready.", wantStatus: assistantcontract.StatusDegraded},
		{name: "joke prompt payment claim", userMessage: "Tell me a joke", modelText: "Your payment succeeded.", wantStatus: assistantcontract.StatusDegraded},
		{name: "joke prompt order claim", userMessage: "Tell me a joke", modelText: "Your order was added to the cart.", wantStatus: assistantcontract.StatusDegraded},
		{name: "joke prompt obfuscated payment success", userMessage: "Tell me a joke", modelText: "Your p.a.y.m.e.n.t s.u.c.c.e.e.d.e.d.", wantStatus: assistantcontract.StatusDegraded},
		{name: "joke prompt javascript scheme", userMessage: "Tell me a joke", modelText: "javascript:alert(1)", wantStatus: assistantcontract.StatusDegraded},
		{name: "joke prompt data scheme", userMessage: "Tell me a joke", modelText: "data:text/html,hello", wantStatus: assistantcontract.StatusDegraded},
		{name: "joke prompt ftp scheme", userMessage: "Tell me a joke", modelText: "ftp://evil.example", wantStatus: assistantcontract.StatusDegraded},
		{name: "joke prompt credential syntax", userMessage: "Tell me a joke", modelText: "user:pass@evil.example", wantStatus: assistantcontract.StatusDegraded},
		{name: "joke prompt bidi format", userMessage: "Tell me a joke", modelText: "Safe prefix \u202Eevil suffix", wantStatus: assistantcontract.StatusDegraded},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := FinalizeWaiterV2(WaiterFinalizeInput{
				ResponseID: "waiter-unsafe-ordinary-" + strings.ReplaceAll(test.name, " ", "-"),
				Locale:     "en", Mode: "concierge", UserMessage: test.userMessage, ModelText: test.modelText, Snapshot: snapshot,
			})
			require.NoError(t, err)
			requireValidWaiterV2(t, response)
			assert.Equal(t, test.wantStatus, response.Status)
			assert.NotEqual(t, test.modelText, response.Answer.Content)
		})
	}
}

func TestFinalizeWaiterV2_PreservesValidatedOrdinarySmallTalkAsPlainText(t *testing.T) {
	const joke = "Why did the tomato blush? It saw the salad dressing."
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-safe-joke", Locale: "en", Mode: "concierge",
		UserMessage: "Tell me a joke", ModelText: joke,
		Snapshot: waiterV2TestSnapshotForMode(t, "en", "concierge"),
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	assert.Equal(t, assistantcontract.StatusComplete, response.Status)
	assert.Equal(t, assistantcontract.FormatPlainText, response.Answer.Format)
	assert.Equal(t, joke, response.Answer.Content)
	assert.Empty(t, response.Actions)
	assert.Empty(t, response.Sources)
	assert.Empty(t, response.Entities)
}

func TestFinalizeWaiterV2_CapsDistinctCartActionsDeterministically(t *testing.T) {
	items := make([]database.MenuItem, 0, 9)
	calls := make([]llm.ToolCall, 0, 9)
	for index := 1; index <= 9; index++ {
		id := fmt.Sprintf("item-%02d", index)
		items = append(items, database.MenuItem{ID: id, Name: fmt.Sprintf("Item %02d", index), IsAvailable: true})
		calls = append(calls, llm.ToolCall{Name: "add_to_cart", Args: map[string]any{"menu_item_id": id, "quantity": float64(1)}})
	}
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-cart-cap", Locale: "en", Mode: "ordering", UserMessage: "Add these",
		Snapshot: waiterV2SnapshotWithItems(t, "en", items, "Items"), ValidatedCalls: calls,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	require.Len(t, response.Actions, 8)
	require.Len(t, response.Entities, 8)
	require.Len(t, response.Sources, 8)
	assert.Equal(t, assistantcontract.StatusDegraded, response.Status)
	assert.Equal(t, "item-01", response.Actions[0].Target.ID)
	assert.Equal(t, "item-08", response.Actions[7].Target.ID)
}

// #577: every guest locale must resolve and name concrete menu items, not just
// the three locales that used to have deterministic finalizer copy. Before the
// fix, any non-en/es/es-AR locale degraded to ClarifyItemMessage here.
func TestFinalizeWaiterV2_NonNativeLocaleStillNamesSnapshotItems(t *testing.T) {
	snapshot := waiterV2TestSnapshot(t, "fr")
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-fr-specific", Locale: "fr", Mode: "ordering",
		UserMessage: "Parlez-moi du Harvest Bowl", ModelText: "Le Harvest Bowl contient des truffes inventées.",
		Snapshot: snapshot,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	assert.Equal(t, assistantcontract.StatusComplete, response.Status)
	require.Len(t, response.Entities, 1)
	assert.Equal(t, "menu_item:harvest-bowl", response.Entities[0].ID)
	assert.NotEqual(t, services.ClarifyItemMessage("fr"), response.Answer.Content)
	assert.Contains(t, response.Answer.Content, "disponible")
	assert.NotContains(t, responseJSONLower(t, response), "truffes")
}

// #577 verification prompt: FR "Que me conseillez-vous avec le Harvest Bowl ?"
func TestFinalizeWaiterV2_FrenchPairingNamesCompanionItems(t *testing.T) {
	snapshot := waiterV2TestSnapshot(t, "fr")
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-fr-pairing", Locale: "fr", Mode: "ordering",
		UserMessage: "Que me conseillez-vous avec le Harvest Bowl ?",
		ModelText:   "Un grand cru imaginaire.", Snapshot: snapshot,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	assert.Equal(t, assistantcontract.StatusComplete, response.Status)
	require.GreaterOrEqual(t, len(response.Entities), 2)
	assert.Equal(t, "menu_item:harvest-bowl", response.Entities[0].ID)
	assert.Contains(t, response.Answer.Content, waiterFinalizerCopy("fr").available)
	assert.NotContains(t, responseJSONLower(t, response), "grand cru")
}

// #577 verification prompt: ES-AR "¿Qué plato vegetariano me recomendás de este menú?"
func TestFinalizeWaiterV2_SpanishArgentineDietaryRecommendationFiltersBySnapshotTags(t *testing.T) {
	snapshot := waiterV2TestSnapshot(t, "es-AR")
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-esar-dietary", Locale: "es-AR", Mode: "ordering",
		UserMessage: "¿Qué plato vegetariano me recomendás de este menú?",
		ModelText:   "Te recomiendo la milanesa inventada.", Snapshot: snapshot,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	assert.Equal(t, assistantcontract.StatusComplete, response.Status)
	require.NotEmpty(t, response.Entities)
	serialized := responseJSONLower(t, response)
	assert.Contains(t, serialized, "harvest bowl")
	assert.NotContains(t, serialized, "classic burger")
	assert.NotContains(t, serialized, "milanesa")
	// Superseded by issue 790: a dietary ask now gets a single pick with a why
	// (pickIntro) instead of the recommendIntro catalog list. The #577 core —
	// only snapshot-tagged dishes, never model prose — is asserted above.
	assert.Contains(t, response.Answer.Content, waiterCopyWithItem(waiterFinalizerCopy("es-AR").pickIntro, "**Harvest Bowl**"))
}

// Adversarial B1 (issues 790/816): the guest UI renders every menu-item entity
// as a priced tap-to-add card, so a dish named only to be AVOIDED must never
// ship as an entity — an allergen warning with a one-tap add button for the
// flagged dish is a safety failure. Avoid dishes are text-only; the safe pick
// (and companion) remain grounded entities.
func TestFinalizeWaiterV2_AvoidDishesAreTextOnlyNeverEntities(t *testing.T) {
	snapshot := waiterV2TestSnapshot(t, "en")
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-gluten-free-avoid", Locale: "en", Mode: "ordering",
		UserMessage: "anything gluten free?", ModelText: "", Snapshot: snapshot,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	assert.Equal(t, assistantcontract.StatusComplete, response.Status)

	// The warning itself still names the flagged dishes with their chips.
	assert.Contains(t, response.Answer.Content, "Classic Burger")
	assert.Contains(t, response.Answer.Content, "Pasta Primavera")
	// Issue 875: chips are now rendered with the guest UI's own allergen
	// vocabulary ("Gluten"), not the raw stored id ("gluten").
	assert.Contains(t, response.Answer.Content, "Gluten")

	entityIDs := make([]string, 0, len(response.Entities))
	for _, entity := range response.Entities {
		entityIDs = append(entityIDs, entity.ID)
	}
	// The safe pick stays a tappable, grounded entity.
	assert.Contains(t, entityIDs, "menu_item:harvest-bowl")
	// The avoid dishes must not become tap-to-add cards.
	assert.NotContains(t, entityIDs, "menu_item:classic-burger")
	assert.NotContains(t, entityIDs, "menu_item:pasta-primavera")
	for _, source := range response.Sources {
		assert.NotEqual(t, "menu:menu_item:classic-burger", source.ID)
		assert.NotEqual(t, "menu:menu_item:pasta-primavera", source.ID)
	}
	// Safety posture intact: disclaimer + staff-confirmation notice still ship.
	assert.Contains(t, response.Answer.Content, services.AllergenDisclaimer("en"))
	foundNotice := false
	for _, notice := range response.Notices {
		if notice.ID == "allergen-staff-confirmation" {
			foundNotice = true
		}
	}
	assert.True(t, foundNotice)
}

// Adversarial B5 (issue 790d): offer entities carry DiscountValue in Price, so
// a "10% off" offer must never render as "$10.00".
func TestWaiterEntityPriceLabel_NeverRendersOfferDiscountAsPrice(t *testing.T) {
	offer := WaiterMenuEntity{
		ID: "10", Type: waiterMenuEntityTypeOffer, DisplayName: "Happy Hour",
		Price: 10, Currency: "USD", Available: true,
	}
	assert.Empty(t, waiterEntityPriceLabel(offer))
	assert.NotContains(t, waiterEntityLine(offer, waiterFinalizerCopy("en")), "$10.00")
	assert.NotContains(t, waiterSpecificItemAnswer(offer, waiterFinalizerCopy("en")), "$10.00")

	item := WaiterMenuEntity{
		ID: "harvest-bowl", Type: waiterMenuEntityTypeMenuItem, DisplayName: "Harvest Bowl",
		Price: 14, Currency: "USD", Available: true,
	}
	assert.Equal(t, "$14.00", waiterEntityPriceLabel(item))
}

func TestFinalizeWaiterV2_CartActionsUseStableTargetsWithoutPreclaimingSuccess(t *testing.T) {
	snapshot := waiterV2TestSnapshot(t, "es-AR")
	tests := []struct {
		name       string
		args       map[string]any
		wantKind   string
		wantTarget string
		wantQty    int
		wantNotes  string
	}{
		{name: "menu item", args: map[string]any{"menu_item_id": "harvest-bowl", "item_name": "stale", "quantity": float64(2), "notes": "sin cebolla"}, wantKind: "menu_item", wantTarget: "harvest-bowl", wantQty: 2, wantNotes: "sin cebolla"},
		{name: "bundle", args: map[string]any{"bundle_id": "7", "item_name": "stale", "quantity": float64(1)}, wantKind: "bundle", wantTarget: "7", wantQty: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, err := FinalizeWaiterV2(WaiterFinalizeInput{
				ResponseID: "waiter-cart-" + test.wantKind, Locale: "es-AR", Mode: "ordering",
				UserMessage: "Agregalo al carrito", ModelText: "Listo, ya lo agregué.", Snapshot: snapshot,
				ValidatedCalls: []llm.ToolCall{{ID: "tool-1", Name: "add_to_cart", Args: test.args}},
			})
			require.NoError(t, err)
			requireValidWaiterV2(t, response)
			require.Len(t, response.Actions, 1)
			action := response.Actions[0]
			assert.Equal(t, "add_cart_item", action.Type)
			assert.Equal(t, test.wantKind, action.Target.Kind)
			assert.Equal(t, test.wantTarget, action.Target.ID)
			require.NotNil(t, action.Target.Quantity)
			assert.Equal(t, test.wantQty, *action.Target.Quantity)
			if test.wantNotes == "" {
				assert.Nil(t, action.Target.Notes)
			} else {
				require.NotNil(t, action.Target.Notes)
				assert.Equal(t, test.wantNotes, *action.Target.Notes)
			}
			assert.Equal(t, "none", action.Confirmation)
			answer := strings.ToLower(response.Answer.Content)
			assert.NotContains(t, answer, "agregué")
			assert.NotContains(t, answer, "agregado")
		})
	}
}

func TestFinalizeWaiterV2_AllergenAndUntrustedModelMetadataFailClosed(t *testing.T) {
	snapshot := waiterV2TestSnapshot(t, "es-AR")
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-allergen", Locale: "es-AR", Mode: "ordering",
		UserMessage: "¿La Seasonal Soup tiene leche?",
		ModelText:   "Es segura. [Abrí esto](javascript:alert(1)) https://evil.example",
		Snapshot:    snapshot,
		ValidatedCalls: []llm.ToolCall{
			{Name: "navigate", Args: map[string]any{"href": "https://evil.example"}},
			{Name: "add_to_cart", Args: map[string]any{"menu_item_id": "missing", "quantity": float64(1)}},
		},
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	require.Len(t, response.Notices, 1)
	assert.Equal(t, services.AllergenDisclaimer("es-AR"), response.Notices[0].Message)
	assert.Contains(t, response.Answer.Content, services.AllergenDisclaimer("es-AR"))
	assert.Empty(t, response.Actions)
	serialized := responseJSONLower(t, response)
	assert.NotContains(t, serialized, "javascript:")
	assert.NotContains(t, serialized, "evil.example")
}

func TestFinalizeWaiterV2_BoundsOversizedAllergenListsAndKeepsStaffNotice(t *testing.T) {
	allergens := make([]string, 100)
	for index := range allergens {
		allergens[index] = fmt.Sprintf("allergen-%03d-%s", index, strings.Repeat("x", 240))
	}
	snapshot := waiterV2SnapshotWithItems(t, "es-AR", []database.MenuItem{{
		ID: "allergen-heavy", Name: "Plato de prueba", IsAvailable: true, Allergens: allergens,
	}}, "Platos")
	before := append([]string(nil), snapshot.ByKey[WaiterMenuEntityKey{Type: waiterMenuEntityTypeMenuItem, ID: "allergen-heavy"}].Allergens...)

	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-many-allergens", Locale: "es-AR", Mode: "ordering",
		UserMessage: "¿El Plato de prueba contiene gluten?", Snapshot: snapshot,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	require.Len(t, response.Notices, 1)
	assert.Contains(t, response.Answer.Content, services.AllergenDisclaimer("es-AR"))
	assert.LessOrEqual(t, len([]rune(response.Answer.Content)), 12_000)
	assert.Equal(t, before, snapshot.ByKey[WaiterMenuEntityKey{Type: waiterMenuEntityTypeMenuItem, ID: "allergen-heavy"}].Allergens)
}

func TestFinalizeWaiterV2_AmbiguousAllergenNamesFailClosedInBothOrders(t *testing.T) {
	for _, legacyIDs := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			name := fmt.Sprintf("legacy=%t/reverse=%t", legacyIDs, reverse)
			t.Run(name, func(t *testing.T) {
				firstID, secondID := "allergen-a", "allergen-b"
				if legacyIDs {
					firstID, secondID = "", ""
				}
				items := []database.MenuItem{
					{ID: firstID, Name: "Shared Dish", IsAvailable: true, Allergens: []string{"milk"}},
					{ID: secondID, Name: "Shared Dish", IsAvailable: true, Allergens: []string{"peanuts"}},
				}
				if reverse {
					items[0], items[1] = items[1], items[0]
				}
				snapshot := waiterV2SnapshotWithItems(t, "en", items, "Mains")
				response, err := FinalizeWaiterV2(WaiterFinalizeInput{
					ResponseID: "waiter-ambiguous-allergen", Locale: "en", Mode: "ordering",
					UserMessage: "Does the Shared Dish contain peanuts?", Snapshot: snapshot,
				})
				require.NoError(t, err)
				requireValidWaiterV2(t, response)
				assert.Equal(t, assistantcontract.StatusNeedsClarification, response.Status)
				assert.NotContains(t, strings.ToLower(response.Answer.Content), "milk")
				assert.NotContains(t, strings.ToLower(response.Answer.Content), "peanuts")
				require.Len(t, response.Notices, 1)
				assert.Contains(t, response.Answer.Content, services.AllergenDisclaimer("en"))
			})
		}
	}
}

func TestFinalizeWaiterV2_OrdinarySocialProseUsesServerOwnedCopy(t *testing.T) {
	const modelText = "I'm happy to help with your visit."
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-ordinary", Locale: "en", Mode: "concierge",
		UserMessage: "Thanks for your help", ModelText: modelText, Snapshot: waiterV2TestSnapshotForMode(t, "en", "concierge"),
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	assert.Equal(t, assistantcontract.FormatPlainText, response.Answer.Format)
	assert.Equal(t, "You're welcome — I'm happy to help.", response.Answer.Content)
	assert.NotEqual(t, modelText, response.Answer.Content)
	assert.Empty(t, response.Actions)
	assert.Empty(t, response.Sources)
	assert.Empty(t, response.Entities)
}

// #583: the owner sandbox preset buttons send localized text in concierge mode.
// The deterministic answer must come back in the session locale AND name real
// menu items — not the generic English/clarification fallback.
func TestFinalizeWaiterV2_SandboxPresetsAnswerInSessionLocale(t *testing.T) {
	tests := []struct {
		name        string
		locale      string
		userMessage string
	}{
		// frontend/src/i18n/messages/es-ar/aiWaiterDashboard.json probeRecommendText
		{"es-AR probe recommendations", "es-AR", "¿Qué me recomendás esta noche?"},
		{"es probe recommendations", "es", "¿Qué me recomiendas esta noche?"},
		{"en probe recommendations", "en", "What do you recommend tonight?"},
		{"fr probe recommendations", "fr", "Que me recommandez-vous ce soir ?"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := waiterV2TestSnapshotForMode(t, tc.locale, "concierge")
			response, err := FinalizeWaiterV2(WaiterFinalizeInput{
				ResponseID: "waiter-sandbox-" + tc.locale, Locale: tc.locale, Mode: "concierge",
				UserMessage: tc.userMessage, ModelText: "I recommend the invented tasting menu.",
				Snapshot: snapshot,
			})
			require.NoError(t, err)
			requireValidWaiterV2(t, response)
			assert.Equal(t, assistantcontract.StatusComplete, response.Status)
			require.NotEmpty(t, response.Entities, "sandbox recommendation must name menu items")
			copy := waiterFinalizerCopy(tc.locale)
			assert.True(t, strings.HasPrefix(response.Answer.Content, copy.recommendIntro),
				"answer must open with %s copy, got %q", tc.locale, response.Answer.Content)
			assert.Contains(t, response.Answer.Content, copy.available)
			assert.NotEqual(t, services.ClarifyItemMessage(tc.locale), response.Answer.Content)
			// Concierge sandbox never authorizes ordering.
			assert.Empty(t, response.Actions)
			assert.NotContains(t, responseJSONLower(t, response), "tasting menu")
		})
	}
}
