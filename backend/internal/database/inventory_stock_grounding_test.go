package database

import (
	"encoding/json"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupStockGroundingDB builds an in-memory business whose Steak Plate depends
// on a fully-depleted "Premium Beef" ingredient (the demo's stockout), while
// the Harvest Bowl's ingredient is plentiful. AvailabilitySyncMode is "warn"
// (the demo default) so the test proves the AI grounding hides an unmakeable
// dish even when the operator has NOT hard-blocked it on the menu.
func setupStockGroundingDB(t *testing.T) *Business {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open("file:stock-grounding?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	db = gormDB
	if err := db.AutoMigrate(
		&Business{},
		&InventorySettings{},
		&InventoryItem{},
		&InventoryRecipe{},
		&Menu{},
	); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}

	business := &Business{Name: "Stock Grounding"}
	if err := db.Create(business).Error; err != nil {
		t.Fatalf("create business: %v", err)
	}
	if err := db.Create(&InventorySettings{
		BusinessID:              business.ID,
		InventoryEnabled:        true,
		LowStockWarningsEnabled: true,
		AvailabilitySyncMode:    InventoryAvailabilityModeWarn,
	}).Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}

	menu := []MenuCategory{{
		ID:   "mains",
		Name: "Mains",
		Items: []MenuItem{
			{ID: "demo-steak", Name: "Steak Plate", Price: 42, IsAvailable: true},
			{ID: "demo-bowl", Name: "Harvest Bowl", Price: 18.5, IsAvailable: true},
		},
	}}
	rawMenu, err := json.Marshal(menu)
	if err != nil {
		t.Fatalf("marshal menu: %v", err)
	}
	if err := db.Create(&Menu{BusinessID: business.ID, Categories: string(rawMenu), IsActive: true}).Error; err != nil {
		t.Fatalf("create menu: %v", err)
	}

	beef := InventoryItem{BusinessID: business.ID, Name: "Premium Beef", Unit: "kg", CurrentQuantity: 0, IsActive: true}
	greens := InventoryItem{BusinessID: business.ID, Name: "Mixed Greens", Unit: "kg", CurrentQuantity: 48, IsActive: true}
	if err := db.Create(&beef).Error; err != nil {
		t.Fatalf("create beef: %v", err)
	}
	if err := db.Create(&greens).Error; err != nil {
		t.Fatalf("create greens: %v", err)
	}
	if err := db.Create([]InventoryRecipe{
		{BusinessID: business.ID, MenuItemID: "demo-steak", MenuItemName: "Steak Plate", InventoryItemID: beef.ID, QuantityRequired: 0.35},
		{BusinessID: business.ID, MenuItemID: "demo-bowl", MenuItemName: "Harvest Bowl", InventoryItemID: greens.ID, QuantityRequired: 0.25},
	}).Error; err != nil {
		t.Fatalf("create recipes: %v", err)
	}
	return business
}

func TestUnrecommendableMenuItemIDs_OutOfStockHiddenInWarnMode(t *testing.T) {
	business := setupStockGroundingDB(t)

	hidden, err := UnrecommendableMenuItemIDs(business.ID)
	if err != nil {
		t.Fatalf("UnrecommendableMenuItemIDs: %v", err)
	}
	if !hidden["demo-steak"] {
		t.Errorf("expected demo-steak (beef depleted) to be unrecommendable, got hidden=%v", hidden)
	}
	if hidden["demo-bowl"] {
		t.Errorf("expected demo-bowl (greens in stock) to remain recommendable, got hidden=%v", hidden)
	}
}

func TestUnrecommendableMenuItemIDs_RestockClearsHidden(t *testing.T) {
	business := setupStockGroundingDB(t)

	// Restock the beef above one serving; the steak becomes recommendable again.
	if err := db.Model(&InventoryItem{}).
		Where("business_id = ? AND name = ?", business.ID, "Premium Beef").
		Update("current_quantity", 10).Error; err != nil {
		t.Fatalf("restock beef: %v", err)
	}

	hidden, err := UnrecommendableMenuItemIDs(business.ID)
	if err != nil {
		t.Fatalf("UnrecommendableMenuItemIDs: %v", err)
	}
	if hidden["demo-steak"] {
		t.Errorf("expected demo-steak to be recommendable after restock, got hidden=%v", hidden)
	}
	if len(hidden) != 0 {
		t.Errorf("expected no hidden items after restock, got %v", hidden)
	}
}
