package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupAIMenuImportTranslationDB(t *testing.T) {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Menu{},
		&database.BusinessLanguage{},
		&database.Translation{},
	))
}

// TestSeedImportedMenuTranslations_TranslatesAppendedCategories is the D4c
// regression: after an AI menu import appends categories, the synchronous
// seeding core must write menu translation rows for the business's configured
// guest languages — AI-onboarded menus must not silently stay single-language.
func TestSeedImportedMenuTranslations_TranslatesAppendedCategories(t *testing.T) {
	setupAIMenuImportTranslationDB(t)
	installGoogleTranslateStub(t)
	configureBatchTranslationService(t, "test-api-key")

	business := createTranslationHandlerTestBusiness(t)
	business.DefaultLanguage = "en"
	require.NoError(t, database.GetDB().Save(business).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID: business.ID, LanguageCode: "en", IsDefault: true,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID: business.ID, LanguageCode: "es", IsDefault: false,
	}).Error)

	// Simulate the import: append extracted categories the way both import
	// handlers do.
	_, err := database.AppendMenuCategories(business.ID, []database.MenuCategory{
		{
			Name:        "Starters",
			Description: "Small plates",
			Items: []database.MenuItem{
				{Name: "Bruschetta", Description: "Grilled bread, tomato"},
			},
		},
	})
	require.NoError(t, err)

	seedImportedMenuTranslations(business.ID)

	// category index 0 name row in es.
	var catRows int64
	require.NoError(t, database.GetDB().Model(&database.Translation{}).
		Where("business_id = ? AND entity_type = ? AND entity_id = ? AND language_code = ?",
			business.ID, "category", 0, "es").
		Count(&catRows).Error)
	assert.NotZero(t, catRows, "imported category must gain an es translation row")

	// menu_item entity_id 0*1000+0 name row in es.
	var itemRows int64
	require.NoError(t, database.GetDB().Model(&database.Translation{}).
		Where("business_id = ? AND entity_type = ? AND entity_id = ? AND language_code = ?",
			business.ID, "menu_item", 0, "es").
		Count(&itemRows).Error)
	assert.NotZero(t, itemRows, "imported menu item must gain an es translation row")
}

// The seeding core must be a silent no-op when the translation service is not
// configured (no Google key) — import must never fail or log-spam because of it.
func TestSeedImportedMenuTranslations_NoOpWithoutService(t *testing.T) {
	setupAIMenuImportTranslationDB(t)
	// No configureBatchTranslationService call — service disabled.

	business := createTranslationHandlerTestBusiness(t)
	assert.NotPanics(t, func() { seedImportedMenuTranslations(business.ID) })
}
