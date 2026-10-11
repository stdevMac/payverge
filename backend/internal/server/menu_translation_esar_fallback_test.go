package server

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Empty or identity es-AR rows must not shadow usable es menu copy (#386).
func TestApplyTranslationsToMenu_EsARFallsBackPastEmptyAndIdentityRows(t *testing.T) {
	db := setupMenuTranslationTestDB(t)
	const bizID = uint(86)

	seedTranslationWithOriginal(t, db, bizID, "category", 0, "name", "es", "Mains", "Principales")
	seedTranslationWithOriginal(t, db, bizID, "menu_item", 0, "name", "es", "Harvest Bowl", "Bowl de la cosecha")
	seedTranslationWithOriginal(t, db, bizID, "menu_item", 0, "description", "es", "Roasted vegetables, grains, herbs", "Verduras asadas, granos y hierbas")

	// Failed/partial es-AR backfill: empty name + identity description.
	seedTranslationWithOriginal(t, db, bizID, "category", 0, "name", "es-AR", "Mains", "")
	seedTranslationWithOriginal(t, db, bizID, "menu_item", 0, "name", "es-AR", "Harvest Bowl", "Harvest Bowl")
	seedTranslationWithOriginal(t, db, bizID, "menu_item", 0, "description", "es-AR", "Roasted vegetables, grains, herbs", "")

	categories := []database.MenuCategory{
		{
			Name:        "Mains",
			Description: "Best-selling demo dishes",
			Items: []database.MenuItem{
				{
					Name:        "Harvest Bowl",
					Description: "Roasted vegetables, grains, herbs",
				},
			},
		},
	}

	translated, missing := applyTranslationsToMenu(bizID, categories, "es-AR")
	require.Len(t, translated, 1)
	require.Equal(t, "Principales", translated[0].Name)
	require.Equal(t, "Bowl de la cosecha", translated[0].Items[0].Name)
	require.Equal(t, "Verduras asadas, granos y hierbas", translated[0].Items[0].Description)
	require.True(t, missing, "category description has no es/es-AR row")
}

func TestApplyTranslationsToMenu_IdentityOnlyRowIsMissing(t *testing.T) {
	db := setupMenuTranslationTestDB(t)
	const bizID = uint(87)

	seedTranslationWithOriginal(t, db, bizID, "category", 0, "name", "es", "Mains", "Mains")

	categories := []database.MenuCategory{{Name: "Mains"}}
	translated, missing := applyTranslationsToMenu(bizID, categories, "es")
	require.Len(t, translated, 1)
	require.Equal(t, "Mains", translated[0].Name)
	require.True(t, missing, "identity/echo rows are not fresh translations")
}

// A newer identity row (provider echoed the English source as "es") must not
// shadow an older real translation for the same language (#514).
func TestApplyTranslationsToMenu_NewerIdentityDoesNotShadowOlderRow(t *testing.T) {
	db := setupMenuTranslationTestDB(t)
	const bizID = uint(88)

	seedTranslationWithOriginal(t, db, bizID, "menu_item", 2, "name", "es", "Market Tacos", "Tacos del mercado")
	seedTranslationWithOriginal(t, db, bizID, "menu_item", 2, "description", "es", "Three tacos with salsa verde", "Tres tacos con salsa verde")
	// Later backfill echo — higher id, same language, translated == original.
	seedTranslationWithOriginal(t, db, bizID, "menu_item", 2, "name", "es", "Market Tacos", "Market Tacos")
	seedTranslationWithOriginal(t, db, bizID, "menu_item", 2, "description", "es", "Three tacos with salsa verde", "Three tacos with salsa verde")

	categories := []database.MenuCategory{
		{
			Name: "Mains",
			Items: []database.MenuItem{
				{ID: "demo-bowl", Name: "Harvest Bowl", Description: "Roasted vegetables"},
				{ID: "demo-steak", Name: "Steak Plate", Description: "Charred steak"},
				{ID: "demo-tacos", Name: "Market Tacos", Description: "Three tacos with salsa verde"},
			},
		},
	}

	translated, missing := applyTranslationsToMenu(bizID, categories, "es")
	require.Len(t, translated, 1)
	require.Len(t, translated[0].Items, 3)
	require.Equal(t, "Tacos del mercado", translated[0].Items[2].Name)
	require.Equal(t, "Tres tacos con salsa verde", translated[0].Items[2].Description)
	_ = missing
}
