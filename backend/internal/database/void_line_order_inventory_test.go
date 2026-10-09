package database

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	voidLineMenuA = "menu-a"
	voidLineMenuB = "menu-b"
	voidLineIDA   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	voidLineIDB   = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	voidLineOnly  = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
)

func seedVoidLineInventory(t *testing.T) (*Business, *InventoryItem, *InventoryItem) {
	t.Helper()
	business := helperBusiness(t, 0, 0)
	require.NoError(t, db.Create(&InventorySettings{
		BusinessID:                business.ID,
		InventoryEnabled:          true,
		AutoDeductOnOrderApproval: true,
		LowStockWarningsEnabled:   true,
		AvailabilitySyncMode:      InventoryAvailabilityModeWarn,
	}).Error)
	stockX := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Stock X",
		Unit:            "unit",
		CurrentQuantity: 10,
		IsActive:        true,
	}
	stockY := &InventoryItem{
		BusinessID:      business.ID,
		Name:            "Stock Y",
		Unit:            "unit",
		CurrentQuantity: 20,
		IsActive:        true,
	}
	require.NoError(t, db.Create(stockX).Error)
	require.NoError(t, db.Create(stockY).Error)
	require.NoError(t, db.Create([]InventoryRecipe{
		{
			BusinessID:       business.ID,
			MenuItemID:       voidLineMenuA,
			MenuItemName:     "Dish A",
			InventoryItemID:  stockX.ID,
			QuantityRequired: 1.5,
		},
		{
			BusinessID:       business.ID,
			MenuItemID:       voidLineMenuB,
			MenuItemName:     "Dish B",
			InventoryItemID:  stockY.ID,
			QuantityRequired: 2,
		},
	}).Error)
	return business, stockX, stockY
}

func approveOrderForVoid(t *testing.T, order *Order, items []OrderItem) {
	t.Helper()
	require.NoError(t, db.Model(&Order{}).Where("id = ?", order.ID).Update("status", OrderStatusApproved).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var loaded Order
		if err := tx.First(&loaded, order.ID).Error; err != nil {
			return err
		}
		return DeductApprovedOrderInventoryTx(tx, &loaded, items, "manager@example.com")
	}))
}

func attachOrderBillLines(t *testing.T, bill *Bill, order *Order, lines []BillItem) {
	t.Helper()
	raw, err := json.Marshal(lines)
	require.NoError(t, err)
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).Update("items", string(raw)).Error)
	for i := range lines {
		lines[i].BillID = bill.ID
	}
	require.NoError(t, db.Create(&lines).Error)
}

func orderItemLines(t *testing.T, orderID uint) []OrderItem {
	t.Helper()
	var order Order
	require.NoError(t, db.First(&order, orderID).Error)
	items, err := parseOrderItemsSnapshot(order.Items)
	require.NoError(t, err)
	return items
}

func sumMovementDelta(t *testing.T, orderID uint, movementType, menuItemID string) float64 {
	t.Helper()
	var movements []InventoryMovement
	require.NoError(t, db.Where(
		"reference_order_id = ? AND movement_type = ? AND menu_item_id = ?",
		orderID, movementType, menuItemID,
	).Find(&movements).Error)
	total := 0.0
	for _, movement := range movements {
		total += movement.QuantityDelta
	}
	return total
}

func TestAdjustBillItem_VoidLineRemovesKitchenItemAndRestoresInventory(t *testing.T) {
	setupOrderTestDB(t)
	business, stockX, stockY := seedVoidLineInventory(t)
	bill := helperBill(t, business, nil, 0)
	items := []OrderItem{
		{ID: voidLineIDA, MenuItemID: voidLineMenuA, MenuItemName: "Dish A", Quantity: 2, Price: 10, Subtotal: 20},
		{ID: voidLineIDB, MenuItemID: voidLineMenuB, MenuItemName: "Dish B", Quantity: 1, Price: 8, Subtotal: 8},
	}
	order := helperOrder(t, business, bill, items)
	approveOrderForVoid(t, order, items)

	orderID := order.ID
	attachOrderBillLines(t, bill, order, []BillItem{
		{
			ID:         normalizeBillItemUUID(voidLineIDA),
			MenuItemID: voidLineMenuA,
			Name:       "Dish A",
			Price:      10,
			Quantity:   2,
			Subtotal:   20,
			ItemType:   "menu_item",
			OrderID:    &orderID,
		},
		{
			ID:         normalizeBillItemUUID(voidLineIDB),
			MenuItemID: voidLineMenuB,
			Name:       "Dish B",
			Price:      8,
			Quantity:   1,
			Subtotal:   8,
			ItemType:   "menu_item",
			OrderID:    &orderID,
		},
	})

	var beforeX, beforeY InventoryItem
	require.NoError(t, db.First(&beforeX, stockX.ID).Error)
	require.NoError(t, db.First(&beforeY, stockY.ID).Error)
	assert.InDelta(t, 7.0, beforeX.CurrentQuantity, 0.001)
	assert.InDelta(t, 18.0, beforeY.CurrentQuantity, 0.001)

	_, updatedItems, err := AdjustBillItem(bill.ID, normalizeBillItemUUID(voidLineIDA), "staff@example.com", nil, true, "guest sent dish A back")
	require.NoError(t, err)
	require.Len(t, updatedItems, 1)
	assert.Equal(t, "Dish B", updatedItems[0].Name)

	remaining := orderItemLines(t, order.ID)
	require.Len(t, remaining, 1)
	assert.Equal(t, voidLineIDB, remaining[0].ID)
	assert.Equal(t, voidLineMenuB, remaining[0].MenuItemID)

	var reloaded Order
	require.NoError(t, db.First(&reloaded, order.ID).Error)
	assert.Equal(t, OrderStatusApproved, reloaded.Status)
	assert.Nil(t, reloaded.CancelledAt)

	var afterX, afterY InventoryItem
	require.NoError(t, db.First(&afterX, stockX.ID).Error)
	require.NoError(t, db.First(&afterY, stockY.ID).Error)
	assert.InDelta(t, beforeX.CurrentQuantity+3, afterX.CurrentQuantity, 0.001)
	assert.InDelta(t, beforeY.CurrentQuantity, afterY.CurrentQuantity, 0.001)
	assert.InDelta(t, 3.0, sumMovementDelta(t, order.ID, InventoryMovementTypeOrderRestoration, voidLineMenuA), 0.001)
	assert.InDelta(t, 0.0, sumMovementDelta(t, order.ID, InventoryMovementTypeOrderRestoration, voidLineMenuB), 0.001)

	var history []BillHistoryEvent
	require.NoError(t, db.Where("bill_id = ?", bill.ID).Order("id ASC").Find(&history).Error)
	require.Len(t, history, 1)
	assert.Equal(t, float64(order.ID), history[0].Details["order_id"])
	assert.Equal(t, false, history[0].Details["order_cancelled"])
}

func TestAdjustBillItem_VoidOnlyLineCancelsOrder(t *testing.T) {
	setupOrderTestDB(t)
	business := helperBusiness(t, 0, 0)
	bill := helperBill(t, business, nil, 0)
	items := []OrderItem{
		{ID: voidLineOnly, MenuItemID: "menu-only", MenuItemName: "Solo", Quantity: 1, Price: 9, Subtotal: 9},
	}
	order := helperOrder(t, business, bill, items)
	require.NoError(t, db.Model(&Order{}).Where("id = ?", order.ID).Update("status", OrderStatusApproved).Error)

	orderID := order.ID
	lineID := normalizeBillItemUUID(voidLineOnly)
	attachOrderBillLines(t, bill, order, []BillItem{
		{
			ID:         lineID,
			MenuItemID: "menu-only",
			Name:       "Solo",
			Price:      9,
			Quantity:   1,
			Subtotal:   9,
			ItemType:   "menu_item",
			OrderID:    &orderID,
		},
	})

	_, updatedItems, err := AdjustBillItem(bill.ID, lineID, "staff@example.com", nil, true, "guest cancelled the only plate")
	require.NoError(t, err)
	assert.Empty(t, updatedItems)

	var reloaded Order
	require.NoError(t, db.First(&reloaded, order.ID).Error)
	assert.Equal(t, OrderStatusOrderCancelled, reloaded.Status)
	assert.Equal(t, "staff@example.com", reloaded.CancelledBy)
	assert.Equal(t, "guest cancelled the only plate", reloaded.CancelReason)
	assert.NotNil(t, reloaded.CancelledAt)
	assert.Empty(t, orderItemLines(t, order.ID))

	var history []BillHistoryEvent
	require.NoError(t, db.Where("bill_id = ?", bill.ID).Find(&history).Error)
	require.Len(t, history, 1)
	assert.Equal(t, float64(order.ID), history[0].Details["order_id"])
	assert.Equal(t, true, history[0].Details["order_cancelled"])
}

func TestRestoreCancelledOrderInventoryTx_AfterLineVoidRestoresOnlyRemainder(t *testing.T) {
	setupOrderTestDB(t)
	business, stockX, stockY := seedVoidLineInventory(t)
	bill := helperBill(t, business, nil, 0)
	items := []OrderItem{
		{ID: voidLineIDA, MenuItemID: voidLineMenuA, MenuItemName: "Dish A", Quantity: 2, Price: 10, Subtotal: 20},
		{ID: voidLineIDB, MenuItemID: voidLineMenuB, MenuItemName: "Dish B", Quantity: 1, Price: 8, Subtotal: 8},
	}
	order := helperOrder(t, business, bill, items)
	approveOrderForVoid(t, order, items)
	orderID := order.ID
	attachOrderBillLines(t, bill, order, []BillItem{
		{
			ID:         normalizeBillItemUUID(voidLineIDA),
			MenuItemID: voidLineMenuA,
			Name:       "Dish A",
			Price:      10,
			Quantity:   2,
			Subtotal:   20,
			ItemType:   "menu_item",
			OrderID:    &orderID,
		},
		{
			ID:         normalizeBillItemUUID(voidLineIDB),
			MenuItemID: voidLineMenuB,
			Name:       "Dish B",
			Price:      8,
			Quantity:   1,
			Subtotal:   8,
			ItemType:   "menu_item",
			OrderID:    &orderID,
		},
	})

	_, _, err := AdjustBillItem(bill.ID, normalizeBillItemUUID(voidLineIDA), "staff@example.com", nil, true, "void A before cancel")
	require.NoError(t, err)

	var afterVoidX, afterVoidY InventoryItem
	require.NoError(t, db.First(&afterVoidX, stockX.ID).Error)
	require.NoError(t, db.First(&afterVoidY, stockY.ID).Error)
	assert.InDelta(t, 10.0, afterVoidX.CurrentQuantity, 0.001)
	assert.InDelta(t, 18.0, afterVoidY.CurrentQuantity, 0.001)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var loaded Order
		if err := tx.First(&loaded, order.ID).Error; err != nil {
			return err
		}
		return RestoreCancelledOrderInventoryTx(tx, &loaded, "manager@example.com")
	}))

	var afterCancelX, afterCancelY InventoryItem
	require.NoError(t, db.First(&afterCancelX, stockX.ID).Error)
	require.NoError(t, db.First(&afterCancelY, stockY.ID).Error)
	assert.InDelta(t, afterVoidX.CurrentQuantity, afterCancelX.CurrentQuantity, 0.001, "A must not be restored twice")
	assert.InDelta(t, afterVoidY.CurrentQuantity+2, afterCancelY.CurrentQuantity, 0.001, "only B's consumption is restored")
	assert.InDelta(t, 3.0, sumMovementDelta(t, order.ID, InventoryMovementTypeOrderRestoration, voidLineMenuA), 0.001)
	assert.InDelta(t, 2.0, sumMovementDelta(t, order.ID, InventoryMovementTypeOrderRestoration, voidLineMenuB), 0.001)
	assert.InDelta(t, -3.0, sumMovementDelta(t, order.ID, InventoryMovementTypeOrderConsumption, voidLineMenuA), 0.001)
	assert.InDelta(t, -2.0, sumMovementDelta(t, order.ID, InventoryMovementTypeOrderConsumption, voidLineMenuB), 0.001)

	var movementCount int64
	require.NoError(t, db.Model(&InventoryMovement{}).Where("reference_order_id = ?", order.ID).Count(&movementCount).Error)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var loaded Order
		if err := tx.First(&loaded, order.ID).Error; err != nil {
			return err
		}
		return RestoreCancelledOrderInventoryTx(tx, &loaded, "manager@example.com")
	}))

	var againX, againY InventoryItem
	require.NoError(t, db.First(&againX, stockX.ID).Error)
	require.NoError(t, db.First(&againY, stockY.ID).Error)
	assert.InDelta(t, afterCancelX.CurrentQuantity, againX.CurrentQuantity, 0.001)
	assert.InDelta(t, afterCancelY.CurrentQuantity, againY.CurrentQuantity, 0.001)
	var movementCountAgain int64
	require.NoError(t, db.Model(&InventoryMovement{}).Where("reference_order_id = ?", order.ID).Count(&movementCountAgain).Error)
	assert.Equal(t, movementCount, movementCountAgain)
}

// A bill line that carries an order id but is no longer on that ticket (for
// example re-keyed by a merge) must not restore stock: the ticket still holds
// the dish, so a later cancel of the order restores it, and restoring at the
// void too would count it twice.
func TestAdjustBillItem_VoidLineNotOnTicketLeavesOrderAndStock(t *testing.T) {
	setupOrderTestDB(t)
	business, stockX, _ := seedVoidLineInventory(t)
	bill := helperBill(t, business, nil, 0)
	items := []OrderItem{
		{ID: voidLineIDA, MenuItemID: voidLineMenuA, MenuItemName: "Dish A", Quantity: 2, Price: 10, Subtotal: 20},
	}
	order := helperOrder(t, business, bill, items)
	approveOrderForVoid(t, order, items)

	orderID := order.ID
	strayID := normalizeBillItemUUID("dddddddd-dddd-4ddd-8ddd-dddddddddddd")
	attachOrderBillLines(t, bill, order, []BillItem{
		{
			ID:         strayID,
			MenuItemID: voidLineMenuA,
			Name:       "Dish A",
			Price:      10,
			Quantity:   2,
			Subtotal:   20,
			ItemType:   "menu_item",
			OrderID:    &orderID,
		},
	})

	var before InventoryItem
	require.NoError(t, db.First(&before, stockX.ID).Error)

	_, _, err := AdjustBillItem(bill.ID, strayID, "staff@example.com", nil, true, "wrong dish")
	require.NoError(t, err)

	remaining := orderItemLines(t, order.ID)
	require.Len(t, remaining, 1, "the ticket keeps its dish")
	var reloaded Order
	require.NoError(t, db.First(&reloaded, order.ID).Error)
	assert.Equal(t, OrderStatusApproved, reloaded.Status)

	var after InventoryItem
	require.NoError(t, db.First(&after, stockX.ID).Error)
	assert.InDelta(t, before.CurrentQuantity, after.CurrentQuantity, 0.001)
	assert.InDelta(t, 0.0, sumMovementDelta(t, order.ID, InventoryMovementTypeOrderRestoration, voidLineMenuA), 0.001)
}
