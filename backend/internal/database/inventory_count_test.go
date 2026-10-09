package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRecordInventoryCount_SetsAbsoluteTargetUnderConcurrentDeduction is the
// INV-M1 regression: a physical count must reconcile on-hand to the operator's
// observed shelf value (an ABSOLUTE target), computing the delta against the
// LIVE locked row — not a stale client-side basis. A deduction that lands
// between page-load and save must not corrupt the count.
func TestRecordInventoryCount_SetsAbsoluteTargetUnderConcurrentDeduction(t *testing.T) {
	setupOrderTestDB(t)
	business := helperBusiness(t, 0, 0)

	item := &InventoryItem{
		BusinessID: business.ID, Name: "Olive Oil", Unit: "bottle", CurrentQuantity: 100, IsActive: true,
	}
	require.NoError(t, db.Create(item).Error)

	// A concurrent deduction lands between the operator opening the drawer
	// (which snapshotted 100) and the save: 100 -> 70.
	require.NoError(t, db.Model(&InventoryItem{}).Where("id = ?", item.ID).
		Update("current_quantity", 70).Error)

	// Operator physically counts the shelf = 88 and submits that absolute value.
	updated, movement, err := RecordInventoryCount(business.ID, item.ID, 88, "physical count", "staff")
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.NotNil(t, movement)

	// On-hand must equal exactly the counted value — NOT 88-30=58 (the old
	// stale-delta bug) and NOT 88 minus anything.
	require.InDelta(t, 88.0, updated.CurrentQuantity, 0.001)
	var reloaded InventoryItem
	require.NoError(t, db.First(&reloaded, item.ID).Error)
	require.InDelta(t, 88.0, reloaded.CurrentQuantity, 0.001)

	// The correction movement records the variance against the LIVE value (70 -> 88 = +18).
	require.Equal(t, InventoryMovementTypeCorrection, movement.MovementType)
	require.InDelta(t, 18.0, movement.QuantityDelta, 0.001)
	require.InDelta(t, 70.0, movement.QuantityBefore, 0.001)
	require.InDelta(t, 88.0, movement.QuantityAfter, 0.001)
}

// TestRecordInventoryCount_ZeroVarianceRecordsMovement: a count that matches the
// system value is a legitimate "verified" event — it records a zero-delta
// correction (audit trail) and returns a non-nil movement, rather than erroring
// the way a relative zero-delta adjustment does.
func TestRecordInventoryCount_ZeroVarianceRecordsMovement(t *testing.T) {
	setupOrderTestDB(t)
	business := helperBusiness(t, 0, 0)

	item := &InventoryItem{
		BusinessID: business.ID, Name: "Salt", Unit: "kg", CurrentQuantity: 50, IsActive: true,
	}
	require.NoError(t, db.Create(item).Error)

	updated, movement, err := RecordInventoryCount(business.ID, item.ID, 50, "physical count", "staff")
	require.NoError(t, err)
	require.NotNil(t, movement)
	require.InDelta(t, 50.0, updated.CurrentQuantity, 0.001)
	require.InDelta(t, 0.0, movement.QuantityDelta, 0.001)
	require.InDelta(t, 50.0, movement.QuantityBefore, 0.001)
	require.InDelta(t, 50.0, movement.QuantityAfter, 0.001)
}

// TestGetInventorySummary_ClampsNegativeMaxServings is the INV-L4 regression: a
// warn-mode oversell can drive an ingredient's quantity negative, which made
// MaxPossibleServings = floor(negative/required) render as a negative servings
// count. It must clamp to 0 (the -1 "untracked" sentinel is reserved for items
// with no constraining recipe).
func TestGetInventorySummary_ClampsNegativeMaxServings(t *testing.T) {
	setupOrderTestDB(t)
	business := helperBusiness(t, 0, 0)
	require.NoError(t, db.Create(&InventorySettings{
		BusinessID:                business.ID,
		InventoryEnabled:          true,
		AutoDeductOnOrderApproval: true,
		LowStockWarningsEnabled:   true,
		AvailabilitySyncMode:      InventoryAvailabilityModeWarn,
	}).Error)

	flour := &InventoryItem{
		BusinessID: business.ID, Name: "Flour", Unit: "kg",
		CurrentQuantity: -3, ReorderThreshold: 5, IsActive: true,
	}
	require.NoError(t, db.Create(flour).Error)

	helperMenu(t, business, []MenuCategory{
		{ID: "cat-1", Name: "Main", Items: []MenuItem{{ID: "burger-id", Name: "Burger", IsAvailable: true}}},
	})
	require.NoError(t, db.Create(&InventoryRecipe{
		BusinessID: business.ID, MenuItemID: "burger-id", MenuItemName: "Burger",
		InventoryItemID: flour.ID, QuantityRequired: 1,
	}).Error)

	summary, err := GetInventorySummary(business.ID)
	require.NoError(t, err)

	statusByID := make(map[string]InventoryMenuItemStatus, len(summary.MenuItemStatuses))
	for _, st := range summary.MenuItemStatuses {
		statusByID[st.MenuItemID] = st
	}
	burger := statusByID["burger-id"]
	require.Equal(t, "out_of_stock", burger.Status)
	require.Equal(t, 0, burger.MaxPossibleServings, "oversold ingredient must yield 0 servings, not negative")
}

// TestRecordInventoryCount_RejectsNegativeTarget: a physical count can never be
// negative; the target is validated before any write.
func TestRecordInventoryCount_RejectsNegativeTarget(t *testing.T) {
	setupOrderTestDB(t)
	business := helperBusiness(t, 0, 0)

	item := &InventoryItem{
		BusinessID: business.ID, Name: "Pepper", Unit: "kg", CurrentQuantity: 5, IsActive: true,
	}
	require.NoError(t, db.Create(item).Error)

	_, _, err := RecordInventoryCount(business.ID, item.ID, -1, "physical count", "staff")
	require.Error(t, err)

	var reloaded InventoryItem
	require.NoError(t, db.First(&reloaded, item.ID).Error)
	require.InDelta(t, 5.0, reloaded.CurrentQuantity, 0.001, "rejected count must not mutate quantity")

	var movements []InventoryMovement
	require.NoError(t, db.Where("inventory_item_id = ?", item.ID).Find(&movements).Error)
	require.Len(t, movements, 0, "no movement on a rejected count")
}
