package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func setupAllergenTranslationDB(t *testing.T) {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.BusinessLanguage{},
		&database.Translation{},
	))
}

// TestAutoTranslateMenuItem_DoesNotTranslateAllergenOrDietaryIDs is the D4d
// regression: allergens/dietary tags are canonical IDs localized at render time
// (menu.allergenNames.* / items.allergenNames.*). Machine-translating them into
// free text broke the guest chip/icon maps. Name/description/options must still
// translate; allergen and dietary_tag rows must NOT be written.
func TestAutoTranslateMenuItem_DoesNotTranslateAllergenOrDietaryIDs(t *testing.T) {
	setupAllergenTranslationDB(t)
	installGoogleTranslateStub(t)
	SetTranslationService(services.NewTranslationService(database.GetDBWrapper(), "test-api-key"))
	t.Cleanup(func() { SetTranslationService(nil) })

	business := createTranslationHandlerTestBusiness(t)
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID: business.ID, LanguageCode: "en", IsDefault: true,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID: business.ID, LanguageCode: "fr", IsDefault: false,
	}).Error)

	require.NoError(t, autoTranslateMenuItem(
		business.ID, 0, 0,
		"Cheese Board", "Assorted cheeses",
		[]database.MenuItemOption{{Name: "Large"}},
	))

	// Content still translates.
	var itemRows int64
	require.NoError(t, database.GetDB().Model(&database.Translation{}).
		Where("business_id = ? AND entity_type = ? AND language_code = ?",
			business.ID, "menu_item", "fr").
		Count(&itemRows).Error)
	assert.Equal(t, int64(2), itemRows, "item name + description must still translate")

	var optionRows int64
	require.NoError(t, database.GetDB().Model(&database.Translation{}).
		Where("business_id = ? AND entity_type = ? AND language_code = ?",
			business.ID, "menu_item_option", "fr").
		Count(&optionRows).Error)
	assert.Equal(t, int64(1), optionRows, "option names must still translate")

	// IDs must NOT be translated.
	var allergenRows int64
	require.NoError(t, database.GetDB().Model(&database.Translation{}).
		Where("business_id = ? AND entity_type = ?", business.ID, "allergen").
		Count(&allergenRows).Error)
	assert.Zero(t, allergenRows, "allergen IDs must not be machine-translated")

	var tagRows int64
	require.NoError(t, database.GetDB().Model(&database.Translation{}).
		Where("business_id = ? AND entity_type = ?", business.ID, "dietary_tag").
		Count(&tagRows).Error)
	assert.Zero(t, tagRows, "dietary-tag IDs must not be machine-translated")
}
