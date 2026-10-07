package database

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// inventoryPreloadSQLRecorder captures emitted SQL so that the InventoryItem
// preload projection on the two cold inventory list reads can be asserted at
// the query layer (CG-1). Mirrors reservation_read_projection_test.go's helper.
type inventoryPreloadSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *inventoryPreloadSQLRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *inventoryPreloadSQLRecorder) reset() { r.statements = nil }

// selectStarOnInventoryItems returns true if any captured statement is a
// SELECT * from inventory_items — the over-fetch we are removing.
func (r *inventoryPreloadSQLRecorder) selectStarOnInventoryItems() bool {
	for _, s := range r.statements {
		norm := strings.ToLower(strings.TrimSpace(s))
		if strings.HasPrefix(norm, "select *") &&
			(strings.Contains(norm, "from `inventory_items`") ||
				strings.Contains(norm, "from \"inventory_items\"") ||
				strings.Contains(norm, "from inventory_items")) {
			return true
		}
	}
	return false
}

// inventoryItemPreloadMentionsColumn returns true when at least one captured
// statement selecting from inventory_items includes the given column name.
func (r *inventoryPreloadSQLRecorder) inventoryItemPreloadMentionsColumn(col string) bool {
	needleBacktick := "`" + strings.ToLower(col) + "`"
	needleDouble := `"` + strings.ToLower(col) + `"`
	needlePlain := strings.ToLower(col)
	for _, s := range r.statements {
		norm := strings.ToLower(s)
		if !(strings.Contains(norm, "from `inventory_items`") ||
			strings.Contains(norm, "from \"inventory_items\"") ||
			strings.Contains(norm, "from inventory_items")) {
			continue
		}
		if strings.Contains(norm, needleBacktick) ||
			strings.Contains(norm, needleDouble) ||
			strings.Contains(norm, needlePlain) {
			return true
		}
	}
	return false
}

func setupInventoryPreloadProjectionDB(t testing.TB, rec *inventoryPreloadSQLRecorder) *gorm.DB {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: rec})
	require.NoError(t, err, "open in-memory database")

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	db = gormDB
	require.NoError(t, db.AutoMigrate(
		&Business{},
		&InventoryItem{},
		&InventoryRecipe{},
		&InventoryMovement{},
	))
	return gormDB
}

// seedInventoryPreloadData seeds a business with a fully-populated InventoryItem
// (all columns set, including the over-fetched ones), a recipe and a movement
// referencing it. Returns the seeded business and item so tests can compare
// the projected-vs-full field set.
func seedInventoryPreloadData(t testing.TB) (*Business, *InventoryItem) {
	t.Helper()

	business := &Business{
		BusinessId:   fmt.Sprintf("inv-proj-%d", time.Now().UnixNano()),
		Name:         "Projected Kitchen",
		OwnerAddress: fmt.Sprintf("0xinvproj%x", time.Now().UnixNano()),
	}
	require.NoError(t, db.Create(business).Error)

	item := &InventoryItem{
		BusinessID:       business.ID,
		Name:             "Organic Tomato",
		SKU:              "TOM-001",
		Category:         "Produce",
		Unit:             "kg",
		CurrentQuantity:  42.5,
		ReorderThreshold: 5.0,
		CostPerUnit:      1.25,
		IsActive:         true,
	}
	require.NoError(t, db.Create(item).Error)

	require.NoError(t, db.Create(&InventoryRecipe{
		BusinessID:       business.ID,
		MenuItemID:       "tomato-salad",
		MenuItemName:     "Tomato Salad",
		InventoryItemID:  item.ID,
		QuantityRequired: 0.2,
	}).Error)

	require.NoError(t, db.Create(&InventoryMovement{
		BusinessID:      business.ID,
		InventoryItemID: item.ID,
		MovementType:    InventoryMovementTypePurchase,
		QuantityDelta:   10.0,
		QuantityBefore:  32.5,
		QuantityAfter:   42.5,
		Reason:          "weekly purchase",
		Actor:           "manager@example.com",
	}).Error)

	return business, item
}

// TestInventoryMovementPreloadDropsSelectStar asserts that:
//  1. (RED before fix) — the preload query does NOT SELECT * from inventory_items.
//  2. The FE-consumed columns name and cost_per_unit are present in the projected SELECT.
//  3. The loaded movement's InventoryItem.Name and CostPerUnit are still populated
//     (response shape preserved).
//  4. The over-fetched columns (reorder_threshold, sku, category) are NOT selected.
func TestInventoryMovementPreloadDropsSelectStar(t *testing.T) {
	rec := &inventoryPreloadSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupInventoryPreloadProjectionDB(t, rec)

	business, item := seedInventoryPreloadData(t)

	rec.reset()
	movements, _, err := ListInventoryMovementsFiltered(business.ID, InventoryMovementFilter{Limit: 50})
	require.NoError(t, err)
	require.Len(t, movements, 1, "expected exactly one movement")

	// Access-shape assertion: no SELECT * on inventory_items.
	assert.False(t, rec.selectStarOnInventoryItems(),
		"ListInventoryMovementsFiltered must not SELECT * from inventory_items; project only consumed columns")

	// Projected columns must appear in the SELECT (FE renders these).
	assert.True(t, rec.inventoryItemPreloadMentionsColumn("name"),
		"projected inventory_items SELECT must include 'name' (FE renders movement.inventory_item?.name)")
	assert.True(t, rec.inventoryItemPreloadMentionsColumn("cost_per_unit"),
		"projected inventory_items SELECT must include 'cost_per_unit' (FE renders movement.inventory_item.cost_per_unit)")

	// Over-fetched columns must NOT appear.
	assert.False(t, rec.inventoryItemPreloadMentionsColumn("reorder_threshold"),
		"projected inventory_items SELECT must NOT include 'reorder_threshold' (not consumed by movements FE)")
	assert.False(t, rec.inventoryItemPreloadMentionsColumn("is_active"),
		"projected inventory_items SELECT must NOT include 'is_active' (not consumed by movements FE)")

	// Response-shape preservation: FE-consumed fields must still hydrate.
	require.NotNil(t, movements[0].InventoryItem.ID, "InventoryItem must be hydrated")
	assert.Equal(t, item.Name, movements[0].InventoryItem.Name,
		"movement.InventoryItem.Name must survive projection (FE renders it)")
	assert.InDelta(t, item.CostPerUnit, movements[0].InventoryItem.CostPerUnit, 0.001,
		"movement.InventoryItem.CostPerUnit must survive projection (FE renders it)")
}

// TestInventoryRecipePreloadDropsSelectStar asserts that:
//  1. (RED before fix) — the preload query does NOT SELECT * from inventory_items.
//  2. The backend-consumed column name is present in the projected SELECT.
//  3. The loaded recipe's InventoryItem.Name is still populated (backend summary uses it).
func TestInventoryRecipePreloadDropsSelectStar(t *testing.T) {
	rec := &inventoryPreloadSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupInventoryPreloadProjectionDB(t, rec)

	business, item := seedInventoryPreloadData(t)

	rec.reset()
	recipes, err := ListInventoryRecipesByBusinessID(business.ID)
	require.NoError(t, err)
	require.Len(t, recipes, 1, "expected exactly one recipe")

	// Access-shape assertion: no SELECT * on inventory_items.
	assert.False(t, rec.selectStarOnInventoryItems(),
		"ListInventoryRecipesByBusinessID must not SELECT * from inventory_items; project only consumed columns")

	// 'name' must be projected (GetInventorySummary reads recipe.InventoryItem.Name as a label fallback).
	assert.True(t, rec.inventoryItemPreloadMentionsColumn("name"),
		"projected inventory_items SELECT must include 'name' (GetInventorySummary reads recipe.InventoryItem.Name)")

	// Over-fetched columns must NOT appear.
	assert.False(t, rec.inventoryItemPreloadMentionsColumn("reorder_threshold"),
		"projected inventory_items SELECT must NOT include 'reorder_threshold' (not consumed by recipes)")
	assert.True(t, rec.inventoryItemPreloadMentionsColumn("cost_per_unit"),
		"projected inventory_items SELECT for recipes must include 'cost_per_unit'")
	assert.False(t, rec.inventoryItemPreloadMentionsColumn("is_active"),
		"projected inventory_items SELECT must NOT include 'is_active' (not consumed by recipes path)")

	// Response-shape preservation: Name and cost must survive projection.
	assert.Equal(t, item.Name, recipes[0].InventoryItem.Name,
		"recipe.InventoryItem.Name must survive projection (GetInventorySummary uses it as label fallback)")
	assert.InDelta(t, item.CostPerUnit, recipes[0].InventoryItem.CostPerUnit, 0.001,
		"recipe.InventoryItem.CostPerUnit must survive projection")
}
