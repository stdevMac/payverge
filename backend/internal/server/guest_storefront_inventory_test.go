package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type storefrontMenuWire struct {
	Menu struct {
		ItemOrderability map[string]services.Orderability `json:"item_orderability"`
	} `json:"menu"`
	Categories []database.MenuCategory `json:"categories"`
}

func seedParrillaStorefrontMenu(t *testing.T, beefQty float64) (*database.Business, *database.InventoryItem) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.InventoryItem{},
		&database.InventoryRecipe{},
		&database.InventorySettings{},
		&database.Translation{},
	))
	services.ResetPricingCache()
	t.Cleanup(services.ResetPricingCache)

	business := createPublicBusinessRouteTestBusiness(t, "parrilla-quebracho-azul", true, true)
	require.NoError(t, database.GetDB().Model(business).Updates(map[string]interface{}{
		"default_language": "es",
		"default_currency": "ARS",
		"display_currency": "ARS",
		"kitchen_enabled":  true,
		"orders_enabled":   true,
		"is_demo":          true,
		"kind":             database.BusinessKindDemo,
	}).Error)
	business.DefaultLanguage = "es"
	business.DefaultCurrency = "ARS"
	business.DisplayCurrency = "ARS"
	business.KitchenEnabled = true
	business.OrdersEnabled = true
	business.IsDemo = true
	business.Kind = database.BusinessKindDemo

	require.NoError(t, database.GetDB().Create(&database.InventorySettings{
		BusinessID:           business.ID,
		InventoryEnabled:     true,
		AvailabilitySyncMode: database.InventoryAvailabilityModeHardBlock,
	}).Error)

	cats := []database.MenuCategory{{
		ID: "principales", Name: "Principales",
		Items: []database.MenuItem{
			{ID: "demo-bife", Name: "Bife de chorizo", Price: 34000, IsAvailable: true},
			{ID: "demo-ensalada", Name: "Ensalada mixta", Price: 6500, IsAvailable: true},
			{ID: "demo-sorrentinos", Name: "Sorrentinos", Price: 12000, IsAvailable: true},
		},
	}}
	raw, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID, Categories: string(raw), IsActive: true, Version: 1,
	}).Error)

	beef := seedInventoryItem(t, database.GetDB(), business.ID, "Bife", beefQty, true)
	require.NoError(t, database.GetDB().Model(beef).Update("unit", "kg").Error)
	greens := seedInventoryItem(t, database.GetDB(), business.ID, "Verduras", 18, true)
	pasta := seedInventoryItem(t, database.GetDB(), business.ID, "Masa", 9, true)
	seedRecipe(t, database.GetDB(), business.ID, "demo-bife", beef.ID, 0.35)
	seedRecipe(t, database.GetDB(), business.ID, "demo-ensalada", greens.ID, 0.2)
	seedRecipe(t, database.GetDB(), business.ID, "demo-sorrentinos", pasta.ID, 0.25)
	return business, beef
}

func requestStorefrontMenu(t *testing.T, customURL, language, ifNoneMatch string) (*httptest.ResponseRecorder, storefrontMenuWire) {
	t.Helper()
	path := "/business/" + customURL + "/menu"
	if language != "" {
		path += "?language=" + language
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: customURL}}
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	if ifNoneMatch != "" {
		c.Request.Header.Set("If-None-Match", ifNoneMatch)
	}
	GetMenuByBusinessCustomUrl(c)
	if w.Code == http.StatusNotModified {
		return w, storefrontMenuWire{}
	}
	var payload storefrontMenuWire
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload), w.Body.String())
	return w, payload
}

func findStorefrontItem(t *testing.T, categories []database.MenuCategory, id string) database.MenuItem {
	t.Helper()
	for _, category := range categories {
		for _, item := range category.Items {
			if item.ID == id {
				return item
			}
		}
	}
	t.Fatalf("menu item %q missing from storefront categories", id)
	return database.MenuItem{}
}

func setBeefQuantity(t *testing.T, businessID uint, qty float64) {
	t.Helper()
	require.NoError(t, database.GetDB().Exec(
		"UPDATE inventory_items SET current_quantity = ? WHERE business_id = ? AND name = ?",
		qty, businessID, "Bife",
	).Error)
	services.ResetPricingCache()
}

func assertStorefrontBifeEightySixed(t *testing.T, payload storefrontMenuWire) {
	t.Helper()
	require.NotEmpty(t, payload.Categories)
	bife := findStorefrontItem(t, payload.Categories, "demo-bife")
	ensalada := findStorefrontItem(t, payload.Categories, "demo-ensalada")
	sorrentinos := findStorefrontItem(t, payload.Categories, "demo-sorrentinos")

	require.False(t, bife.IsAvailable, "storefront catalog must un-sell 86'd Bife")
	require.Equal(t, "out_of_stock", bife.InventoryStatus)
	decision, ok := payload.Menu.ItemOrderability["demo-bife"]
	require.True(t, ok, "item_orderability must include demo-bife")
	require.Equal(t, services.OrderabilityInventoryOut, decision.State)
	require.False(t, decision.Orderable)

	require.True(t, ensalada.IsAvailable, "stocked Ensalada must stay sellable")
	require.Empty(t, ensalada.InventoryStatus)
	require.Equal(t, services.Orderability{Orderable: true, State: services.OrderabilityAvailable}, payload.Menu.ItemOrderability["demo-ensalada"])

	require.True(t, sorrentinos.IsAvailable, "stocked Sorrentinos must stay sellable")
	require.Empty(t, sorrentinos.InventoryStatus)
	require.Equal(t, services.Orderability{Orderable: true, State: services.OrderabilityAvailable}, payload.Menu.ItemOrderability["demo-sorrentinos"])

	bifeJSON, err := json.Marshal(bife)
	require.NoError(t, err)
	var bifeWire map[string]any
	require.NoError(t, json.Unmarshal(bifeJSON, &bifeWire))
	require.Equal(t, false, bifeWire["is_available"])
	require.Equal(t, "out_of_stock", bifeWire["inventory_status"])
}

func assertStorefrontBifeSellable(t *testing.T, payload storefrontMenuWire) {
	t.Helper()
	bife := findStorefrontItem(t, payload.Categories, "demo-bife")
	require.True(t, bife.IsAvailable)
	require.Empty(t, bife.InventoryStatus)
	require.Equal(t, services.Orderability{Orderable: true, State: services.OrderabilityAvailable}, payload.Menu.ItemOrderability["demo-bife"])
	require.True(t, findStorefrontItem(t, payload.Categories, "demo-ensalada").IsAvailable)
	require.True(t, findStorefrontItem(t, payload.Categories, "demo-sorrentinos").IsAvailable)
}

// #862: /b/parrilla-quebracho-azul still sold 86'd Bife while guest table home
// already refused it. The public storefront menu must stamp the same
// is_available=effective + inventory_status contract on top-level categories
// and nested item_orderability.
func TestGetMenuByBusinessCustomUrl_EightySixesOutOfStockBifeOnStorefront(t *testing.T) {
	business, _ := seedParrillaStorefrontMenu(t, 0)

	w, payload := requestStorefrontMenu(t, business.CustomURL, "", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertStorefrontBifeEightySixed(t, payload)
}

func TestGetMenuByBusinessCustomUrl_TranslatedCatalogKeepsBifeEightySix(t *testing.T) {
	business, _ := seedParrillaStorefrontMenu(t, 0)

	w, payload := requestStorefrontMenu(t, business.CustomURL, "en", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertStorefrontBifeEightySixed(t, payload)
}

// #862: guestMenuETag ignored inventory, so If-None-Match 304 / shared caches
// kept the pre-86 body after beef 12 → 0 while menu.Version stayed put.
func TestGetMenuByBusinessCustomUrl_ETagChangesWhenBeefQuantityHitsZero(t *testing.T) {
	business, _ := seedParrillaStorefrontMenu(t, 12)

	first, stocked := requestStorefrontMenu(t, business.CustomURL, "", "")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	assertStorefrontBifeSellable(t, stocked)
	etagStocked := first.Header().Get("ETag")
	require.NotEmpty(t, etagStocked)

	unchanged, _ := requestStorefrontMenu(t, business.CustomURL, "", etagStocked)
	require.Equal(t, http.StatusNotModified, unchanged.Code, "unchanged stock must keep the validator")
	require.Equal(t, etagStocked, unchanged.Header().Get("ETag"))

	repeat, _ := requestStorefrontMenu(t, business.CustomURL, "", "")
	require.Equal(t, http.StatusOK, repeat.Code, repeat.Body.String())
	require.Equal(t, etagStocked, repeat.Header().Get("ETag"), "ETag must stay stable while beef quantity is unchanged")

	setBeefQuantity(t, business.ID, 0)

	staleW, stale := requestStorefrontMenu(t, business.CustomURL, "", etagStocked)
	require.Equal(t, http.StatusOK, staleW.Code, "depleting beef must not 304 the pre-86 storefront body")
	assertStorefrontBifeEightySixed(t, stale)
	etagEightySixed := staleW.Header().Get("ETag")
	require.NotEmpty(t, etagEightySixed)
	require.NotEqual(t, etagStocked, etagEightySixed)

	fresh, depleted := requestStorefrontMenu(t, business.CustomURL, "", "")
	require.Equal(t, http.StatusOK, fresh.Code, fresh.Body.String())
	assertStorefrontBifeEightySixed(t, depleted)
	require.Equal(t, etagEightySixed, fresh.Header().Get("ETag"))

	revalidated, _ := requestStorefrontMenu(t, business.CustomURL, "", etagEightySixed)
	require.Equal(t, http.StatusNotModified, revalidated.Code)

	setBeefQuantity(t, business.ID, 12)
	restockedW, restocked := requestStorefrontMenu(t, business.CustomURL, "", etagEightySixed)
	require.Equal(t, http.StatusOK, restockedW.Code, "restocking beef must invalidate the 86 ETag")
	assertStorefrontBifeSellable(t, restocked)
	require.NotEqual(t, etagEightySixed, restockedW.Header().Get("ETag"))
}
