package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTranslationServiceTestDB(t *testing.T) *gorm.DB {
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
		&database.SupportedLanguage{},
		&database.Translation{},
	))

	return gormDB
}

func TestInitializeDefaultLanguagesSeedsArgentineSpanish(t *testing.T) {
	setupTranslationServiceTestDB(t)

	service := NewTranslationService(database.GetDBWrapper(), "")

	require.NoError(t, service.InitializeDefaultLanguages())

	var language database.SupportedLanguage
	require.NoError(t, database.GetDB().Where("code = ?", "es-AR").First(&language).Error)
	assert.Equal(t, "Argentine Spanish", language.Name)
	assert.Equal(t, "Español (Argentina)", language.NativeName)
	assert.True(t, language.IsActive)
}

// Regression test for the 2026-05-14 "constrain backend translation to
// supported locales" commit, which shrank the seed to en/es/es-AR AND added a
// `UPDATE supported_languages SET is_active = false WHERE code NOT IN
// (operator_set)` sweep that ran on every backend boot. That sweep wiped guest
// menu language support in production.
func TestInitializeDefaultLanguagesSeedsAllGuestLocales(t *testing.T) {
	setupTranslationServiceTestDB(t)

	service := NewTranslationService(database.GetDBWrapper(), "")
	require.NoError(t, service.InitializeDefaultLanguages())

	expected := []string{
		"en", "es", "es-AR",
		"ar", "da", "de", "fr", "hi", "it", "ja", "ko",
		"nl", "no", "pl", "pt", "ru", "sv", "th", "tr", "vi", "zh",
	}
	for _, code := range expected {
		var row database.SupportedLanguage
		err := database.GetDB().Where("code = ?", code).First(&row).Error
		require.NoErrorf(t, err, "locale %q should be seeded", code)
		assert.Truef(t, row.IsActive, "locale %q should be active", code)
	}
}

// Pre-seed a guest locale row as inactive (the post-regression production
// state). InitializeDefaultLanguages must flip it back to active without
// requiring a one-off SQL migration.
func TestInitializeDefaultLanguagesReactivatesGuestLocales(t *testing.T) {
	setupTranslationServiceTestDB(t)

	require.NoError(t, database.GetDB().Create(&database.SupportedLanguage{
		Code:       "fr",
		Name:       "French",
		NativeName: "Français",
		IsActive:   false,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.SupportedLanguage{
		Code:       "ar",
		Name:       "Arabic",
		NativeName: "العربية",
		IsActive:   false,
	}).Error)

	service := NewTranslationService(database.GetDBWrapper(), "")
	require.NoError(t, service.InitializeDefaultLanguages())

	for _, code := range []string{"fr", "ar"} {
		var row database.SupportedLanguage
		require.NoError(t, database.GetDB().Where("code = ?", code).First(&row).Error)
		assert.Truef(t, row.IsActive, "locale %q should be reactivated", code)
	}
}

// Operators may hand-add an experimental locale (e.g. via direct DB write).
// Initialize must leave unknown-to-registry rows alone — no global sweep that
// disables them. This invariant is what the 2026-05-14 regression violated.
func TestInitializeDefaultLanguagesPreservesUnknownCodes(t *testing.T) {
	setupTranslationServiceTestDB(t)

	require.NoError(t, database.GetDB().Create(&database.SupportedLanguage{
		Code:       "xx-experimental",
		Name:       "Experimental",
		NativeName: "Experimental",
		IsActive:   true,
	}).Error)

	service := NewTranslationService(database.GetDBWrapper(), "")
	require.NoError(t, service.InitializeDefaultLanguages())

	var row database.SupportedLanguage
	require.NoError(t, database.GetDB().Where("code = ?", "xx-experimental").First(&row).Error)
	assert.True(t, row.IsActive, "unknown locales must not be deactivated by the seed")
}

func TestTranslateTextWithSourcePreservesCanonicalResultKey(t *testing.T) {
	setupTranslationServiceTestDB(t)

	var providerRequest GoogleTranslateRequest
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&providerRequest))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"translations":[{"translatedText":"hola"}]}}`))
	}))
	defer provider.Close()

	service := NewTranslationService(database.GetDBWrapper(), "test-api-key")
	service.apiBaseURL = provider.URL

	translations, err := service.TranslateTextWithSource("hello", "en", []string{"es-AR"})

	require.NoError(t, err)
	assert.Equal(t, "hola", translations["es-AR"])
	assert.NotContains(t, translations, "es")
	// Google Cloud Translation v2 (NMT) rejects region-qualified Spanish, so the
	// provider request is sent with the registry's TranslationProviderTarget
	// ("es" for es-AR) while the result map stays keyed by the canonical "es-AR".
	assert.Equal(t, "es", providerRequest.Target)
}

func TestTranslateTextOmitsAutoSource(t *testing.T) {
	setupTranslationServiceTestDB(t)

	var raw map[string]any
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&raw))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"translations":[{"translatedText":"Hallo"}]}}`))
	}))
	defer provider.Close()

	service := NewTranslationService(database.GetDBWrapper(), "test-api-key")
	service.apiBaseURL = provider.URL

	translations, err := service.TranslateText("Hello", []string{"de"})
	require.NoError(t, err)
	assert.Equal(t, "Hallo", translations["de"])
	_, hasSource := raw["source"]
	assert.False(t, hasSource, "Google Translate v2 must omit source for detect; source:%v is rejected", raw["source"])
}

func TestTranslateTextWithSourceMapsRegionalSource(t *testing.T) {
	setupTranslationServiceTestDB(t)

	var providerRequest GoogleTranslateRequest
	var raw map[string]any
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&raw))
		encoded, err := json.Marshal(raw)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(encoded, &providerRequest))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"translations":[{"translatedText":"hello"}]}}`))
	}))
	defer provider.Close()

	service := NewTranslationService(database.GetDBWrapper(), "test-api-key")
	service.apiBaseURL = provider.URL

	translations, err := service.TranslateTextWithSource("hola", "es-AR", []string{"en"})
	require.NoError(t, err)
	assert.Equal(t, "hello", translations["en"])
	assert.Equal(t, "es", providerRequest.Source)
	assert.NotEqual(t, "auto", raw["source"])
	assert.NotEqual(t, "es-AR", raw["source"])
}

func TestTranslateTextWithSourceProviderErrorDoesNotReturnOriginal(t *testing.T) {
	setupTranslationServiceTestDB(t)

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid Value"}}`))
	}))
	defer provider.Close()

	service := NewTranslationService(database.GetDBWrapper(), "test-api-key")
	service.apiBaseURL = provider.URL

	translations, err := service.TranslateTextWithSource("Burger", "en", []string{"de"})
	require.Error(t, err)
	assert.Nil(t, translations)
}

func TestTranslateMenuItemToLanguagesProvider400DoesNotPersist(t *testing.T) {
	setupTranslationServiceTestDB(t)

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid Value"}}`))
	}))
	defer provider.Close()

	service := NewTranslationService(database.GetDBWrapper(), "test-api-key")
	service.apiBaseURL = provider.URL

	err := service.TranslateMenuItemToLanguages(1, 9, "Burger", "Beef patty", []string{"de"})
	require.Error(t, err)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Translation{}).
		Where("business_id = ? AND entity_id = ?", 1, 9).
		Count(&count).Error)
	assert.Zero(t, count, "provider failures must not persist identity translations")
}

func TestTranslateMenuItemRetriesIdentityRows(t *testing.T) {
	setupTranslationServiceTestDB(t)

	require.NoError(t, database.GetDB().Create(&database.Translation{
		BusinessID:       11,
		EntityType:       "menu_item",
		EntityID:         3,
		FieldName:        "name",
		LanguageCode:     "de",
		OriginalText:     "Burger",
		TranslatedText:   "Burger",
		IsAutoTranslated: true,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.Translation{
		BusinessID:       11,
		EntityType:       "menu_item",
		EntityID:         3,
		FieldName:        "description",
		LanguageCode:     "de",
		OriginalText:     "Beef patty",
		TranslatedText:   "Beef patty",
		IsAutoTranslated: true,
	}).Error)

	var providerCalls int
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"translations":[{"translatedText":"Hamburger"}]}}`))
	}))
	defer provider.Close()

	service := NewTranslationService(database.GetDBWrapper(), "test-api-key")
	service.apiBaseURL = provider.URL

	require.NoError(t, service.TranslateMenuItemToLanguages(11, 3, "Burger", "Beef patty", []string{"de"}))
	assert.GreaterOrEqual(t, providerCalls, 1, "identity/echo rows must not be treated as fresh")
}

func TestTranslateTextWithSourceRejectsUnsupportedTargetLanguage(t *testing.T) {
	setupTranslationServiceTestDB(t)

	calledProvider := false
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledProvider = true
		w.WriteHeader(http.StatusOK)
	}))
	defer provider.Close()

	service := NewTranslationService(database.GetDBWrapper(), "test-api-key")
	service.apiBaseURL = provider.URL

	// "xx-not-in-registry" is intentionally not a real locale — fr/de/etc. are
	// registered guest locales and would be accepted post-restoration.
	translations, err := service.TranslateTextWithSource("hello", "en", []string{"xx-not-in-registry"})

	require.Error(t, err)
	assert.Nil(t, translations)
	assert.Contains(t, err.Error(), "unsupported guest target language")
	assert.False(t, calledProvider)
}

func TestTranslateCategorySkipsUnsupportedBusinessLanguageRows(t *testing.T) {
	setupTranslationServiceTestDB(t)

	business := &database.Business{
		BusinessId:     "biz-unsupported-language-row",
		Name:           "Unsupported Language Business",
		OwnerAddress:   "0xservice-owner",
		SettlementAddr: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		TippingAddr:    "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID:   business.ID,
		LanguageCode: "en",
		IsDefault:    true,
		DisplayOrder: 0,
	}).Error)
	// "xx-not-in-registry" is an unknown locale — TranslateCategory must skip
	// it without writing translation rows. Real-language codes (fr/de/etc.)
	// are now registered guest locales and would be accepted.
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID:   business.ID,
		LanguageCode: "xx-not-in-registry",
		IsDefault:    false,
		DisplayOrder: 1,
	}).Error)

	service := NewTranslationService(database.GetDBWrapper(), "")

	require.NoError(t, service.TranslateCategory(business.ID, 0, "Desserts", "Sweet plates"))

	var unsupportedCount int64
	require.NoError(t, database.GetDB().
		Model(&database.Translation{}).
		Where("business_id = ? AND language_code = ?", business.ID, "xx-not-in-registry").
		Count(&unsupportedCount).Error)
	assert.Zero(t, unsupportedCount)
}
