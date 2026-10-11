package database

import (
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupOrderTestDB creates an in-memory SQLite DB with all tables needed for order tests.
func setupOrderTestDB(t testing.TB) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err, "open in-memory database")

	// SQLite in-memory: single connection ensures all operations see the same DB
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	db = gormDB

	require.NoError(t, db.AutoMigrate(
		&Business{},
		&InventorySettings{},
		&InventoryItem{},
		&InventoryRecipe{},
		&InventoryMovement{},
		&Menu{},
		&Offer{},
		&Table{},
		&Bill{},
		&BillHistoryEvent{},
		&Order{},
		&DeliveryOrder{},
		&DeliveryStatusHistory{},
	))

	// Manually create bill_items table (sqlite-compatible)
	db.Exec("DROP TABLE IF EXISTS bill_items")
	require.NoError(t, db.Exec(`
		CREATE TABLE bill_items (
			id TEXT PRIMARY KEY,
			bill_id INTEGER NOT NULL,
			menu_item_id TEXT DEFAULT '',
			name TEXT NOT NULL,
			price REAL NOT NULL,
			quantity INTEGER NOT NULL,
			options TEXT,
			item_type TEXT DEFAULT 'menu_item',
			bundle_id INTEGER,
			parent_bundle_id INTEGER,
			source_offer_id INTEGER,
			order_id INTEGER,
			subtotal REAL NOT NULL,
			created_at DATETIME
		)
	`).Error)
}

// helperBusiness creates a business with optional tax/service-fee rates.
func helperBusiness(t testing.TB, taxRate, serviceFeeRate float64) *Business {
	t.Helper()
	b := &Business{Name: "Test Resto", TaxRate: taxRate, ServiceFeeRate: serviceFeeRate}
	require.NoError(t, db.Create(b).Error)
	return b
}

// helperBill creates an open bill belonging to the business.
func helperBill(t testing.TB, business *Business, existingItems []BillItem, subtotal float64) *Bill {
	t.Helper()
	itemsJSON := "[]"
	if len(existingItems) > 0 {
		raw, err := json.Marshal(existingItems)
		require.NoError(t, err)
		itemsJSON = string(raw)
	}
	subtotalCents := int64(math.Round(subtotal * 100))
	bill := &Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-%d", time.Now().UnixNano()),
		Status:      BillStatusOpen,
		Items:       itemsJSON,
		Subtotal:    subtotalCents,
		TotalAmount: subtotalCents,
	}
	require.NoError(t, db.Create(bill).Error)

	// Also sync relation rows for existing items
	for i := range existingItems {
		existingItems[i].BillID = bill.ID
		if existingItems[i].ID == "" {
			existingItems[i].ID = "existing-" + existingItems[i].Name
		}
	}
	if len(existingItems) > 0 {
		require.NoError(t, db.Create(&existingItems).Error)
	}
	return bill
}

// helperOrder creates an order with the given items.
func helperOrder(t testing.TB, business *Business, bill *Bill, items []OrderItem) *Order {
	t.Helper()
	itemsJSON, err := json.Marshal(items)
	require.NoError(t, err)

	order := &Order{
		BillID:      bill.ID,
		BusinessID:  business.ID,
		OrderNumber: "O-test",
		Status:      OrderStatusPending,
		CreatedBy:   "guest",
		Items:       string(itemsJSON),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, db.Create(order).Error)
	return order
}

// ─── STATE MACHINE TRANSITION VALIDATION ───

func TestValidateTransition_ValidTransitions(t *testing.T) {
	validCases := []struct {
		from OrderStatus
		to   OrderStatus
	}{
		{OrderStatusPending, OrderStatusApproved},
		{OrderStatusPending, OrderStatusOrderCancelled},
		{OrderStatusApproved, OrderStatusInKitchen},
		{OrderStatusApproved, OrderStatusOrderCancelled},
		{OrderStatusInKitchen, OrderStatusOrderReady},
		{OrderStatusInKitchen, OrderStatusOrderCancelled},
		{OrderStatusOrderReady, OrderStatusOrderDelivered},
		{OrderStatusOrderReady, OrderStatusOrderCancelled},
	}

	for _, tc := range validCases {
		t.Run(string(tc.from)+"→"+string(tc.to), func(t *testing.T) {
			err := ValidateTransition(tc.from, tc.to)
			assert.NoError(t, err)
		})
	}
}

func TestValidateTransition_InvalidTransitions(t *testing.T) {
	invalidCases := []struct {
		from OrderStatus
		to   OrderStatus
	}{
		{OrderStatusPending, OrderStatusInKitchen},       // skip approved
		{OrderStatusPending, OrderStatusOrderReady},      // skip 2 steps
		{OrderStatusPending, OrderStatusOrderDelivered},  // skip 3 steps
		{OrderStatusApproved, OrderStatusPending},        // backwards
		{OrderStatusApproved, OrderStatusOrderReady},     // skip kitchen
		{OrderStatusInKitchen, OrderStatusPending},       // backwards
		{OrderStatusOrderDelivered, OrderStatusPending},  // terminal → any
		{OrderStatusOrderDelivered, OrderStatusApproved}, // terminal → any
		{OrderStatusOrderCancelled, OrderStatusPending},  // terminal → any
		{OrderStatusOrderCancelled, OrderStatusApproved}, // terminal → any
	}

	for _, tc := range invalidCases {
		t.Run(string(tc.from)+"→"+string(tc.to)+"_invalid", func(t *testing.T) {
			err := ValidateTransition(tc.from, tc.to)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidStatusTransition)
		})
	}
}

func TestUpdateOrderStatus_RejectsInvalidTransition(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)
	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "sm-1", MenuItemName: "Soup", Price: 7, Quantity: 1, Subtotal: 7},
	})

	// Try to skip from pending to in_kitchen (must go through approved first)
	err := UpdateOrderStatus(order.ID, OrderStatusInKitchen, "", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidStatusTransition)

	// Verify order is still pending
	var updated Order
	require.NoError(t, db.First(&updated, order.ID).Error)
	assert.Equal(t, OrderStatusPending, updated.Status)
}

func TestGetOrderBusinessIDByIDReturnsBusinessID(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 100)
	order := helperOrder(t, biz, bill, []OrderItem{{
		MenuItemName: "Soup",
		Quantity:     1,
		Price:        10,
		Subtotal:     10,
	}})

	businessID, err := GetOrderBusinessIDByID(order.ID)

	require.NoError(t, err)
	require.Equal(t, biz.ID, businessID)
}

// ─── BASIC APPROVAL ───

func TestUpdateOrderStatus_ApproveAddsItemsToBill(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 10, 5) // 10% tax, 5% service fee
	bill := helperBill(t, biz, nil, 0)

	orderItems := []OrderItem{
		{ID: "item-1", MenuItemName: "Burger", Price: 10, Quantity: 2, Subtotal: 20},
		{ID: "item-2", MenuItemName: "Fries", Price: 5, Quantity: 1, Subtotal: 5},
	}
	order := helperOrder(t, biz, bill, orderItems)

	err := UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", "")
	require.NoError(t, err)

	// Verify order status
	var updated Order
	require.NoError(t, db.First(&updated, order.ID).Error)
	assert.Equal(t, OrderStatusApproved, updated.Status)
	assert.Equal(t, "staff", updated.ApprovedBy)
	assert.NotNil(t, updated.ApprovedAt)

	// Verify bill items were added
	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	var billItems []BillItem
	require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &billItems))
	assert.Equal(t, 2, len(billItems))
	assert.Equal(t, "Burger", billItems[0].Name)
	assert.Equal(t, "Fries", billItems[1].Name)

	// Verify totals (subtotal=25, tax=2.50, service=1.25, total=28.75)
	assert.InDelta(t, 2500.0, savedBill.Subtotal, 0.01)
	assert.InDelta(t, 250.0, savedBill.TaxAmount, 0.01)
	assert.InDelta(t, 125.0, savedBill.ServiceFeeAmount, 0.01)
	assert.InDelta(t, 2875.0, savedBill.TotalAmount, 0.01)

	// Verify relational bill_items
	var relItems []BillItem
	require.NoError(t, db.Where("bill_id = ?", bill.ID).Find(&relItems).Error)
	assert.Equal(t, 2, len(relItems))

	var history []BillHistoryEvent
	require.NoError(t, db.Where("bill_id = ?", bill.ID).Find(&history).Error)
	require.Len(t, history, 1)
	assert.Equal(t, BillHistoryEventOrderApproved, history[0].EventType)
	assert.Equal(t, "staff", history[0].Actor)
	assert.Equal(t, order.OrderNumber, history[0].OrderNumber)
}

// ─── RE-APPROVAL GUARD ───

func TestUpdateOrderStatus_ReApprovalIsNoOp(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	orderItems := []OrderItem{
		{ID: "item-1", MenuItemName: "Burger", Price: 10, Quantity: 1, Subtotal: 10},
	}
	order := helperOrder(t, biz, bill, orderItems)

	// First approval
	err := UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", "")
	require.NoError(t, err)

	// Second approval — should be a no-op, NOT duplicate items
	err = UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", "")
	require.NoError(t, err)

	// Verify only 1 bill item (not 2)
	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	var billItems []BillItem
	require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &billItems))
	assert.Equal(t, 1, len(billItems), "re-approval should NOT duplicate items")
	assert.InDelta(t, 1000.0, savedBill.Subtotal, 0.01)

	var relItems []BillItem
	require.NoError(t, db.Where("bill_id = ?", bill.ID).Find(&relItems).Error)
	assert.Equal(t, 1, len(relItems), "relational items should not be duplicated")
}

// ─── APPROVAL MERGES WITH EXISTING BILL ITEMS ───

func TestUpdateOrderStatus_MergesWithExistingBillItems(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)

	existingItems := []BillItem{
		{ID: "pre-existing-1", MenuItemID: "coffee", Name: "Coffee", Price: 3, Quantity: 1, Subtotal: 3, ItemType: "menu_item"},
	}
	bill := helperBill(t, biz, existingItems, 3)

	orderItems := []OrderItem{
		{ID: "new-1", MenuItemName: "Cake", Price: 8, Quantity: 1, Subtotal: 8},
	}
	order := helperOrder(t, biz, bill, orderItems)

	err := UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", "")
	require.NoError(t, err)

	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	var billItems []BillItem
	require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &billItems))
	assert.Equal(t, 2, len(billItems), "should have existing + new item")
	assert.InDelta(t, 1100.0, savedBill.Subtotal, 0.01)

	var relItems []BillItem
	require.NoError(t, db.Where("bill_id = ?", bill.ID).Find(&relItems).Error)
	assert.Equal(t, 2, len(relItems))
}

// ─── ITEM-ID PARITY BETWEEN JSON SNAPSHOT AND RELATIONAL ROWS ───

// TestUpdateOrderStatus_ApprovePreservesItemIDParity guards the invariant that
// the bills.items JSON snapshot and the relational bill_items rows carry the
// SAME item IDs after an order is approved. Order lines arrive with synthetic,
// non-UUID IDs ("item-1", "bundle-5-...") and normalizeBillItemUUID generates a
// fresh random UUID for any non-UUID input — so normalizing independently for
// the snapshot and the relational rows assigns DIFFERENT UUIDs to the same item,
// silently diverging the two stores and breaking itemized splitting (which keys
// items by ID).
func TestUpdateOrderStatus_ApprovePreservesItemIDParity(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	orderItems := []OrderItem{
		{ID: "item-1", MenuItemName: "Burger", Price: 10, Quantity: 2, Subtotal: 20},
		{ID: "bundle-5-1700000000", MenuItemName: "Combo", Price: 15, Quantity: 1, Subtotal: 15},
	}
	order := helperOrder(t, biz, bill, orderItems)
	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", ""))

	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	var jsonItems []BillItem
	require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &jsonItems))

	var relItems []BillItem
	require.NoError(t, db.Where("bill_id = ?", bill.ID).Find(&relItems).Error)

	require.Len(t, jsonItems, 2)
	require.Len(t, relItems, 2)

	jsonIDs := make(map[string]bool, len(jsonItems))
	for _, it := range jsonItems {
		assert.NotEmpty(t, it.ID, "JSON snapshot item must have a non-empty ID")
		jsonIDs[it.ID] = true
	}
	for _, it := range relItems {
		assert.True(t, jsonIDs[it.ID],
			"relational bill_items row ID %q must match a JSON snapshot ID — the two stores must not diverge", it.ID)
	}
}

// ─── MULTIPLE ORDERS FOR SAME BILL (SEQUENTIAL) ───

func TestUpdateOrderStatus_MultipleOrdersSameBill(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 10, 0) // 10% tax only
	bill := helperBill(t, biz, nil, 0)

	order1 := helperOrder(t, biz, bill, []OrderItem{
		{ID: "a1", MenuItemName: "Burger", Price: 10, Quantity: 1, Subtotal: 10},
	})
	order2 := helperOrder(t, biz, bill, []OrderItem{
		{ID: "b1", MenuItemName: "Pizza", Price: 15, Quantity: 1, Subtotal: 15},
	})

	require.NoError(t, UpdateOrderStatus(order1.ID, OrderStatusApproved, "staff", ""))
	require.NoError(t, UpdateOrderStatus(order2.ID, OrderStatusApproved, "staff", ""))

	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	var billItems []BillItem
	require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &billItems))
	assert.Equal(t, 2, len(billItems))
	assert.InDelta(t, 2500.0, savedBill.Subtotal, 0.01)
	assert.InDelta(t, 250.0, savedBill.TaxAmount, 0.01) // 10% of 25
	assert.InDelta(t, 2750.0, savedBill.TotalAmount, 0.01)
}

// ─── STATUS TRANSITIONS ───

func TestUpdateOrderStatus_PendingToCancelled(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)
	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "x1", MenuItemName: "Soup", Price: 7, Quantity: 1, Subtotal: 7},
	})

	err := UpdateOrderStatus(order.ID, OrderStatusOrderCancelled, "staff", "")
	require.NoError(t, err)

	var updated Order
	require.NoError(t, db.First(&updated, order.ID).Error)
	assert.Equal(t, OrderStatusOrderCancelled, updated.Status)

	// Bill should remain empty (cancelled orders don't add items)
	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	assert.InDelta(t, 0.0, savedBill.Subtotal, 0.01)
}

func TestGetOrders_KeepsInKitchenOnActiveBill(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)
	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "k1", MenuItemName: "Pasta", Price: 12, Quantity: 1, Subtotal: 12},
	})
	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", ""))
	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusInKitchen, "kitchen", ""))

	result, err := GetOrdersByBusinessIDPaginated(
		biz.ID,
		"pending,approved,in_kitchen,ready,delivered,cancelled",
		PaginationParams{Page: 1, PageSize: 20},
		OrderListOptions{ActiveBillsOnly: true},
	)
	require.NoError(t, err)
	require.Len(t, result.Data, 1)
	assert.Equal(t, order.ID, result.Data[0].ID)
	assert.Equal(t, OrderStatusInKitchen, result.Data[0].Status)
}

func TestGetOrders_KeepsInKitchenOnClosedZeroBill(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)
	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "k0", MenuItemName: "Comp steak", Price: 0, Quantity: 1, Subtotal: 0},
	})
	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", ""))
	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusInKitchen, "kitchen", ""))
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"status":       BillStatusClosed,
		"total_amount": int64(0),
	}).Error)

	live, err := GetOrdersByBusinessIDPaginated(
		biz.ID,
		"pending,approved,in_kitchen,ready",
		PaginationParams{Page: 1, PageSize: 20},
		OrderListOptions{ActiveBillsOnly: true},
	)
	require.NoError(t, err)
	require.Len(t, live.Data, 1)
	assert.Equal(t, order.ID, live.Data[0].ID)
	assert.Equal(t, OrderStatusInKitchen, live.Data[0].Status)

	pendingOnly, err := GetOrdersByBusinessIDPaginated(
		biz.ID,
		"pending",
		PaginationParams{Page: 1, PageSize: 20},
		OrderListOptions{ActiveBillsOnly: true},
	)
	require.NoError(t, err)
	assert.Empty(t, pendingOnly.Data)
}

func TestUpdateOrderStatus_ApprovedToInKitchen(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)
	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "k1", MenuItemName: "Pasta", Price: 12, Quantity: 1, Subtotal: 12},
	})

	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", ""))
	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusInKitchen, "", ""))

	var updated Order
	require.NoError(t, db.First(&updated, order.ID).Error)
	assert.Equal(t, OrderStatusInKitchen, updated.Status)

	// Bill items should still be 1 (in_kitchen doesn't re-add)
	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	var billItems []BillItem
	require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &billItems))
	assert.Equal(t, 1, len(billItems))
}

func TestUpdateOrderStatus_FullLifecycle(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)
	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "lc1", MenuItemName: "Steak", Price: 30, Quantity: 1, Subtotal: 30},
	})

	statuses := []OrderStatus{
		OrderStatusApproved,
		OrderStatusInKitchen,
		OrderStatusOrderReady,
		OrderStatusOrderDelivered,
	}

	for _, s := range statuses {
		require.NoError(t, UpdateOrderStatus(order.ID, s, "staff", ""))
	}

	var final Order
	require.NoError(t, db.First(&final, order.ID).Error)
	assert.Equal(t, OrderStatusOrderDelivered, final.Status)

	// Items should be added only once (during approval)
	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	var billItems []BillItem
	require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &billItems))
	assert.Equal(t, 1, len(billItems), "items should only be added once during approval")
}

// ─── EDGE CASES ───

func TestUpdateOrderStatus_NonExistentOrder(t *testing.T) {
	setupOrderTestDB(t)
	err := UpdateOrderStatus(99999, OrderStatusApproved, "staff", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get order")
}

func TestUpdateOrderStatus_RejectsClosedBillApproval(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).Update("status", BillStatusClosed).Error)
	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "closed-1", MenuItemName: "Soup", Price: 7, Quantity: 1, Subtotal: 7},
	})

	err := UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bill is not open")

	var updated Order
	require.NoError(t, db.First(&updated, order.ID).Error)
	assert.Equal(t, OrderStatusPending, updated.Status)
}

func TestUpdateOrderStatus_EmptyOrderItems(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)
	order := helperOrder(t, biz, bill, []OrderItem{}) // empty items

	err := UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", "")
	require.NoError(t, err)

	var updated Order
	require.NoError(t, db.First(&updated, order.ID).Error)
	assert.Equal(t, OrderStatusApproved, updated.Status)

	// Bill should remain unchanged
	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	assert.InDelta(t, 0.0, savedBill.Subtotal, 0.01)
}

func TestUpdateOrderStatus_ApprovedByEmptyOnApproval(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)
	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "e1", MenuItemName: "Tea", Price: 2, Quantity: 1, Subtotal: 2},
	})

	// Approve with empty approved_by
	err := UpdateOrderStatus(order.ID, OrderStatusApproved, "", "")
	require.NoError(t, err)

	var updated Order
	require.NoError(t, db.First(&updated, order.ID).Error)
	assert.Equal(t, OrderStatusApproved, updated.Status)
	assert.Empty(t, updated.ApprovedBy, "approved_by should remain empty when not provided")
	assert.Nil(t, updated.ApprovedAt, "approved_at should not be set when approved_by is empty")
}

func TestUpdateOrderStatus_OrderItemsWithOptions(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	orderItems := []OrderItem{
		{
			ID:           "opt-1",
			MenuItemName: "Burger",
			Price:        10,
			Quantity:     1,
			Subtotal:     13, // 10 + 3 for cheese option
			Options: []MenuItemOption{
				{ID: "o1", Name: "Extra Cheese", PriceChange: 3},
			},
		},
	}
	order := helperOrder(t, biz, bill, orderItems)

	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", ""))

	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	var billItems []BillItem
	require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &billItems))

	assert.Equal(t, 1, len(billItems))
	assert.InDelta(t, 1300.0, savedBill.Subtotal, 0.01)
	// Verify options preserved
	assert.Equal(t, 1, len(billItems[0].Options))
	assert.Equal(t, "Extra Cheese", billItems[0].Options[0].Name)
}

func TestUpdateOrderStatus_ZeroPriceItems(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 10, 5)
	bill := helperBill(t, biz, nil, 0)

	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "free-1", MenuItemName: "Complimentary Water", Price: 0, Quantity: 3, Subtotal: 0},
	})

	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", ""))

	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	assert.InDelta(t, 0.0, savedBill.Subtotal, 0.01)
	assert.InDelta(t, 0.0, savedBill.TotalAmount, 0.01)
}

func TestUpdateOrderStatus_ItemTypeDefaults(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "dt-1", MenuItemName: "Burger", Price: 10, Quantity: 1, Subtotal: 10, ItemType: ""},
		{ID: "dt-2", MenuItemName: "Bundle", Price: 20, Quantity: 1, Subtotal: 20, ItemType: "bundle"},
		{ID: "dt-3", MenuItemName: "  ", Price: 5, Quantity: 1, Subtotal: 5, ItemType: "  "},
	})

	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", ""))

	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	var billItems []BillItem
	require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &billItems))

	assert.Equal(t, 3, len(billItems))
	assert.Equal(t, "menu_item", billItems[0].ItemType, "empty ItemType should default to menu_item")
	assert.Equal(t, "bundle", billItems[1].ItemType, "explicit ItemType should be preserved")
	assert.Equal(t, "menu_item", billItems[2].ItemType, "whitespace-only ItemType should default to menu_item")
}

func TestUpdateOrderStatus_MenuItemIDFallback(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "mid-1", MenuItemName: "Burger", MenuItemID: "", Price: 10, Quantity: 1, Subtotal: 10},
		{ID: "mid-2", MenuItemName: "Pizza", MenuItemID: "pizza-123", Price: 15, Quantity: 1, Subtotal: 15},
		{ID: "mid-3", MenuItemName: "Fries", MenuItemID: "   ", Price: 5, Quantity: 1, Subtotal: 5},
	})

	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", ""))

	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	var billItems []BillItem
	require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &billItems))

	assert.Equal(t, "Burger", billItems[0].MenuItemID, "empty MenuItemID should fall back to name")
	assert.Equal(t, "pizza-123", billItems[1].MenuItemID, "explicit MenuItemID preserved")
	assert.Equal(t, "Fries", billItems[2].MenuItemID, "whitespace MenuItemID should fall back to name")
}

// ─── TAX & SERVICE FEE CALCULATIONS ───

func TestUpdateOrderStatus_TaxAndServiceFeeCalculation(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 8.25, 3.5) // 8.25% tax, 3.5% service
	bill := helperBill(t, biz, nil, 0)

	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "t1", MenuItemName: "Steak", Price: 40, Quantity: 1, Subtotal: 40},
		{ID: "t2", MenuItemName: "Wine", Price: 25, Quantity: 2, Subtotal: 50},
	})

	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", ""))

	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	// subtotal = 90, tax = 90 * 8.25/100 = 7.425, service = 90 * 3.5/100 = 3.15
	assert.InDelta(t, 9000.0, savedBill.Subtotal, 0.01)
	assert.InDelta(t, 743.0, savedBill.TaxAmount, 0.01)
	assert.InDelta(t, 315.0, savedBill.ServiceFeeAmount, 0.01)
	assert.InDelta(t, 10058.0, savedBill.TotalAmount, 0.01)
}

func TestUpdateOrderStatus_ZeroTaxAndServiceFee(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "z1", MenuItemName: "Water", Price: 5, Quantity: 1, Subtotal: 5},
	})

	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", ""))

	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	assert.InDelta(t, 500.0, savedBill.Subtotal, 0.01)
	assert.InDelta(t, 0.0, savedBill.TaxAmount, 0.01)
	assert.InDelta(t, 0.0, savedBill.ServiceFeeAmount, 0.01)
	assert.InDelta(t, 500.0, savedBill.TotalAmount, 0.01)
}

func TestUpdateOrderStatus_CumulativeTotalsMultipleOrders(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 10, 0)

	existingItems := []BillItem{
		{ID: "pre-1", MenuItemID: "coffee", Name: "Coffee", Price: 5, Quantity: 2, Subtotal: 10, ItemType: "menu_item"},
	}
	bill := helperBill(t, biz, existingItems, 10) // starts with $10 subtotal

	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "c1", MenuItemName: "Cake", Price: 15, Quantity: 1, Subtotal: 15},
	})

	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", ""))

	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	// subtotal = 10 (existing) + 15 (new) = 25
	assert.InDelta(t, 2500.0, savedBill.Subtotal, 0.01)
	assert.InDelta(t, 250.0, savedBill.TaxAmount, 0.01)
	assert.InDelta(t, 2750.0, savedBill.TotalAmount, 0.01)
}

// ─── CONCURRENT APPROVAL (RACE CONDITION) ───

func TestUpdateOrderStatus_ConcurrentReApproval(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "race-1", MenuItemName: "Burger", Price: 10, Quantity: 1, Subtotal: 10},
	})

	// Fire 10 concurrent approvals of the same order
	var wg sync.WaitGroup
	errors := make([]error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errors[idx] = UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", "")
		}(i)
	}
	wg.Wait()

	// All should succeed (first one actually approves, rest are no-ops)
	for i, err := range errors {
		assert.NoError(t, err, "concurrent approval %d should not error", i)
	}

	// Items must appear exactly once
	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	var billItems []BillItem
	require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &billItems))
	assert.Equal(t, 1, len(billItems), "concurrent re-approvals must not duplicate items")
	assert.InDelta(t, 1000.0, savedBill.Subtotal, 0.01)
}

// ─── LARGE ORDER ───

func TestUpdateOrderStatus_LargeOrder(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 5, 2)
	bill := helperBill(t, biz, nil, 0)

	// Create 50-item order
	items := make([]OrderItem, 50)
	for i := range items {
		items[i] = OrderItem{
			ID:           "large-" + time.Now().Format("150405") + "-" + string(rune('A'+i%26)),
			MenuItemName: "Item",
			Price:        float64(i + 1),
			Quantity:     1,
			Subtotal:     float64(i + 1),
		}
	}
	order := helperOrder(t, biz, bill, items)

	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", ""))

	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	var billItems []BillItem
	require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &billItems))
	assert.Equal(t, 50, len(billItems))

	// Sum: 1+2+...+50 = 1275
	assert.InDelta(t, 127500.0, savedBill.Subtotal, 0.01)
}

// ─── BILL ITEMS RELATION INTEGRITY ───

func TestUpdateOrderStatus_RelationalItemsSyncedCorrectly(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)

	existingItems := []BillItem{
		{ID: "rel-pre-1", MenuItemID: "soup", Name: "Soup", Price: 6, Quantity: 1, Subtotal: 6, ItemType: "menu_item"},
	}
	bill := helperBill(t, biz, existingItems, 6)

	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "rel-new-1", MenuItemName: "Salad", Price: 8, Quantity: 1, Subtotal: 8},
	})

	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", ""))

	// Verify relational table has exactly 2 items
	var relItems []BillItem
	require.NoError(t, db.Where("bill_id = ?", bill.ID).Order("name").Find(&relItems).Error)
	assert.Equal(t, 2, len(relItems))

	names := []string{relItems[0].Name, relItems[1].Name}
	assert.Contains(t, names, "Soup")
	assert.Contains(t, names, "Salad")
}

// ─── CANCEL AFTER APPROVAL ───

func TestUpdateOrderStatus_CancelAfterApproval(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)
	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "ca-1", MenuItemName: "Steak", Price: 30, Quantity: 1, Subtotal: 30},
	})

	// Approve first
	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", ""))

	// Cancel after approval — the order's items must be REMOVED from the bill and
	// the total recomputed, so the guest is not charged for a cancelled order.
	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusOrderCancelled, "staff", "Staff mistake"))

	var updated Order
	require.NoError(t, db.First(&updated, order.ID).Error)
	assert.Equal(t, OrderStatusOrderCancelled, updated.Status)
	assert.Equal(t, "staff", updated.CancelledBy)
	assert.Equal(t, "Staff mistake", updated.CancelReason)
	assert.NotNil(t, updated.CancelledAt)

	// The cancelled order's items are removed and totals recomputed back to zero.
	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	assert.InDelta(t, 0.0, savedBill.Subtotal, 0.01, "cancelled order items must be removed from the bill")
	assert.InDelta(t, 0.0, savedBill.TotalAmount, 0.01, "total must be recomputed after removing cancelled items")

	// Both stores (JSON snapshot + relational rows) must be cleared in sync.
	var jsonItems []BillItem
	require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &jsonItems))
	assert.Empty(t, jsonItems, "JSON snapshot must drop the cancelled order's items")
	var relItems []BillItem
	require.NoError(t, db.Where("bill_id = ?", bill.ID).Find(&relItems).Error)
	assert.Empty(t, relItems, "relational rows for the cancelled order must be deleted")

	var history []BillHistoryEvent
	require.NoError(t, db.Where("bill_id = ?", bill.ID).Order("id").Find(&history).Error)
	require.Len(t, history, 2)
	assert.Equal(t, BillHistoryEventOrderApproved, history[0].EventType)
	assert.Equal(t, BillHistoryEventOrderCanceled, history[1].EventType)
	assert.Equal(t, "Staff mistake", history[1].Reason)
}

// TestUpdateOrderStatus_CancelOneOfTwoApprovedOrders proves cancelling one
// approved order removes ONLY that order's items, leaving a second approved
// order's items (and any pre-existing items) on the bill with a correct total.
func TestUpdateOrderStatus_CancelOneOfTwoApprovedOrders(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 10, 0) // 10% tax

	existingItems := []BillItem{
		{ID: "keep-pre-1", MenuItemID: "coffee", Name: "Coffee", Price: 4, Quantity: 1, Subtotal: 4, ItemType: "menu_item"},
	}
	bill := helperBill(t, biz, existingItems, 4)

	order1 := helperOrder(t, biz, bill, []OrderItem{
		{ID: "o1-burger", MenuItemName: "Burger", Price: 10, Quantity: 1, Subtotal: 10},
	})
	order2 := helperOrder(t, biz, bill, []OrderItem{
		{ID: "o2-pizza", MenuItemName: "Pizza", Price: 16, Quantity: 1, Subtotal: 16},
	})
	require.NoError(t, UpdateOrderStatus(order1.ID, OrderStatusApproved, "staff", ""))
	require.NoError(t, UpdateOrderStatus(order2.ID, OrderStatusApproved, "staff", ""))

	// Cancel order1 only → its $10 burger leaves; Coffee ($4) + Pizza ($16) remain.
	require.NoError(t, UpdateOrderStatus(order1.ID, OrderStatusOrderCancelled, "staff", "out of stock"))

	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	assert.InDelta(t, 2000.0, savedBill.Subtotal, 0.01, "subtotal = Coffee 4 + Pizza 16 = 20")
	assert.InDelta(t, 200.0, savedBill.TaxAmount, 0.01, "10% of 20")
	assert.InDelta(t, 2200.0, savedBill.TotalAmount, 0.01)

	var relItems []BillItem
	require.NoError(t, db.Where("bill_id = ?", bill.ID).Order("name").Find(&relItems).Error)
	require.Len(t, relItems, 2)
	names := []string{relItems[0].Name, relItems[1].Name}
	assert.Contains(t, names, "Coffee")
	assert.Contains(t, names, "Pizza")
	assert.NotContains(t, names, "Burger")
}

// TestUpdateOrderStatus_CancelApprovedOrderPreservesLoyaltyDiscount proves the
// recompute on cancel goes through the discount-aware path: a loyalty redemption
// already applied to the bill must remain applied (net total), not be silently
// erased by recomputing a gross total.
func TestUpdateOrderStatus_CancelApprovedOrderPreservesLoyaltyDiscount(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)

	existingItems := []BillItem{
		{ID: "loy-pre-1", MenuItemID: "coffee", Name: "Coffee", Price: 20, Quantity: 1, Subtotal: 20, ItemType: "menu_item"},
	}
	bill := helperBill(t, biz, existingItems, 20)
	// Apply a $3 loyalty discount to the bill (gross 2000 -> net 1700).
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"loyalty_discount_cents": 300,
		"total_amount":           1700,
	}).Error)

	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "loy-burger", MenuItemName: "Burger", Price: 10, Quantity: 1, Subtotal: 10},
	})
	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", ""))
	// After approval: gross 3000, net 2700.
	var afterApprove Bill
	require.NoError(t, db.First(&afterApprove, bill.ID).Error)
	assert.InDelta(t, 2700.0, afterApprove.TotalAmount, 0.01, "approve keeps discount applied")

	// Cancel the order → only Coffee ($20) remains: gross 2000, net 2000-300 = 1700.
	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusOrderCancelled, "staff", "changed mind"))

	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	assert.InDelta(t, 2000.0, savedBill.Subtotal, 0.01)
	assert.Equal(t, int64(300), savedBill.LoyaltyDiscountCents, "discount must remain applied")
	assert.InDelta(t, 1700.0, savedBill.TotalAmount, 0.01, "total stays net of the loyalty discount (2000 - 300)")
}

// B-8: legacy bills can carry items that exist ONLY in the bills.items JSON
// snapshot (no relational bill_items row). Cancelling an approved order must
// not silently drop them when it rebuilds the snapshot.
func TestUpdateOrderStatus_CancelPreservesLegacyJSONOnlyItems(t *testing.T) {
	setupOrderTestDB(t)
	business := helperBusiness(t, 0, 0)

	legacy := BillItem{ID: "legacy-uuid-1", Name: "Legacy Soup", Price: 7, Quantity: 1, ItemType: "menu_item", Subtotal: 7}
	orderBillItem := BillItem{ID: "11111111-1111-4111-8111-111111111111", Name: "Burger", Price: 10, Quantity: 1, ItemType: "menu_item", Subtotal: 10}
	snapshot, err := json.Marshal([]BillItem{legacy, orderBillItem})
	require.NoError(t, err)

	bill := &Bill{
		BusinessID: business.ID, BillNumber: "B-legacy-1", Status: BillStatusOpen,
		Items: string(snapshot), Subtotal: 1700, TotalAmount: 1700,
	}
	require.NoError(t, db.Create(bill).Error)

	order := &Order{
		BillID: bill.ID, BusinessID: business.ID, OrderNumber: "O-legacy-1",
		Status: OrderStatusApproved, CreatedBy: "guest", Items: "[]",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(order).Error)

	rel := orderBillItem
	rel.BillID = bill.ID
	rel.OrderID = &order.ID
	require.NoError(t, db.Create(&rel).Error)

	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusOrderCancelled, "staff:1", "wrong table"))

	var updated Bill
	require.NoError(t, db.First(&updated, bill.ID).Error)
	var items []BillItem
	require.NoError(t, json.Unmarshal([]byte(updated.Items), &items))
	require.Len(t, items, 1, "legacy JSON-only item must survive the rebuild")
	assert.Equal(t, "legacy-uuid-1", items[0].ID)
	assert.Equal(t, int64(700), updated.Subtotal, "totals must include the preserved legacy item")
}
