package database

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestApproveDeliveryLinkedOrder_EmptyActor verifies that passing an empty actor
// string still transitions the order to approved but leaves approved_by empty
// and approved_at nil, matching the UpdateOrderStatus convention.
func TestApproveDeliveryLinkedOrder_EmptyActor(t *testing.T) {
	setupOrderTestDB(t)

	biz := helperBusiness(t, 0, 0)

	bill := &Bill{
		BusinessID:  biz.ID,
		BillNumber:  "B-DLV-ACTOR",
		Status:      BillStatusOpen,
		Items:       "[]",
		Subtotal:    500,
		TotalAmount: 500,
	}
	require.NoError(t, db.Create(bill).Error)

	order := helperOrder(t, biz, bill, []OrderItem{})

	err := ApproveDeliveryLinkedOrder(order.ID, "")
	require.NoError(t, err)

	var got Order
	require.NoError(t, db.First(&got, order.ID).Error)
	if got.Status != OrderStatusApproved {
		t.Fatalf("order status = %s, want %s", got.Status, OrderStatusApproved)
	}
	if got.ApprovedBy != "" {
		t.Fatalf("approved_by = %q, want empty string when actor is empty", got.ApprovedBy)
	}
	if got.ApprovedAt != nil {
		t.Fatalf("approved_at = %v, want nil when actor is empty", got.ApprovedAt)
	}
}

// TestApproveDeliveryLinkedOrder_InvalidTransition verifies that calling
// ApproveDeliveryLinkedOrder on an order in a terminal state (cancelled) returns
// an error and leaves the order status unchanged.
func TestApproveDeliveryLinkedOrder_InvalidTransition(t *testing.T) {
	setupOrderTestDB(t)

	biz := helperBusiness(t, 0, 0)

	bill := &Bill{
		BusinessID:  biz.ID,
		BillNumber:  "B-DLV-CANCEL",
		Status:      BillStatusOpen,
		Items:       "[]",
		Subtotal:    500,
		TotalAmount: 500,
	}
	require.NoError(t, db.Create(bill).Error)

	order := helperOrder(t, biz, bill, []OrderItem{})

	// Force the order into cancelled — a terminal state with no outgoing transitions.
	require.NoError(t, db.Model(&Order{}).Where("id = ?", order.ID).
		Update("status", OrderStatusOrderCancelled).Error)

	err := ApproveDeliveryLinkedOrder(order.ID, "staff:1")
	if err == nil {
		t.Fatal("expected an error approving a cancelled order, got nil")
	}

	var got Order
	require.NoError(t, db.First(&got, order.ID).Error)
	if got.Status != OrderStatusOrderCancelled {
		t.Fatalf("order status = %s, want %s (must be unchanged after failed transition)",
			got.Status, OrderStatusOrderCancelled)
	}
}

// TestApproveDeliveryLinkedOrder_DoesNotTouchBill verifies that
// ApproveDeliveryLinkedOrder transitions the order pending→approved and deducts
// inventory WITHOUT appending items to the bill or recomputing its totals.
// Guest delivery checkout already wrote the bill with the correct items +
// total_amount (which includes the delivery fee and driver tip). Calling the
// normal UpdateOrderStatus path on approve would double the line items and
// recompute total_amount from item subtotals alone, erasing the fee/tip.
func TestApproveDeliveryLinkedOrder_DoesNotTouchBill(t *testing.T) {
	// Reuse the same in-memory SQLite fixture as the rest of the database package
	// order tests.
	setupOrderTestDB(t)

	biz := helperBusiness(t, 0, 0)

	// Bill written at checkout: one $10 pizza item + $5 delivery fee + $2 driver tip = $17 total.
	// Stored as cents per the money contract.
	pizzaItems := []BillItem{
		{ID: "i1", Name: "Pizza", Price: 10, Quantity: 1, Subtotal: 10, ItemType: "menu_item"},
	}
	itemsJSON, err := json.Marshal(pizzaItems)
	require.NoError(t, err)

	bill := &Bill{
		BusinessID:  biz.ID,
		BillNumber:  "B-DLV-1",
		Status:      BillStatusOpen,
		Items:       string(itemsJSON),
		Subtotal:    1000, // $10.00 in cents
		TaxAmount:   0,
		TotalAmount: 1700, // $17.00 in cents — includes $5 fee + $2 tip
	}
	require.NoError(t, db.Create(bill).Error)

	// Order mirrors the checkout items.
	orderItems := []OrderItem{
		{ID: "i1", MenuItemName: "Pizza", Price: 10, Quantity: 1, Subtotal: 10},
	}
	order := helperOrder(t, biz, bill, orderItems)

	// --- Act ---
	err = ApproveDeliveryLinkedOrder(order.ID, "staff:1")
	require.NoError(t, err)

	// --- Assert: order transitioned to approved ---
	var gotOrder Order
	require.NoError(t, db.First(&gotOrder, order.ID).Error)
	if gotOrder.Status != OrderStatusApproved {
		t.Fatalf("order status = %s, want %s", gotOrder.Status, OrderStatusApproved)
	}
	if gotOrder.ApprovedBy != "staff:1" {
		t.Fatalf("approved_by = %q, want %q", gotOrder.ApprovedBy, "staff:1")
	}
	if gotOrder.ApprovedAt == nil {
		t.Fatal("approved_at must be set")
	}

	// --- Assert: bill untouched ---
	var gotBill Bill
	require.NoError(t, db.First(&gotBill, bill.ID).Error)

	if gotBill.TotalAmount != 1700 {
		t.Fatalf("bill total_amount mutated: got %d want 1700 (delivery fee/tip erased)", gotBill.TotalAmount)
	}
	if gotBill.Subtotal != 1000 {
		t.Fatalf("bill subtotal mutated: got %d want 1000 (items doubled)", gotBill.Subtotal)
	}
	if gotBill.Items != string(itemsJSON) {
		t.Fatalf("bill items mutated — delivery approval must not append items\ngot:  %s\nwant: %s",
			gotBill.Items, string(itemsJSON))
	}

	// --- Assert: idempotent — second call is a no-op, not an error ---
	err = ApproveDeliveryLinkedOrder(order.ID, "staff:1")
	if err != nil {
		t.Fatalf("re-approve must be a no-op: %v", err)
	}

	// Bill still untouched after the no-op re-approve.
	var gotBill2 Bill
	require.NoError(t, db.First(&gotBill2, bill.ID).Error)
	if gotBill2.TotalAmount != 1700 || gotBill2.Subtotal != 1000 {
		t.Fatalf("re-approve mutated the bill: subtotal=%d total=%d", gotBill2.Subtotal, gotBill2.TotalAmount)
	}
}

// TestUpdateOrderStatus_RefusesDeliveryLinkedApprove verifies that the DB-layer
// guard in UpdateOrderStatus refuses to approve an order that has a delivery_orders
// row pointing at it. Such orders must go through ApproveDeliveryLinkedOrder; the
// normal approve path would double-bill by appending items and recomputing totals.
func TestUpdateOrderStatus_RefusesDeliveryLinkedApprove(t *testing.T) {
	setupOrderTestDB(t)

	biz := helperBusiness(t, 0, 0)

	bill := &Bill{
		BusinessID:  biz.ID,
		BillNumber:  "B-DLV-GUARD",
		Status:      BillStatusOpen,
		Items:       "[]",
		Subtotal:    1500,
		TotalAmount: 1700, // includes delivery fee + tip
	}
	require.NoError(t, db.Create(bill).Error)

	order := helperOrder(t, biz, bill, []OrderItem{})

	// Seed the delivery_orders row that links this order — this is what the guard checks.
	delivery := &DeliveryOrder{
		BusinessID:     biz.ID,
		BillID:         bill.ID,
		OrderID:        &order.ID,
		DeliveryNumber: "DLV-GUARD-1",
		DeliveryType:   DeliveryTypeInHouse,
		Status:         DeliveryStatusPending,
		CustomerName:   "Test Customer",
		CustomerPhone:  "+15555555555",
	}
	require.NoError(t, db.Create(delivery).Error)

	// --- Act: attempt to approve via the standard path ---
	err := UpdateOrderStatus(order.ID, OrderStatusApproved, "staff:1", "")

	// --- Assert: error must fire ---
	if err == nil {
		t.Fatal("UpdateOrderStatus must refuse to approve a delivery-linked order, got nil error")
	}
	if !strings.Contains(err.Error(), "delivery-linked") {
		t.Fatalf("expected 'delivery-linked' in error, got: %v", err)
	}

	// --- Assert: order and bill are unchanged ---
	var gotOrder Order
	require.NoError(t, db.First(&gotOrder, order.ID).Error)
	if gotOrder.Status != OrderStatusPending {
		t.Fatalf("order status = %s, want %s (must not have been mutated)", gotOrder.Status, OrderStatusPending)
	}

	var gotBill Bill
	require.NoError(t, db.First(&gotBill, bill.ID).Error)
	if gotBill.TotalAmount != 1700 {
		t.Fatalf("bill total_amount mutated to %d, want 1700", gotBill.TotalAmount)
	}
	if gotBill.Subtotal != 1500 {
		t.Fatalf("bill subtotal mutated to %d, want 1500", gotBill.Subtotal)
	}
}
