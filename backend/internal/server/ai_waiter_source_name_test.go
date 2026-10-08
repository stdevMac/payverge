package server

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sourceNameSnapshot mirrors the showroom carta: the menu rows are Spanish and a
// stored English translation layer is what an ?lang=en guest asks against.
func sourceNameSnapshot(t *testing.T, locale string) WaiterMenuSnapshot {
	t.Helper()
	build := func(en bool) []database.MenuCategory {
		name := func(es, enName string) string {
			if en {
				return enName
			}
			return es
		}
		return []database.MenuCategory{
			{ID: "parrilla", Name: name("De la parrilla", "From the grill"), SortOrder: 1, Items: []database.MenuItem{
				{ID: "demo-ojo-de-bife", Name: name("Ojo de bife", "Ribeye"), Description: name("A la parrilla", "From the grill"), Price: 39500, Currency: "ARS", IsAvailable: true, SortOrder: 1},
				{ID: "demo-entrana", Name: name("Entraña", "Skirt steak"), Price: 31500, Currency: "ARS", IsAvailable: true, SortOrder: 2},
			}},
			{ID: "bodega", Name: name("Bodega y barra", "Wine and bar"), SortOrder: 2, Items: []database.MenuItem{
				{ID: "demo-agua", Name: name("Agua sin gas", "Still water"), Price: 3500, Currency: "ARS", IsAvailable: true, SortOrder: 1},
			}},
		}
	}
	snapshot, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
		Business:     &database.Business{ID: 142, DefaultLanguage: "es", IsActive: true, KitchenEnabled: true, OrdersEnabled: true},
		Locale:       locale,
		Mode:         "ordering",
		BusinessOpen: true,
		Categories:   build(locale == "en"), SourceCategories: build(false),
		HiddenItemIDs: map[string]bool{},
	})
	require.NoError(t, err)
	return snapshot
}

// Issue 869: the English waiter answered "Ribeye" / "Still water" while the
// guest card still reads "Ojo de bife" / "Agua sin gas", so guest and waiter
// were not talking about the same dish. Prose must name the dish as it is
// printed on the card.
func TestFinalizeWaiterV2_TranslatedAnswerNamesTheCardDish(t *testing.T) {
	snapshot := sourceNameSnapshot(t, "en")

	t.Run("specific item", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-869-item", Locale: "en", Mode: "ordering",
			UserMessage: "What is the ribeye?", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.Contains(t, response.Answer.Content, "Ribeye")
		assert.Contains(t, response.Answer.Content, "Ojo de bife", "the guest must be able to find the dish on the card")
		// Identity stays canonical: the parenthetical is prose only.
		require.Len(t, response.Entities, 1)
		assert.Equal(t, "Ribeye", response.Entities[0].DisplayName)
		require.Len(t, response.Sources, 1)
		assert.Equal(t, "Ribeye", response.Sources[0].Title)
	})

	t.Run("recommendation list", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-869-list", Locale: "en", Mode: "ordering",
			UserMessage: "What do you recommend?", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.Contains(t, response.Answer.Content, "Ojo de bife")
	})

	t.Run("cart action", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-869-cart", Locale: "en", Mode: "ordering",
			UserMessage: "Add a still water please", Snapshot: snapshot,
			ValidatedCalls: []llm.ToolCall{{Name: "add_to_cart", Args: map[string]any{"menu_item_id": "demo-agua", "quantity": float64(1)}}},
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		require.Len(t, response.Actions, 1)
		assert.Equal(t, "demo-agua", response.Actions[0].Target.ID, "the action still targets the canonical menu ID")
		assert.Contains(t, response.Actions[0].Label, "Agua sin gas")
		assert.Contains(t, response.Answer.Content, "Agua sin gas")
	})
}

// A venue answering in its own language must not repeat itself.
func TestFinalizeWaiterV2_SourceLanguageAnswerIsNotDoubled(t *testing.T) {
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-869-es", Locale: "es", Mode: "ordering",
		UserMessage: "¿Cuánto cuesta el ojo de bife?", Snapshot: sourceNameSnapshot(t, "es"),
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	assert.Contains(t, response.Answer.Content, "**Ojo de bife**")
	assert.NotContains(t, response.Answer.Content, "(Ojo de bife)")
}

// The source name is snapshot-owned identity, so it must survive onto every
// entity projection the finalizer reads from.
func TestBuildWaiterMenuSnapshot_KeepsSourceName(t *testing.T) {
	snapshot := sourceNameSnapshot(t, "en")
	key := WaiterMenuEntityKey{Type: waiterMenuEntityTypeMenuItem, ID: "demo-ojo-de-bife"}

	entity, ok := snapshot.ByKey[key]
	require.True(t, ok)
	assert.Equal(t, "Ribeye", entity.DisplayName)
	assert.Equal(t, "Ojo de bife", entity.SourceName)
	assert.Equal(t, "Ojo de bife", snapshot.OrderableByKey[key].SourceName)
	assert.Equal(t, "Ojo de bife", snapshot.RecommendableByKey[key].SourceName)

	// An untranslated menu carries no second name to print.
	native := sourceNameSnapshot(t, "es")
	assert.Empty(t, waiterEntityProseSuffix(native.ByKey[key]))
}
