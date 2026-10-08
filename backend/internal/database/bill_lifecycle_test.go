package database

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ─── FULL LIFECYCLE ───

func TestBillLifecycle_CreateOrderApprovePayClose(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 10, 5)
	bill := helperBill(t, biz, nil, 0)

	// 1. Create order
	orderItems := []OrderItem{
		{ID: "lc-1", MenuItemName: "Steak", Price: 30, Quantity: 1, Subtotal: 30},
		{ID: "lc-2", MenuItemName: "Wine", Price: 15, Quantity: 2, Subtotal: 30},
	}
	order := helperOrder(t, biz, bill, orderItems)

	// 2. Approve order, then finish kitchen. CloseBill refuses live tickets.
	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "manager", ""))
	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusInKitchen, "kitchen", ""))
	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusOrderReady, "kitchen", ""))
	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusOrderDelivered, "expo", ""))

	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	assert.InDelta(t, 6000.0, savedBill.Subtotal, 0.01)
	assert.InDelta(t, 600.0, savedBill.TaxAmount, 0.01)        // 10%
	assert.InDelta(t, 300.0, savedBill.ServiceFeeAmount, 0.01) // 5%
	assert.InDelta(t, 6900.0, savedBill.TotalAmount, 0.01)

	// 3. Simulate payment
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"paid_amount": 6900,
	}).Error)

	// 4. Close bill
	require.NoError(t, CloseBill(bill.ID))

	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	assert.Equal(t, BillStatusClosed, savedBill.Status)
	assert.NotNil(t, savedBill.ClosedAt)
	assert.NotNil(t, savedBill.SettledAt)
	assert.WithinDuration(t, *savedBill.ClosedAt, *savedBill.SettledAt, time.Millisecond)
}

func TestBillSettlementTimestampIsImmutableAcrossReopenAndResettle(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	firstSettlement := time.Date(2026, 7, 31, 23, 59, 30, 0, time.UTC)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return updateBillLifecycleTx(tx, bill.ID, map[string]interface{}{
			"status":    BillStatusPaid,
			"closed_at": &firstSettlement,
		})
	}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return updateBillLifecycleTx(tx, bill.ID, map[string]interface{}{
			"status":    BillStatusOpen,
			"closed_at": nil,
		})
	}))
	secondSettlement := firstSettlement.Add(24 * time.Hour)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return updateBillLifecycleTx(tx, bill.ID, map[string]interface{}{
			"status":    BillStatusPaid,
			"closed_at": &secondSettlement,
		})
	}))

	var saved Bill
	require.NoError(t, db.First(&saved, bill.ID).Error)
	require.NotNil(t, saved.SettledAt)
	require.WithinDuration(t, firstSettlement, *saved.SettledAt, time.Millisecond)
}

// ─── MULTIPLE ORDERS ───

func TestBillLifecycle_MultipleOrders(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 10, 0)
	bill := helperBill(t, biz, nil, 0)

	orders := [][]OrderItem{
		{{ID: "m1", MenuItemName: "Burger", Price: 10, Quantity: 1, Subtotal: 10}},
		{{ID: "m2", MenuItemName: "Pizza", Price: 15, Quantity: 1, Subtotal: 15}},
		{{ID: "m3", MenuItemName: "Pasta", Price: 12, Quantity: 2, Subtotal: 24}},
	}

	for _, items := range orders {
		o := helperOrder(t, biz, bill, items)
		require.NoError(t, UpdateOrderStatus(o.ID, OrderStatusApproved, "staff", ""))
	}

	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)

	var billItems []BillItem
	require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &billItems))
	assert.Equal(t, 3, len(billItems))

	// 10 + 15 + 24 = 49
	assert.InDelta(t, 4900.0, savedBill.Subtotal, 0.01)
	assert.InDelta(t, 490.0, savedBill.TaxAmount, 0.01)
	assert.InDelta(t, 5390.0, savedBill.TotalAmount, 0.01)

	// Verify relational items
	var relItems []BillItem
	require.NoError(t, db.Where("bill_id = ?", bill.ID).Find(&relItems).Error)
	assert.Equal(t, 3, len(relItems))
}

// ─── ORDER REJECTION ───

func TestBillLifecycle_OrderRejection(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	// First order approved
	o1 := helperOrder(t, biz, bill, []OrderItem{
		{ID: "r1", MenuItemName: "Soup", Price: 8, Quantity: 1, Subtotal: 8},
	})
	require.NoError(t, UpdateOrderStatus(o1.ID, OrderStatusApproved, "staff", ""))

	// Second order cancelled
	o2 := helperOrder(t, biz, bill, []OrderItem{
		{ID: "r2", MenuItemName: "Salad", Price: 12, Quantity: 1, Subtotal: 12},
	})
	require.NoError(t, UpdateOrderStatus(o2.ID, OrderStatusOrderCancelled, "staff", "Sold out"))

	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)

	// Only the approved order's items should appear
	var billItems []BillItem
	require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &billItems))
	assert.Equal(t, 1, len(billItems))
	assert.Equal(t, "Soup", billItems[0].Name)
	assert.InDelta(t, 800.0, savedBill.Subtotal, 0.01)
}

// ─── APPEND TO EXISTING ITEMS ───

func TestBillLifecycle_AppendToExistingItems(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)

	existing := []BillItem{
		{ID: "ex-1", MenuItemID: "coffee", Name: "Coffee", Price: 4, Quantity: 2, Subtotal: 8, ItemType: "menu_item"},
		{ID: "ex-2", MenuItemID: "tea", Name: "Tea", Price: 3, Quantity: 1, Subtotal: 3, ItemType: "menu_item"},
	}
	bill := helperBill(t, biz, existing, 11)

	// Append via order
	o := helperOrder(t, biz, bill, []OrderItem{
		{ID: "new-1", MenuItemName: "Cake", Price: 7, Quantity: 1, Subtotal: 7},
	})
	require.NoError(t, UpdateOrderStatus(o.ID, OrderStatusApproved, "staff", ""))

	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)

	var billItems []BillItem
	require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &billItems))
	assert.Equal(t, 3, len(billItems), "should have 2 existing + 1 new")

	names := make([]string, len(billItems))
	for i, item := range billItems {
		names[i] = item.Name
	}
	assert.Contains(t, names, "Coffee")
	assert.Contains(t, names, "Tea")
	assert.Contains(t, names, "Cake")

	assert.InDelta(t, 1800.0, savedBill.Subtotal, 0.01)

	// Verify relational table also has 3 items
	var relItems []BillItem
	require.NoError(t, db.Where("bill_id = ?", bill.ID).Find(&relItems).Error)
	assert.Equal(t, 3, len(relItems))
}

// ─── BILL NUMBER GENERATION ───

func TestGenerateUniqueBillNumber(t *testing.T) {
	bn1, err := generateUniqueBillNumber(1)
	require.NoError(t, err)
	assert.Contains(t, bn1, "B1-")
	assert.Len(t, bn1, 15) // "B1-" (3) + 12 chars = 15

	bn2, err := generateUniqueBillNumber(1)
	require.NoError(t, err)
	assert.NotEqual(t, bn1, bn2, "bill numbers should be unique")
}

// ─── CREATE BILL ───

func TestCreateBill_GeneratesBillNumber(t *testing.T) {
	setupTestDB(t)
	business := &Business{Name: "Test Resto"}
	require.NoError(t, db.Create(business).Error)

	bill := &Bill{
		BusinessID: business.ID,
		Status:     BillStatusOpen,
	}
	items := []BillItem{
		{MenuItemID: "i1", Name: "Water", Price: 0, Quantity: 1, Subtotal: 0},
	}

	err := CreateBill(bill, items)
	require.NoError(t, err)
	assert.NotEmpty(t, bill.BillNumber)
	assert.Contains(t, bill.BillNumber, "B")
}

// ─── CLOSE BILL ───

func TestCloseBill(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	require.NoError(t, CloseBill(bill.ID))

	var saved Bill
	require.NoError(t, db.First(&saved, bill.ID).Error)
	assert.Equal(t, BillStatusVoided, saved.Status, "empty $0 walk-in close is a void, not a closed check")
	assert.NotNil(t, saved.ClosedAt)
}

// ─── GET BILL ITEMS PREFER RELATIONAL ───

func TestGetBillItemsPreferRelational_FromRelation(t *testing.T) {
	setupTestDB(t)
	business := &Business{Name: "Test Resto"}
	require.NoError(t, db.Create(business).Error)

	bill := &Bill{
		BusinessID: business.ID,
		BillNumber: "B-rel-test-" + time.Now().Format("150405"),
		Status:     BillStatusOpen,
		Items:      `[{"name":"JSONBurger","price":10,"quantity":1,"subtotal":10}]`,
	}
	require.NoError(t, db.Create(bill).Error)

	// Insert relational items
	relItem := BillItem{
		ID:       "rel-item-1",
		BillID:   bill.ID,
		Name:     "RelBurger",
		Price:    10,
		Quantity: 1,
		Subtotal: 10,
	}
	require.NoError(t, db.Create(&relItem).Error)

	items, err := GetBillItemsPreferRelational(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, len(items))
	assert.Equal(t, "RelBurger", items[0].Name, "should prefer relational data over JSON")
}

func TestGetBillItemsPreferRelational_FallbackToJSON(t *testing.T) {
	setupTestDB(t)
	business := &Business{Name: "Test Resto"}
	require.NoError(t, db.Create(business).Error)

	bill := &Bill{
		BusinessID: business.ID,
		BillNumber: "B-json-test-" + time.Now().Format("150405"),
		Status:     BillStatusOpen,
		Items:      `[{"name":"LegacyBurger","price":10,"quantity":1,"subtotal":10}]`,
	}
	require.NoError(t, db.Create(bill).Error)

	// No relational items — should fall back to JSON
	items, err := GetBillItemsPreferRelational(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, len(items))
	assert.Equal(t, "LegacyBurger", items[0].Name)
}
