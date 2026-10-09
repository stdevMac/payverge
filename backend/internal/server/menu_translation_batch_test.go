package server

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupMenuTranslationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&database.Translation{}))
	return gormDB
}

func seedTranslation(t *testing.T, db *gorm.DB, businessID uint, entityType string, entityID uint, field, lang, text string) {
	t.Helper()
	seedTranslationWithOriginal(t, db, businessID, entityType, entityID, field, lang, "", text)
}

// seedTranslationWithOriginal is seedTranslation plus an explicit OriginalText.
// The menu read path (applyTranslationsToMenu) now compares the stored
// OriginalText against the current source text and treats a mismatch as missing
// (R3-MB-3 self-heal), so menu-path fixtures must seed the source text they
// expect the translation to apply against.
func seedTranslationWithOriginal(t *testing.T, db *gorm.DB, businessID uint, entityType string, entityID uint, field, lang, original, text string) {
	t.Helper()
	require.NoError(t, db.Create(&database.Translation{
		BusinessID:     businessID,
		EntityType:     entityType,
		EntityID:       entityID,
		FieldName:      field,
		LanguageCode:   lang,
		OriginalText:   original,
		TranslatedText: text,
	}).Error)
}

// TestApplyTranslationsToMenu_BatchesIntoSingleQuery is the access-shape guard for
// the hot guest menu path: regardless of how many categories/items/options the
// menu has, translation application must issue exactly ONE query (the batched
// load), not the previous ~2·(categories+items)+options+allergens+tags per-field
// N+1. It also asserts the translations are actually applied from the batch.
func TestApplyTranslationsToMenu_BatchesIntoSingleQuery(t *testing.T) {
	db := setupMenuTranslationTestDB(t)
	const bizID = uint(7)
	const lang = "es"

	// Translate category 0 (name+desc), item (0,0) name+desc, and item (0,0)'s
	// first option/allergen/tag, to exercise every entity-type branch.
	// Category 0 / item (0,0) source text below is "Cat 0"/"orig" and
	// "Item 0-0"/"orig"; seed the matching OriginalText so the freshness compare
	// on the read path accepts these rows.
	seedTranslationWithOriginal(t, db, bizID, "category", 0, "name", lang, "Cat 0", "Bebidas")
	seedTranslationWithOriginal(t, db, bizID, "category", 0, "description", lang, "orig", "Refrescos")
	seedTranslationWithOriginal(t, db, bizID, "menu_item", 0, "name", lang, "Item 0-0", "Café")
	seedTranslationWithOriginal(t, db, bizID, "menu_item", 0, "description", lang, "orig", "Recién hecho")
	seedTranslation(t, db, bizID, "menu_item_option", 0*1000+0, "name", lang, "Grande")
	seedTranslation(t, db, bizID, "allergen", 0*10000+0, "name", lang, "Lácteos")
	seedTranslation(t, db, bizID, "dietary_tag", 0*100000+0, "name", lang, "Vegano")

	// A multi-category, multi-item menu — enough that an N+1 would be obvious.
	categories := make([]database.MenuCategory, 3)
	for i := range categories {
		items := make([]database.MenuItem, 4)
		for j := range items {
			items[j] = database.MenuItem{
				Name: fmt.Sprintf("Item %d-%d", i, j), Description: "orig",
				Options:     []database.MenuItemOption{{Name: "Large"}, {Name: "Small"}},
				Allergens:   []string{"Dairy", "Nuts"},
				DietaryTags: []string{"Vegan", "GF"},
			}
		}
		categories[i] = database.MenuCategory{Name: fmt.Sprintf("Cat %d", i), Description: "orig", Items: items}
	}

	var queries int64
	require.NoError(t, db.Callback().Row().After("gorm:row").
		Register("test_raw_query_counter", func(tx *gorm.DB) { atomic.AddInt64(&queries, 1) }))
	defer func() { _ = db.Callback().Row().Remove("test_raw_query_counter") }()

	translated, missing := applyTranslationsToMenu(bizID, categories, lang)

	assert.Equal(t, int64(1), atomic.LoadInt64(&queries),
		"translation application must use exactly one batched query regardless of menu size")

	// Batch-applied translations are correct.
	assert.Equal(t, "Bebidas", translated[0].Name)
	assert.Equal(t, "Refrescos", translated[0].Description)
	assert.Equal(t, "Café", translated[0].Items[0].Name)
	assert.Equal(t, "Recién hecho", translated[0].Items[0].Description)
	assert.Equal(t, "Grande", translated[0].Items[0].Options[0].Name)
	assert.Equal(t, "Lácteos", translated[0].Items[0].Allergens[0])
	assert.Equal(t, "Vegano", translated[0].Items[0].DietaryTags[0])
	// Untranslated fields fall back to the original.
	assert.Equal(t, "Small", translated[0].Items[0].Options[1].Name)
	assert.Equal(t, "Nuts", translated[0].Items[0].Allergens[1])
	assert.Equal(t, "Cat 1", translated[1].Name) // category 1 not translated
	// Missing category/item name/description translations flag a backfill need.
	assert.True(t, missing, "untranslated category/item names must flag missing=true")
}

// TestApplyTranslationsToMenu_LatestRowWinsAndEmptyIsMiss pins the semantics of
// the batched lookup against the prior Order("id DESC").First() behaviour: the
// highest-id row wins, and if that latest row is empty the field is treated as a
// miss (falls back to original) even when an older non-empty row exists.
func TestApplyTranslationsToMenu_LatestRowWinsAndEmptyIsMiss(t *testing.T) {
	db := setupMenuTranslationTestDB(t)
	const bizID = uint(9)
	const lang = "es"

	// category 0 name: older non-empty, then a newer empty row → must be a miss.
	// OriginalText matches the source "Original" so the freshness compare accepts
	// the rows (the miss here is driven by the empty latest, not a stale source).
	seedTranslationWithOriginal(t, db, bizID, "category", 0, "name", lang, "Original", "Antiguo")
	seedTranslationWithOriginal(t, db, bizID, "category", 0, "name", lang, "Original", "")
	// category 0 description: two non-empty rows → latest wins.
	seedTranslationWithOriginal(t, db, bizID, "category", 0, "description", lang, "origDesc", "v1")
	seedTranslationWithOriginal(t, db, bizID, "category", 0, "description", lang, "origDesc", "v2")

	categories := []database.MenuCategory{{Name: "Original", Description: "origDesc"}}
	translated, missing := applyTranslationsToMenu(bizID, categories, lang)

	assert.Equal(t, "Original", translated[0].Name, "latest-empty translation is a miss → original kept")
	assert.Equal(t, "v2", translated[0].Description, "latest non-empty row wins")
	assert.True(t, missing, "the empty-latest name counts as missing")
}

func TestTranslationLookupLoadsOnlyScopedEffectiveRows(t *testing.T) {
	db := setupMenuTranslationTestDB(t)
	const bizID = uint(17)
	const lang = "es"

	seedTranslationWithOriginal(t, db, bizID, "menu_item", 42, "name", lang, "Steak", "Bistec antiguo")
	seedTranslationWithOriginal(t, db, bizID, "menu_item", 42, "name", lang, "Steak", "Bistec vigente")
	seedTranslationWithOriginal(t, db, bizID, "menu_item", 99, "name", lang, "Other", "Otro")
	seedTranslationWithOriginal(t, db, bizID, "category", 42, "name", lang, "Mains", "Principales")

	var queries int64
	require.NoError(t, db.Callback().Row().After("gorm:row").
		Register("test_scoped_translation_query_counter", func(tx *gorm.DB) { atomic.AddInt64(&queries, 1) }))
	defer func() { _ = db.Callback().Row().Remove("test_scoped_translation_query_counter") }()

	var scope translationLookupScope
	scope.add("menu_item", 42)
	lookup := newTranslationLookup(database.GetDBWrapper(), bizID, lang, scope)
	require.NoError(t, lookup.err)
	assert.Equal(t, int64(1), atomic.LoadInt64(&queries), "one bounded snapshot query must serve the whole response")
	assert.Len(t, lookup.byKey, 1, "only the latest row for the scoped field should enter the snapshot")
	assert.Equal(t, "Bistec vigente", lookup.byKey[translationLookupKey("menu_item", 42, "name")])
	assert.Empty(t, lookup.byKey[translationLookupKey("menu_item", 99, "name")])
}

func TestTranslationLookupHandlesNullableUpdatedAtDeterministically(t *testing.T) {
	db := setupMenuTranslationTestDB(t)
	const bizID = uint(18)
	const lang = "es"

	seedTranslationWithOriginal(t, db, bizID, "menu_item", 42, "name", lang, "Steak", "Bistec")
	var row database.Translation
	require.NoError(t, db.Where("business_id = ? AND entity_id = ?", bizID, 42).First(&row).Error)
	require.NoError(t, db.Exec("UPDATE translations SET updated_at = NULL WHERE id = ?", row.ID).Error)

	var scope translationLookupScope
	scope.add("menu_item", 42)
	first := newTranslationLookup(database.GetDBWrapper(), bizID, lang, scope)
	require.NoError(t, first.err, "a nullable updated_at must not abort the guest translation snapshot")
	require.NotEmpty(t, first.revision)

	second := newTranslationLookup(database.GetDBWrapper(), bizID, lang, scope)
	require.NoError(t, second.err)
	assert.Equal(t, first.revision, second.revision, "NULL timestamps must hash deterministically")
	assert.Equal(t, "Bistec", first.byKey[translationLookupKey("menu_item", 42, "name")])
}

func TestTranslationRevisionDigestDistinguishesNullAndZeroUpdatedAt(t *testing.T) {
	base := translationSnapshotRow{
		ID: 1, EntityType: "menu_item", EntityID: 42, FieldName: "name",
		LanguageCode: "es", OriginalText: sql.NullString{String: "Steak", Valid: true}, TranslatedText: "Bistec",
	}
	nullTimestamp := base
	nullTimestamp.UpdatedAt = sql.NullTime{}
	zeroTimestamp := base
	zeroTimestamp.UpdatedAt = sql.NullTime{Time: time.Time{}, Valid: true}

	assert.NotEqual(t, translationRevisionDigest([]translationSnapshotRow{nullTimestamp}),
		translationRevisionDigest([]translationSnapshotRow{zeroTimestamp}),
		"NULL and an explicit zero timestamp must have distinct deterministic digest inputs")
}

func TestApplyTranslationsToMenuDoesNotApplyAmbiguousPositionActualCollision(t *testing.T) {
	db := setupMenuTranslationTestDB(t)
	const bizID = uint(19)
	const lang = "es"
	seedTranslationWithOriginal(t, db, bizID, "menu_item", 1, "name", lang, "Same source", "No sé a qué elemento pertenece")

	categories := []database.MenuCategory{{
		Items: []database.MenuItem{
			{ID: "1", Name: "Same source"}, // actual ID 1 collides with item position 1 below.
			{ID: "item-two", Name: "Same source"},
		},
	}}
	translated, missing := applyTranslationsToMenu(bizID, categories, lang)

	assert.True(t, missing, "an ambiguous translation must remain unresolved")
	assert.Equal(t, "Same source", translated[0].Items[0].Name)
	assert.Equal(t, "Same source", translated[0].Items[1].Name)
}

func TestTranslationRevisionDigestIsFullWidthAndChangesWithContent(t *testing.T) {
	base := translationSnapshotRow{
		ID: 1, EntityType: "menu_item", EntityID: 42, FieldName: "name",
		LanguageCode: "es", OriginalText: sql.NullString{String: "Steak", Valid: true}, TranslatedText: "Bistec",
		UpdatedAt: sql.NullTime{Time: time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC), Valid: true},
	}
	changed := base
	changed.TranslatedText = "Plato de bistec"

	before := translationRevisionDigest([]translationSnapshotRow{base})
	after := translationRevisionDigest([]translationSnapshotRow{changed})
	require.Len(t, before, sha256.Size*2, "revision must contain the complete SHA-256 hex digest")
	require.Len(t, after, sha256.Size*2, "revision must contain the complete SHA-256 hex digest")
	assert.NotEqual(t, before, after, "changed translation content must never reuse the old revision")
}

// TestApplyBusinessContentTranslations_BatchesIntoSingleQuery guards the same
// access shape for the guest storefront page (business prose + special features):
// one batched query regardless of how many special features there are.
func TestApplyBusinessContentTranslations_BatchesIntoSingleQuery(t *testing.T) {
	db := setupMenuTranslationTestDB(t)
	const bizID = uint(11)
	const lang = "es"

	seedTranslation(t, db, bizID, "business", bizID, "description", lang, "Descripción")
	seedTranslation(t, db, bizID, "business", bizID, "welcome_message", lang, "Bienvenido")
	seedTranslation(t, db, bizID, "business", bizID, "about_story", lang, "Historia")
	seedTranslation(t, db, bizID, "special_feature", 100, "title", lang, "Terraza")

	business := &database.Business{
		Name: "Café", Description: "desc", WelcomeMessage: "welcome", AboutStory: "about",
	}
	business.ID = bizID
	features := []database.BusinessSpecialFeature{
		{Title: "Patio", Description: "outdoor"},
		{Title: "Wifi", Description: "free"},
		{Title: "Parking", Description: "lot"},
	}
	features[0].ID = 100

	var queries int64
	require.NoError(t, db.Callback().Row().After("gorm:row").
		Register("test_query_counter", func(tx *gorm.DB) { atomic.AddInt64(&queries, 1) }))
	defer func() { _ = db.Callback().Row().Remove("test_query_counter") }()

	missing := applyBusinessContentTranslations(business, features, nil, lang)

	assert.Equal(t, int64(1), atomic.LoadInt64(&queries),
		"business-content translation must use exactly one batched query regardless of feature count")
	assert.Equal(t, "Descripción", business.Description)
	assert.Equal(t, "Bienvenido", business.WelcomeMessage)
	assert.Equal(t, "Historia", business.AboutStory)
	assert.Equal(t, "Terraza", features[0].Title)
	assert.True(t, missing, "untranslated feature titles/descriptions flag a backfill need")
}
