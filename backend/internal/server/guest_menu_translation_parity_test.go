package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// seedPublicTableMenuParityBusiness builds a business with custom URL + table +
// bilingual menu so /business/:customUrl/menu and /guest/table/:code/menu can be
// compared (GUEST-003).
func seedPublicTableMenuParityBusiness(t *testing.T) (*database.Business, *database.Table) {
	t.Helper()
	setupPublicGuestTableHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Translation{}))
	services.ResetPricingCache()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	business := &database.Business{
		BusinessId:          "parity-" + suffix,
		Name:                "Parity Cafe " + suffix,
		CustomURL:           "parity-cafe-" + suffix,
		IsActive:            true,
		BusinessPageEnabled: true,
		DefaultLanguage:     "en",
		OwnerAddress:        "0xOWNER",
		SettlementAddr:      "0x1111111111111111111111111111111111111111",
		TippingAddr:         "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID: business.ID, LanguageCode: "en", IsDefault: true, DisplayOrder: 0,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID: business.ID, LanguageCode: "es", IsDefault: false, DisplayOrder: 1,
	}).Error)

	categories := []database.MenuCategory{
		{
			ID:   "mains",
			Name: "Mains",
			Items: []database.MenuItem{
				{ID: "demo-bowl", Name: "Harvest Bowl", Description: "Roasted vegetables, grains, herbs", Price: 18.5, IsAvailable: true},
				{ID: "demo-steak", Name: "Steak Plate", Description: "Charred steak, potatoes, chimichurri", Price: 42, IsAvailable: true},
			},
		},
	}
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID,
		Categories: string(raw),
		IsActive:   true,
		Version:    1,
	}).Error)
	offer := &database.Offer{
		BusinessID:    business.ID,
		Name:          "Summer Special",
		Description:   "Twenty percent off",
		DiscountType:  "percentage",
		DiscountValue: 20,
		WeekdayMask:   127,
		IsActive:      true,
		ApplicableTo:  "all",
	}
	require.NoError(t, database.GetDB().Create(offer).Error)
	bundle := &database.Bundle{
		BusinessID:  business.ID,
		Name:        "Harvest Pair",
		Description: "Bowl and drink together",
		Price:       24,
		Items:       `[{"menu_item_id":"demo-bowl","quantity":1}]`,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(bundle).Error)

	// Position-based Spanish rows (entity_id = cat*1000+item). Include category
	// name plus promotion rows so the complete fixture can exercise all guest
	// menu representations without triggering incomplete-cache behavior.
	now := time.Now().UTC()
	rows := []database.Translation{
		{BusinessID: business.ID, EntityType: "category", EntityID: 0, FieldName: "name", LanguageCode: "es", OriginalText: "Mains", TranslatedText: "Principales", CreatedAt: now, UpdatedAt: now},
		{BusinessID: business.ID, EntityType: "menu_item", EntityID: 0, FieldName: "name", LanguageCode: "es", OriginalText: "Harvest Bowl", TranslatedText: "Bowl de la cosecha", CreatedAt: now, UpdatedAt: now},
		{BusinessID: business.ID, EntityType: "menu_item", EntityID: 0, FieldName: "description", LanguageCode: "es", OriginalText: "Roasted vegetables, grains, herbs", TranslatedText: "Verduras asadas, granos y hierbas", CreatedAt: now, UpdatedAt: now},
		{BusinessID: business.ID, EntityType: "menu_item", EntityID: 1, FieldName: "name", LanguageCode: "es", OriginalText: "Steak Plate", TranslatedText: "Plato de bistec", CreatedAt: now, UpdatedAt: now},
		{BusinessID: business.ID, EntityType: "menu_item", EntityID: 1, FieldName: "description", LanguageCode: "es", OriginalText: "Charred steak, potatoes, chimichurri", TranslatedText: "Bistec a la parrilla, papas y chimichurri", CreatedAt: now, UpdatedAt: now},
		{BusinessID: business.ID, EntityType: offerTranslationEntityType, EntityID: offer.ID, FieldName: "name", LanguageCode: "es", OriginalText: offer.Name, TranslatedText: "Especial de verano", CreatedAt: now, UpdatedAt: now},
		{BusinessID: business.ID, EntityType: offerTranslationEntityType, EntityID: offer.ID, FieldName: "description", LanguageCode: "es", OriginalText: offer.Description, TranslatedText: "Veinte por ciento de descuento", CreatedAt: now, UpdatedAt: now},
		{BusinessID: business.ID, EntityType: bundleTranslationEntityType, EntityID: bundle.ID, FieldName: "name", LanguageCode: "es", OriginalText: bundle.Name, TranslatedText: "Pareja de cosecha", CreatedAt: now, UpdatedAt: now},
		{BusinessID: business.ID, EntityType: bundleTranslationEntityType, EntityID: bundle.ID, FieldName: "description", LanguageCode: "es", OriginalText: bundle.Description, TranslatedText: "Bowl y bebida juntos", CreatedAt: now, UpdatedAt: now},
	}
	for i := range rows {
		require.NoError(t, database.GetDB().Create(&rows[i]).Error)
	}

	table := &database.Table{
		BusinessID: business.ID,
		TableCode:  "PARITY-" + suffix,
		Name:       "Table 1",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)
	return business, table
}

type guestMenuPayload struct {
	Categories []database.MenuCategory `json:"categories"`
	Offers     []database.Offer        `json:"offers"`
	Bundles    []database.Bundle       `json:"bundles"`
}

func decodeGuestMenuPayload(t *testing.T, body []byte) guestMenuPayload {
	t.Helper()
	var payload guestMenuPayload
	require.NoError(t, json.Unmarshal(body, &payload))
	return payload
}

func decodeGuestMenuCategories(t *testing.T, body []byte) []database.MenuCategory {
	t.Helper()
	var payload guestMenuPayload
	require.NoError(t, json.Unmarshal(body, &payload))
	return payload.Categories
}

func requestPublicParityMenu(t *testing.T, business *database.Business, language string, includeLanguage bool, ifNoneMatch string) (*httptest.ResponseRecorder, guestMenuPayload) {
	t.Helper()
	path := "/business/" + business.CustomURL + "/menu"
	if includeLanguage {
		path += "?language=" + language
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	if ifNoneMatch != "" {
		c.Request.Header.Set("If-None-Match", ifNoneMatch)
	}
	GetMenuByBusinessCustomUrl(c)
	if w.Code == http.StatusNotModified {
		return w, guestMenuPayload{}
	}
	return w, decodeGuestMenuPayload(t, w.Body.Bytes())
}

func requestTableParityMenu(t *testing.T, table *database.Table, language string, includeLanguage bool, ifNoneMatch string) (*httptest.ResponseRecorder, guestMenuPayload) {
	t.Helper()
	path := "/guest/table/" + table.TableCode + "/menu"
	if includeLanguage {
		path += "?language=" + language
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	if ifNoneMatch != "" {
		c.Request.Header.Set("If-None-Match", ifNoneMatch)
	}
	GetMenuByTableCode(c)
	if w.Code == http.StatusNotModified {
		return w, guestMenuPayload{}
	}
	return w, decodeGuestMenuPayload(t, w.Body.Bytes())
}

func TestGetTableByCodePublic_AppliesSpanishCatalogTranslations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, table := seedPublicTableMenuParityBusiness(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/guest/table/"+table.TableCode+"?language=es",
		nil,
	)
	GetTableByCodePublic(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var payload struct {
		Categories []database.MenuCategory `json:"categories"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.NotEmpty(t, payload.Categories)
	assert.Equal(t, "Principales", payload.Categories[0].Name)
	require.NotEmpty(t, payload.Categories[0].Items)
	assert.Equal(t, "Bowl de la cosecha", payload.Categories[0].Items[0].Name)
	assert.NotEqual(t, "Harvest Bowl", payload.Categories[0].Items[0].Name)
}

func TestPublicAndTableMenus_SpanishParityForSameBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, table := seedPublicTableMenuParityBusiness(t)

	// Public /b path
	publicW := httptest.NewRecorder()
	publicC, _ := gin.CreateTestContext(publicW)
	publicC.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	publicC.Request = httptest.NewRequest(http.MethodGet, "/business/"+business.CustomURL+"/menu?language=es", nil)
	GetMenuByBusinessCustomUrl(publicC)
	require.Equal(t, http.StatusOK, publicW.Code, publicW.Body.String())
	publicPayload := decodeGuestMenuPayload(t, publicW.Body.Bytes())
	publicCats := publicPayload.Categories

	// Table /t path
	tableW := httptest.NewRecorder()
	tableC, _ := gin.CreateTestContext(tableW)
	tableC.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	tableC.Request = httptest.NewRequest(http.MethodGet, "/guest/table/"+table.TableCode+"/menu?language=es", nil)
	GetMenuByTableCode(tableC)
	require.Equal(t, http.StatusOK, tableW.Code, tableW.Body.String())
	tablePayload := decodeGuestMenuPayload(t, tableW.Body.Bytes())
	tableCats := tablePayload.Categories

	require.Len(t, publicCats, 1)
	require.Len(t, tableCats, 1)
	require.Len(t, publicCats[0].Items, 2)
	require.Len(t, tableCats[0].Items, 2)

	for i := range publicCats[0].Items {
		assert.Equal(t, tableCats[0].Items[i].Name, publicCats[0].Items[i].Name,
			"public and table name must match for item %d", i)
		assert.Equal(t, tableCats[0].Items[i].Description, publicCats[0].Items[i].Description,
			"public and table description must match for item %d", i)
	}
	assert.Equal(t, "Plato de bistec", publicCats[0].Items[1].Name)
	assert.Equal(t, "Bistec a la parrilla, papas y chimichurri", publicCats[0].Items[1].Description)
	require.Len(t, publicPayload.Offers, 1)
	require.Len(t, tablePayload.Offers, 1)
	assert.Equal(t, tablePayload.Offers[0].Name, publicPayload.Offers[0].Name)
	assert.Equal(t, tablePayload.Offers[0].Description, publicPayload.Offers[0].Description)
	assert.Equal(t, "Especial de verano", publicPayload.Offers[0].Name)
	require.Len(t, publicPayload.Bundles, 1)
	require.Len(t, tablePayload.Bundles, 1)
	assert.Equal(t, tablePayload.Bundles[0].Name, publicPayload.Bundles[0].Name)
	assert.Equal(t, tablePayload.Bundles[0].Description, publicPayload.Bundles[0].Description)
	assert.Equal(t, "Pareja de cosecha", publicPayload.Bundles[0].Name)
}

func TestGuestMenuETag_ChangesAfterTranslationBackfillTimestamp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, _ := seedPublicTableMenuParityBusiness(t)

	revision := func() string {
		t.Helper()
		var scope translationLookupScope
		scope.add("menu_item", 1)
		lookup := newTranslationLookup(database.GetDBWrapper(), business.ID, "es", scope)
		require.NoError(t, lookup.err)
		return lookup.revision
	}

	before := revision()
	require.NotEmpty(t, before)

	// Simulate a backfill update that touches only updated_at and does not
	// bump menu.Version.
	require.NoError(t, database.GetDB().Model(&database.Translation{}).
		Where("business_id = ? AND language_code = ? AND entity_type = ? AND entity_id = ? AND field_name = ?",
			business.ID, "es", "menu_item", 1, "name").
		Update("updated_at", time.Now().UTC().Add(time.Minute)).Error)

	after := revision()
	assert.NotEqual(t, before, after, "translation write must change the guest menu revision")

	base := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	etagBefore := guestMenuETag(business.ID, 1, "es", base, before)
	etagAfter := guestMenuETag(business.ID, 1, "es", base, after)
	assert.NotEqual(t, etagBefore, etagAfter)
}

func TestPublicMenu_LanguageSwitchAndCacheMissHit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, _ := seedPublicTableMenuParityBusiness(t)

	// Cache miss / first Spanish fetch
	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	c1.Request = httptest.NewRequest(http.MethodGet, "/business/"+business.CustomURL+"/menu?language=es", nil)
	GetMenuByBusinessCustomUrl(c1)
	require.Equal(t, http.StatusOK, w1.Code)
	etag := w1.Header().Get("ETag")
	require.NotEmpty(t, etag)
	esCats := decodeGuestMenuCategories(t, w1.Body.Bytes())
	require.Equal(t, "Plato de bistec", esCats[0].Items[1].Name)

	// Cache hit via If-None-Match
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	c2.Request = httptest.NewRequest(http.MethodGet, "/business/"+business.CustomURL+"/menu?language=es", nil)
	c2.Request.Header.Set("If-None-Match", etag)
	GetMenuByBusinessCustomUrl(c2)
	assert.Equal(t, http.StatusNotModified, w2.Code)

	// Language switch to English must not reuse Spanish body semantics
	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	c3.Request = httptest.NewRequest(http.MethodGet, "/business/"+business.CustomURL+"/menu?language=en", nil)
	GetMenuByBusinessCustomUrl(c3)
	require.Equal(t, http.StatusOK, w3.Code)
	enCats := decodeGuestMenuCategories(t, w3.Body.Bytes())
	require.Equal(t, "Steak Plate", enCats[0].Items[1].Name)
	assert.NotEqual(t, etag, w3.Header().Get("ETag"))
}

func TestTableMenu_UnchangedConditionalRequestReturns304(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, table := seedPublicTableMenuParityBusiness(t)

	first, payload := requestTableParityMenu(t, table, "es", true, "")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	require.Len(t, payload.Categories[0].Items, 2)
	etag := first.Header().Get("ETag")
	require.NotEmpty(t, etag)

	unchanged, _ := requestTableParityMenu(t, table, "es", true, etag)
	require.Equal(t, http.StatusNotModified, unchanged.Code)
	require.Equal(t, etag, unchanged.Header().Get("ETag"))
}

func TestPublicAndTableMenus_IncompleteTranslationsPreserveSourceAndSkip304(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, table := seedPublicTableMenuParityBusiness(t)
	backfillKey := fmt.Sprintf("%d:%s", business.ID, "es")
	menuTranslationBackfillInFlight.Store(backfillKey, struct{}{})
	promotionTranslationBackfillInFlight.Store(backfillKey, struct{}{})
	t.Cleanup(func() {
		menuTranslationBackfillInFlight.Delete(backfillKey)
		promotionTranslationBackfillInFlight.Delete(backfillKey)
	})

	var offer database.Offer
	require.NoError(t, database.GetDB().Where("business_id = ?", business.ID).First(&offer).Error)
	var bundle database.Bundle
	require.NoError(t, database.GetDB().Where("business_id = ?", business.ID).First(&bundle).Error)
	for _, entity := range []struct {
		entityType string
		entityID   uint
	}{
		{entityType: "menu_item", entityID: 1},
		{entityType: offerTranslationEntityType, entityID: offer.ID},
		{entityType: bundleTranslationEntityType, entityID: bundle.ID},
	} {
		require.NoError(t, database.GetDB().Where(
			"business_id = ? AND language_code = ? AND entity_type = ? AND entity_id = ?",
			business.ID, "es", entity.entityType, entity.entityID,
		).Delete(&database.Translation{}).Error)
	}

	assertIncomplete := func(w *httptest.ResponseRecorder, payload guestMenuPayload) string {
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
		require.Len(t, payload.Categories, 1)
		require.Len(t, payload.Categories[0].Items, 2)
		assert.Equal(t, "Steak Plate", payload.Categories[0].Items[1].Name)
		assert.Equal(t, "Charred steak, potatoes, chimichurri", payload.Categories[0].Items[1].Description)
		require.Len(t, payload.Offers, 1)
		assert.Equal(t, "Summer Special", payload.Offers[0].Name)
		assert.Equal(t, "Twenty percent off", payload.Offers[0].Description)
		require.Len(t, payload.Bundles, 1)
		assert.Equal(t, "Harvest Pair", payload.Bundles[0].Name)
		assert.Equal(t, "Bowl and drink together", payload.Bundles[0].Description)
		return w.Header().Get("ETag")
	}

	publicFirst, publicPayload := requestPublicParityMenu(t, business, "es", true, "")
	publicETag := assertIncomplete(publicFirst, publicPayload)
	require.NotEmpty(t, publicETag)
	publicRetry, publicRetryPayload := requestPublicParityMenu(t, business, "es", true, publicETag)
	assertIncomplete(publicRetry, publicRetryPayload)

	tableFirst, tablePayload := requestTableParityMenu(t, table, "es", true, "")
	tableETag := assertIncomplete(tableFirst, tablePayload)
	require.NotEmpty(t, tableETag)
	tableRetry, tableRetryPayload := requestTableParityMenu(t, table, "es", true, tableETag)
	assertIncomplete(tableRetry, tableRetryPayload)
}

func TestPublicAndTableMenus_OmittedAndEmptyLanguageUseDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, table := seedPublicTableMenuParityBusiness(t)

	for _, includeLanguage := range []bool{false, true} {
		publicW, publicPayload := requestPublicParityMenu(t, business, "", includeLanguage, "")
		require.Equal(t, http.StatusOK, publicW.Code, publicW.Body.String())
		require.Equal(t, "Steak Plate", publicPayload.Categories[0].Items[1].Name)
		require.Equal(t, "Charred steak, potatoes, chimichurri", publicPayload.Categories[0].Items[1].Description)

		tableW, tablePayload := requestTableParityMenu(t, table, "", includeLanguage, "")
		require.Equal(t, http.StatusOK, tableW.Code, tableW.Body.String())
		require.Equal(t, "Steak Plate", tablePayload.Categories[0].Items[1].Name)
		require.Equal(t, "Charred steak, potatoes, chimichurri", tablePayload.Categories[0].Items[1].Description)
	}
}

// TestPublicAndTableMenus_ChangedTranslationRowsStayInParityAcrossCachePaths
// protects the live GUEST-003 failure mode: a public response can be retained
// by a conditional cache hit while the table route reads the changed
// translation row on a cache miss. A changed translation must advance the
// shared guest-menu revision and force the public route to return the new body.
func TestPublicAndTableMenus_ChangedTranslationRowsStayInParityAcrossCachePaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, table := seedPublicTableMenuParityBusiness(t)

	requestPublic := func(ifNoneMatch string) (*httptest.ResponseRecorder, []database.MenuCategory) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
		c.Request = httptest.NewRequest(http.MethodGet, "/business/"+business.CustomURL+"/menu?language=es", nil)
		if ifNoneMatch != "" {
			c.Request.Header.Set("If-None-Match", ifNoneMatch)
		}
		GetMenuByBusinessCustomUrl(c)
		if w.Code == http.StatusNotModified {
			return w, nil
		}
		return w, decodeGuestMenuCategories(t, w.Body.Bytes())
	}

	// Prime the public cache with the original translation.
	firstPublic, firstCategories := requestPublic("")
	require.Equal(t, http.StatusOK, firstPublic.Code, firstPublic.Body.String())
	oldETag := firstPublic.Header().Get("ETag")
	require.NotEmpty(t, oldETag)
	require.Equal(t, "Plato de bistec", firstCategories[0].Items[1].Name)
	publicCacheHit, _ := requestPublic(oldETag)
	require.Equal(t, http.StatusNotModified, publicCacheHit.Code)

	// Change only the translation row. This mirrors a translation backfill that
	// updates content in place and does not bump menu.Version.
	require.NoError(t, database.GetDB().Model(&database.Translation{}).Where(
		"business_id = ? AND language_code = ? AND entity_type = ? AND entity_id = ? AND field_name = ?",
		business.ID, "es", "menu_item", 1, "name",
	).UpdateColumn("translated_text", "Bistec actualizado").Error)
	require.NoError(t, database.GetDB().Model(&database.Translation{}).Where(
		"business_id = ? AND language_code = ? AND entity_type = ? AND entity_id = ? AND field_name = ?",
		business.ID, "es", "menu_item", 1, "description",
	).
		UpdateColumn("translated_text", "Bistec, papas y chimichurri actualizados").Error)

	// The table representation is a fresh body read and must expose the change.
	tableW := httptest.NewRecorder()
	tableC, _ := gin.CreateTestContext(tableW)
	tableC.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	tableC.Request = httptest.NewRequest(http.MethodGet, "/guest/table/"+table.TableCode+"/menu?language=es", nil)
	GetMenuByTableCode(tableC)
	require.Equal(t, http.StatusOK, tableW.Code, tableW.Body.String())
	tableCategories := decodeGuestMenuCategories(t, tableW.Body.Bytes())
	require.Equal(t, "Bistec actualizado", tableCategories[0].Items[1].Name)
	require.Equal(t, "Bistec, papas y chimichurri actualizados", tableCategories[0].Items[1].Description)

	// A public conditional request must not 304 the old body after the row
	// changes; it must return the same updated representation as /t.
	publicAfterChange, publicCategoriesAfterChange := requestPublic(oldETag)
	require.Equal(t, http.StatusOK, publicAfterChange.Code, publicAfterChange.Body.String())
	newETag := publicAfterChange.Header().Get("ETag")
	require.NotEmpty(t, newETag)
	assert.NotEqual(t, oldETag, newETag)
	require.Equal(t, "Bistec actualizado", publicCategoriesAfterChange[0].Items[1].Name)
	require.Equal(t, "Bistec, papas y chimichurri actualizados", publicCategoriesAfterChange[0].Items[1].Description)
	assert.Equal(t, tableCategories[0].Items[1].Name, publicCategoriesAfterChange[0].Items[1].Name)
	assert.Equal(t, tableCategories[0].Items[1].Description, publicCategoriesAfterChange[0].Items[1].Description)
	newPublicCacheHit, _ := requestPublic(newETag)
	require.Equal(t, http.StatusNotModified, newPublicCacheHit.Code)
}

func TestPublicAndTableMenus_ApplyNumericEntityIDsNotArrayIndexes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, table := seedPublicTableMenuParityBusiness(t)

	var menu database.Menu
	require.NoError(t, database.GetDB().Where("business_id = ?", business.ID).First(&menu).Error)
	var categories []database.MenuCategory
	require.NoError(t, json.Unmarshal([]byte(menu.Categories), &categories))
	require.Len(t, categories, 1)
	categories[0].ID = "42"
	categories[0].Items[0].ID = "4242"
	categories[0].Items[1].ID = "5252"
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Model(&database.Menu{}).
		Where("id = ?", menu.ID).Update("categories", string(raw)).Error)
	services.ResetPricingCache()

	// Remove the legacy positional rows so a passing response proves the
	// lookup scope and overlay both honor the IDs carried by the menu itself.
	require.NoError(t, database.GetDB().Where(
		"business_id = ? AND language_code = ? AND ((entity_type = ? AND entity_id IN ?) OR (entity_type = ? AND entity_id IN ?))",
		business.ID, "es", "category", []uint{0}, "menu_item", []uint{0, 1},
	).Delete(&database.Translation{}).Error)
	now := time.Now().UTC()
	rows := []database.Translation{
		{BusinessID: business.ID, EntityType: "category", EntityID: 42, FieldName: "name", LanguageCode: "es", OriginalText: "Mains", TranslatedText: "Platos principales", CreatedAt: now, UpdatedAt: now},
		{BusinessID: business.ID, EntityType: "menu_item", EntityID: 4242, FieldName: "name", LanguageCode: "es", OriginalText: "Harvest Bowl", TranslatedText: "Bowl traducido", CreatedAt: now, UpdatedAt: now},
		{BusinessID: business.ID, EntityType: "menu_item", EntityID: 4242, FieldName: "description", LanguageCode: "es", OriginalText: "Roasted vegetables, grains, herbs", TranslatedText: "Verduras traducidas", CreatedAt: now, UpdatedAt: now},
		{BusinessID: business.ID, EntityType: "menu_item", EntityID: 5252, FieldName: "name", LanguageCode: "es", OriginalText: "Steak Plate", TranslatedText: "Plato traducido", CreatedAt: now, UpdatedAt: now},
		{BusinessID: business.ID, EntityType: "menu_item", EntityID: 5252, FieldName: "description", LanguageCode: "es", OriginalText: "Charred steak, potatoes, chimichurri", TranslatedText: "Bistec traducido", CreatedAt: now, UpdatedAt: now},
	}
	for i := range rows {
		require.NoError(t, database.GetDB().Create(&rows[i]).Error)
	}

	publicW, publicPayload := requestPublicParityMenu(t, business, "es", true, "")
	tableW, tablePayload := requestTableParityMenu(t, table, "es", true, "")
	require.Equal(t, http.StatusOK, publicW.Code, publicW.Body.String())
	require.Equal(t, http.StatusOK, tableW.Code, tableW.Body.String())
	assert.Equal(t, "Platos principales", publicPayload.Categories[0].Name)
	assert.Equal(t, "Bowl traducido", publicPayload.Categories[0].Items[0].Name)
	assert.Equal(t, "Plato traducido", publicPayload.Categories[0].Items[1].Name)
	assert.Equal(t, publicPayload.Categories, tablePayload.Categories)
}

func TestPublicAndTableMenus_EsARFallsBackToEsRows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, table := seedPublicTableMenuParityBusiness(t)

	assertSpanishMenu := func(t *testing.T, payload guestMenuPayload, cacheControl string) {
		t.Helper()
		require.Len(t, payload.Categories, 1)
		require.Len(t, payload.Categories[0].Items, 2)
		assert.Equal(t, "Principales", payload.Categories[0].Name)
		assert.Equal(t, "Bowl de la cosecha", payload.Categories[0].Items[0].Name)
		assert.Equal(t, "Plato de bistec", payload.Categories[0].Items[1].Name)
		assert.Equal(t, "Bistec a la parrilla, papas y chimichurri", payload.Categories[0].Items[1].Description)
		require.Len(t, payload.Offers, 1)
		assert.Equal(t, "Especial de verano", payload.Offers[0].Name)
		require.Len(t, payload.Bundles, 1)
		assert.Equal(t, "Pareja de cosecha", payload.Bundles[0].Name)
		assert.NotContains(t, cacheControl, "no-store",
			"complete es fallback must not be treated as an incomplete es-AR menu")
	}

	publicW, publicPayload := requestPublicParityMenu(t, business, "es-AR", true, "")
	require.Equal(t, http.StatusOK, publicW.Code, publicW.Body.String())
	assertSpanishMenu(t, publicPayload, publicW.Header().Get("Cache-Control"))

	tableW, tablePayload := requestTableParityMenu(t, table, "es-AR", true, "")
	require.Equal(t, http.StatusOK, tableW.Code, tableW.Body.String())
	assertSpanishMenu(t, tablePayload, tableW.Header().Get("Cache-Control"))

	assert.Equal(t, publicPayload.Categories[0].Items[1].Name, tablePayload.Categories[0].Items[1].Name)
	assert.Equal(t, publicPayload.Offers[0].Name, tablePayload.Offers[0].Name)
	assert.Equal(t, publicPayload.Bundles[0].Name, tablePayload.Bundles[0].Name)

	// Neutral es and English keep their own exact-code behavior.
	esPublic, esPayload := requestPublicParityMenu(t, business, "es", true, "")
	require.Equal(t, http.StatusOK, esPublic.Code)
	assert.Equal(t, "Plato de bistec", esPayload.Categories[0].Items[1].Name)
	enPublic, enPayload := requestPublicParityMenu(t, business, "en", true, "")
	require.Equal(t, http.StatusOK, enPublic.Code)
	assert.Equal(t, "Steak Plate", enPayload.Categories[0].Items[1].Name)
}

func TestPublicAndTableMenus_EsARRowsWinOverEsFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, table := seedPublicTableMenuParityBusiness(t)
	now := time.Now().UTC()
	require.NoError(t, database.GetDB().Create(&database.Translation{
		BusinessID: business.ID, EntityType: "menu_item", EntityID: 1, FieldName: "name",
		LanguageCode: "es-AR", OriginalText: "Steak Plate", TranslatedText: "Bife de chorizo",
		CreatedAt: now, UpdatedAt: now,
	}).Error)

	publicW, publicPayload := requestPublicParityMenu(t, business, "es-AR", true, "")
	tableW, tablePayload := requestTableParityMenu(t, table, "es-AR", true, "")
	require.Equal(t, http.StatusOK, publicW.Code, publicW.Body.String())
	require.Equal(t, http.StatusOK, tableW.Code, tableW.Body.String())
	assert.Equal(t, "Bife de chorizo", publicPayload.Categories[0].Items[1].Name)
	assert.Equal(t, "Bife de chorizo", tablePayload.Categories[0].Items[1].Name)
	assert.Equal(t, "Bistec a la parrilla, papas y chimichurri", publicPayload.Categories[0].Items[1].Description,
		"fields without an es-AR row must still reuse the es translation")

	esPublic, esPayload := requestPublicParityMenu(t, business, "es", true, "")
	require.Equal(t, http.StatusOK, esPublic.Code)
	assert.Equal(t, "Plato de bistec", esPayload.Categories[0].Items[1].Name,
		"generic es must not pick up es-AR-only overrides")
}

func TestGuestMenuTranslationQueryErrorsPropagate(t *testing.T) {
	setupPublicGuestTableHandlerTestDB(t)
	previousDB := database.GetDB()
	database.SetTestDB(nil)
	t.Cleanup(func() { database.SetTestDB(previousDB) })

	categories := []database.MenuCategory{{
		ID: "42", Name: "Mains",
		Items: []database.MenuItem{{ID: "4242", Name: "Steak Plate", Description: "Charred steak"}},
	}}
	translatedCategories, translatedOffers, translatedBundles, missingMenu, missingPromotions, revision, err :=
		translateGuestMenuForLanguage(7, categories, nil, nil, "es")
	require.Error(t, err)
	assert.Nil(t, translatedCategories)
	assert.Nil(t, translatedOffers)
	assert.Nil(t, translatedBundles)
	assert.False(t, missingMenu)
	assert.False(t, missingPromotions)
	assert.Empty(t, revision)
}
