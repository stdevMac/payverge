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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetMenuIncludesOperatorOrderabilityProjection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&database.Business{}, &database.Menu{}, &database.InventorySettings{},
		&database.InventoryItem{}, &database.InventoryRecipe{},
	))
	database.SetTestDB(db)

	business := database.Business{BusinessId: "operator-menu", Name: "Operator menu", OwnerAddress: "0xowner"}
	require.NoError(t, db.Create(&business).Error)
	categories := []database.MenuCategory{{
		ID: "mains", Name: "Mains",
		Items: []database.MenuItem{{ID: "burger", Name: "Burger", Price: 10, IsAvailable: true}},
	}}
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID, Categories: string(raw), IsActive: true}).Error)
	require.NoError(t, db.Create(&database.InventorySettings{
		BusinessID: business.ID, InventoryEnabled: true,
		AvailabilitySyncMode: database.InventoryAvailabilityModeHardBlock,
	}).Error)
	stock := database.InventoryItem{BusinessID: business.ID, Name: "Patties", Unit: "unit", CurrentQuantity: 0, IsActive: true}
	require.NoError(t, db.Create(&stock).Error)
	require.NoError(t, db.Create(&database.InventoryRecipe{
		BusinessID: business.ID, MenuItemID: "burger", InventoryItemID: stock.ID, QuantityRequired: 1,
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/businesses/operator-menu/menu", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "operator-menu"}}
	GetMenu(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response struct {
		ItemOrderability map[string]services.Orderability `json:"item_orderability"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, services.OrderabilityInventoryOut, response.ItemOrderability["burger"].State)
	assert.False(t, response.ItemOrderability["burger"].Orderable)
}

func TestGetMenu_ZeroBeefMarksSteakNotHarvestBowl(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&database.Business{}, &database.Menu{}, &database.InventorySettings{},
		&database.InventoryItem{}, &database.InventoryRecipe{}, &database.Translation{},
	))
	database.SetTestDB(db)

	business := database.Business{
		BusinessId: "demo-admin-8-ai-pro", Name: "AI Pro", OwnerAddress: "0xowner",
		DefaultLanguage: "en",
	}
	require.NoError(t, db.Create(&business).Error)
	categories := []database.MenuCategory{{
		ID: "mains", Name: "Mains",
		Items: []database.MenuItem{
			{ID: "demo-bowl", Name: "Harvest Bowl", Price: 18.5, IsAvailable: true, DietaryTags: []string{"vegetarian"}, InventoryStatus: "out_of_stock"},
			{ID: "demo-steak", Name: "Steak Plate", Price: 42, IsAvailable: true},
		},
	}}
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID, Categories: string(raw), IsActive: true, Version: 1}).Error)
	require.NoError(t, db.Create(&database.InventorySettings{
		BusinessID: business.ID, InventoryEnabled: true,
		AvailabilitySyncMode: database.InventoryAvailabilityModeWarn,
	}).Error)
	greens := database.InventoryItem{BusinessID: business.ID, Name: "Mixed Greens", Unit: "kg", CurrentQuantity: 33.25, IsActive: true}
	beef := database.InventoryItem{BusinessID: business.ID, Name: "Premium Beef", Unit: "kg", CurrentQuantity: 0, IsActive: true}
	require.NoError(t, db.Create(&greens).Error)
	require.NoError(t, db.Create(&beef).Error)
	require.NoError(t, db.Create([]database.InventoryRecipe{
		{BusinessID: business.ID, MenuItemID: "demo-bowl", MenuItemName: "Harvest Bowl", InventoryItemID: greens.ID, QuantityRequired: 0.25},
		// #727 shape B: correct steak id, name drifted onto the bowl (id-wins).
		{BusinessID: business.ID, MenuItemID: "demo-steak", MenuItemName: "Harvest Bowl", InventoryItemID: beef.ID, QuantityRequired: 0.35},
	}).Error)
	require.NoError(t, db.Create(&database.Translation{
		BusinessID: business.ID, EntityType: "menu_item", EntityID: 0,
		FieldName: "name", LanguageCode: "es", OriginalText: "Harvest Bowl", TranslatedText: "Bowl de la cosecha",
	}).Error)
	require.NoError(t, db.Create(&database.Translation{
		BusinessID: business.ID, EntityType: "menu_item", EntityID: 1,
		FieldName: "name", LanguageCode: "es", OriginalText: "Steak Plate", TranslatedText: "Plato de bistec",
	}).Error)

	gin.SetMode(gin.TestMode)
	for _, lang := range []string{"en", "es"} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/businesses/demo-admin-8-ai-pro/menu?language="+lang, nil)
		ctx.Params = gin.Params{{Key: "id", Value: "demo-admin-8-ai-pro"}}
		GetMenu(ctx)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

		var response struct {
			ParsedCategories []database.MenuCategory          `json:"parsed_categories"`
			ItemOrderability map[string]services.Orderability `json:"item_orderability"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		require.Equal(t, services.OrderabilityInventoryOut, response.ItemOrderability["demo-steak"].State, lang)
		assert.False(t, response.ItemOrderability["demo-steak"].Orderable, lang)
		assert.True(t, response.ItemOrderability["demo-bowl"].Orderable, lang)
		assert.NotEqual(t, services.OrderabilityInventoryOut, response.ItemOrderability["demo-bowl"].State, lang)

		require.Len(t, response.ParsedCategories, 1)
		require.Len(t, response.ParsedCategories[0].Items, 2)
		bowl, steak := response.ParsedCategories[0].Items[0], response.ParsedCategories[0].Items[1]
		assert.Equal(t, "demo-bowl", bowl.ID)
		assert.Empty(t, bowl.InventoryStatus, "leftover bowl inventory_status must be cleared")
		assert.Equal(t, "demo-steak", steak.ID)
		assert.Equal(t, "out_of_stock", steak.InventoryStatus)
		if lang == "es" {
			assert.Equal(t, "Bowl de la cosecha", bowl.Name)
			assert.Equal(t, "Plato de bistec", steak.Name)
		}
	}
}

// Shape-B drift (#727): the recipe row keeps the CORRECT menu_item_id
// (demo-steak) but its menu_item_name drifted to another live dish
// ("Harvest Bowl"). Blind name-wins resolution flips the 86 onto the
// vegetarian bowl and leaves the steak sellable — exactly the Menu Builder
// symptom QA re-verified at sha-c77f5ea530cd. The stored id is the strong key
// (#727 B2): the conflicted beef row resolves id-wins to demo-steak, with no
// database heal.
func TestGetMenu_DriftedRecipeNameStill86sSteakNotHarvestBowl(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&database.Business{}, &database.Menu{}, &database.InventorySettings{},
		&database.InventoryItem{}, &database.InventoryRecipe{},
	))
	database.SetTestDB(db)

	business := database.Business{
		BusinessId: "demo-admin-8-ai-pro", Name: "AI Pro", OwnerAddress: "0xowner",
		DefaultLanguage: "en",
	}
	require.NoError(t, db.Create(&business).Error)
	categories := []database.MenuCategory{{
		ID: "mains", Name: "Mains",
		Items: []database.MenuItem{
			{ID: "demo-bowl", Name: "Harvest Bowl", Price: 18.5, IsAvailable: true, DietaryTags: []string{"vegetarian"}},
			{ID: "demo-steak", Name: "Steak Plate", Price: 42, IsAvailable: true},
		},
	}}
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID, Categories: string(raw), IsActive: true, Version: 1}).Error)
	require.NoError(t, db.Create(&database.InventorySettings{
		BusinessID: business.ID, InventoryEnabled: true,
		AvailabilitySyncMode: database.InventoryAvailabilityModeWarn,
	}).Error)
	greens := database.InventoryItem{BusinessID: business.ID, Name: "Mixed Greens", Unit: "kg", CurrentQuantity: 33.25, IsActive: true}
	beef := database.InventoryItem{BusinessID: business.ID, Name: "Premium Beef", Unit: "kg", CurrentQuantity: 0, IsActive: true}
	require.NoError(t, db.Create(&greens).Error)
	require.NoError(t, db.Create(&beef).Error)
	require.NoError(t, db.Create([]database.InventoryRecipe{
		// Consistent claim: greens genuinely feed the bowl.
		{BusinessID: business.ID, MenuItemID: "demo-bowl", MenuItemName: "Harvest Bowl", InventoryItemID: greens.ID, QuantityRequired: 0.25},
		// Shape-B drift: correct id, wrong (bowl) name.
		{BusinessID: business.ID, MenuItemID: "demo-steak", MenuItemName: "Harvest Bowl", InventoryItemID: beef.ID, QuantityRequired: 0.35},
	}).Error)

	// Database layer first: summary must group the beef recipe under the steak.
	summary, err := database.GetInventorySummary(business.ID)
	require.NoError(t, err)
	statusByID := map[string]database.InventoryMenuItemStatus{}
	for _, status := range summary.MenuItemStatuses {
		statusByID[status.MenuItemID] = status
	}
	assert.Equal(t, "out_of_stock", statusByID["demo-steak"].Status,
		"zero beef must 86 the Steak Plate even when the recipe name drifted")
	assert.NotEqual(t, "out_of_stock", statusByID["demo-bowl"].Status,
		"the vegetarian Harvest Bowl must stay sellable")

	// #727 B2: reconcile must NOT rewrite the conflicted row — it resolves
	// id-wins at read time, so the stored id and even the drifted name stay.
	var untouched database.InventoryRecipe
	require.NoError(t, db.Where("business_id = ? AND inventory_item_id = ?", business.ID, beef.ID).First(&untouched).Error)
	assert.Equal(t, "demo-steak", untouched.MenuItemID)
	assert.Equal(t, "Harvest Bowl", untouched.MenuItemName)

	// Operator wire: Menu Builder chips read this projection.
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/businesses/demo-admin-8-ai-pro/menu", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "demo-admin-8-ai-pro"}}
	GetMenu(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		ParsedCategories []database.MenuCategory          `json:"parsed_categories"`
		ItemOrderability map[string]services.Orderability `json:"item_orderability"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, services.OrderabilityInventoryOut, response.ItemOrderability["demo-steak"].State)
	assert.False(t, response.ItemOrderability["demo-steak"].Orderable)
	assert.True(t, response.ItemOrderability["demo-bowl"].Orderable)
	assert.NotEqual(t, services.OrderabilityInventoryOut, response.ItemOrderability["demo-bowl"].State)

	require.Len(t, response.ParsedCategories, 1)
	require.Len(t, response.ParsedCategories[0].Items, 2)
	bowl, steak := response.ParsedCategories[0].Items[0], response.ParsedCategories[0].Items[1]
	assert.Equal(t, "demo-bowl", bowl.ID)
	assert.Empty(t, bowl.InventoryStatus, "bowl must not carry the steak's 86 stamp")
	assert.Equal(t, "demo-steak", steak.ID)
	assert.Equal(t, "out_of_stock", steak.InventoryStatus)
}
