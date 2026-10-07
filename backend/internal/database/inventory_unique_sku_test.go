package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestInventoryItems_UniqueBusinessSKUConstraint proves the DB-level backstop
// (composite unique index idx_inventory_items_biz_sku) is
// enforced: a second item with the same (business_id, sku) must be rejected so
// duplicate SKUs can never re-inflate GetInventorySummary counts/value.
func TestInventoryItems_UniqueBusinessSKUConstraint(t *testing.T) {
	setupOrderTestDB(t)
	// Mirror prod: the partial unique index is created by the genesis schema,
	// not GORM auto-migrate (which can't express the partial WHERE sku <> '').
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_inventory_items_biz_sku ON inventory_items (business_id, sku) WHERE sku <> ''`).Error)
	business := helperBusiness(t, 0, 0)

	first := &InventoryItem{BusinessID: business.ID, Name: "Mixed Greens", SKU: "DEMO-GREENS", Unit: "kg", CurrentQuantity: 48, IsActive: true}
	require.NoError(t, db.Create(first).Error)

	// Same natural key must collide.
	dup := &InventoryItem{BusinessID: business.ID, Name: "Mixed Greens (dup)", SKU: "DEMO-GREENS", Unit: "kg", CurrentQuantity: 12, IsActive: true}
	require.Error(t, db.Create(dup).Error, "second item with existing (business_id, sku) must be rejected")

	// A different SKU under the same business is fine.
	other := &InventoryItem{BusinessID: business.ID, Name: "Premium Beef", SKU: "DEMO-BEEF", Unit: "kg", CurrentQuantity: 22, IsActive: true}
	require.NoError(t, db.Create(other).Error)

	// Blank SKUs (valid, optional operator items) coexist thanks to the partial
	// predicate — the whole reason the index is partial rather than plain.
	blankA := &InventoryItem{BusinessID: business.ID, Name: "Napkins", SKU: "", Unit: "unit", CurrentQuantity: 500, IsActive: true}
	blankB := &InventoryItem{BusinessID: business.ID, Name: "Straws", SKU: "", Unit: "unit", CurrentQuantity: 300, IsActive: true}
	require.NoError(t, db.Create(blankA).Error)
	require.NoError(t, db.Create(blankB).Error, "blank SKUs must not collide under the partial index")
}

// TestGetInventorySummary_TotalItemsMatchesDistinctSKUs is the access-shape
// guard for the summary-inflation symptom (inventory.go GetInventorySummary
// returns TotalItems = len(items) and sums per row). With the unique constraint
// in place, TotalItems can never exceed the distinct real-SKU count.
func TestGetInventorySummary_TotalItemsMatchesDistinctSKUs(t *testing.T) {
	setupOrderTestDB(t)
	business := helperBusiness(t, 0, 0)
	require.NoError(t, db.Create(&InventorySettings{
		BusinessID:              business.ID,
		InventoryEnabled:        true,
		LowStockWarningsEnabled: true,
		AvailabilitySyncMode:    InventoryAvailabilityModeWarn,
	}).Error)

	skus := []string{"DEMO-GREENS", "DEMO-BEEF", "DEMO-TEA"}
	for _, sku := range skus {
		require.NoError(t, db.Create(&InventoryItem{
			BusinessID: business.ID, Name: sku, SKU: sku, Unit: "kg", CurrentQuantity: 20, IsActive: true,
		}).Error)
	}

	summary, err := GetInventorySummary(business.ID)
	require.NoError(t, err)

	var distinctSKUs int64
	require.NoError(t, db.Model(&InventoryItem{}).
		Where("business_id = ? AND sku <> ''", business.ID).
		Distinct("sku").Count(&distinctSKUs).Error)

	require.Equal(t, int64(len(skus)), distinctSKUs)
	require.Equal(t, len(skus), summary.TotalItems, "TotalItems must equal distinct SKU count — no duplicate inflation")
}
