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

func setupBusinessContentTranslationDB(t *testing.T) {
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
		&database.BusinessSpecialFeature{},
		&database.BusinessGalleryImage{},
		&database.Translation{},
	))
}

// TestApplyBusinessContentTranslations_ReportsMissingUntilTranslated is the
// regression for the storefront "Why choose us" / hero copy not translating when
// a guest language is added: applyBusinessContentTranslations must flag missing
// before any translation exists, and TranslateBusinessContentForLanguages must
// populate rows so the next read overlays the translated text.
func TestApplyBusinessContentTranslations_ReportsMissingUntilTranslated(t *testing.T) {
	setupBusinessContentTranslationDB(t)
	installGoogleTranslateStub(t)
	SetTranslationService(services.NewTranslationService(database.GetDBWrapper(), "test-api-key"))
	t.Cleanup(func() { SetTranslationService(nil) })

	business := createTranslationHandlerTestBusiness(t)
	business.DefaultLanguage = "en"
	business.Description = "Premium lounge"
	require.NoError(t, database.GetDB().Save(business).Error)

	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID: business.ID, LanguageCode: "en", IsDefault: true,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID: business.ID, LanguageCode: "fr", IsDefault: false,
	}).Error)

	feature := &database.BusinessSpecialFeature{
		BusinessID:  business.ID,
		Title:       "AI concierge",
		Description: "Sofia handles questions",
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(feature).Error)

	features, err := database.GetBusinessSpecialFeatures(business.ID)
	require.NoError(t, err)
	require.Len(t, features, 1)

	before := *business
	missingBefore := applyBusinessContentTranslations(&before, features, nil, "fr")
	assert.True(t, missingBefore, "expected missing=true before translation")
	assert.Equal(t, "Premium lounge", before.Description, "untranslated read keeps the original text")

	require.NoError(t, TranslateBusinessContentForLanguages(business.ID, []string{"en", "fr"}))

	featuresAfter, err := database.GetBusinessSpecialFeatures(business.ID)
	require.NoError(t, err)
	after := *business
	missingAfter := applyBusinessContentTranslations(&after, featuresAfter, nil, "fr")
	assert.False(t, missingAfter, "expected missing=false once translations exist")

	var descRow database.Translation
	require.NoError(t, database.GetDB().Where(
		"business_id = ? AND entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?",
		business.ID, "business", business.ID, "description", "fr",
	).First(&descRow).Error)

	var titleRow database.Translation
	require.NoError(t, database.GetDB().Where(
		"business_id = ? AND entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?",
		business.ID, "special_feature", feature.ID, "title", "fr",
	).First(&titleRow).Error)
	assert.Equal(t, featuresAfter[0].Title, titleRow.TranslatedText)
}

// TestDeleteSpecialFeatureTranslations_ClearsOnlyFeatureRows verifies the
// wholesale clear used after a feature edit leaves other entity translations
// (menu, business prose) intact.
func TestDeleteSpecialFeatureTranslations_ClearsOnlyFeatureRows(t *testing.T) {
	setupBusinessContentTranslationDB(t)

	business := createTranslationHandlerTestBusiness(t)

	require.NoError(t, database.GetDB().Create(&database.Translation{
		BusinessID: business.ID, EntityType: "special_feature", EntityID: 1,
		FieldName: "title", LanguageCode: "fr", TranslatedText: "x",
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.Translation{
		BusinessID: business.ID, EntityType: "business", EntityID: business.ID,
		FieldName: "description", LanguageCode: "fr", TranslatedText: "y",
	}).Error)

	require.NoError(t, deleteSpecialFeatureTranslations(business.ID))

	var featureCount int64
	require.NoError(t, database.GetDB().Model(&database.Translation{}).
		Where("business_id = ? AND entity_type = ?", business.ID, "special_feature").
		Count(&featureCount).Error)
	assert.Zero(t, featureCount)

	var businessCount int64
	require.NoError(t, database.GetDB().Model(&database.Translation{}).
		Where("business_id = ? AND entity_type = ?", business.ID, "business").
		Count(&businessCount).Error)
	assert.Equal(t, int64(1), businessCount)
}

// TestGalleryCaptionTranslationLifecycle is the D2 regression: gallery captions
// must flow through the same translate-on-save / apply-on-read / clear-on-replace
// pipeline as special features, keyed by entity_type "gallery_image" + row ID.
func TestGalleryCaptionTranslationLifecycle(t *testing.T) {
	setupBusinessContentTranslationDB(t)
	installGoogleTranslateStub(t)
	SetTranslationService(services.NewTranslationService(database.GetDBWrapper(), "test-api-key"))
	t.Cleanup(func() { SetTranslationService(nil) })

	business := createTranslationHandlerTestBusiness(t)
	business.DefaultLanguage = "en"
	require.NoError(t, database.GetDB().Save(business).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID: business.ID, LanguageCode: "en", IsDefault: true,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID: business.ID, LanguageCode: "fr", IsDefault: false,
	}).Error)

	img := &database.BusinessGalleryImage{
		BusinessID: business.ID,
		ImageURL:   "https://cdn.example.com/terrace.jpg",
		Caption:    "Our rooftop terrace at sunset",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(img).Error)

	images := []database.BusinessGalleryImage{*img}

	// Before translation: apply flags missing and keeps the source caption.
	missingBefore := applyBusinessContentTranslations(&(*business), nil, images, "fr")
	assert.True(t, missingBefore, "expected missing=true before caption translation")
	assert.Equal(t, "Our rooftop terrace at sunset", images[0].Caption)

	// Translate (stub echoes the source text back).
	require.NoError(t, TranslateBusinessContentForLanguages(business.ID, []string{"en", "fr"}))

	var row database.Translation
	require.NoError(t, database.GetDB().Where(
		"business_id = ? AND entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?",
		business.ID, "gallery_image", img.ID, "caption", "fr",
	).First(&row).Error)
	assert.Equal(t, "Our rooftop terrace at sunset", row.OriginalText)

	// After translation: apply overlays and no longer flags missing.
	imagesAfter := []database.BusinessGalleryImage{*img}
	missingAfter := applyBusinessContentTranslations(&(*business), nil, imagesAfter, "fr")
	assert.False(t, missingAfter, "expected missing=false once caption translation exists")
	assert.Equal(t, row.TranslatedText, imagesAfter[0].Caption)

	// Wholesale-replace clear: gallery rows only, other entities untouched.
	require.NoError(t, database.GetDB().Create(&database.Translation{
		BusinessID: business.ID, EntityType: "business", EntityID: business.ID,
		FieldName: "description", LanguageCode: "fr", TranslatedText: "y",
	}).Error)
	require.NoError(t, deleteGalleryImageTranslations(business.ID))

	var galleryCount int64
	require.NoError(t, database.GetDB().Model(&database.Translation{}).
		Where("business_id = ? AND entity_type = ?", business.ID, "gallery_image").
		Count(&galleryCount).Error)
	assert.Zero(t, galleryCount)

	var businessCount int64
	require.NoError(t, database.GetDB().Model(&database.Translation{}).
		Where("business_id = ? AND entity_type = ?", business.ID, "business").
		Count(&businessCount).Error)
	assert.Equal(t, int64(1), businessCount)
}
