package database

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateInventoryAdjustmentAdvancesGuestOrderabilityRevision(t *testing.T) {
	setupOrderTestDB(t)

	business := helperBusiness(t, 0, 0)
	item := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Tomato",
		Unit:            "kg",
		CurrentQuantity: 1,
		IsActive:        true,
	}
	require.NoError(t, db.Create(item).Error)

	oldRevision := time.Date(2025, time.January, 1, 0, 0, 0, 123000, time.UTC)
	require.NoError(t, db.Model(&Business{}).
		Where("id = ?", business.ID).
		UpdateColumn("updated_at", oldRevision).Error)

	_, _, err := CreateInventoryAdjustment(
		business.ID,
		item.ID,
		InventoryMovementTypeRestock,
		1,
		"Delivery received",
		"manager@example.com",
	)
	require.NoError(t, err)

	var refreshed Business
	require.NoError(t, db.First(&refreshed, business.ID).Error)
	assert.True(t, refreshed.UpdatedAt.After(oldRevision),
		"stock mutations must advance the revision used by guest menu ETags")
}

func helperMenu(t *testing.T, business *Business, categories []MenuCategory) *Menu {
	t.Helper()
	raw, err := json.Marshal(categories)
	require.NoError(t, err)

	menu := &Menu{
		BusinessID: business.ID,
		Categories: string(raw),
		IsActive:   true,
		Version:    1,
	}
	require.NoError(t, db.Create(menu).Error)
	return menu
}

func TestUniqueSortedUintIDs(t *testing.T) {
	got := uniqueSortedUintIDs([]uint{9, 2, 9, 4, 2, 1})
	require.Equal(t, []uint{1, 2, 4, 9}, got)
}

func TestUpdateOrderStatus_ApproveDeductsInventoryForRecipes(t *testing.T) {
	setupOrderTestDB(t)

	business := helperBusiness(t, 0, 0)
	require.NoError(t, db.Create(&InventorySettings{
		BusinessID:                business.ID,
		InventoryEnabled:          true,
		AutoDeductOnOrderApproval: true,
		LowStockWarningsEnabled:   true,
		AvailabilitySyncMode:      InventoryAvailabilityModeWarn,
	}).Error)

	bun := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Bun",
		Unit:            "pcs",
		CurrentQuantity: 10,
		IsActive:        true,
	}
	patty := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Patty",
		Unit:            "pcs",
		CurrentQuantity: 5,
		IsActive:        true,
	}
	require.NoError(t, db.Create(bun).Error)
	require.NoError(t, db.Create(patty).Error)
	require.NoError(t, db.Create([]InventoryRecipe{
		{
			BusinessID:       business.ID,
			MenuItemID:       "burger-id",
			MenuItemName:     "Burger",
			InventoryItemID:  bun.ID,
			QuantityRequired: 1,
		},
		{
			BusinessID:       business.ID,
			MenuItemID:       "burger-id",
			MenuItemName:     "Burger",
			InventoryItemID:  patty.ID,
			QuantityRequired: 1,
		},
	}).Error)

	bill := helperBill(t, business, nil, 0)
	order := helperOrder(t, business, bill, []OrderItem{
		{
			ID:           "burger-line-1",
			MenuItemID:   "burger-id",
			MenuItemName: "Burger",
			Quantity:     2,
			Price:        12,
			Subtotal:     24,
		},
	})
	oldRevision := time.Date(2025, time.January, 1, 0, 0, 0, 456000, time.UTC)
	require.NoError(t, db.Model(&Business{}).
		Where("id = ?", business.ID).
		UpdateColumn("updated_at", oldRevision).Error)

	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "manager@example.com", ""))

	var refreshedBun InventoryItem
	var refreshedPatty InventoryItem
	require.NoError(t, db.First(&refreshedBun, bun.ID).Error)
	require.NoError(t, db.First(&refreshedPatty, patty.ID).Error)
	assert.Equal(t, 8.0, refreshedBun.CurrentQuantity)
	assert.Equal(t, 3.0, refreshedPatty.CurrentQuantity)
	var refreshedBusiness Business
	require.NoError(t, db.First(&refreshedBusiness, business.ID).Error)
	assert.True(t, refreshedBusiness.UpdatedAt.After(oldRevision),
		"order-driven deductions must advance the guest orderability revision")

	var movements []InventoryMovement
	require.NoError(t, db.Where("reference_order_id = ?", order.ID).Order("id ASC").Find(&movements).Error)
	require.Len(t, movements, 2)
	assert.Equal(t, InventoryMovementTypeOrderConsumption, movements[0].MovementType)
	assert.Equal(t, -2.0, movements[0].QuantityDelta)
	assert.Equal(t, uint(order.ID), *movements[0].ReferenceOrderID)

	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "manager@example.com", ""))

	movements = nil
	require.NoError(t, db.Where("reference_order_id = ?", order.ID).Find(&movements).Error)
	assert.Len(t, movements, 2)
	require.NoError(t, db.First(&refreshedBun, bun.ID).Error)
	require.NoError(t, db.First(&refreshedPatty, patty.ID).Error)
	assert.Equal(t, 8.0, refreshedBun.CurrentQuantity)
	assert.Equal(t, 3.0, refreshedPatty.CurrentQuantity)
}

func TestUpdateOrderStatus_HardBlockPreventsApprovalWhenInventoryWouldGoNegative(t *testing.T) {
	setupOrderTestDB(t)

	business := helperBusiness(t, 0, 0)
	require.NoError(t, db.Create(&InventorySettings{
		BusinessID:                business.ID,
		InventoryEnabled:          true,
		AutoDeductOnOrderApproval: true,
		LowStockWarningsEnabled:   true,
		AvailabilitySyncMode:      InventoryAvailabilityModeHardBlock,
	}).Error)

	patty := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Patty",
		Unit:            "pcs",
		CurrentQuantity: 1,
		IsActive:        true,
	}
	require.NoError(t, db.Create(patty).Error)
	require.NoError(t, db.Create(&InventoryRecipe{
		BusinessID:       business.ID,
		MenuItemID:       "burger-id",
		MenuItemName:     "Burger",
		InventoryItemID:  patty.ID,
		QuantityRequired: 1,
	}).Error)

	bill := helperBill(t, business, nil, 0)
	order := helperOrder(t, business, bill, []OrderItem{
		{
			ID:           "burger-line-1",
			MenuItemID:   "burger-id",
			MenuItemName: "Burger",
			Quantity:     2,
			Price:        12,
			Subtotal:     24,
		},
	})

	err := UpdateOrderStatus(order.ID, OrderStatusApproved, "manager@example.com", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inventory hard block prevents approving order")
	assert.Contains(t, err.Error(), "Patty")

	var refreshedPatty InventoryItem
	require.NoError(t, db.First(&refreshedPatty, patty.ID).Error)
	assert.Equal(t, 1.0, refreshedPatty.CurrentQuantity)

	var refreshedOrder Order
	require.NoError(t, db.First(&refreshedOrder, order.ID).Error)
	assert.Equal(t, OrderStatusPending, refreshedOrder.Status)

	var movements []InventoryMovement
	require.NoError(t, db.Where("reference_order_id = ?", order.ID).Find(&movements).Error)
	assert.Len(t, movements, 0)
}

// deductApprovedOrderCommitting runs the deduction and commits even when it
// returns an error, so a check that should have run first cannot hide a
// partial write behind the caller's rollback.
func deductApprovedOrderCommitting(t *testing.T, order *Order, items []OrderItem) error {
	t.Helper()
	tx := db.Begin()
	require.NoError(t, tx.Error)
	err := DeductApprovedOrderInventoryTx(tx, order, items, "manager@example.com")
	require.NoError(t, tx.Commit().Error)
	return err
}

func TestDeductApprovedOrderInventoryTx_HardBlockInactiveIngredientDeductsNothing(t *testing.T) {
	setupOrderTestDB(t)

	business := helperBusiness(t, 0, 0)
	require.NoError(t, db.Create(&InventorySettings{
		BusinessID:                business.ID,
		InventoryEnabled:          true,
		AutoDeductOnOrderApproval: true,
		AvailabilitySyncMode:      InventoryAvailabilityModeHardBlock,
	}).Error)

	cheese := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Cheese",
		Unit:            "pcs",
		CurrentQuantity: 8,
		IsActive:        true,
	}
	patty := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Patty",
		Unit:            "pcs",
		CurrentQuantity: 5,
		IsActive:        true,
	}
	require.NoError(t, db.Create(cheese).Error)
	require.NoError(t, db.Create(patty).Error)
	require.NoError(t, db.Model(patty).UpdateColumn("is_active", false).Error)
	require.NoError(t, db.Create([]InventoryRecipe{
		{
			BusinessID:       business.ID,
			MenuItemID:       "burger-id",
			MenuItemName:     "Burger",
			InventoryItemID:  cheese.ID,
			QuantityRequired: 1,
		},
		{
			BusinessID:       business.ID,
			MenuItemID:       "burger-id",
			MenuItemName:     "Burger",
			InventoryItemID:  patty.ID,
			QuantityRequired: 1,
		},
	}).Error)

	items := []OrderItem{{
		ID:           "burger-line-1",
		MenuItemID:   "burger-id",
		MenuItemName: "Burger",
		Quantity:     2,
		Price:        12,
		Subtotal:     24,
	}}
	bill := helperBill(t, business, nil, 0)
	order := helperOrder(t, business, bill, items)

	err := deductApprovedOrderCommitting(t, order, items)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inventory hard block prevents approving order: a required ingredient for Burger is unavailable")

	var refreshedCheese InventoryItem
	var refreshedPatty InventoryItem
	require.NoError(t, db.First(&refreshedCheese, cheese.ID).Error)
	require.NoError(t, db.First(&refreshedPatty, patty.ID).Error)
	assert.Equal(t, 8.0, refreshedCheese.CurrentQuantity)
	assert.Equal(t, 5.0, refreshedPatty.CurrentQuantity)

	var movements []InventoryMovement
	require.NoError(t, db.Find(&movements).Error)
	assert.Len(t, movements, 0)
}

func TestDeductApprovedOrderInventoryTx_HardBlockMissingIngredientDeductsNothing(t *testing.T) {
	setupOrderTestDB(t)

	business := helperBusiness(t, 0, 0)
	require.NoError(t, db.Create(&InventorySettings{
		BusinessID:                business.ID,
		InventoryEnabled:          true,
		AutoDeductOnOrderApproval: true,
		AvailabilitySyncMode:      InventoryAvailabilityModeHardBlock,
	}).Error)

	bun := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Bun",
		Unit:            "pcs",
		CurrentQuantity: 6,
		IsActive:        true,
	}
	ghost := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Ghost Spice",
		Unit:            "g",
		CurrentQuantity: 4,
		IsActive:        true,
	}
	require.NoError(t, db.Create(bun).Error)
	require.NoError(t, db.Create(ghost).Error)
	require.NoError(t, db.Create([]InventoryRecipe{
		{
			BusinessID:       business.ID,
			MenuItemID:       "burger-id",
			MenuItemName:     "Burger",
			InventoryItemID:  bun.ID,
			QuantityRequired: 1,
		},
		{
			BusinessID:       business.ID,
			MenuItemID:       "burger-id",
			MenuItemName:     "Burger",
			InventoryItemID:  ghost.ID,
			QuantityRequired: 1,
		},
	}).Error)
	require.NoError(t, db.Delete(ghost).Error)

	items := []OrderItem{{
		ID:           "burger-line-1",
		MenuItemID:   "burger-id",
		MenuItemName: "Burger",
		Quantity:     1,
		Price:        12,
		Subtotal:     12,
	}}
	bill := helperBill(t, business, nil, 0)
	order := helperOrder(t, business, bill, items)

	err := deductApprovedOrderCommitting(t, order, items)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inventory hard block prevents approving order: a required ingredient for Burger is unavailable")

	var refreshedBun InventoryItem
	require.NoError(t, db.First(&refreshedBun, bun.ID).Error)
	assert.Equal(t, 6.0, refreshedBun.CurrentQuantity)

	var movements []InventoryMovement
	require.NoError(t, db.Find(&movements).Error)
	assert.Len(t, movements, 0)
}

func TestDeductApprovedOrderInventoryTx_WarnSkipsInactiveIngredient(t *testing.T) {
	setupOrderTestDB(t)

	business := helperBusiness(t, 0, 0)
	require.NoError(t, db.Create(&InventorySettings{
		BusinessID:                business.ID,
		InventoryEnabled:          true,
		AutoDeductOnOrderApproval: true,
		AvailabilitySyncMode:      InventoryAvailabilityModeWarn,
	}).Error)

	bun := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Bun",
		Unit:            "pcs",
		CurrentQuantity: 10,
		IsActive:        true,
	}
	patty := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Patty",
		Unit:            "pcs",
		CurrentQuantity: 5,
		IsActive:        true,
	}
	require.NoError(t, db.Create(bun).Error)
	require.NoError(t, db.Create(patty).Error)
	require.NoError(t, db.Model(patty).UpdateColumn("is_active", false).Error)
	require.NoError(t, db.Create([]InventoryRecipe{
		{
			BusinessID:       business.ID,
			MenuItemID:       "burger-id",
			MenuItemName:     "Burger",
			InventoryItemID:  bun.ID,
			QuantityRequired: 1,
		},
		{
			BusinessID:       business.ID,
			MenuItemID:       "burger-id",
			MenuItemName:     "Burger",
			InventoryItemID:  patty.ID,
			QuantityRequired: 1,
		},
	}).Error)

	items := []OrderItem{{
		ID:           "burger-line-1",
		MenuItemID:   "burger-id",
		MenuItemName: "Burger",
		Quantity:     2,
		Price:        12,
		Subtotal:     24,
	}}
	bill := helperBill(t, business, nil, 0)
	order := helperOrder(t, business, bill, items)

	err := deductApprovedOrderCommitting(t, order, items)
	require.NoError(t, err)

	var refreshedBun InventoryItem
	var refreshedPatty InventoryItem
	require.NoError(t, db.First(&refreshedBun, bun.ID).Error)
	require.NoError(t, db.First(&refreshedPatty, patty.ID).Error)
	assert.Equal(t, 8.0, refreshedBun.CurrentQuantity)
	assert.Equal(t, 5.0, refreshedPatty.CurrentQuantity)

	var movements []InventoryMovement
	require.NoError(t, db.Where("reference_order_id = ?", order.ID).Find(&movements).Error)
	require.Len(t, movements, 1)
	assert.Equal(t, bun.ID, movements[0].InventoryItemID)
	assert.Equal(t, -2.0, movements[0].QuantityDelta)
}

func TestDeductApprovedOrderInventoryTx_HardBlockCombinedDemandDeductsNothing(t *testing.T) {
	setupOrderTestDB(t)

	business := helperBusiness(t, 0, 0)
	require.NoError(t, db.Create(&InventorySettings{
		BusinessID:                business.ID,
		InventoryEnabled:          true,
		AutoDeductOnOrderApproval: true,
		AvailabilitySyncMode:      InventoryAvailabilityModeHardBlock,
	}).Error)

	flour := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Flour",
		Unit:            "kg",
		CurrentQuantity: 5,
		IsActive:        true,
	}
	basil := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Basil",
		Unit:            "g",
		CurrentQuantity: 10,
		IsActive:        true,
	}
	require.NoError(t, db.Create(flour).Error)
	require.NoError(t, db.Create(basil).Error)
	require.NoError(t, db.Create([]InventoryRecipe{
		{
			BusinessID:       business.ID,
			MenuItemID:       "pasta-id",
			MenuItemName:     "Pasta",
			InventoryItemID:  flour.ID,
			QuantityRequired: 3,
		},
		{
			BusinessID:       business.ID,
			MenuItemID:       "pasta-id",
			MenuItemName:     "Pasta",
			InventoryItemID:  basil.ID,
			QuantityRequired: 1,
		},
		{
			BusinessID:       business.ID,
			MenuItemID:       "pizza-id",
			MenuItemName:     "Pizza",
			InventoryItemID:  flour.ID,
			QuantityRequired: 3,
		},
	}).Error)

	items := []OrderItem{
		{
			ID:           "pasta-line",
			MenuItemID:   "pasta-id",
			MenuItemName: "Pasta",
			Quantity:     1,
			Price:        14,
			Subtotal:     14,
		},
		{
			ID:           "pizza-line",
			MenuItemID:   "pizza-id",
			MenuItemName: "Pizza",
			Quantity:     1,
			Price:        16,
			Subtotal:     16,
		},
	}
	bill := helperBill(t, business, nil, 0)
	order := helperOrder(t, business, bill, items)

	err := deductApprovedOrderCommitting(t, order, items)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inventory hard block prevents approving order")
	assert.Contains(t, err.Error(), "Flour")

	var refreshedFlour InventoryItem
	var refreshedBasil InventoryItem
	require.NoError(t, db.First(&refreshedFlour, flour.ID).Error)
	require.NoError(t, db.First(&refreshedBasil, basil.ID).Error)
	assert.Equal(t, 5.0, refreshedFlour.CurrentQuantity)
	assert.Equal(t, 10.0, refreshedBasil.CurrentQuantity)

	var movements []InventoryMovement
	require.NoError(t, db.Find(&movements).Error)
	assert.Len(t, movements, 0)
}

func TestCreateInventoryItem_RejectsNegativeCurrentQuantity(t *testing.T) {
	setupOrderTestDB(t)

	business := helperBusiness(t, 0, 0)
	err := CreateInventoryItem(&InventoryItem{
		BusinessID:      business.ID,
		Name:            "Flour",
		Unit:            "kg",
		CurrentQuantity: -1,
		IsActive:        true,
	}, "manager@example.com")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "current quantity cannot be negative")
}

func TestCreateInventoryAdjustment_RejectsAdjustmentsBelowZero(t *testing.T) {
	setupOrderTestDB(t)

	business := helperBusiness(t, 0, 0)
	item := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Tomato",
		Unit:            "kg",
		CurrentQuantity: 1,
		IsActive:        true,
	}
	require.NoError(t, db.Create(item).Error)

	_, _, err := CreateInventoryAdjustment(
		business.ID,
		item.ID,
		InventoryMovementTypeManualAdjustment,
		-2,
		"Shrinkage",
		"manager@example.com",
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "below zero")

	var refreshed InventoryItem
	require.NoError(t, db.First(&refreshed, item.ID).Error)
	assert.Equal(t, 1.0, refreshed.CurrentQuantity)
}

func TestGetInventorySummary_ComputesItemAndMenuWarnings(t *testing.T) {
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
		BusinessID:       business.ID,
		Name:             "Flour",
		Unit:             "kg",
		CurrentQuantity:  0,
		ReorderThreshold: 5,
		IsActive:         true,
	}
	cheese := &InventoryItem{
		BusinessID:       business.ID,
		Name:             "Cheese",
		Unit:             "kg",
		CurrentQuantity:  4,
		ReorderThreshold: 10,
		IsActive:         true,
	}
	tomato := &InventoryItem{
		BusinessID:       business.ID,
		Name:             "Tomato",
		Unit:             "kg",
		CurrentQuantity:  20,
		ReorderThreshold: 5,
		IsActive:         true,
	}
	require.NoError(t, db.Create(flour).Error)
	require.NoError(t, db.Create(cheese).Error)
	require.NoError(t, db.Create(tomato).Error)

	helperMenu(t, business, []MenuCategory{
		{
			ID:   "cat-1",
			Name: "Main",
			Items: []MenuItem{
				{ID: "burger-id", Name: "Burger", IsAvailable: true},
				{ID: "pizza-id", Name: "Pizza", IsAvailable: true},
				{ID: "soup-id", Name: "Soup", IsAvailable: false},
				{ID: "fries-id", Name: "Fries", IsAvailable: true},
			},
		},
	})

	require.NoError(t, db.Create([]InventoryRecipe{
		{
			BusinessID:       business.ID,
			MenuItemID:       "burger-id",
			MenuItemName:     "Burger",
			InventoryItemID:  flour.ID,
			QuantityRequired: 1,
		},
		{
			BusinessID:       business.ID,
			MenuItemID:       "pizza-id",
			MenuItemName:     "Pizza",
			InventoryItemID:  cheese.ID,
			QuantityRequired: 1,
		},
		{
			BusinessID:       business.ID,
			MenuItemID:       "soup-id",
			MenuItemName:     "Soup",
			InventoryItemID:  tomato.ID,
			QuantityRequired: 2,
		},
	}).Error)

	summary, err := GetInventorySummary(business.ID)
	require.NoError(t, err)

	assert.Equal(t, 3, summary.TotalItems)
	assert.Equal(t, 1, summary.LowStockItems)
	assert.Equal(t, 1, summary.OutOfStockItems)
	assert.Equal(t, 3, summary.TotalRecipes)
	assert.Equal(t, 3, summary.MenuItemsTracked)
	assert.Equal(t, 1, summary.MenuItemsLowStock)
	assert.Equal(t, 1, summary.MenuItemsOutOfStock)

	statusByID := make(map[string]InventoryMenuItemStatus, len(summary.MenuItemStatuses))
	for _, status := range summary.MenuItemStatuses {
		statusByID[status.MenuItemID] = status
	}

	assert.Equal(t, "out_of_stock", statusByID["burger-id"].Status)
	assert.Equal(t, "low_stock", statusByID["pizza-id"].Status)
	assert.Equal(t, "manual_unavailable", statusByID["soup-id"].Status)
	assert.Equal(t, "untracked", statusByID["fries-id"].Status)
	assert.False(t, statusByID["burger-id"].RecommendedAvailable)
	assert.True(t, statusByID["pizza-id"].RecommendedAvailable)
	assert.True(t, statusByID["burger-id"].ShowsWarning)
	assert.True(t, statusByID["burger-id"].BlocksSale, "warn-mode zero stock must block sale")
	assert.True(t, statusByID["pizza-id"].ShowsWarning)
	assert.False(t, statusByID["pizza-id"].BlocksSale)
}

// TestGetInventorySummary_ComputesTotalStockValue asserts the summary returns
// the total on-hand value (sum of max(0, qty) * cost_per_unit), so the FE stat
// card reads one number instead of recomputing over the item list (fix 9).
func TestGetInventorySummary_ComputesTotalStockValue(t *testing.T) {
	setupOrderTestDB(t)

	business := helperBusiness(t, 0, 0)
	require.NoError(t, db.Create(&InventorySettings{
		BusinessID:       business.ID,
		InventoryEnabled: true,
	}).Error)

	// value = 10*2.50 + 4*1.25 = 25 + 5 = 30. The negative-qty item contributes 0.
	require.NoError(t, db.Create(&InventoryItem{
		BusinessID: business.ID, Name: "Oil", Unit: "L",
		CurrentQuantity: 10, CostPerUnit: 2.5, IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&InventoryItem{
		BusinessID: business.ID, Name: "Salt", Unit: "kg",
		CurrentQuantity: 4, CostPerUnit: 1.25, IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&InventoryItem{
		BusinessID: business.ID, Name: "Oversold", Unit: "kg",
		CurrentQuantity: -3, CostPerUnit: 5, IsActive: true,
	}).Error)

	summary, err := GetInventorySummary(business.ID)
	require.NoError(t, err)
	assert.InDelta(t, 30.0, summary.TotalStockValue, 0.001,
		"total_stock_value must sum max(0, qty)*cost_per_unit across items")
}

func TestGetInventorySummary_TotalStockValueRoundsToCents(t *testing.T) {
	// FIND-043: classic residue 36*4.2 + 29.92*2.1 → 687.032 without rounding.
	setupOrderTestDB(t)
	business := helperBusiness(t, 0, 0)
	require.NoError(t, db.Create(&InventorySettings{
		BusinessID: business.ID, InventoryEnabled: true,
	}).Error)
	require.NoError(t, db.Create(&InventoryItem{
		BusinessID: business.ID, Name: "Greens", Unit: "kg",
		CurrentQuantity: 36, CostPerUnit: 4.2, IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&InventoryItem{
		BusinessID: business.ID, Name: "Beef", Unit: "kg",
		CurrentQuantity: 22, CostPerUnit: 21.5, IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&InventoryItem{
		BusinessID: business.ID, Name: "Tea", Unit: "L",
		CurrentQuantity: 29.92, CostPerUnit: 2.1, IsActive: true,
	}).Error)

	summary, err := GetInventorySummary(business.ID)
	require.NoError(t, err)
	assert.Equal(t, 687.03, summary.TotalStockValue)
}

func TestUpdateOrderStatus_CancelAfterApprovalRestoresInventory(t *testing.T) {
	setupOrderTestDB(t)

	business := helperBusiness(t, 0, 0)
	require.NoError(t, db.Create(&InventorySettings{
		BusinessID:                business.ID,
		InventoryEnabled:          true,
		AutoDeductOnOrderApproval: true,
		LowStockWarningsEnabled:   true,
		AvailabilitySyncMode:      InventoryAvailabilityModeWarn,
	}).Error)

	patty := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Patty",
		Unit:            "pcs",
		CurrentQuantity: 5,
		IsActive:        true,
	}
	require.NoError(t, db.Create(patty).Error)
	require.NoError(t, db.Create(&InventoryRecipe{
		BusinessID:       business.ID,
		MenuItemID:       "burger-id",
		MenuItemName:     "Burger",
		InventoryItemID:  patty.ID,
		QuantityRequired: 1,
	}).Error)

	bill := helperBill(t, business, nil, 0)
	order := helperOrder(t, business, bill, []OrderItem{
		{
			ID:           "burger-line-1",
			MenuItemID:   "burger-id",
			MenuItemName: "Burger",
			Quantity:     2,
			Price:        12,
			Subtotal:     24,
		},
	})

	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "manager@example.com", ""))
	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusOrderCancelled, "manager@example.com", "Staff mistake"))

	var refreshedPatty InventoryItem
	require.NoError(t, db.First(&refreshedPatty, patty.ID).Error)
	assert.Equal(t, 5.0, refreshedPatty.CurrentQuantity)

	var movements []InventoryMovement
	require.NoError(t, db.Where("reference_order_id = ?", order.ID).Order("id ASC").Find(&movements).Error)
	require.Len(t, movements, 2)
	assert.Equal(t, InventoryMovementTypeOrderConsumption, movements[0].MovementType)
	assert.Equal(t, InventoryMovementTypeOrderRestoration, movements[1].MovementType)
	assert.Equal(t, 2.0, movements[1].QuantityDelta)

	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusOrderCancelled, "manager@example.com", "Duplicate cancel"))
	movements = nil
	require.NoError(t, db.Where("reference_order_id = ?", order.ID).Order("id ASC").Find(&movements).Error)
	assert.Len(t, movements, 2)
}

func TestGetInventorySummary_RespectsWarningAndSyncFlags(t *testing.T) {
	setupOrderTestDB(t)

	business := helperBusiness(t, 0, 0)
	require.NoError(t, db.Create(&InventorySettings{
		BusinessID:                business.ID,
		InventoryEnabled:          true,
		AutoDeductOnOrderApproval: true,
		AvailabilitySyncMode:      InventoryAvailabilityModeManual,
	}).Error)
	require.NoError(t, db.Model(&InventorySettings{}).
		Where("business_id = ?", business.ID).
		Update("low_stock_warnings_enabled", false).Error)

	flour := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Flour",
		Unit:            "kg",
		CurrentQuantity: 0,
		IsActive:        true,
	}
	cheese := &InventoryItem{
		BusinessID:       business.ID,
		Name:             "Cheese",
		Unit:             "kg",
		CurrentQuantity:  4,
		ReorderThreshold: 10,
		IsActive:         true,
	}
	require.NoError(t, db.Create(flour).Error)
	require.NoError(t, db.Create(cheese).Error)

	helperMenu(t, business, []MenuCategory{
		{
			ID:   "cat-1",
			Name: "Main",
			Items: []MenuItem{
				{ID: "burger-id", Name: "Burger", IsAvailable: true},
				{ID: "pizza-id", Name: "Pizza", IsAvailable: true},
			},
		},
	})

	require.NoError(t, db.Create([]InventoryRecipe{
		{
			BusinessID:       business.ID,
			MenuItemID:       "burger-id",
			MenuItemName:     "Burger",
			InventoryItemID:  flour.ID,
			QuantityRequired: 1,
		},
		{
			BusinessID:       business.ID,
			MenuItemID:       "pizza-id",
			MenuItemName:     "Pizza",
			InventoryItemID:  cheese.ID,
			QuantityRequired: 1,
		},
	}).Error)

	summary, err := GetInventorySummary(business.ID)
	require.NoError(t, err)

	statusByID := make(map[string]InventoryMenuItemStatus, len(summary.MenuItemStatuses))
	for _, status := range summary.MenuItemStatuses {
		statusByID[status.MenuItemID] = status
	}

	assert.Equal(t, "out_of_stock", statusByID["burger-id"].Status)
	assert.True(t, statusByID["burger-id"].RecommendedAvailable)
	assert.False(t, statusByID["burger-id"].ShowsWarning)
	assert.False(t, statusByID["burger-id"].BlocksSale)
	assert.Equal(t, "low_stock", statusByID["pizza-id"].Status)
	assert.False(t, statusByID["pizza-id"].ShowsWarning)
	assert.False(t, statusByID["pizza-id"].BlocksSale)

	require.NoError(t, db.Model(&InventorySettings{}).
		Where("business_id = ?", business.ID).
		Updates(map[string]interface{}{
			"availability_sync_mode":     InventoryAvailabilityModeHardBlock,
			"low_stock_warnings_enabled": true,
		}).Error)

	summary, err = GetInventorySummary(business.ID)
	require.NoError(t, err)
	statusByID = make(map[string]InventoryMenuItemStatus, len(summary.MenuItemStatuses))
	for _, status := range summary.MenuItemStatuses {
		statusByID[status.MenuItemID] = status
	}

	assert.False(t, statusByID["burger-id"].RecommendedAvailable)
	assert.True(t, statusByID["burger-id"].ShowsWarning)
	assert.True(t, statusByID["burger-id"].BlocksSale)
	assert.True(t, statusByID["pizza-id"].ShowsWarning)
	assert.False(t, statusByID["pizza-id"].BlocksSale)
}

func TestGetInventorySummary_DisabledInventoryDoesNotEmitStockWarningsOrBlocks(t *testing.T) {
	setupOrderTestDB(t)

	business := helperBusiness(t, 0, 0)
	require.NoError(t, db.Create(&InventorySettings{
		BusinessID:                business.ID,
		InventoryEnabled:          false,
		AutoDeductOnOrderApproval: true,
		LowStockWarningsEnabled:   true,
		AvailabilitySyncMode:      InventoryAvailabilityModeHardBlock,
	}).Error)

	flour := &InventoryItem{
		BusinessID:       business.ID,
		Name:             "Flour",
		Unit:             "kg",
		CurrentQuantity:  0,
		ReorderThreshold: 5,
		IsActive:         true,
	}
	require.NoError(t, db.Create(flour).Error)

	helperMenu(t, business, []MenuCategory{
		{
			ID:   "cat-1",
			Name: "Main",
			Items: []MenuItem{
				{ID: "burger-id", Name: "Burger", IsAvailable: true},
				{ID: "soup-id", Name: "Soup", IsAvailable: false},
			},
		},
	})

	require.NoError(t, db.Create(&InventoryRecipe{
		BusinessID:       business.ID,
		MenuItemID:       "burger-id",
		MenuItemName:     "Burger",
		InventoryItemID:  flour.ID,
		QuantityRequired: 1,
	}).Error)

	summary, err := GetInventorySummary(business.ID)
	require.NoError(t, err)

	assert.Equal(t, 1, summary.TotalItems)
	assert.Equal(t, 1, summary.TotalRecipes)
	assert.Equal(t, 0, summary.LowStockItems)
	assert.Equal(t, 0, summary.OutOfStockItems)
	assert.Equal(t, 0, summary.MenuItemsTracked)
	assert.Equal(t, 0, summary.MenuItemsLowStock)
	assert.Equal(t, 0, summary.MenuItemsOutOfStock)
	assert.Empty(t, summary.LowStockDetails)
	assert.Empty(t, summary.OutOfStockDetails)

	statusByID := make(map[string]InventoryMenuItemStatus, len(summary.MenuItemStatuses))
	for _, status := range summary.MenuItemStatuses {
		statusByID[status.MenuItemID] = status
	}

	assert.Equal(t, "untracked", statusByID["burger-id"].Status)
	assert.True(t, statusByID["burger-id"].RecommendedAvailable)
	assert.False(t, statusByID["burger-id"].ShowsWarning)
	assert.False(t, statusByID["burger-id"].BlocksSale)
	assert.Equal(t, "manual_unavailable", statusByID["soup-id"].Status)
	assert.False(t, statusByID["soup-id"].RecommendedAvailable)
	assert.False(t, statusByID["soup-id"].ShowsWarning)
	assert.True(t, statusByID["soup-id"].BlocksSale)
}

func TestUpdateInventoryItem_AuditsQuantityChangesAndDeactivation(t *testing.T) {
	setupOrderTestDB(t)

	business := helperBusiness(t, 0, 0)
	item := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Milk",
		Unit:            "l",
		CurrentQuantity: 5,
		IsActive:        true,
	}
	require.NoError(t, CreateInventoryItem(item, "owner@example.com"))

	require.NoError(t, db.Create(&InventoryRecipe{
		BusinessID:       business.ID,
		MenuItemID:       "coffee-id",
		MenuItemName:     "Coffee",
		InventoryItemID:  item.ID,
		QuantityRequired: 1,
	}).Error)

	previous := *item
	item.CurrentQuantity = 9
	item.IsActive = false
	require.NoError(t, UpdateInventoryItem(item, previous, "owner@example.com"))

	var refreshed InventoryItem
	require.NoError(t, db.First(&refreshed, item.ID).Error)
	assert.Equal(t, 9.0, refreshed.CurrentQuantity)
	assert.False(t, refreshed.IsActive)

	var recipes []InventoryRecipe
	require.NoError(t, db.Where("inventory_item_id = ?", item.ID).Find(&recipes).Error)
	assert.Len(t, recipes, 0)

	var movements []InventoryMovement
	require.NoError(t, db.Where("inventory_item_id = ?", item.ID).Order("id ASC").Find(&movements).Error)
	require.Len(t, movements, 2)
	assert.Equal(t, InventoryMovementTypeCorrection, movements[0].MovementType)
	assert.Equal(t, 5.0, movements[0].QuantityDelta)
	assert.Equal(t, InventoryMovementTypeCorrection, movements[1].MovementType)
	assert.Equal(t, 4.0, movements[1].QuantityDelta)
}

// #727 B2: on an id/name conflict the stored id wins — it is the strong key
// (multi-ingredient dishes share it across rows) and it is what the guest
// orderability JOIN keys on, so operator and guest cannot diverge. The name
// only decides when the id no longer matches any live dish.
func TestResolveInventoryRecipeMenuItem_IdWinsOnConflict(t *testing.T) {
	byID := map[string]inventoryMenuLookup{
		"demo-bowl":  {ID: "demo-bowl", Name: "Harvest Bowl"},
		"demo-steak": {ID: "demo-steak", Name: "Steak Plate"},
	}
	byName := map[string]inventoryMenuLookup{
		"harvest bowl": {ID: "demo-bowl", Name: "Harvest Bowl"},
		"steak plate":  {ID: "demo-steak", Name: "Steak Plate"},
	}

	// Conflicted pairing: id points at the bowl, name drifted to the steak.
	// The row stays the bowl's (it may be the bowl's second ingredient).
	got := resolveInventoryRecipeMenuItem(InventoryRecipe{
		MenuItemID:   "demo-bowl",
		MenuItemName: "Steak Plate",
	}, byID, byName)
	assert.Equal(t, "demo-bowl", got.ID)
	assert.Equal(t, "Harvest Bowl", got.Name)

	// #727 shape B: correct id, name drifted onto the bowl — stays the steak.
	got = resolveInventoryRecipeMenuItem(InventoryRecipe{
		MenuItemID:   "demo-steak",
		MenuItemName: "Harvest Bowl",
	}, byID, byName)
	assert.Equal(t, "demo-steak", got.ID)
	assert.Equal(t, "Steak Plate", got.Name)

	// Healthy pairing keeps the id.
	got = resolveInventoryRecipeMenuItem(InventoryRecipe{
		MenuItemID:   "demo-steak",
		MenuItemName: "Steak Plate",
	}, byID, byName)
	assert.Equal(t, "demo-steak", got.ID)

	// Id-only recipe still resolves when the name is blank.
	got = resolveInventoryRecipeMenuItem(InventoryRecipe{
		MenuItemID: "demo-bowl",
	}, byID, byName)
	assert.Equal(t, "demo-bowl", got.ID)

	// Dead id: the name is the only live referent and wins.
	got = resolveInventoryRecipeMenuItem(InventoryRecipe{
		MenuItemID: "gone-id", MenuItemName: "Steak Plate",
	}, byID, byName)
	assert.Equal(t, "demo-steak", got.ID)

	// Conflict detection feeds reconcile's no-heal rule.
	assert.True(t, inventoryRecipeMenuItemConflicted(InventoryRecipe{
		MenuItemID: "demo-bowl", MenuItemName: "Steak Plate",
	}, byID, byName))
	assert.False(t, inventoryRecipeMenuItemConflicted(InventoryRecipe{
		MenuItemID: "demo-steak", MenuItemName: "Steak Plate",
	}, byID, byName))
	assert.False(t, inventoryRecipeMenuItemConflicted(InventoryRecipe{
		MenuItemID: "gone-id", MenuItemName: "Steak Plate",
	}, byID, byName))
}

func TestGetInventorySummary_ZeroBeefEightySixesSteakNotHarvestBowl(t *testing.T) {
	setupOrderTestDB(t)

	business := helperBusiness(t, 0, 0)
	require.NoError(t, db.Create(&InventorySettings{
		BusinessID:           business.ID,
		InventoryEnabled:     true,
		AvailabilitySyncMode: InventoryAvailabilityModeWarn,
	}).Error)

	greens := &InventoryItem{
		BusinessID: business.ID, Name: "Mixed Greens", Unit: "kg",
		CurrentQuantity: 48, IsActive: true,
	}
	beef := &InventoryItem{
		BusinessID: business.ID, Name: "Premium Beef", Unit: "kg",
		CurrentQuantity: 0, IsActive: true,
	}
	require.NoError(t, db.Create(greens).Error)
	require.NoError(t, db.Create(beef).Error)

	helperMenu(t, business, []MenuCategory{
		{
			ID:   "mains",
			Name: "Mains",
			Items: []MenuItem{
				{ID: "demo-bowl", Name: "Harvest Bowl", DietaryTags: []string{"vegetarian"}, IsAvailable: true},
				{ID: "demo-steak", Name: "Steak Plate", IsAvailable: true},
			},
		},
	})

	require.NoError(t, db.Create([]InventoryRecipe{
		{
			BusinessID: business.ID, MenuItemID: "demo-bowl", MenuItemName: "Harvest Bowl",
			InventoryItemID: greens.ID, QuantityRequired: 0.25,
		},
		// #727 shape B (the QA-verified prod shape): the beef row keeps the
		// correct steak id but its NAME drifted onto the bowl. Zero beef must
		// 86 Steak Plate, not the vegetarian bowl.
		{
			BusinessID: business.ID, MenuItemID: "demo-steak", MenuItemName: "Harvest Bowl",
			InventoryItemID: beef.ID, QuantityRequired: 0.35,
		},
	}).Error)

	summary, err := GetInventorySummary(business.ID)
	require.NoError(t, err)

	statusByID := make(map[string]InventoryMenuItemStatus, len(summary.MenuItemStatuses))
	for _, status := range summary.MenuItemStatuses {
		statusByID[status.MenuItemID] = status
	}

	require.Equal(t, "out_of_stock", statusByID["demo-steak"].Status)
	assert.Equal(t, "Steak Plate", statusByID["demo-steak"].MenuItemName)
	assert.True(t, statusByID["demo-steak"].BlocksSale)
	assert.Contains(t, statusByID["demo-steak"].AffectedInventory, "Premium Beef")

	assert.NotEqual(t, "out_of_stock", statusByID["demo-bowl"].Status, "vegetarian Harvest Bowl must not be 86'd by beef")
	assert.False(t, statusByID["demo-bowl"].BlocksSale)
	assert.NotContains(t, statusByID["demo-bowl"].AffectedInventory, "Premium Beef")

	// #727 B2: the conflicted row is resolved id-wins at read time and must
	// NOT be rewritten in the database.
	var beefRecipe InventoryRecipe
	require.NoError(t, db.Where("business_id = ? AND inventory_item_id = ?", business.ID, beef.ID).
		First(&beefRecipe).Error)
	assert.Equal(t, "demo-steak", beefRecipe.MenuItemID)
	assert.Equal(t, "Harvest Bowl", beefRecipe.MenuItemName, "summary must not heal a conflicted row")
}

// Issue 945: zero beef must 86 every recipe-linked beef plate, not only demo-bife.
// Ensalada / sorrentinos keep their own stocked recipes and stay sellable.
func TestGetInventorySummary_ZeroBeefEightySixesAllBeefDishesNotEnsalada(t *testing.T) {
	setupOrderTestDB(t)

	business := helperBusiness(t, 0, 0)
	require.NoError(t, db.Create(&InventorySettings{
		BusinessID:           business.ID,
		InventoryEnabled:     true,
		AvailabilitySyncMode: InventoryAvailabilityModeWarn,
	}).Error)

	greens := &InventoryItem{
		BusinessID: business.ID, Name: "Verdura de estación", Unit: "kg",
		CurrentQuantity: 48, IsActive: true,
	}
	pasta := &InventoryItem{
		BusinessID: business.ID, Name: "Masa para sorrentinos", Unit: "kg",
		CurrentQuantity: 12, IsActive: true,
	}
	beef := &InventoryItem{
		BusinessID: business.ID, Name: "Bife de chorizo (media res)", Unit: "kg",
		CurrentQuantity: 0, IsActive: true,
	}
	require.NoError(t, db.Create(greens).Error)
	require.NoError(t, db.Create(pasta).Error)
	require.NoError(t, db.Create(beef).Error)

	helperMenu(t, business, []MenuCategory{
		{
			ID:   "parrilla",
			Name: "Parrilla",
			Items: []MenuItem{
				{ID: "demo-bife", Name: "Bife de chorizo", IsAvailable: true},
				{ID: "demo-ojo-de-bife", Name: "Ojo de bife", IsAvailable: true},
				{ID: "demo-asado-tira", Name: "Asado de tira", IsAvailable: true},
				{ID: "demo-parrillada", Name: "Parrillada para dos", IsAvailable: true},
			},
		},
		{
			ID:   "otros",
			Name: "Otros",
			Items: []MenuItem{
				{ID: "demo-ensalada", Name: "Ensalada mixta", DietaryTags: []string{"vegan"}, IsAvailable: true},
				{ID: "demo-sorrentinos", Name: "Sorrentinos caseros", IsAvailable: true},
			},
		},
	})

	require.NoError(t, db.Create([]InventoryRecipe{
		{BusinessID: business.ID, MenuItemID: "demo-bife", MenuItemName: "Bife de chorizo", InventoryItemID: beef.ID, QuantityRequired: 0.40},
		{BusinessID: business.ID, MenuItemID: "demo-ojo-de-bife", MenuItemName: "Ojo de bife", InventoryItemID: beef.ID, QuantityRequired: 0.45},
		{BusinessID: business.ID, MenuItemID: "demo-asado-tira", MenuItemName: "Asado de tira", InventoryItemID: beef.ID, QuantityRequired: 0.35},
		{BusinessID: business.ID, MenuItemID: "demo-parrillada", MenuItemName: "Parrillada para dos", InventoryItemID: beef.ID, QuantityRequired: 0.80},
		{BusinessID: business.ID, MenuItemID: "demo-ensalada", MenuItemName: "Ensalada mixta", InventoryItemID: greens.ID, QuantityRequired: 0.20},
		{BusinessID: business.ID, MenuItemID: "demo-sorrentinos", MenuItemName: "Sorrentinos caseros", InventoryItemID: pasta.ID, QuantityRequired: 0.25},
	}).Error)

	summary, err := GetInventorySummary(business.ID)
	require.NoError(t, err)

	statusByID := make(map[string]InventoryMenuItemStatus, len(summary.MenuItemStatuses))
	for _, status := range summary.MenuItemStatuses {
		statusByID[status.MenuItemID] = status
	}

	for _, id := range []string{"demo-bife", "demo-ojo-de-bife", "demo-asado-tira", "demo-parrillada"} {
		got, ok := statusByID[id]
		require.True(t, ok, "summary must include %s", id)
		assert.Equal(t, "out_of_stock", got.Status, id)
		assert.True(t, got.BlocksSale, id)
		assert.True(t, got.HasRecipe, id)
		assert.Contains(t, got.AffectedInventory, "Bife de chorizo (media res)", id)
	}

	assert.NotEqual(t, "out_of_stock", statusByID["demo-ensalada"].Status)
	assert.False(t, statusByID["demo-ensalada"].BlocksSale)
	assert.NotContains(t, statusByID["demo-ensalada"].AffectedInventory, "Bife de chorizo (media res)")

	assert.NotEqual(t, "out_of_stock", statusByID["demo-sorrentinos"].Status)
	assert.False(t, statusByID["demo-sorrentinos"].BlocksSale)
	assert.NotContains(t, statusByID["demo-sorrentinos"].AffectedInventory, "Bife de chorizo (media res)")
}

func seedDriftedBeefOnHarvestBowl(t *testing.T, beefQty float64) (*Business, *InventoryItem, *InventoryItem) {
	t.Helper()
	business := helperBusiness(t, 0, 0)
	require.NoError(t, db.Create(&InventorySettings{
		BusinessID:                business.ID,
		InventoryEnabled:          true,
		AutoDeductOnOrderApproval: true,
		AvailabilitySyncMode:      InventoryAvailabilityModeWarn,
	}).Error)

	greens := &InventoryItem{
		BusinessID: business.ID, Name: "Mixed Greens", Unit: "kg",
		CurrentQuantity: 48, IsActive: true,
	}
	beef := &InventoryItem{
		BusinessID: business.ID, Name: "Premium Beef", Unit: "kg",
		CurrentQuantity: beefQty, IsActive: true,
	}
	require.NoError(t, db.Create(greens).Error)
	require.NoError(t, db.Create(beef).Error)

	helperMenu(t, business, []MenuCategory{
		{
			ID:   "mains",
			Name: "Mains",
			Items: []MenuItem{
				{ID: "demo-bowl", Name: "Harvest Bowl", DietaryTags: []string{"vegetarian"}, IsAvailable: true},
				{ID: "demo-steak", Name: "Steak Plate", IsAvailable: true},
			},
		},
	})

	require.NoError(t, db.Create([]InventoryRecipe{
		{
			BusinessID: business.ID, MenuItemID: "demo-bowl", MenuItemName: "Harvest Bowl",
			InventoryItemID: greens.ID, QuantityRequired: 0.25,
		},
		// #727 shape B: correct steak id, name drifted onto the bowl.
		{
			BusinessID: business.ID, MenuItemID: "demo-steak", MenuItemName: "Harvest Bowl",
			InventoryItemID: beef.ID, QuantityRequired: 0.35,
		},
	}).Error)
	return business, greens, beef
}

func TestOutOfStockMenuItemIDs_ZeroBeefMarksSteakNotHarvestBowl(t *testing.T) {
	setupOrderTestDB(t)
	business, _, beef := seedDriftedBeefOnHarvestBowl(t, 0)

	oos, err := OutOfStockMenuItemIDs(business.ID)
	require.NoError(t, err)
	assert.True(t, oos["demo-steak"], "zero beef must mark Steak Plate out of stock")
	assert.False(t, oos["demo-bowl"], "vegetarian Harvest Bowl must not be in the OOS id set")

	var beefRecipe InventoryRecipe
	require.NoError(t, db.Where("business_id = ? AND inventory_item_id = ?", business.ID, beef.ID).
		First(&beefRecipe).Error)
	assert.Equal(t, "demo-steak", beefRecipe.MenuItemID)

	require.NoError(t, db.Model(&beefRecipe).Update("menu_item_name", "").Error)
	oos, err = OutOfStockMenuItemIDs(business.ID)
	require.NoError(t, err)
	assert.True(t, oos["demo-steak"], "persisted steak id must survive a later blank name")
	assert.False(t, oos["demo-bowl"], "blank name must not re-86 the bowl")
}

func TestDeductApprovedOrderInventory_DriftedBeefConsumesSteakNotBowl(t *testing.T) {
	setupOrderTestDB(t)
	business, greens, beef := seedDriftedBeefOnHarvestBowl(t, 10)

	bowlBill := helperBill(t, business, nil, 14)
	bowlOrder := &Order{
		BillID:      bowlBill.ID,
		BusinessID:  business.ID,
		OrderNumber: "O-bowl",
		Status:      OrderStatusPending,
		CreatedBy:   "guest",
		Items: mustJSON(t, []OrderItem{{
			ID: "bowl-line", MenuItemID: "demo-bowl", MenuItemName: "Harvest Bowl",
			Quantity: 1, Price: 14, Subtotal: 14,
		}}),
	}
	require.NoError(t, db.Create(bowlOrder).Error)
	require.NoError(t, UpdateOrderStatus(bowlOrder.ID, OrderStatusApproved, "manager@example.com", ""))

	require.NoError(t, db.First(greens, greens.ID).Error)
	require.NoError(t, db.First(beef, beef.ID).Error)
	assert.InDelta(t, 47.75, greens.CurrentQuantity, 0.001)
	assert.Equal(t, 10.0, beef.CurrentQuantity, "a Harvest Bowl order must not burn Premium Beef")

	var beefRecipe InventoryRecipe
	require.NoError(t, db.Where("business_id = ? AND inventory_item_id = ?", business.ID, beef.ID).
		First(&beefRecipe).Error)
	assert.Equal(t, "demo-steak", beefRecipe.MenuItemID)

	steakBill := helperBill(t, business, nil, 24)
	steakOrder := &Order{
		BillID:      steakBill.ID,
		BusinessID:  business.ID,
		OrderNumber: "O-steak",
		Status:      OrderStatusPending,
		CreatedBy:   "guest",
		Items: mustJSON(t, []OrderItem{{
			ID: "steak-line", MenuItemID: "demo-steak", MenuItemName: "Steak Plate",
			Quantity: 1, Price: 24, Subtotal: 24,
		}}),
	}
	require.NoError(t, db.Create(steakOrder).Error)
	require.NoError(t, UpdateOrderStatus(steakOrder.ID, OrderStatusApproved, "manager@example.com", ""))

	require.NoError(t, db.First(beef, beef.ID).Error)
	assert.InDelta(t, 9.65, beef.CurrentQuantity, 0.001, "a Steak Plate order must consume Premium Beef")
}

// #727 (B2): recipes are one row per (dish, ingredient), so a multi-ingredient
// dish legitimately owns several rows that all share its menu_item_id. When one
// of those rows' NAME drifts onto a recipe-less live dish, the row still
// belongs to the id's dish: the stored id must win, the bowl's own empty
// ingredient must 86 the bowl, and reconcile must write NOTHING — a heuristic
// guess must never bake a recipe row onto a dish that has no recipes.
func TestReconcile_MultiIngredientDriftedNameStaysOnItsDish(t *testing.T) {
	setupOrderTestDB(t)

	business := helperBusiness(t, 0, 0)
	require.NoError(t, db.Create(&InventorySettings{
		BusinessID:           business.ID,
		InventoryEnabled:     true,
		AvailabilitySyncMode: InventoryAvailabilityModeWarn,
	}).Error)

	quinoa := &InventoryItem{
		BusinessID: business.ID, Name: "Quinoa", Unit: "kg",
		CurrentQuantity: 12, IsActive: true,
	}
	dressing := &InventoryItem{
		BusinessID: business.ID, Name: "House Dressing", Unit: "l",
		CurrentQuantity: 0, IsActive: true,
	}
	require.NoError(t, db.Create(quinoa).Error)
	require.NoError(t, db.Create(dressing).Error)

	helperMenu(t, business, []MenuCategory{
		{
			ID:   "mains",
			Name: "Mains",
			Items: []MenuItem{
				{ID: "demo-bowl", Name: "Harvest Bowl", IsAvailable: true},
				// Steak Plate exists on the menu but has NO recipe rows.
				{ID: "demo-steak", Name: "Steak Plate", IsAvailable: true},
			},
		},
	})

	require.NoError(t, db.Create([]InventoryRecipe{
		{
			BusinessID: business.ID, MenuItemID: "demo-bowl", MenuItemName: "Harvest Bowl",
			InventoryItemID: quinoa.ID, QuantityRequired: 0.2,
		},
		// Second bowl ingredient whose name drifted onto the recipe-less steak.
		{
			BusinessID: business.ID, MenuItemID: "demo-bowl", MenuItemName: "Steak Plate",
			InventoryItemID: dressing.ID, QuantityRequired: 0.05,
		},
	}).Error)

	summary, err := GetInventorySummary(business.ID)
	require.NoError(t, err)
	statusByID := make(map[string]InventoryMenuItemStatus, len(summary.MenuItemStatuses))
	for _, status := range summary.MenuItemStatuses {
		statusByID[status.MenuItemID] = status
	}
	assert.Equal(t, "out_of_stock", statusByID["demo-bowl"].Status,
		"the bowl's own empty dressing must 86 the bowl")
	assert.Contains(t, statusByID["demo-bowl"].AffectedInventory, "House Dressing")
	assert.False(t, statusByID["demo-steak"].HasRecipe,
		"the recipe-less steak must not inherit a recipe row by heuristic")
	assert.NotEqual(t, "out_of_stock", statusByID["demo-steak"].Status)

	oos, err := OutOfStockMenuItemIDs(business.ID)
	require.NoError(t, err)
	assert.True(t, oos["demo-bowl"], "guest orderability must 86 the bowl too (operator == guest)")
	assert.False(t, oos["demo-steak"], "guest orderability must leave the recipe-less steak sellable")

	// Zero rows written by reconcile: both rows keep stored id AND name.
	var rows []InventoryRecipe
	require.NoError(t, db.Where("business_id = ?", business.ID).
		Order("inventory_item_id ASC").Find(&rows).Error)
	require.Len(t, rows, 2)
	assert.Equal(t, "demo-bowl", rows[0].MenuItemID)
	assert.Equal(t, "Harvest Bowl", rows[0].MenuItemName)
	assert.Equal(t, "demo-bowl", rows[1].MenuItemID,
		"the conflicted row must keep its stored id — no heuristic id rewrite")
	assert.Equal(t, "Steak Plate", rows[1].MenuItemName,
		"the drifted name must not be healed by a guess either")
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return string(raw)
}
