package server

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// multiItemPriceSnapshot mirrors the showroom carta: Spanish source rows with an
// English translation layer, so a guest asking in either language exercises the
// same alias index production uses.
func multiItemPriceSnapshot(t *testing.T, locale string) WaiterMenuSnapshot {
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
				{ID: "demo-ojo-de-bife", Name: name("Ojo de bife", "Ribeye"), Price: 39500, Currency: "ARS", IsAvailable: true, SortOrder: 1},
				{ID: "demo-parrillada", Name: name("Parrillada para dos", "Mixed grill for two"), Price: 68000, Currency: "ARS", IsAvailable: true, SortOrder: 2},
			}},
			{ID: "guarniciones", Name: name("Guarniciones y ensaladas", "Sides and salads"), SortOrder: 2, Items: []database.MenuItem{
				{ID: "demo-ensalada", Name: name("Ensalada mixta", "Mixed salad"), Price: 12500, Currency: "ARS", IsAvailable: true, SortOrder: 1},
			}},
			{ID: "bodega", Name: name("Bodega y barra", "Wine and bar"), SortOrder: 3, Items: []database.MenuItem{
				{ID: "demo-malbec-copa", Name: name("Copa de Malbec", "Malbec by the glass"), Price: 6500, Currency: "ARS", IsAvailable: true, SortOrder: 1},
				{ID: "demo-malbec-botella", Name: name("Malbec (botella)", "Malbec (bottle)"), Price: 28500, Currency: "ARS", IsAvailable: true, SortOrder: 2},
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

// Issue 868: asking the price of several dishes in one sentence resolved every
// name correctly and then threw the result away — the guest got "No encontré
// eso en el menú". Each named dish must come back with its snapshot price.
func TestFinalizeWaiterV2_MultiItemPriceAskListsEveryNamedItem(t *testing.T) {
	t.Run("es", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-868-es", Locale: "es", Mode: "ordering",
			UserMessage: "¿Cuánto salen el ojo de bife, la parrillada y la copa de malbec?",
			Snapshot:    multiItemPriceSnapshot(t, "es"),
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.Equal(t, assistantcontract.StatusComplete, response.Status)
		assert.NotContains(t, response.Answer.Content, "No encontré eso en el menú")
		for _, want := range []string{
			"Ojo de bife", "ARS 39500.00",
			"Parrillada para dos", "ARS 68000.00",
			"Copa de Malbec", "ARS 6500.00",
		} {
			assert.Contains(t, response.Answer.Content, want)
		}
		// The guest asked for the glass, so the bottle row must not be padded in.
		assert.NotContains(t, response.Answer.Content, "Malbec (botella)")
		assert.Len(t, response.Entities, 3, "every listed dish must be a grounded entity")
		assert.Len(t, response.Sources, 3, "every listed dish must carry its menu source")
		assert.Empty(t, response.Actions, "a price question never orders anything")
	})

	t.Run("en", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-868-en", Locale: "en", Mode: "ordering",
			UserMessage: "How much is the ribeye, the mixed grill and the malbec?",
			Snapshot:    multiItemPriceSnapshot(t, "en"),
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.Equal(t, assistantcontract.StatusComplete, response.Status)
		for _, want := range []string{"Ribeye", "ARS 39500.00", "Mixed grill for two", "ARS 68000.00", "Malbec by the glass"} {
			assert.Contains(t, response.Answer.Content, want)
		}
		// "mixed" is a one-token alias of Mixed salad; the guest asked about the
		// mixed grill, so the salad must never be quoted back as an answer.
		assert.NotContains(t, response.Answer.Content, "Mixed salad")
	})
}

// The weaker match is only dropped when a longer matched name literally
// contains it — two dishes the guest named separately both survive.
func TestFinalizeWaiterV2_MultiItemPriceKeepsDistinctlyNamedDishes(t *testing.T) {
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-868-distinct", Locale: "es", Mode: "ordering",
		UserMessage: "¿Cuánto cuestan la ensalada mixta y la parrillada para dos?",
		Snapshot:    multiItemPriceSnapshot(t, "es"),
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	assert.Contains(t, response.Answer.Content, "Ensalada mixta")
	assert.Contains(t, response.Answer.Content, "Parrillada para dos")
	assert.Len(t, response.Entities, 2)
}

// sameNameSnapshot carries two rows the kitchen tells apart by id and the guest
// cannot tell apart at all: one display name, two prices.
func sameNameSnapshot(t *testing.T) WaiterMenuSnapshot {
	t.Helper()
	categories := []database.MenuCategory{{ID: "principales", Name: "Principales", SortOrder: 1, Items: []database.MenuItem{
		{ID: "house-salad-a", Name: "Ensalada de la Casa", Price: 1200, Currency: "USD", IsAvailable: true, SortOrder: 1},
		{ID: "house-salad-b", Name: "Ensalada de la Casa", Price: 1300, Currency: "USD", IsAvailable: true, SortOrder: 2},
	}}}
	snapshot, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
		Business:     &database.Business{ID: 143, DefaultLanguage: "es", IsActive: true, KitchenEnabled: true, OrdersEnabled: true},
		Locale:       "es",
		Mode:         "ordering",
		BusinessOpen: true,
		Categories:   categories, SourceCategories: categories,
		HiddenItemIDs: map[string]bool{},
	})
	require.NoError(t, err)
	return snapshot
}

// Two menu rows sharing one display name are an ambiguity, not the multi-item
// question issue 868 exists to price. Listing both prints the same name twice
// with two prices, the guest still cannot say which one they meant, and the
// cart validator refuses the name for exactly that reason — so the turn has to
// ask, not answer.
func TestFinalizeWaiterV2_SameNamedMatchesAskForClarification(t *testing.T) {
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-868-same-name", Locale: "es", Mode: "ordering",
		UserMessage: "¿Qué tal la Ensalada de la Casa?",
		Snapshot:    sameNameSnapshot(t),
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	assert.Equal(t, assistantcontract.StatusNeedsClarification, response.Status)
	assert.Contains(t, response.Answer.Content, "No encontré")
	assert.Empty(t, response.Entities, "an ambiguous name must not ground either row")
	assert.Empty(t, response.Sources)
	assert.Empty(t, response.Actions)
}

// Guardrail: the single-item answer (name, price, availability, description)
// must not change shape.
func TestFinalizeWaiterV2_SingleItemPriceAskUnchanged(t *testing.T) {
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-868-single", Locale: "es", Mode: "ordering",
		UserMessage: "¿Cuánto cuesta el ojo de bife?",
		Snapshot:    multiItemPriceSnapshot(t, "es"),
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	assert.Equal(t, assistantcontract.StatusComplete, response.Status)
	assert.Contains(t, response.Answer.Content, "**Ojo de bife**")
	assert.Contains(t, response.Answer.Content, "ARS 39500.00")
	assert.Len(t, response.Entities, 1)
}
