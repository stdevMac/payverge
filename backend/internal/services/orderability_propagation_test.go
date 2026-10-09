package services

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestProjectOrderability_ZeroBeefEightySixesAllBeefDishesNotEnsalada(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&database.Business{}, &database.Menu{}, &database.InventorySettings{},
		&database.InventoryItem{}, &database.InventoryRecipe{},
	))
	database.SetTestDB(db)

	business := &database.Business{
		BusinessId: "parrilla-945", Name: "Parrilla Quebracho Azul", OwnerAddress: "0xowner",
		KitchenEnabled: true, OrdersEnabled: true,
	}
	require.NoError(t, db.Create(business).Error)
	require.NoError(t, db.Create(&database.InventorySettings{
		BusinessID: business.ID, InventoryEnabled: true,
		AvailabilitySyncMode: database.InventoryAvailabilityModeWarn,
	}).Error)

	categories := []database.MenuCategory{
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
				{ID: "demo-ensalada", Name: "Ensalada mixta", Price: 12500, IsAvailable: true, DietaryTags: []string{"vegan"}},
				{ID: "demo-sorrentinos", Name: "Sorrentinos caseros", Price: 19800, IsAvailable: true},
			},
		},
	}
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Menu{
		BusinessID: business.ID, Categories: string(raw), IsActive: true, Version: 1,
	}).Error)

	greens := database.InventoryItem{
		BusinessID: business.ID, Name: "Verdura de estación", Unit: "kg",
		CurrentQuantity: 48, IsActive: true,
	}
	pasta := database.InventoryItem{
		BusinessID: business.ID, Name: "Masa para sorrentinos", Unit: "kg",
		CurrentQuantity: 12, IsActive: true,
	}
	beef := database.InventoryItem{
		BusinessID: business.ID, Name: "Bife de chorizo (media res)", Unit: "kg",
		CurrentQuantity: 0, IsActive: true,
	}
	require.NoError(t, db.Create(&greens).Error)
	require.NoError(t, db.Create(&pasta).Error)
	require.NoError(t, db.Create(&beef).Error)
	require.NoError(t, db.Create([]database.InventoryRecipe{
		{BusinessID: business.ID, MenuItemID: "demo-bife", MenuItemName: "Bife de chorizo", InventoryItemID: beef.ID, QuantityRequired: 0.40},
		{BusinessID: business.ID, MenuItemID: "demo-ojo-de-bife", MenuItemName: "Ojo de bife", InventoryItemID: beef.ID, QuantityRequired: 0.45},
		{BusinessID: business.ID, MenuItemID: "demo-asado-tira", MenuItemName: "Asado de tira", InventoryItemID: beef.ID, QuantityRequired: 0.35},
		{BusinessID: business.ID, MenuItemID: "demo-parrillada", MenuItemName: "Parrillada para dos", InventoryItemID: beef.ID, QuantityRequired: 0.80},
		{BusinessID: business.ID, MenuItemID: "demo-ensalada", MenuItemName: "Ensalada mixta", InventoryItemID: greens.ID, QuantityRequired: 0.20},
		{BusinessID: business.ID, MenuItemID: "demo-sorrentinos", MenuItemName: "Sorrentinos caseros", InventoryItemID: pasta.ID, QuantityRequired: 0.25},
	}).Error)

	projection := ProjectOrderability(business, categories, OrderabilityContextGuest, true)
	ApplyInventorySellability(categories, projection, false)

	blocked := []string{"demo-bife", "demo-ojo-de-bife", "demo-asado-tira", "demo-parrillada"}
	for _, id := range blocked {
		got := projection[id]
		assert.False(t, got.Orderable, id)
		assert.Equal(t, OrderabilityInventoryOut, got.State, id)
	}
	assert.True(t, projection["demo-ensalada"].Orderable)
	assert.Equal(t, OrderabilityAvailable, projection["demo-ensalada"].State)
	assert.True(t, projection["demo-sorrentinos"].Orderable)
	assert.Equal(t, OrderabilityAvailable, projection["demo-sorrentinos"].State)

	byID := map[string]database.MenuItem{}
	for _, cat := range categories {
		for _, item := range cat.Items {
			byID[item.ID] = item
		}
	}
	for _, id := range blocked {
		assert.False(t, byID[id].IsAvailable, id)
		assert.Equal(t, "out_of_stock", byID[id].InventoryStatus, id)
	}
	assert.True(t, byID["demo-ensalada"].IsAvailable)
	assert.Empty(t, byID["demo-ensalada"].InventoryStatus)
	assert.True(t, byID["demo-sorrentinos"].IsAvailable)
	assert.Empty(t, byID["demo-sorrentinos"].InventoryStatus)
}
