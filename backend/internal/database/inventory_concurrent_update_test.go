package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUpdateInventoryItem_RejectsDeductionBelowZero guards the non-negative floor
// for UpdateInventoryItem: a deduction whose delta would drive the locked
// (live) quantity negative must be rejected, mirroring the guard that already
// exists in CreateInventoryAdjustment.
func TestUpdateInventoryItem_RejectsDeductionBelowZero(t *testing.T) {
	setupOrderTestDB(t)
	business := helperBusiness(t, 0, 0)

	item := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Tomato",
		Unit:            "kg",
		CurrentQuantity: 3,
		IsActive:        true,
	}
	require.NoError(t, db.Create(item).Error)

	// Operator saw 3 kg, tries to set it to 0 (delta = -3), which would be
	// fine. But a concurrent deduction has already reduced the live quantity
	// to 1, so the delta of -3 would drive it to -2 — must be rejected.
	previous := *item

	// Simulate concurrent deduction: 3 -> 1
	require.NoError(t, db.Model(&InventoryItem{}).Where("id = ?", item.ID).
		Update("current_quantity", 1).Error)

	// Operator submits a change that would go to 0 relative to what they saw (delta = -3).
	edited := previous
	edited.CurrentQuantity = 0

	err := UpdateInventoryItem(&edited, previous, "staff")
	require.Error(t, err, "deduction that would drive live quantity below zero must be rejected")
	require.Contains(t, err.Error(), "below zero")

	// Quantity must remain at the live value, not go negative.
	var reloaded InventoryItem
	require.NoError(t, db.First(&reloaded, item.ID).Error)
	require.InDelta(t, 1.0, reloaded.CurrentQuantity, 0.001, "quantity must not have changed")

	// No movement should have been recorded.
	var movements []InventoryMovement
	require.NoError(t, db.Where("inventory_item_id = ?", item.ID).Find(&movements).Error)
	require.Len(t, movements, 0, "no movement should be recorded for a rejected deduction")
}

// TestUpdateInventoryItem_PreservesConcurrentDeduction guards the lost-update
// race: the handler reads the item unlocked and passes that snapshot as
// `previous`. If a deduction commits before the editor saves, the operator's
// quantity change must apply as a delta against the LIVE quantity (re-read under
// lock), not overwrite it with the stale snapshot.
func TestUpdateInventoryItem_PreservesConcurrentDeduction(t *testing.T) {
	setupOrderTestDB(t)
	business := helperBusiness(t, 0, 0)

	item := &InventoryItem{
		BusinessID: business.ID, Name: "Flour", Unit: "kg", CurrentQuantity: 100, IsActive: true,
	}
	require.NoError(t, db.Create(item).Error)

	// Handler's stale (unlocked) read.
	previous := *item

	// A concurrent deduction lands between the read and the save: 100 -> 70.
	require.NoError(t, db.Model(&InventoryItem{}).Where("id = ?", item.ID).
		Update("current_quantity", 70).Error)

	// Operator edits quantity to 120 (a +20 change vs the 100 they saw) and renames.
	edited := previous
	edited.CurrentQuantity = 120
	edited.Name = "Flour (bag)"
	require.NoError(t, UpdateInventoryItem(&edited, previous, "staff"))

	var reloaded InventoryItem
	require.NoError(t, db.First(&reloaded, item.ID).Error)
	// 70 (live, deduction preserved) + 20 (operator delta) = 90 — not 120 (stale
	// absolute) and not 100 (clobbered).
	require.InDelta(t, 90.0, reloaded.CurrentQuantity, 0.001, "concurrent deduction must be preserved")
	require.Equal(t, "Flour (bag)", reloaded.Name, "non-quantity edits must persist")
	require.InDelta(t, 90.0, edited.CurrentQuantity, 0.001, "returned struct must reflect the true quantity")

	// The correction movement should record the +20 delta against the live value.
	var movements []InventoryMovement
	require.NoError(t, db.Where("inventory_item_id = ? AND movement_type = ?", item.ID, InventoryMovementTypeCorrection).Find(&movements).Error)
	require.Len(t, movements, 1)
	require.InDelta(t, 20.0, movements[0].QuantityDelta, 0.001)
	require.InDelta(t, 70.0, movements[0].QuantityBefore, 0.001)
	require.InDelta(t, 90.0, movements[0].QuantityAfter, 0.001)
}
