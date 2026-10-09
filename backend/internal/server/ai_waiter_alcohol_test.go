package server

import (
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// alcoholCartaSnapshot mirrors the showroom carta that produced the live
// answer: a vegetarian salad, a wine by the glass, and a soft drink.
func alcoholCartaSnapshot(t *testing.T, locale string) WaiterMenuSnapshot {
	t.Helper()
	categories := []database.MenuCategory{
		{ID: "entradas", Name: "Entradas", SortOrder: 1, Items: []database.MenuItem{
			{ID: "demo-ensalada", Name: "Ensalada de estación", Price: 11500, Currency: "ARS", IsAvailable: true, DietaryTags: []string{"vegetarian"}, SortOrder: 1},
		}},
		{ID: "parrilla", Name: "De la parrilla", SortOrder: 2, Items: []database.MenuItem{
			{ID: "demo-ojo-de-bife", Name: "Ojo de bife", Price: 39500, Currency: "ARS", IsAvailable: true, SortOrder: 1},
		}},
		{ID: "vinos", Name: "Vinos", SortOrder: 3, Items: []database.MenuItem{
			{ID: "demo-malbec", Name: "Copa de Malbec", Description: "Mendoza, cosecha 2021", Price: 8500, Currency: "ARS", IsAvailable: true, DietaryTags: []string{"vegetarian", "vegan"}, SortOrder: 1},
		}},
		{ID: "bebidas", Name: "Bebidas", SortOrder: 4, Items: []database.MenuItem{
			{ID: "demo-limonada", Name: "Limonada casera", Price: 4200, Currency: "ARS", IsAvailable: true, DietaryTags: []string{"vegetarian", "vegan"}, SortOrder: 1},
		}},
	}
	snapshot, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
		Business:     &database.Business{ID: 142, DefaultLanguage: "es", IsActive: true, KitchenEnabled: true, OrdersEnabled: true},
		Locale:       locale,
		Mode:         "ordering",
		BusinessOpen: true,
		Categories:   categories, SourceCategories: categories,
		HiddenItemIDs: map[string]bool{},
	})
	require.NoError(t, err)
	return snapshot
}

// Issue 941: a combined kids / vegetarian / pregnancy ask was answered with
// "Ensalada de estación" paired with a "Copa de Malbec". The deterministic
// finalizer had no alcohol awareness at all: the companion is simply the first
// candidate from another category, and wine is tagged vegetarian.
func TestFinalizeWaiterV2_PregnancyAndKidsAskNeverSuggestsAlcohol(t *testing.T) {
	for _, tc := range []struct {
		name    string
		locale  string
		message string
	}{
		{"pregnancy", "es", "una de nosotras está embarazada, ¿qué opción vegetarian nos recomendás?"},
		{"kids", "es", "venimos con los chicos, ¿qué opción vegetarian tienen?"},
		{"explicit alcohol free", "es", "algo vegetarian y sin alcohol, por favor"},
		{"english pregnancy", "en", "my wife is pregnant, any vegetarian option?"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := alcoholCartaSnapshot(t, tc.locale)
			response, err := FinalizeWaiterV2(WaiterFinalizeInput{
				ResponseID: "waiter-941-" + tc.name, Locale: tc.locale, Mode: "ordering",
				UserMessage: tc.message, Snapshot: snapshot,
			})
			require.NoError(t, err)
			requireValidWaiterV2(t, response)
			lower := strings.ToLower(response.Answer.Content)
			assert.NotContains(t, lower, "malbec")
			assert.NotContains(t, lower, "copa de")
			entityIDs := make([]string, 0, len(response.Entities))
			for _, entity := range response.Entities {
				entityIDs = append(entityIDs, entity.ID)
			}
			// A wine must not ship as a one-tap add card either.
			assert.NotContains(t, entityIDs, "menu_item:demo-malbec")
			require.NotEmpty(t, response.Entities, "the guest still gets a real pick")
		})
	}
}

// No regression: an ordinary ask still gets the wine.
func TestFinalizeWaiterV2_OrdinaryRecommendationStillOffersWine(t *testing.T) {
	snapshot := alcoholCartaSnapshot(t, "es")
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-941-ordinary", Locale: "es", Mode: "ordering",
		UserMessage: "¿qué opción vegetarian me recomendás?", Snapshot: snapshot,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	assert.Contains(t, strings.ToLower(response.Answer.Content), "malbec")
}

func TestWaiterAlcoholFreeAsk(t *testing.T) {
	for _, tc := range []struct {
		message string
		want    bool
	}{
		{"una de nosotras está embarazada", true},
		{"estoy embarazada", true},
		{"venimos con los chicos", true},
		{"algo para los niños", true},
		{"my wife is pregnant", true},
		{"anything for the kids?", true},
		{"something non-alcoholic please", true},
		{"algo sin alcohol", true},
		{"¿qué vino me recomendás?", false},
		{"¿qué me recomendás?", false},
		{"", false},
	} {
		t.Run(tc.message, func(t *testing.T) {
			assert.Equal(t, tc.want, waiterAlcoholFreeAsk(tc.message))
		})
	}
}

func TestWaiterEntityIsAlcoholic(t *testing.T) {
	snapshot := alcoholCartaSnapshot(t, "es")
	categoryNames := waiterCategoryNamesByID(snapshot)
	for _, tc := range []struct {
		name   string
		entity WaiterMenuEntity
		want   bool
	}{
		{"wine by name", WaiterMenuEntity{DisplayName: "Copa de Malbec", CategoryID: "vinos"}, true},
		{"beer by name", WaiterMenuEntity{DisplayName: "Cerveza artesanal"}, true},
		{"wine only in the source name", WaiterMenuEntity{DisplayName: "House pour", SourceName: "Copa de vino tinto"}, true},
		{"alcohol only in the description", WaiterMenuEntity{DisplayName: "Postre del día", Description: "Flan con whisky"}, true},
		{"alcohol only in the category", WaiterMenuEntity{DisplayName: "Selección del sommelier", CategoryID: "vinos"}, true},
		{"soft drink", WaiterMenuEntity{DisplayName: "Limonada casera", CategoryID: "bebidas"}, false},
		{"salad", WaiterMenuEntity{DisplayName: "Ensalada de estación", CategoryID: "entradas"}, false},
		// "vino" must not fire inside "vinagre".
		{"vinaigrette is not wine", WaiterMenuEntity{DisplayName: "Ensalada", Description: "con vinagre de manzana"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, waiterEntityIsAlcoholic(tc.entity, categoryNames))
		})
	}
}

// wineForwardCartaSnapshot is the carta that broke the first 941 fix: the wine
// list sorts first, every wine carries the vegetarian and vegan tags, and there
// are more wines than maxWaiterRecommendationEntities. Truncating to the top 3
// before filtering therefore leaves nothing at all.
func wineForwardCartaSnapshot(t *testing.T, locale string) WaiterMenuSnapshot {
	t.Helper()
	wineTags := []string{"vegetarian", "vegan"}
	categories := []database.MenuCategory{
		{ID: "vinos", Name: "Vinos", SortOrder: 1, Items: []database.MenuItem{
			{ID: "demo-malbec", Name: "Copa de Malbec", Description: "Mendoza, cosecha 2021", Price: 8500, Currency: "ARS", IsAvailable: true, DietaryTags: wineTags, SortOrder: 1},
			{ID: "demo-cabernet", Name: "Copa de Cabernet Sauvignon", Price: 9200, Currency: "ARS", IsAvailable: true, DietaryTags: wineTags, SortOrder: 2},
			{ID: "demo-torrontes", Name: "Copa de Torrontés", Price: 7800, Currency: "ARS", IsAvailable: true, DietaryTags: wineTags, SortOrder: 3},
			{ID: "demo-espumante", Name: "Copa de espumante", Price: 9900, Currency: "ARS", IsAvailable: true, DietaryTags: wineTags, SortOrder: 4},
		}},
		{ID: "entradas", Name: "Entradas", SortOrder: 2, Items: []database.MenuItem{
			{ID: "demo-ensalada", Name: "Ensalada de estación", Price: 11500, Currency: "ARS", IsAvailable: true, DietaryTags: []string{"vegetarian", "vegan"}, SortOrder: 1},
		}},
		{ID: "parrilla", Name: "De la parrilla", SortOrder: 3, Items: []database.MenuItem{
			{ID: "demo-ojo-de-bife", Name: "Ojo de bife", Price: 39500, Currency: "ARS", IsAvailable: true, SortOrder: 1},
		}},
		{ID: "bebidas", Name: "Bebidas", SortOrder: 4, Items: []database.MenuItem{
			{ID: "demo-limonada", Name: "Limonada casera", Price: 4200, Currency: "ARS", IsAvailable: true, DietaryTags: []string{"vegetarian", "vegan"}, SortOrder: 1},
		}},
	}
	snapshot, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
		Business:     &database.Business{ID: 142, DefaultLanguage: "es", IsActive: true, KitchenEnabled: true, OrdersEnabled: true},
		Locale:       locale,
		Mode:         "ordering",
		BusinessOpen: true,
		Categories:   categories, SourceCategories: categories,
		HiddenItemIDs: map[string]bool{},
	})
	require.NoError(t, err)
	return snapshot
}

func waiterResponseEntityIDs(response assistantcontract.Response) []string {
	ids := make([]string, 0, len(response.Entities))
	for _, entity := range response.Entities {
		ids = append(ids, entity.ID)
	}
	return ids
}

// requireNoAlcoholNamed asserts that neither the prose nor the structured cards
// name a drink from the wine list — a card is a one-tap ADD button, so an
// alcohol-free answer has to be clean in both places.
func requireNoAlcoholNamed(t *testing.T, response assistantcontract.Response) {
	t.Helper()
	lower := strings.ToLower(response.Answer.Content)
	for _, word := range []string{"malbec", "cabernet", "torrontés", "torrontes", "espumante", "copa de"} {
		assert.NotContains(t, lower, word, "alcohol named in prose")
	}
	ids := waiterResponseEntityIDs(response)
	for _, id := range []string{"menu_item:demo-malbec", "menu_item:demo-cabernet", "menu_item:demo-torrontes", "menu_item:demo-espumante"} {
		assert.NotContains(t, ids, id, "alcohol shipped as a tap-to-add card")
	}
}

// Issue 941, defect 1: the alcohol filter ran after RecommendableEntities had
// already truncated to the top 3. On a wine-forward carta those 3 are all wine,
// so the filter emptied the slice and a pregnant guest was told "No encontré eso
// en el menú" — worse than the pre-fix answer, which at least included food.
func TestFinalizeWaiterV2_AlcoholFreeAskSurvivesWineForwardCarta(t *testing.T) {
	snapshot := wineForwardCartaSnapshot(t, "es")
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-941-wine-forward", Locale: "es", Mode: "ordering",
		UserMessage: "estoy embarazada, ¿qué opción vegetarian me recomendás?", Snapshot: snapshot,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	assert.NotEqual(t, assistantcontract.StatusNeedsClarification, response.Status,
		"a wine-forward carta must not empty the pick: %q", response.Answer.Content)
	require.NotEmpty(t, response.Entities, "the guest still gets a real pick")
	requireNoAlcoholNamed(t, response)
	assert.Contains(t, waiterResponseEntityIDs(response), "menu_item:demo-ensalada")
}

// Issue 941, defect 2: finalizeWaiterPairing never received the guest's message,
// so the pairing path stayed completely unfiltered — "estoy embarazada, ¿qué va
// bien con el ojo de bife?" answered with a Copa de Malbec.
func TestFinalizeWaiterV2_PregnancyPairingNeverPoursWine(t *testing.T) {
	for _, tc := range []struct {
		name    string
		message string
	}{
		{"pregnancy", "estoy embarazada, ¿qué va bien con el ojo de bife?"},
		{"kids", "venimos con los chicos, ¿qué va bien con el ojo de bife?"},
		{"explicit", "sin alcohol por favor, ¿qué va bien con el ojo de bife?"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := wineForwardCartaSnapshot(t, "es")
			response, err := FinalizeWaiterV2(WaiterFinalizeInput{
				ResponseID: "waiter-941-pairing-" + tc.name, Locale: "es", Mode: "ordering",
				UserMessage: tc.message, Snapshot: snapshot,
			})
			require.NoError(t, err)
			requireValidWaiterV2(t, response)
			requireNoAlcoholNamed(t, response)
			assert.NotEqual(t, assistantcontract.StatusNeedsClarification, response.Status,
				"a non-alcoholic pairing exists: %q", response.Answer.Content)
		})
	}
}

// No regression: an unrestricted guest asking for a pairing still gets the wine.
func TestFinalizeWaiterV2_OrdinaryPairingStillOffersWine(t *testing.T) {
	snapshot := wineForwardCartaSnapshot(t, "es")
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-941-pairing-ordinary", Locale: "es", Mode: "ordering",
		UserMessage: "¿qué va bien con el ojo de bife?", Snapshot: snapshot,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	assert.Contains(t, strings.ToLower(response.Answer.Content), "malbec")
	assert.Contains(t, waiterResponseEntityIDs(response), "menu_item:demo-malbec")
}

// The substitute path has the same shape as the pairing path and the same gap.
func TestFinalizeWaiterV2_AlcoholFreeAlternativesNeverPourWine(t *testing.T) {
	snapshot := wineForwardCartaSnapshot(t, "es")
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-941-alternatives", Locale: "es", Mode: "ordering",
		UserMessage: "estoy embarazada, ¿qué otra opción tienen en lugar del ojo de bife?", Snapshot: snapshot,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	requireNoAlcoholNamed(t, response)
}
