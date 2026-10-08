package database

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newFoodCostTestDB opens an isolated in-memory SQLite DB, migrates the
// tables GetRecipeCostsForBusiness reads, registers it as the package DB,
// and returns the wrapper.
func newFoodCostTestDB(t *testing.T) (*DB, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	// Error-level logger: surfaces real query errors but suppresses GORM's
	// unconditional Warn when a registered query callback is removed in
	// t.Cleanup (gorm@v1.30.0 callbacks.go:229 warns regardless of name).
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &InventoryItem{}, &InventoryRecipe{}))
	SetTestDB(gormDB)
	return GetDBWrapper(), gormDB
}

func TestGetRecipeCostsForBusiness_JoinsCostAndFlagsMissing(t *testing.T) {
	db, gormDB := newFoodCostTestDB(t)
	biz := Business{Name: "Margin Bistro"}
	require.NoError(t, gormDB.Create(&biz).Error)

	beef := InventoryItem{BusinessID: biz.ID, Name: "Beef", Unit: "kg", CostPerUnit: 10.0}
	bun := InventoryItem{BusinessID: biz.ID, Name: "Bun", Unit: "unit", CostPerUnit: 0.0} // missing cost
	require.NoError(t, gormDB.Create(&beef).Error)
	require.NoError(t, gormDB.Create(&bun).Error)

	require.NoError(t, gormDB.Create(&InventoryRecipe{
		BusinessID: biz.ID, MenuItemID: "burger", MenuItemName: "Burger",
		InventoryItemID: beef.ID, QuantityRequired: 0.2,
	}).Error)
	require.NoError(t, gormDB.Create(&InventoryRecipe{
		BusinessID: biz.ID, MenuItemID: "burger", MenuItemName: "Burger",
		InventoryItemID: bun.ID, QuantityRequired: 1,
	}).Error)

	rows, err := db.GetRecipeCostsForBusiness(biz.ID)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	byCost := map[float64]RecipeIngredientCost{}
	for _, r := range rows {
		byCost[r.CostPerUnit] = r
	}
	require.Equal(t, "Burger", byCost[10.0].MenuItemName)
	require.InDelta(t, 0.2, byCost[10.0].QuantityRequired, 0.0001)
	require.True(t, byCost[10.0].HasCost)
	require.False(t, byCost[0.0].HasCost)
}

func TestGetRecipeCostsForBusiness_NoNPlusOne(t *testing.T) {
	db, gormDB := newFoodCostTestDB(t)
	biz := Business{Name: "N+1 Bistro"}
	require.NoError(t, gormDB.Create(&biz).Error)

	// 3 menu items, 2 distinct ingredients each (6 recipe rows, 6 items).
	for i := 0; i < 3; i++ {
		a := InventoryItem{BusinessID: biz.ID, Name: fmt.Sprintf("A%d", i), Unit: "kg", CostPerUnit: 1.5}
		b := InventoryItem{BusinessID: biz.ID, Name: fmt.Sprintf("B%d", i), Unit: "kg", CostPerUnit: 2.5}
		require.NoError(t, gormDB.Create(&a).Error)
		require.NoError(t, gormDB.Create(&b).Error)
		menu := fmt.Sprintf("item-%d", i)
		require.NoError(t, gormDB.Create(&InventoryRecipe{BusinessID: biz.ID, MenuItemID: menu, MenuItemName: menu, InventoryItemID: a.ID, QuantityRequired: 1}).Error)
		require.NoError(t, gormDB.Create(&InventoryRecipe{BusinessID: biz.ID, MenuItemID: menu, MenuItemName: menu, InventoryItemID: b.ID, QuantityRequired: 1}).Error)
	}

	cbName := "count_item_queries_" + t.Name()
	var itemQueries int
	require.NoError(t, gormDB.Callback().Query().After("gorm:query").
		Register(cbName, func(tx *gorm.DB) {
			if strings.Contains(strings.ToLower(tx.Statement.SQL.String()), "inventory_items") {
				itemQueries++
			}
		}))
	t.Cleanup(func() { _ = gormDB.Callback().Query().Remove(cbName) })

	rows, err := db.GetRecipeCostsForBusiness(biz.ID)
	require.NoError(t, err)
	require.Len(t, rows, 6)
	require.Equal(t, 1, itemQueries, "inventory_items must be loaded in ONE batched query, not per-recipe (N+1)")
}

func TestGetRecipeCostsForBusiness_BusinessIsolation(t *testing.T) {
	db, gormDB := newFoodCostTestDB(t)

	bizA := Business{Name: "Alpha Cafe", BusinessId: "biz-a"}
	bizB := Business{Name: "Beta Cafe", BusinessId: "biz-b"}
	require.NoError(t, gormDB.Create(&bizA).Error)
	require.NoError(t, gormDB.Create(&bizB).Error)

	itemA := InventoryItem{BusinessID: bizA.ID, Name: "Cheese", Unit: "kg", CostPerUnit: 5.0}
	itemB := InventoryItem{BusinessID: bizB.ID, Name: "Tomato", Unit: "kg", CostPerUnit: 3.0}
	require.NoError(t, gormDB.Create(&itemA).Error)
	require.NoError(t, gormDB.Create(&itemB).Error)

	require.NoError(t, gormDB.Create(&InventoryRecipe{
		BusinessID: bizA.ID, MenuItemID: "pizza-a", MenuItemName: "Pizza A",
		InventoryItemID: itemA.ID, QuantityRequired: 0.3,
	}).Error)
	require.NoError(t, gormDB.Create(&InventoryRecipe{
		BusinessID: bizB.ID, MenuItemID: "pizza-b", MenuItemName: "Pizza B",
		InventoryItemID: itemB.ID, QuantityRequired: 0.4,
	}).Error)

	rows, err := db.GetRecipeCostsForBusiness(bizA.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1, "must return only the queried business's recipes")
	require.Equal(t, "Pizza A", rows[0].MenuItemName)
	require.Equal(t, 5.0, rows[0].CostPerUnit)
	require.True(t, rows[0].HasCost)
}
