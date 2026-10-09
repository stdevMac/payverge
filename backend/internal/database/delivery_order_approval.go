package database

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm/clause"
)

// ApproveDeliveryLinkedOrder transitions a guest-delivery order from
// pending → approved WITHOUT the bill-item append and total recompute that
// UpdateOrderStatus performs on the approve path.
//
// Guest delivery checkout already wrote the bill with the correct items and a
// total_amount that includes the delivery fee and driver tip. Running the normal
// approve path would double every line item on the bill and recompute
// total_amount from item subtotals alone — silently erasing the fee and tip.
//
// This function mirrors UpdateOrderStatus for everything that still applies:
//   - FOR UPDATE row locking to prevent TOCTOU races
//   - ValidateTransition state-machine check
//   - Inventory deduction (the kitchen-start moment)
//   - approved_by / approved_at stamping (matching the UpdateOrderStatus convention
//     of setting them only when actor is non-empty)
//   - Bill history event (BillHistoryEventOrderApproved) via createBillHistoryEventsTx
//
// Approving an already-approved order is a no-op.
func ApproveDeliveryLinkedOrder(orderID uint, actor string) error {
	tx := db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Read current order with a real FOR UPDATE row lock to prevent concurrent
	// double-approval (the old gorm:query_option Set was a v1 no-op, B-1).
	var order Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, orderID).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to get order: %w", err)
	}

	// No-op guard: already approved is the idempotent case.
	if order.Status == OrderStatusApproved {
		tx.Rollback()
		return nil
	}

	// Validate the state machine allows pending → approved.
	if err := ValidateTransition(order.Status, OrderStatusApproved); err != nil {
		tx.Rollback()
		return err
	}

	// Parse order items for inventory deduction.
	var orderItems []OrderItem
	if order.Items != "" {
		if err := json.Unmarshal([]byte(order.Items), &orderItems); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to unmarshal order items: %w", err)
		}
	}

	// Deduct inventory — the kitchen-start moment. This matches what
	// UpdateOrderStatus does on the approve path before it writes bill items.
	if err := DeductApprovedOrderInventoryTx(tx, &order, orderItems, actor); err != nil {
		tx.Rollback()
		return err
	}

	// Stamp order approved. Mirror UpdateOrderStatus: only set approved_by /
	// approved_at when actor is non-empty.
	now := time.Now()
	updates := map[string]interface{}{
		"status":     OrderStatusApproved,
		"updated_at": now,
	}
	if actor != "" {
		updates["approved_by"] = actor
		updates["approved_at"] = now
	}

	// Conditional on the locked-read status — same belt-and-suspenders as
	// UpdateOrderStatus: a committed cancel can never be overwritten.
	approveUpdate := tx.Model(&Order{}).
		Where("id = ? AND status = ?", orderID, order.Status).
		Updates(updates)
	if approveUpdate.Error != nil {
		tx.Rollback()
		return fmt.Errorf("failed to approve delivery order: %w", approveUpdate.Error)
	}
	if approveUpdate.RowsAffected == 0 {
		tx.Rollback()
		return fmt.Errorf("%w: order %d changed concurrently (expected status %q)", ErrInvalidStatusTransition, orderID, order.Status)
	}

	// Write a bill history event using the same helper UpdateOrderStatus uses,
	// so audit consumers see a consistent event type. We note delivery_linked in
	// Details so operators can distinguish this approval path from a dine-in approve.
	historyBill := &Bill{ID: order.BillID, BusinessID: order.BusinessID}
	historyEvents := []BillHistoryEvent{
		{
			BillID:      order.BillID,
			BusinessID:  order.BusinessID,
			EventType:   BillHistoryEventOrderApproved,
			Actor:       actor,
			OrderID:     &order.ID,
			OrderNumber: order.OrderNumber,
			Details: map[string]interface{}{
				"delivery_linked": true,
				// items_count not items_added: delivery bill items already exist — approval adds nothing.
				"items_count": len(orderItems),
			},
		},
	}

	if err := createBillHistoryEventsTx(tx, historyBill, historyEvents); err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit delivery order approval: %w", err)
	}

	return nil
}
