package demo

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// grillCutsFromTheMediaRes is every carta dish butchered off the demo's half
// carcass. Matambrito is pork and the offal/bodegón plates are prepped ahead,
// so they stay orderable while the parrilla is out of beef.
var grillCutsFromTheMediaRes = []string{
	"demo-bife",
	"demo-ojo-de-bife",
	"demo-asado-tira",
	"demo-entrana",
	"demo-parrillada",
}

func demoMenuNamesByID(t *testing.T, db *gorm.DB, businessID uint) map[string]string {
	t.Helper()
	var menu database.Menu
	require.NoError(t, db.Where("business_id = ?", businessID).First(&menu).Error)
	var categories []database.MenuCategory
	require.NoError(t, json.Unmarshal([]byte(menu.Categories), &categories))
	names := map[string]string{}
	for _, category := range categories {
		for _, item := range category.Items {
			names[item.ID] = item.Name
		}
	}
	require.NotEmpty(t, names)
	return names
}

// outOfStockMenuItemIDs mirrors database.OutOfStockMenuItemIDs — same JOIN and
// same predicate — because that function reads the package-level prod handle
// and cannot be pointed at the test DB. Keep the two in sync.
func outOfStockMenuItemIDs(t *testing.T, db *gorm.DB, businessID uint) map[string]bool {
	t.Helper()
	var ids []string
	require.NoError(t, db.Table("inventory_recipes r").
		Select("DISTINCT r.menu_item_id").
		Joins("LEFT JOIN inventory_items i ON i.id = r.inventory_item_id AND i.business_id = r.business_id").
		Where("r.business_id = ? AND (i.id IS NULL OR i.is_active = ? OR (r.quantity_required > 0 AND i.current_quantity < r.quantity_required))", businessID, false).
		Scan(&ids).Error)
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func beefRecipesByMenuItem(t *testing.T, db *gorm.DB, businessID, inventoryItemID uint) map[string]database.InventoryRecipe {
	t.Helper()
	var recipes []database.InventoryRecipe
	require.NoError(t, db.Where("business_id = ? AND inventory_item_id = ?", businessID, inventoryItemID).
		Find(&recipes).Error)
	byMenuItem := make(map[string]database.InventoryRecipe, len(recipes))
	for _, recipe := range recipes {
		_, dup := byMenuItem[recipe.MenuItemID]
		require.Falsef(t, dup, "duplicate beef recipe for %s", recipe.MenuItemID)
		byMenuItem[recipe.MenuItemID] = recipe
	}
	return byMenuItem
}

func demoBeefItem(t *testing.T, db *gorm.DB, businessID uint) database.InventoryItem {
	t.Helper()
	var beef database.InventoryItem
	require.NoError(t, db.Where("business_id = ? AND sku LIKE ?", businessID, "DEMO-%-BEEF").First(&beef).Error)
	return beef
}

// #945: the media res was only ever linked to demo-bife, so when the kitchen
// ran out of beef the carta went on selling ojo de bife, asado de tira,
// entraña and the parrillada (and the grill bundle that contains it).
func TestDemoBeefShortage86sEveryCutOffTheMediaRes(t *testing.T) {
	db := newFKEnforcedDemoDB(t)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_inventory_items_biz_sku ON inventory_items (business_id, sku) WHERE sku <> ''`).Error)
	admin := seedAdmin(t, db, "demo-beef-cascade@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 5})
	require.NoError(t, mustEnsure(svc, admin.ID))

	var businesses []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Order("id ASC").Find(&businesses).Error)
	require.Len(t, businesses, 2)

	for _, business := range businesses {
		menuNames := demoMenuNamesByID(t, db, business.ID)
		beef := demoBeefItem(t, db, business.ID)
		recipes := beefRecipesByMenuItem(t, db, business.ID, beef.ID)
		require.Lenf(t, recipes, len(grillCutsFromTheMediaRes),
			"business %d must yield one recipe per cut off the media res", business.ID)

		for _, menuItemID := range grillCutsFromTheMediaRes {
			recipe, ok := recipes[menuItemID]
			require.Truef(t, ok, "business %d is missing the %s recipe", business.ID, menuItemID)
			require.Greaterf(t, recipe.QuantityRequired, 0.0,
				"%s must consume beef, or it never constrains availability", menuItemID)
			require.Equalf(t, menuNames[menuItemID], recipe.MenuItemName,
				"%s recipe name must match the live carta dish", menuItemID)
		}

		// The kitchen runs out mid-service: every cut off that carcass is 86'd.
		require.NoError(t, db.Model(&database.InventoryItem{}).
			Where("id = ?", beef.ID).Update("current_quantity", 0).Error)
		oos := outOfStockMenuItemIDs(t, db, business.ID)
		for _, menuItemID := range grillCutsFromTheMediaRes {
			require.Truef(t, oos[menuItemID], "%s must be 86'd when the media res is out", menuItemID)
		}
		// Dishes that do not come off the carcass keep selling.
		require.False(t, oos["demo-matambrito"], "pork matambrito must stay orderable")
		require.False(t, oos["demo-choripan"], "choripán must stay orderable")
	}
}

// The grill bundle sells the parrillada, so gating the dish is what carries the
// 86 into "Noche de parrilla para dos". Recipes are menu-item-only — a bundle
// id has no live dish to resolve against — so the bundle cascade must ride on
// its BundleItemRef, and that ref must point at a gated dish.
func TestDemoGrillBundleReferencesAGatedCut(t *testing.T) {
	db := newFKEnforcedDemoDB(t)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_inventory_items_biz_sku ON inventory_items (business_id, sku) WHERE sku <> ''`).Error)
	admin := seedAdmin(t, db, "demo-beef-bundle@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 5})
	require.NoError(t, mustEnsure(svc, admin.ID))

	var businesses []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Order("id ASC").Find(&businesses).Error)
	require.Len(t, businesses, 2)

	for _, business := range businesses {
		var bundle database.Bundle
		require.NoError(t, db.Where("business_id = ?", business.ID).First(&bundle).Error)
		var refs []database.BundleItemRef
		require.NoError(t, json.Unmarshal([]byte(bundle.Items), &refs))
		require.NotEmpty(t, refs)

		beef := demoBeefItem(t, db, business.ID)
		recipes := beefRecipesByMenuItem(t, db, business.ID, beef.ID)
		gated := false
		for _, ref := range refs {
			if _, ok := recipes[ref.MenuItemID]; ok {
				gated = true
			}
		}
		require.Truef(t, gated,
			"bundle %q on business %d must contain a dish the beef shortage gates", bundle.Name, business.ID)
	}
}

// Demos seeded before #945 carry a single beef recipe (plus, since #727,
// possibly one drifted onto another dish). A same-seed-version ensure must
// backfill the missing cuts and drop the drift without duplicating anything.
func TestEnsureBackfillsMissingBeefRecipes(t *testing.T) {
	db := newFKEnforcedDemoDB(t)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_inventory_items_biz_sku ON inventory_items (business_id, sku) WHERE sku <> ''`).Error)
	admin := seedAdmin(t, db, "demo-beef-backfill@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 5})
	require.NoError(t, mustEnsure(svc, admin.ID))

	var businesses []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Order("id ASC").Find(&businesses).Error)
	require.Len(t, businesses, 2)

	// Rewind to the pre-#945 shape: only demo-bife, plus a drifted pairing.
	for _, business := range businesses {
		beef := demoBeefItem(t, db, business.ID)
		require.NoError(t, db.Where("business_id = ? AND inventory_item_id = ? AND menu_item_id <> ?",
			business.ID, beef.ID, "demo-bife").Delete(&database.InventoryRecipe{}).Error)
		require.NoError(t, db.Create(&database.InventoryRecipe{
			BusinessID: business.ID, MenuItemID: "demo-fritas", MenuItemName: "Papas fritas",
			InventoryItemID: beef.ID, QuantityRequired: 0.30,
		}).Error)
	}

	require.NoError(t, mustEnsure(svc, admin.ID))

	for _, business := range businesses {
		beef := demoBeefItem(t, db, business.ID)
		recipes := beefRecipesByMenuItem(t, db, business.ID, beef.ID)
		require.Lenf(t, recipes, len(grillCutsFromTheMediaRes),
			"business %d must end with exactly the media-res cuts", business.ID)
		for _, menuItemID := range grillCutsFromTheMediaRes {
			require.Containsf(t, recipes, menuItemID, "backfill missed %s", menuItemID)
		}
		require.NotContains(t, recipes, "demo-fritas", "drifted pairing must be dropped")
	}
}
