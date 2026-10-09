package demo

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// The AR demo carta is authored in Spanish (business default language "es")
// and ships English guest translations so the multilingual menu toggle has
// real content to show during demos.
func TestEnsureDemoLanguagesAndTranslationsSeedsEnglishMenu(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-admin-translations@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed-v1", BaselineDays: 3})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var businesses []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Find(&businesses).Error)
	require.NotEmpty(t, businesses)

	for _, b := range businesses {
		var langs []database.BusinessLanguage
		require.NoError(t, db.Where("business_id = ?", b.ID).Order("display_order asc").Find(&langs).Error)
		require.Len(t, langs, 2, "demo business %s must enable es + en", b.BusinessId)
		require.Equal(t, "es", langs[0].LanguageCode)
		require.True(t, langs[0].IsDefault, "the AR demo carta is authored in Spanish")
		require.Equal(t, "en", langs[1].LanguageCode)
		require.False(t, langs[1].IsDefault)

		// Spot-check one row end to end: the first starter on the carta.
		var tr database.Translation
		require.NoError(t, db.Where(
			"business_id = ? AND language_code = ? AND entity_type = ? AND entity_id = ? AND field_name = ?",
			b.ID, "en", "menu_item", 0, "name",
		).First(&tr).Error, "business %s must carry an English translation for the first menu item", b.BusinessId)
		require.Equal(t, "Provoleta", tr.OriginalText)
		require.Equal(t, "Grilled provoleta cheese", tr.TranslatedText)
		require.Equal(t, "demo_seed", tr.TranslationSource)

		var count int64
		require.NoError(t, db.Model(&database.Translation{}).
			Where("business_id = ? AND language_code = ?", b.ID, "en").
			Count(&count).Error)
		require.EqualValues(t, 58, count,
			"6 categories + 23 items, name + description each")
	}
}

// A second ensure with the same seed version must not duplicate translation
// rows, and hand-corrupted rows must heal back to the seed copy.
func TestEnsureDemoTranslationsIdempotentAndSelfHealing(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-admin-translations-heal@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed-v1", BaselineDays: 3})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var b database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).First(&b).Error)

	// Corrupt one translation the way a stale older seed would leave it.
	require.NoError(t, db.Model(&database.Translation{}).
		Where("business_id = ? AND language_code = ? AND entity_type = ? AND entity_id = ? AND field_name = ?",
			b.ID, "en", "menu_item", 0, "name").
		Updates(map[string]interface{}{
			"original_text":   "Harvest Bowl",
			"translated_text": "Bowl de la cosecha",
		}).Error)

	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var tr database.Translation
	require.NoError(t, db.Where(
		"business_id = ? AND language_code = ? AND entity_type = ? AND entity_id = ? AND field_name = ?",
		b.ID, "en", "menu_item", 0, "name",
	).First(&tr).Error)
	require.Equal(t, "Provoleta", tr.OriginalText, "ensure must heal drifted translation source text")
	require.Equal(t, "Grilled provoleta cheese", tr.TranslatedText, "ensure must heal drifted translation copy")

	var count int64
	require.NoError(t, db.Model(&database.Translation{}).
		Where("business_id = ? AND language_code = ?", b.ID, "en").
		Count(&count).Error)
	require.EqualValues(t, 58, count, "re-ensure must not duplicate translation rows")
}
