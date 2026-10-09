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

// Live shape at sha-49eb55ec13b7, venue 142 (Parrilla Quebracho Azul): item
// orderability said demo-bife = inventory_out and the guest menu returned
// is_available:false + inventory_status:out_of_stock, while the SAME operator
// response returned parsed_categories[demo-bife].is_available = true.
//
// Menu Builder therefore had to reconcile two contradictory fields from one
// payload, and the previous fix only relabelled the button. The builder payload
// now reports the effective answer, so both audiences read one truth; the
// stored operator flag moves to manual_available, which is what the edit form
// and the one-tap 86 own.
func TestGetMenu_BuilderPayloadMatchesGuestOnInventoryBlock(t *testing.T) {
	db := newParrillaMenuDB(t)
	_ = db

	body := getOperatorMenu(t, "demo-admin-8-ai-pro")

	var response struct {
		ParsedCategories []database.MenuCategory          `json:"parsed_categories"`
		ItemOrderability map[string]services.Orderability `json:"item_orderability"`
	}
	require.NoError(t, json.Unmarshal(body, &response))
	require.Len(t, response.ParsedCategories, 1)
	items := map[string]database.MenuItem{}
	for _, it := range response.ParsedCategories[0].Items {
		items[it.ID] = it
	}
	require.Len(t, items, 3)

	bife := items["demo-bife"]
	require.Equal(t, services.OrderabilityInventoryOut, response.ItemOrderability["demo-bife"].State)
	assert.False(t, bife.IsAvailable,
		"builder payload must report the inventory-blocked dish unavailable, like guest does")
	assert.Equal(t, "out_of_stock", bife.InventoryStatus)
	require.NotNil(t, bife.ManualAvailable)
	assert.True(t, *bife.ManualAvailable,
		"inventory must never look like a manual 86 — the operator never switched this dish off")

	// Do not 86 the sellable dishes.
	for _, id := range []string{"demo-ensalada", "demo-sorrentinos"} {
		it := items[id]
		assert.True(t, it.IsAvailable, id)
		assert.Empty(t, it.InventoryStatus, id)
		assert.True(t, response.ItemOrderability[id].Orderable, id)
		require.NotNil(t, it.ManualAvailable, id)
		assert.True(t, *it.ManualAvailable, id)
	}
}

// A dish the operator switched off by hand must stay distinguishable from one
// inventory blocked: manual_available carries the stored flag so re-saving the
// edit form cannot pin an 86 that outlives the restock.
func TestGetMenu_ManualEightySixIsReportedSeparatelyFromInventory(t *testing.T) {
	db := newParrillaMenuDB(t)
	var menu database.Menu
	require.NoError(t, db.Where("business_id = ?", parrillaBusinessID(t, db)).First(&menu).Error)

	var cats []database.MenuCategory
	require.NoError(t, json.Unmarshal([]byte(menu.Categories), &cats))
	for i := range cats[0].Items {
		if cats[0].Items[i].ID == "demo-sorrentinos" {
			cats[0].Items[i].IsAvailable = false
		}
	}
	raw, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, db.Model(&menu).Update("categories", string(raw)).Error)

	body := getOperatorMenu(t, "demo-admin-8-ai-pro")
	var response struct {
		ParsedCategories []database.MenuCategory          `json:"parsed_categories"`
		ItemOrderability map[string]services.Orderability `json:"item_orderability"`
	}
	require.NoError(t, json.Unmarshal(body, &response))
	items := map[string]database.MenuItem{}
	for _, it := range response.ParsedCategories[0].Items {
		items[it.ID] = it
	}

	manual := items["demo-sorrentinos"]
	assert.False(t, manual.IsAvailable)
	require.NotNil(t, manual.ManualAvailable)
	assert.False(t, *manual.ManualAvailable, "a hand-pulled dish reports the manual flag off")
	assert.Equal(t, services.OrderabilityManualDisabled, response.ItemOrderability["demo-sorrentinos"].State)

	inventory := items["demo-bife"]
	assert.False(t, inventory.IsAvailable)
	require.NotNil(t, inventory.ManualAvailable)
	assert.True(t, *inventory.ManualAvailable,
		"inventory-blocked and hand-pulled must not collapse into the same flag")
}

// The stamp is serve-time only: a write path must never persist it into the
// stored menu blob, or a restock would leave manual_available frozen.
func TestGetMenu_ManualAvailableIsNotPersisted(t *testing.T) {
	db := newParrillaMenuDB(t)
	getOperatorMenu(t, "demo-admin-8-ai-pro")

	var menu database.Menu
	require.NoError(t, db.Where("business_id = ?", parrillaBusinessID(t, db)).First(&menu).Error)
	assert.NotContains(t, menu.Categories, "manual_available")
	assert.NotContains(t, menu.Categories, "inventory_status")
}

func parrillaBusinessID(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	var business database.Business
	require.NoError(t, db.Where("business_id = ?", "demo-admin-8-ai-pro").First(&business).Error)
	return business.ID
}

func getOperatorMenu(t *testing.T, slug string) []byte {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/businesses/"+slug+"/menu", nil)
	ctx.Params = gin.Params{{Key: "id", Value: slug}}
	GetMenu(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	return recorder.Body.Bytes()
}

// newParrillaMenuDB builds venue 142's shape: beef at 0 blocks the bife, the
// salad and sorrentinos have stock and stay sellable.
func newParrillaMenuDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&database.Business{}, &database.Menu{}, &database.InventorySettings{},
		&database.InventoryItem{}, &database.InventoryRecipe{},
	))
	database.SetTestDB(db)

	business := database.Business{
		BusinessId: "demo-admin-8-ai-pro", Name: "Parrilla Quebracho Azul", OwnerAddress: "0xowner",
		DefaultLanguage: "es", DefaultCurrency: "ARS", Timezone: "America/Buenos_Aires",
	}
	require.NoError(t, db.Create(&business).Error)

	categories := []database.MenuCategory{{
		ID: "principales", Name: "Principales",
		Items: []database.MenuItem{
			{ID: "demo-bife", Name: "Bife de chorizo", Price: 24000, IsAvailable: true},
			{ID: "demo-ensalada", Name: "Ensalada mixta", Price: 6500, IsAvailable: true},
			{ID: "demo-sorrentinos", Name: "Sorrentinos", Price: 12000, IsAvailable: true},
		},
	}}
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Menu{
		BusinessID: business.ID, Categories: string(raw), IsActive: true, Version: 1,
	}).Error)
	require.NoError(t, db.Create(&database.InventorySettings{
		BusinessID: business.ID, InventoryEnabled: true,
		AvailabilitySyncMode: database.InventoryAvailabilityModeHardBlock,
	}).Error)

	beef := database.InventoryItem{BusinessID: business.ID, Name: "Bife", Unit: "kg", CurrentQuantity: 0, IsActive: true}
	greens := database.InventoryItem{BusinessID: business.ID, Name: "Verduras", Unit: "kg", CurrentQuantity: 18, IsActive: true}
	pasta := database.InventoryItem{BusinessID: business.ID, Name: "Masa", Unit: "kg", CurrentQuantity: 9, IsActive: true}
	require.NoError(t, db.Create(&beef).Error)
	require.NoError(t, db.Create(&greens).Error)
	require.NoError(t, db.Create(&pasta).Error)
	require.NoError(t, db.Create([]database.InventoryRecipe{
		{BusinessID: business.ID, MenuItemID: "demo-bife", MenuItemName: "Bife de chorizo", InventoryItemID: beef.ID, QuantityRequired: 0.35},
		{BusinessID: business.ID, MenuItemID: "demo-ensalada", MenuItemName: "Ensalada mixta", InventoryItemID: greens.ID, QuantityRequired: 0.2},
		{BusinessID: business.ID, MenuItemID: "demo-sorrentinos", MenuItemName: "Sorrentinos", InventoryItemID: pasta.ID, QuantityRequired: 0.25},
	}).Error)
	return db
}
