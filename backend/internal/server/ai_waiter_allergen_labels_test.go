package server

import (
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allergenChipSnapshot is the showroom carta: canonical English allergen ids on
// Spanish dishes, exactly as demo business 142 stores them.
func allergenChipSnapshot(t *testing.T, locale string) WaiterMenuSnapshot {
	t.Helper()
	categories := []database.MenuCategory{
		{ID: "entradas", Name: "Entradas", SortOrder: 1, Items: []database.MenuItem{
			{ID: "demo-empanadas", Name: "Empanada de carne", Price: 2900, Currency: "ARS", IsAvailable: true, Allergens: []string{"gluten", "eggs"}, SortOrder: 1},
			{ID: "demo-provoleta", Name: "Provoleta", Price: 9800, Currency: "ARS", IsAvailable: true, Allergens: []string{"dairy"}, DietaryTags: []string{"vegetarian"}, SortOrder: 2},
		}},
		{ID: "parrilla", Name: "De la parrilla", SortOrder: 2, Items: []database.MenuItem{
			{ID: "demo-ojo-de-bife", Name: "Ojo de bife", Price: 39500, Currency: "ARS", IsAvailable: true, Allergens: []string{}, SortOrder: 1},
			{ID: "demo-entrana", Name: "Entraña", Price: 31500, Currency: "ARS", IsAvailable: true, Allergens: []string{}, SortOrder: 2},
		}},
		{ID: "postres", Name: "Postres", SortOrder: 3, Items: []database.MenuItem{
			{ID: "demo-flan", Name: "Flan casero", Price: 8900, Currency: "ARS", IsAvailable: true, Allergens: []string{"dairy", "eggs"}, DietaryTags: []string{"vegetarian"}, SortOrder: 1},
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

// Issue 875: a Spanish gluten-free answer read "... gluten, eggs" — the stored
// canonical allergen id leaking English into the venue's own language, next to
// a guest card whose chip already says "Huevos".
func TestFinalizeWaiterV2_SpanishAllergenAnswerUsesNativeAllergenNames(t *testing.T) {
	snapshot := allergenChipSnapshot(t, "es")

	t.Run("dietary pick avoid list", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-875-gf", Locale: "es", Mode: "ordering",
			UserMessage: "¿qué hay sin gluten?", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		lower := strings.ToLower(response.Answer.Content)
		assert.NotContains(t, lower, "eggs")
		assert.NotContains(t, lower, "dairy")
		assert.Contains(t, lower, "huevos")
	})

	t.Run("named dish allergen answer", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-875-item", Locale: "es", Mode: "ordering",
			UserMessage: "¿la empanada de carne tiene gluten?", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		lower := strings.ToLower(response.Answer.Content)
		assert.Contains(t, lower, "empanada de carne")
		assert.Contains(t, lower, "huevos")
		assert.NotContains(t, lower, "eggs")
	})
}

// English keeps the canonical wording, and the guest UI's own chip vocabulary is
// the single source for both surfaces.
func TestFinalizeWaiterV2_EnglishAllergenAnswerKeepsCanonicalNames(t *testing.T) {
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-875-en", Locale: "en", Mode: "ordering",
		UserMessage: "does the empanada de carne have allergens?", Snapshot: allergenChipSnapshot(t, "en"),
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	assert.Contains(t, strings.ToLower(response.Answer.Content), "eggs")
}

func TestWaiterAllergenLabelsCoverEveryGuestLocale(t *testing.T) {
	canonical := waiterAllergenLabelsByLocale["en"]
	require.NotEmpty(t, canonical)
	for _, locale := range locales.GuestLocales() {
		table, ok := waiterAllergenLabelsByLocale[locale.Canonical]
		require.True(t, ok, "no allergen labels for guest locale %s", locale.Canonical)
		require.Len(t, table, len(canonical), "%s must label every canonical allergen", locale.Canonical)
		for id := range canonical {
			assert.NotEmpty(t, strings.TrimSpace(table[id]), "%s.%s is empty", locale.Canonical, id)
		}
	}
}

func TestWaiterAllergenLabelNormalizesAndPassesThroughFreeText(t *testing.T) {
	assert.Equal(t, "Huevos", waiterAllergenLabel("es", "Eggs"))
	assert.Equal(t, "Huevos", waiterAllergenLabel("es-AR", " eggs "))
	assert.Equal(t, "Frutos de cáscara", waiterAllergenLabel("es", "tree-nuts"))
	assert.Equal(t, "Dióxido de azufre", waiterAllergenLabel("es", "SO2"))
	// Operator free text is not ours to translate — it must still reach the guest.
	assert.Equal(t, "chimichurri picante", waiterAllergenLabel("es", "chimichurri picante"))
	assert.Equal(t, "Eggs", waiterAllergenLabel("xx-YY", "eggs"))
	assert.Empty(t, waiterAllergenLabel("es", "   "))
	assert.Equal(t, []string{"Gluten", "Huevos"}, waiterLocalizedAllergens("es", []string{"gluten", "", "eggs"}))
}

// The disclaimer is the safety posture, not a bug: it must survive localization.
func TestFinalizeWaiterV2_LocalizedAllergensKeepTheStaffDisclaimer(t *testing.T) {
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-875-disclaimer", Locale: "es", Mode: "ordering",
		UserMessage: "¿la empanada de carne tiene gluten?", Snapshot: allergenChipSnapshot(t, "es"),
	})
	require.NoError(t, err)
	assert.Contains(t, response.Answer.Content, services.AllergenDisclaimer("es"))
}
