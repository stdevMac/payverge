package server

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// seedBackfillMenu wires a business with an English default + Spanish target
// language and a one-category, one-item menu. Mirrors the batch-translation
// test fixture so the entity-ID scheme stays exercised end to end.
func seedBackfillMenu(t *testing.T) *database.Business {
	t.Helper()

	business := createTranslationHandlerTestBusiness(t)
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID:   business.ID,
		LanguageCode: "en",
		IsDefault:    true,
		DisplayOrder: 0,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID:   business.ID,
		LanguageCode: "es",
		IsDefault:    false,
		DisplayOrder: 1,
	}).Error)

	menuCategories := []database.MenuCategory{
		{
			ID:          "cat-0",
			Name:        "Starters",
			Description: "Begin here",
			Items: []database.MenuItem{
				{
					ID:          "item-0",
					Name:        "Soup",
					Description: "Warm bowl",
					Price:       5,
					IsAvailable: true,
				},
			},
		},
	}
	menuPayload, err := json.Marshal(menuCategories)
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID,
		Categories: string(menuPayload),
		IsActive:   true,
		Version:    1,
	}).Error)

	return business
}

// TestApplyTranslationsToMenu_ReportsMissingUntilTranslated is the core
// regression for the storefront "menu not translated when a language is added"
// bug: applyTranslationsToMenu must report missing=true before any translation
// exists, and TranslateBusinessMenuForLanguages must populate rows so the next
// read reports missing=false with the translated text applied.
func TestApplyTranslationsToMenu_ReportsMissingUntilTranslated(t *testing.T) {
	setupTranslationHandlerTestDB(t)
	resetTranslationJobs()
	installGoogleTranslateStub(t)
	configureBatchTranslationService(t, "test-api-key")

	business := seedBackfillMenu(t)

	_, categories, err := database.GetMenuByBusinessID(business.ID)
	require.NoError(t, err)
	require.Len(t, categories, 1)

	// Before any translation exists the read path must flag the menu as missing
	// translations so the storefront kicks a backfill.
	before, missingBefore := applyTranslationsToMenu(business.ID, categories, "es")
	assert.True(t, missingBefore, "expected missing=true before translation")
	assert.Equal(t, "Starters", before[0].Name, "untranslated read keeps the original text")

	// Translating into the configured guest language must populate rows for the
	// same entity IDs the read path looks up.
	require.NoError(t, TranslateBusinessMenuForLanguages(business.ID, []string{"en", "es"}))

	after, missingAfter := applyTranslationsToMenu(business.ID, categories, "es")
	assert.False(t, missingAfter, "expected missing=false once translations exist")
	require.Len(t, after, 1)
	require.Len(t, after[0].Items, 1)

	var categoryRow database.Translation
	require.NoError(t, database.GetDB().Where(
		"business_id = ? AND entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?",
		business.ID, "category", 0, "name", "es",
	).First(&categoryRow).Error)
	assert.Equal(t, after[0].Name, categoryRow.TranslatedText)

	var itemRow database.Translation
	require.NoError(t, database.GetDB().Where(
		"business_id = ? AND entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?",
		business.ID, "menu_item", 0, "name", "es",
	).First(&itemRow).Error)
	assert.Equal(t, after[0].Items[0].Name, itemRow.TranslatedText)
}

// TestApplyTranslationsToMenu_SelfHealsAfterReorder is the R3-MB-3 regression:
// menu translations are keyed by POSITION (categoryIndex*1000+itemIndex), so a
// reorder leaves a stored translation attached to a position whose source text
// has changed. The read path must compare the stored OriginalText against the
// current source and treat a mismatch as MISSING — so backfill regenerates it
// rather than serving item A's translated name on item B forever.
func TestApplyTranslationsToMenu_SelfHealsAfterReorder(t *testing.T) {
	setupTranslationHandlerTestDB(t)
	resetTranslationJobs()
	installGoogleTranslateStub(t)
	configureBatchTranslationService(t, "test-api-key")

	business := seedBackfillMenu(t)

	_, categories, err := database.GetMenuByBusinessID(business.ID)
	require.NoError(t, err)
	require.Len(t, categories, 1)

	// Translate the single item at position 0 so a translation row exists whose
	// OriginalText is "Soup".
	require.NoError(t, TranslateBusinessMenuForLanguages(business.ID, []string{"en", "es"}))

	_, missingAfter := applyTranslationsToMenu(business.ID, categories, "es")
	assert.False(t, missingAfter, "translation should be present before the reorder")

	// Simulate a reorder: a DIFFERENT item now sits at position 0 (categoryIndex
	// 0 * 1000 + itemIndex 0). Its source text no longer matches the stored
	// translation's OriginalText ("Soup").
	reordered := []database.MenuCategory{
		{
			ID:          "cat-0",
			Name:        "Starters",
			Description: "Begin here",
			Items: []database.MenuItem{
				{
					ID:          "item-1",
					Name:        "Bruschetta",
					Description: "Toasted bread",
					Price:       7,
					IsAvailable: true,
				},
			},
		},
	}

	translated, missing := applyTranslationsToMenu(business.ID, reordered, "es")
	assert.True(t, missing, "a position whose OriginalText no longer matches must read as missing")
	assert.Equal(t, "Bruschetta", translated[0].Items[0].Name,
		"the stale translation must NOT be served; the current source text stands until backfill regenerates it")
}

// TestTranslateBusinessMenuForLanguages_DisabledServiceErrors guards the
// no-op-when-disabled contract the language-update goroutine relies on.
func TestTranslateBusinessMenuForLanguages_DisabledServiceErrors(t *testing.T) {
	setupTranslationHandlerTestDB(t)
	resetTranslationJobs()
	configureBatchTranslationService(t, "")

	business := seedBackfillMenu(t)

	assert.False(t, IsBatchTranslationEnabled())
	require.EqualError(t,
		TranslateBusinessMenuForLanguages(business.ID, []string{"es"}),
		"translation service is not enabled",
	)
}
