package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

type promotionTranslationTestService struct {
	calls int
}

func (s *promotionTranslationTestService) TranslateText(text string, targetLanguages []string) (map[string]string, error) {
	s.calls++
	return map[string]string{targetLanguages[0]: "Nuevo traducido"}, nil
}

func (s *promotionTranslationTestService) TranslateTextWithSource(text, sourceLanguage string, targetLanguages []string) (map[string]string, error) {
	s.calls++
	return map[string]string{targetLanguages[0]: "Nuevo traducido"}, nil
}

func setupPromotionLanguageAddDB(t *testing.T) {
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
		&database.Offer{},
		&database.Bundle{},
		&database.Translation{},
	))
}

// TestTranslateExistingPromotionContent_WritesOfferAndBundleRows is the D4b
// regression: the exported hook used by the language-add handler must produce
// offer/bundle translation rows for the configured non-default languages, so
// guests in a newly added language don't wait for a first-hit backfill.
func TestTranslateExistingPromotionContent_WritesOfferAndBundleRows(t *testing.T) {
	setupPromotionLanguageAddDB(t)
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
		BusinessID: business.ID, LanguageCode: "ar", IsDefault: false,
	}).Error)

	offer := &database.Offer{
		BusinessID: business.ID, Name: "Happy hour", Description: "Half-price drinks",
		DiscountType: "percentage", DiscountValue: 50, IsActive: true, ApplicableTo: "all",
	}
	require.NoError(t, database.GetDB().Create(offer).Error)

	bundle := &database.Bundle{
		BusinessID: business.ID, Name: "Family feast", Description: "Feeds four",
		Price: 89, IsActive: true, Items: "[]",
	}
	require.NoError(t, database.GetDB().Create(bundle).Error)

	require.NoError(t, TranslateExistingPromotionContent(business.ID))

	var offerRows int64
	require.NoError(t, database.GetDB().Model(&database.Translation{}).
		Where("business_id = ? AND entity_type = ? AND entity_id = ? AND language_code = ?",
			business.ID, "offer", offer.ID, "ar").
		Count(&offerRows).Error)
	assert.Equal(t, int64(2), offerRows, "offer name + description must be translated")

	var bundleRows int64
	require.NoError(t, database.GetDB().Model(&database.Translation{}).
		Where("business_id = ? AND entity_type = ? AND entity_id = ? AND language_code = ?",
			business.ID, "bundle", bundle.ID, "ar").
		Count(&bundleRows).Error)
	assert.Equal(t, int64(2), bundleRows, "bundle name + description must be translated")
}

func TestTranslatePromotionFieldRegeneratesWhenOriginalTextChanges(t *testing.T) {
	setupPromotionLanguageAddDB(t)
	const businessID = uint(77)
	const offerID = uint(5)
	now := time.Now().UTC()
	require.NoError(t, database.GetDB().Create(&database.Translation{
		BusinessID: businessID, EntityType: offerTranslationEntityType, EntityID: offerID,
		FieldName: "name", LanguageCode: "es", OriginalText: "Old name",
		TranslatedText: "Nombre viejo", CreatedAt: now, UpdatedAt: now,
	}).Error)

	service := &promotionTranslationTestService{}
	require.NoError(t, translatePromotionField(
		database.GetDBWrapper(), service, businessID, offerTranslationEntityType, offerID,
		"name", "New name", "en", "es",
	))
	assert.Equal(t, 1, service.calls, "a changed source must invoke translation instead of reusing stale text")

	var rows []database.Translation
	require.NoError(t, database.GetDB().Where(
		"business_id = ? AND entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?",
		businessID, offerTranslationEntityType, offerID, "name", "es",
	).Order("id ASC").Find(&rows).Error)
	require.Len(t, rows, 2)
	assert.Equal(t, "New name", rows[1].OriginalText)
	assert.Equal(t, "Nuevo traducido", rows[1].TranslatedText)
}

func TestApplyTranslationsToPromotionsTreatsStaleOriginalTextAsMissing(t *testing.T) {
	lookup := &translationLookup{
		byKey: map[string]string{
			translationLookupKey(offerTranslationEntityType, 5, "name"):         "Nombre viejo",
			translationLookupKey(offerTranslationEntityType, 5, "description"):  "Descripción vieja",
			translationLookupKey(bundleTranslationEntityType, 8, "name"):        "Combo viejo",
			translationLookupKey(bundleTranslationEntityType, 8, "description"): "Detalle viejo",
		},
		origByKey: map[string]string{
			translationLookupKey(offerTranslationEntityType, 5, "name"):         "Old name",
			translationLookupKey(offerTranslationEntityType, 5, "description"):  "Old description",
			translationLookupKey(bundleTranslationEntityType, 8, "name"):        "Old combo",
			translationLookupKey(bundleTranslationEntityType, 8, "description"): "Old details",
		},
		ambiguousKeys: map[string]struct{}{},
	}

	offers, offerMissing := applyTranslationsToOffersWithLookup([]database.Offer{{
		BusinessID: 77,
		ID:         5,
		Name:       "New name", Description: "New description",
	}}, "es", lookup)
	bundles, bundleMissing := applyTranslationsToBundlesWithLookup([]database.Bundle{{
		BusinessID: 77,
		ID:         8,
		Name:       "New combo", Description: "New details",
	}}, "es", lookup)

	require.True(t, offerMissing, "stale offer translation must schedule backfill")
	require.True(t, bundleMissing, "stale bundle translation must schedule backfill")
	require.Equal(t, "New name", offers[0].Name, "stale offer text must not be served")
	require.Equal(t, "New description", offers[0].Description, "stale offer description must not be served")
	require.Equal(t, "New combo", bundles[0].Name, "stale bundle text must not be served")
	require.Equal(t, "New details", bundles[0].Description, "stale bundle description must not be served")
}
