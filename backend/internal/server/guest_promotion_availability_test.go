package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUnsellableGuestMenuItemIDs_IgnoresVenueWideStates(t *testing.T) {
	categories := []database.MenuCategory{{
		Items: []database.MenuItem{
			{ID: "demo-steak", Name: "Steak Plate", IsAvailable: true},
			{ID: "demo-bowl", Name: "Harvest Bowl", IsAvailable: true},
			{ID: "demo-tart", Name: "Chocolate Tart", IsAvailable: false},
		},
	}}
	projection := map[string]services.Orderability{
		"demo-steak": {State: services.OrderabilityInventoryOut, Orderable: false},
		"demo-bowl":  {State: services.OrderabilityBusinessClosed, Orderable: false},
		"demo-tart":  {State: services.OrderabilityManualDisabled, Orderable: false},
	}
	hidden := unsellableGuestMenuItemIDs(categories, projection)
	require.True(t, hidden["demo-steak"])
	require.True(t, hidden["demo-tart"])
	require.False(t, hidden["demo-bowl"], "venue-closed is not an item 86")
}

func TestFilterGuestLivePromotions_HidesDateNightAndSteakOffer(t *testing.T) {
	steakID := "demo-steak"
	dateNightItems, err := json.Marshal([]database.BundleItemRef{
		{MenuItemID: "demo-steak", Name: "Steak Plate", Quantity: 1},
		{MenuItemID: "demo-cocktail", Name: "Demo Spritz", Quantity: 2},
	})
	require.NoError(t, err)
	offers := []database.Offer{
		{ID: 13, Name: "$5 Off the Steak Plate", IsActive: true, ApplicableTo: "item", TargetID: &steakID},
		{ID: 2, Name: "Weekday Lunch 15% Off", IsActive: true, ApplicableTo: "all"},
		{ID: 9, Name: "$10 Off Date Night", IsActive: true, ApplicableTo: "bundle", TargetID: strptr("7")},
	}
	bundles := []database.Bundle{
		{ID: 7, Name: "Date Night for Two", IsActive: true, Items: string(dateNightItems)},
	}
	categories := []database.MenuCategory{{
		Items: []database.MenuItem{
			{ID: "demo-steak", Name: "Steak Plate", IsAvailable: true},
			{ID: "demo-cocktail", Name: "Demo Spritz", IsAvailable: true},
		},
	}}
	projection := map[string]services.Orderability{
		"demo-steak":    {State: services.OrderabilityInventoryOut, Orderable: false},
		"demo-cocktail": {State: services.OrderabilityAvailable, Orderable: true},
	}

	liveOffers, liveBundles := filterGuestLivePromotions(offers, bundles, categories, projection)
	require.Len(t, liveBundles, 0, "Date Night must hide while steak is inventory_out")
	require.Len(t, liveOffers, 1)
	require.Equal(t, "Weekday Lunch 15% Off", liveOffers[0].Name)
}

func TestGetTableByCodePublic_HidesUnsellableOffersAndBundles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.InventoryItem{},
		&database.InventoryRecipe{},
		&database.InventorySettings{},
	))
	services.ResetPricingCache()

	business := createSensitiveGuestTableBusiness(t, "86PROMO1")
	table := createGuestPublicTable(t, business.ID, "86PROMO1")
	require.NoError(t, database.GetDB().Create(&database.InventorySettings{
		BusinessID:           business.ID,
		InventoryEnabled:     true,
		AvailabilitySyncMode: database.InventoryAvailabilityModeHardBlock,
	}).Error)

	cats := []database.MenuCategory{{
		ID: "mains", Name: "Mains",
		Items: []database.MenuItem{
			{ID: "demo-steak", Name: "Steak Plate", Price: 42, IsAvailable: true},
			{ID: "demo-bowl", Name: "Harvest Bowl", Price: 18.5, IsAvailable: true},
			{ID: "demo-cocktail", Name: "Demo Spritz", Price: 12, IsAvailable: true},
			{ID: "demo-dessert", Name: "Chocolate Tart", Price: 9, IsAvailable: true},
		},
	}}
	raw, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID, Categories: string(raw), IsActive: true, Version: 1,
	}).Error)

	beef := seedInventoryItem(t, database.GetDB(), business.ID, "Beef", 0, true)
	seedRecipe(t, database.GetDB(), business.ID, "demo-steak", beef.ID, 1)

	dateNightItems, err := json.Marshal([]database.BundleItemRef{
		{MenuItemID: "demo-steak", Name: "Steak Plate", Quantity: 1},
		{MenuItemID: "demo-cocktail", Name: "Demo Spritz", Quantity: 2},
		{MenuItemID: "demo-dessert", Name: "Chocolate Tart", Quantity: 1},
	})
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Bundle{
		BusinessID: business.ID, Name: "Date Night for Two", Price: 68, IsActive: true,
		Items: string(dateNightItems),
	}).Error)
	steakID := "demo-steak"
	require.NoError(t, database.GetDB().Create(&database.Offer{
		BusinessID: business.ID, Name: "$5 Off the Steak Plate", DiscountType: "fixed",
		DiscountValue: 5, IsActive: true, ApplicableTo: "item", TargetID: &steakID, WeekdayMask: 127,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.Offer{
		BusinessID: business.ID, Name: "Weekday Lunch 15% Off", DiscountType: "percentage",
		DiscountValue: 15, IsActive: true, ApplicableTo: "all", WeekdayMask: 127,
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s", table.TableCode), nil)
	GetTableByCodePublic(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Menu struct {
			ItemOrderability map[string]services.Orderability `json:"item_orderability"`
		} `json:"menu"`
		Categories []database.MenuCategory `json:"categories"`
		Offers     []struct {
			Name     string `json:"name"`
			IsActive bool   `json:"is_active"`
		} `json:"offers"`
		Bundles []struct {
			Name     string `json:"name"`
			IsActive bool   `json:"is_active"`
		} `json:"bundles"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, services.OrderabilityInventoryOut, resp.Menu.ItemOrderability["demo-steak"].State)
	require.False(t, resp.Menu.ItemOrderability["demo-steak"].Orderable)
	require.Equal(t, services.Orderability{Orderable: true, State: services.OrderabilityAvailable}, resp.Menu.ItemOrderability["demo-bowl"])
	require.Len(t, resp.Categories, 1)
	require.Len(t, resp.Categories[0].Items, 4)
	for _, item := range resp.Categories[0].Items {
		switch item.ID {
		case "demo-steak":
			require.False(t, item.IsAvailable, "guest table catalog must un-sell inventory-out steak")
			require.Equal(t, "out_of_stock", item.InventoryStatus)
		case "demo-bowl":
			require.True(t, item.IsAvailable, "stocked Harvest Bowl must stay sellable")
			require.Empty(t, item.InventoryStatus)
		}
	}

	for _, bundle := range resp.Bundles {
		require.NotEqual(t, "Date Night for Two", bundle.Name)
		require.True(t, bundle.IsActive)
	}
	names := make([]string, 0, len(resp.Offers))
	for _, offer := range resp.Offers {
		require.NotEqual(t, "$5 Off the Steak Plate", offer.Name)
		names = append(names, offer.Name)
		require.True(t, offer.IsActive)
	}
	require.Contains(t, names, "Weekday Lunch 15% Off")
}

func TestFilterGuestLivePromotions_HidesNocheDeParrillaWhenParrilladaOut(t *testing.T) {
	nocheItems, err := json.Marshal([]database.BundleItemRef{
		{MenuItemID: "demo-parrillada", Name: "Parrillada para dos", Quantity: 1},
		{MenuItemID: "demo-malbec-botella", Name: "Botella de Malbec", Quantity: 1},
		{MenuItemID: "demo-flan", Name: "Flan casero", Quantity: 2},
	})
	require.NoError(t, err)
	malbecOnly, err := json.Marshal([]database.BundleItemRef{
		{MenuItemID: "demo-malbec-botella", Name: "Botella de Malbec", Quantity: 1},
	})
	require.NoError(t, err)
	offers := []database.Offer{
		{ID: 2, Name: "Almuerzo de semana 15% off", IsActive: true, ApplicableTo: "all"},
	}
	bundles := []database.Bundle{
		{ID: 7, Name: "Noche de parrilla para dos", IsActive: true, Items: string(nocheItems)},
		{ID: 8, Name: "Copa de Malbec", IsActive: true, Items: string(malbecOnly)},
	}
	categories := []database.MenuCategory{{
		Items: []database.MenuItem{
			{ID: "demo-parrillada", Name: "Parrillada para dos", IsAvailable: true},
			{ID: "demo-malbec-botella", Name: "Botella de Malbec", IsAvailable: true},
			{ID: "demo-flan", Name: "Flan casero", IsAvailable: true},
			{ID: "demo-ensalada", Name: "Ensalada mixta", IsAvailable: true},
		},
	}}
	projection := map[string]services.Orderability{
		"demo-parrillada":     {State: services.OrderabilityInventoryOut, Orderable: false},
		"demo-malbec-botella": {State: services.OrderabilityAvailable, Orderable: true},
		"demo-flan":           {State: services.OrderabilityAvailable, Orderable: true},
		"demo-ensalada":       {State: services.OrderabilityAvailable, Orderable: true},
	}

	liveOffers, liveBundles := filterGuestLivePromotions(offers, bundles, categories, projection)
	require.Len(t, liveOffers, 1)
	require.Equal(t, "Almuerzo de semana 15% off", liveOffers[0].Name)
	require.Len(t, liveBundles, 1)
	require.Equal(t, "Copa de Malbec", liveBundles[0].Name)
}

func TestGetTableByCodePublic_ZeroBeefHidesNocheDeParrillaAndBeefPlates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.InventoryItem{},
		&database.InventoryRecipe{},
		&database.InventorySettings{},
	))
	services.ResetPricingCache()

	business := createSensitiveGuestTableBusiness(t, "86BEEF01")
	table := createGuestPublicTable(t, business.ID, "86BEEF01")
	require.NoError(t, database.GetDB().Create(&database.InventorySettings{
		BusinessID:           business.ID,
		InventoryEnabled:     true,
		AvailabilitySyncMode: database.InventoryAvailabilityModeWarn,
	}).Error)

	cats := []database.MenuCategory{
		{
			ID: "parrilla", Name: "Parrilla",
			Items: []database.MenuItem{
				{ID: "demo-bife", Name: "Bife de chorizo", Price: 34000, IsAvailable: true},
				{ID: "demo-ojo-de-bife", Name: "Ojo de bife", Price: 39500, IsAvailable: true},
				{ID: "demo-asado-tira", Name: "Asado de tira", Price: 29800, IsAvailable: true},
				{ID: "demo-parrillada", Name: "Parrillada para dos", Price: 68000, IsAvailable: true},
			},
		},
		{
			ID: "otros", Name: "Otros",
			Items: []database.MenuItem{
				{ID: "demo-ensalada", Name: "Ensalada mixta", Price: 12500, IsAvailable: true},
				{ID: "demo-sorrentinos", Name: "Sorrentinos caseros", Price: 19800, IsAvailable: true},
				{ID: "demo-malbec-botella", Name: "Botella de Malbec", Price: 28500, IsAvailable: true},
				{ID: "demo-flan", Name: "Flan casero", Price: 8900, IsAvailable: true},
			},
		},
	}
	raw, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID, Categories: string(raw), IsActive: true, Version: 1,
	}).Error)

	beef := seedInventoryItem(t, database.GetDB(), business.ID, "Bife de chorizo (media res)", 0, true)
	greens := seedInventoryItem(t, database.GetDB(), business.ID, "Verdura de estación", 48, true)
	pasta := seedInventoryItem(t, database.GetDB(), business.ID, "Masa para sorrentinos", 12, true)
	wine := seedInventoryItem(t, database.GetDB(), business.ID, "Malbec de bodega", 35, true)
	custard := seedInventoryItem(t, database.GetDB(), business.ID, "Crema para flan", 8, true)
	seedRecipe(t, database.GetDB(), business.ID, "demo-bife", beef.ID, 0.40)
	seedRecipe(t, database.GetDB(), business.ID, "demo-ojo-de-bife", beef.ID, 0.45)
	seedRecipe(t, database.GetDB(), business.ID, "demo-asado-tira", beef.ID, 0.35)
	seedRecipe(t, database.GetDB(), business.ID, "demo-parrillada", beef.ID, 0.80)
	seedRecipe(t, database.GetDB(), business.ID, "demo-ensalada", greens.ID, 0.20)
	seedRecipe(t, database.GetDB(), business.ID, "demo-sorrentinos", pasta.ID, 0.25)
	seedRecipe(t, database.GetDB(), business.ID, "demo-malbec-botella", wine.ID, 0.75)
	seedRecipe(t, database.GetDB(), business.ID, "demo-flan", custard.ID, 0.15)

	nocheItems, err := json.Marshal([]database.BundleItemRef{
		{MenuItemID: "demo-parrillada", Name: "Parrillada para dos", Quantity: 1},
		{MenuItemID: "demo-malbec-botella", Name: "Botella de Malbec", Quantity: 1},
		{MenuItemID: "demo-flan", Name: "Flan casero", Quantity: 2},
	})
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Bundle{
		BusinessID: business.ID, Name: "Noche de parrilla para dos", Price: 105000, IsActive: true,
		Items: string(nocheItems),
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s", table.TableCode), nil)
	GetTableByCodePublic(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Menu struct {
			ItemOrderability map[string]services.Orderability `json:"item_orderability"`
		} `json:"menu"`
		Categories []database.MenuCategory `json:"categories"`
		Bundles    []struct {
			Name     string `json:"name"`
			IsActive bool   `json:"is_active"`
		} `json:"bundles"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	blocked := []string{"demo-bife", "demo-ojo-de-bife", "demo-asado-tira", "demo-parrillada"}
	for _, id := range blocked {
		require.Equal(t, services.OrderabilityInventoryOut, resp.Menu.ItemOrderability[id].State, id)
		require.False(t, resp.Menu.ItemOrderability[id].Orderable, id)
	}
	require.Equal(t, services.Orderability{Orderable: true, State: services.OrderabilityAvailable}, resp.Menu.ItemOrderability["demo-ensalada"])
	require.Equal(t, services.Orderability{Orderable: true, State: services.OrderabilityAvailable}, resp.Menu.ItemOrderability["demo-sorrentinos"])

	byID := map[string]database.MenuItem{}
	for _, cat := range resp.Categories {
		for _, item := range cat.Items {
			byID[item.ID] = item
		}
	}
	for _, id := range blocked {
		require.False(t, byID[id].IsAvailable, "guest catalog must un-sell %s", id)
		require.Equal(t, "out_of_stock", byID[id].InventoryStatus, id)
	}
	require.True(t, byID["demo-ensalada"].IsAvailable)
	require.Empty(t, byID["demo-ensalada"].InventoryStatus)
	require.True(t, byID["demo-sorrentinos"].IsAvailable)
	require.Empty(t, byID["demo-sorrentinos"].InventoryStatus)

	for _, bundle := range resp.Bundles {
		require.NotEqual(t, "Noche de parrilla para dos", bundle.Name)
		require.True(t, bundle.IsActive)
	}
}
