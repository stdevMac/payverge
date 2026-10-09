package database

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestQuantizeInventoryQuantity_CollapsesFloatResidue(t *testing.T) {
	// Classic residue from repeated 0.08 deductions (Iced Tea recipe).
	noisy := 29.920000000000016
	assert.Equal(t, 29.92, QuantizeInventoryQuantity(noisy))
	assert.Equal(t, 0.08, QuantizeInventoryQuantity(0.08))
	assert.Equal(t, 0.35, QuantizeInventoryQuantity(0.35))
	assert.Equal(t, 10.0, QuantizeInventoryQuantity(10))
}

func TestInventoryModels_OmitEmptyBusinessFromJSON(t *testing.T) {
	// FIND-036: zero-value Business associations used to dump ~3KB of empty
	// business fields per inventory row because encoding/json never omits
	// non-pointer structs even with omitempty.
	item := InventoryItem{
		ID:              9,
		BusinessID:      50,
		Name:            "Tea Concentrate",
		Unit:            "L",
		CurrentQuantity: 29.920000000000016,
	}
	raw, err := json.Marshal(item)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	_, hasBusiness := m["business"]
	assert.False(t, hasBusiness, "inventory item JSON must not embed business")
	assert.Equal(t, float64(50), m["business_id"])

	recipe := InventoryRecipe{
		ID:               1,
		BusinessID:       50,
		MenuItemID:       "demo-tea",
		InventoryItemID:  9,
		QuantityRequired: 0.08,
		InventoryItem:    item,
	}
	raw, err = json.Marshal(recipe)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &m))
	_, hasBusiness = m["business"]
	assert.False(t, hasBusiness, "inventory recipe JSON must not embed business")
	nested, ok := m["inventory_item"].(map[string]any)
	require.True(t, ok)
	_, nestedBiz := nested["business"]
	assert.False(t, nestedBiz, "nested inventory_item must not embed business")

	movement := InventoryMovement{
		ID:              1,
		BusinessID:      50,
		InventoryItemID: 9,
		MovementType:    InventoryMovementTypeOrderConsumption,
		QuantityDelta:   -0.08,
		QuantityBefore:  30,
		QuantityAfter:   29.92,
		InventoryItem:   item,
	}
	raw, err = json.Marshal(movement)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &m))
	_, hasBusiness = m["business"]
	assert.False(t, hasBusiness, "inventory movement JSON must not embed business")

	settings := InventorySettings{ID: 1, BusinessID: 50, InventoryEnabled: true}
	raw, err = json.Marshal(settings)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &m))
	_, hasBusiness = m["business"]
	assert.False(t, hasBusiness, "inventory settings JSON must not embed business")
}

func TestApplyInventoryMovementTx_QuantizesAfter(t *testing.T) {
	setupOrderTestDB(t)

	business := helperBusiness(t, 0, 0)
	item := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Concentrate",
		Unit:            "L",
		CurrentQuantity: 30,
		IsActive:        true,
	}
	require.NoError(t, db.Create(item).Error)

	// Seed noisy on-hand as if prior float residue already lived in the row.
	require.NoError(t, db.Model(item).Update("current_quantity", 29.920000000000016).Error)
	require.NoError(t, db.First(item, item.ID).Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		return applyInventoryMovementTx(tx, item, -0.08, inventoryMovementMetadata{
			MovementType: InventoryMovementTypeOrderConsumption,
			Reason:       "test quantize",
			Actor:        "test",
		})
	})
	require.NoError(t, err)

	assert.Equal(t, 29.84, item.CurrentQuantity)

	var reloaded InventoryItem
	require.NoError(t, db.First(&reloaded, item.ID).Error)
	assert.Equal(t, 29.84, reloaded.CurrentQuantity)

	var movement InventoryMovement
	require.NoError(t, db.Where("inventory_item_id = ?", item.ID).Order("id DESC").First(&movement).Error)
	assert.Equal(t, 29.92, movement.QuantityBefore)
	assert.Equal(t, 29.84, movement.QuantityAfter)
	assert.Equal(t, -0.08, movement.QuantityDelta)
}

func TestListInventoryItemsByBusinessID_QuantizesPresentation(t *testing.T) {
	setupOrderTestDB(t)

	business := helperBusiness(t, 0, 0)
	item := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Tea",
		Unit:            "L",
		CurrentQuantity: 10,
		IsActive:        true,
	}
	require.NoError(t, db.Create(item).Error)
	require.NoError(t, db.Model(item).Update("current_quantity", 29.920000000000016).Error)

	items, err := ListInventoryItemsByBusinessID(business.ID, false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 29.92, items[0].CurrentQuantity)
}

func TestInventoryItemMarshalJSON_PartialProjectionIsSlim(t *testing.T) {
	// Mirrors movement preload Select("id","name","cost_per_unit").
	raw, err := json.Marshal(InventoryItem{ID: 7, Name: "Mixed Greens", CostPerUnit: 4.2})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Equal(t, float64(7), m["id"])
	require.Equal(t, "Mixed Greens", m["name"])
	require.Equal(t, 4.2, m["cost_per_unit"])
	for _, banned := range []string{
		"business_id", "sku", "category", "unit", "current_quantity",
		"reorder_threshold", "is_active", "created_at", "updated_at", "business",
	} {
		if _, ok := m[banned]; ok {
			t.Errorf("partial inventory item must not emit %q", banned)
		}
	}
	require.Len(t, m, 3)
}

func TestInventoryItemMarshalJSON_FullRowKeepsStockFields(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	raw, err := json.Marshal(InventoryItem{
		ID: 7, BusinessID: 50, Name: "Mixed Greens", Unit: "kg",
		CurrentQuantity: 36, CostPerUnit: 4.2, IsActive: true,
		CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Equal(t, float64(50), m["business_id"])
	require.Equal(t, "kg", m["unit"])
	require.Equal(t, true, m["is_active"])
	require.Equal(t, float64(36), m["current_quantity"])
}
