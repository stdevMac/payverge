package database

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// CancelReasonBillClosed is stamped on orders auto-cancelled because their
// bill was closed or voided while they were still pending (B-7). The Stage 3
// guest UI maps this exact string to a translated "bill was closed" message.
const CancelReasonBillClosed = "bill_closed"

// cancelPendingOrdersForClosedBillTx cancels every still-pending order on a
// bill inside the caller's close/void transaction. This removes the
// "unapprovable forever" class: a pending order on a closed bill could never
// be approved (bill-not-open 409) and sat in the queue permanently. Pending
// orders never deducted inventory and never put items on the bill, so no
// inventory restore or bill recompute is needed — just status, history, and
// alert resolution.
func cancelPendingOrdersForClosedBillTx(tx *gorm.DB, bill *Bill, actor string) error {
	var pending []Order
	if err := tx.Where("bill_id = ? AND status = ?", bill.ID, OrderStatusPending).Find(&pending).Error; err != nil {
		return fmt.Errorf("failed to load pending orders for closed bill: %w", err)
	}
	if len(pending) == 0 {
		return nil
	}

	now := time.Now()
	ids := make([]uint, 0, len(pending))
	for _, o := range pending {
		ids = append(ids, o.ID)
	}
	if err := tx.Model(&Order{}).
		Where("id IN ? AND status = ?", ids, OrderStatusPending).
		Updates(map[string]interface{}{
			"status":        OrderStatusOrderCancelled,
			"cancelled_by":  actor,
			"cancel_reason": CancelReasonBillClosed,
			"cancelled_at":  now,
			"updated_at":    now,
		}).Error; err != nil {
		return fmt.Errorf("failed to cancel pending orders for closed bill: %w", err)
	}

	events := make([]BillHistoryEvent, 0, len(pending))
	for i := range pending {
		order := pending[i]
		events = append(events, BillHistoryEvent{
			BillID:      bill.ID,
			BusinessID:  bill.BusinessID,
			EventType:   BillHistoryEventOrderCanceled,
			Actor:       actor,
			Reason:      CancelReasonBillClosed,
			OrderID:     &order.ID,
			OrderNumber: order.OrderNumber,
			Details: map[string]interface{}{
				"previous_status":   string(OrderStatusPending),
				"had_been_approved": false,
				"auto_cancelled":    true,
			},
		})
	}
	if err := createBillHistoryEventsTx(tx, &Bill{ID: bill.ID, BusinessID: bill.BusinessID}, events); err != nil {
		return err
	}

	return resolveOrderAlertsTx(tx, bill.BusinessID, ids, actor)
}

// operationalAlertResolvedPublisher lets the operational_alerts service
// package receive the alert rows that resolveOrderAlertsTx batch-resolves so
// it can emit the same "alert.resolved" SSE frame publishAlert produces.
// The indirection exists because of the import topology: operational_alerts
// imports database, and internal/events also imports database (sse_handler),
// so this package can publish to the hub only through a hook set from above.
// The hook is registered in operational_alerts' init(); when unset (e.g. in
// database-only tests) resolution still works, just without frames.
var operationalAlertResolvedPublisher func(alert OperationalAlert)

// SetOperationalAlertResolvedPublisher registers the callback invoked with
// each alert row batch-resolved by resolveOrderAlertsTx. Called once at init
// by the operational_alerts service package.
func SetOperationalAlertResolvedPublisher(fn func(alert OperationalAlert)) {
	operationalAlertResolvedPublisher = fn
}

// resolveOrderAlertsTx resolves open/claimed order operational alerts inside
// the caller's transaction. The operational_alerts service cannot be used
// here (it imports database — cycle), so the resolution writes the same
// status fields + event rows the service writes, then hands the resolved
// rows to operationalAlertResolvedPublisher so alert.resolved SSE frames go
// out and the FE's urgent repeating alarm stops immediately.
//
// The publish happens after this helper's writes succeed but still inside
// the enclosing transaction (callers own the commit and cannot be reached
// from here). Frames are advisory — if the enclosing tx rolls back after
// this point (rare: only later steps of the close/void can fail), the FE
// self-corrects on its next poll; the opposite failure mode (alarm ringing
// until the next poll on every bill close) is what this fixes.
func resolveOrderAlertsTx(tx *gorm.DB, businessID uint, orderIDs []uint, actor string) error {
	if len(orderIDs) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(orderIDs))
	for _, id := range orderIDs {
		ids = append(ids, int64(id))
	}

	var alerts []OperationalAlert
	if err := tx.Where(
		"business_id = ? AND resource_type = ? AND resource_id IN ? AND status IN ?",
		businessID, OperationalAlertResourceTypeOrder, ids,
		[]OperationalAlertStatus{OperationalAlertStatusOpen, OperationalAlertStatusClaimed},
	).Find(&alerts).Error; err != nil {
		return fmt.Errorf("failed to load order alerts for closed bill: %w", err)
	}
	if len(alerts) == 0 {
		return nil
	}

	now := time.Now()
	metadata, _ := json.Marshal(map[string]string{"reason": CancelReasonBillClosed})

	// Collect alert IDs and batch-resolve all alerts in one UPDATE.
	alertIDs := make([]uint, 0, len(alerts))
	for _, alert := range alerts {
		alertIDs = append(alertIDs, alert.ID)
	}
	if err := tx.Model(&OperationalAlert{}).
		Where("id IN ?", alertIDs).
		Updates(map[string]interface{}{
			"status":        OperationalAlertStatusResolved,
			"resolved_at":   &now,
			"last_event_at": now,
			"updated_at":    now,
		}).Error; err != nil {
		return fmt.Errorf("failed to batch-resolve order alerts for closed bill: %w", err)
	}

	// Build one event row per alert and batch-insert them in a single statement.
	events := make([]OperationalAlertEvent, 0, len(alerts))
	for _, alert := range alerts {
		events = append(events, OperationalAlertEvent{
			AlertID:    alert.ID,
			BusinessID: alert.BusinessID,
			EventType:  OperationalAlertEventTypeResolved,
			ActorName:  actor,
			Metadata:   JSONRawMessage(metadata),
		})
	}
	if err := tx.Create(&events).Error; err != nil {
		return fmt.Errorf("failed to batch-insert alert resolution events for closed bill: %w", err)
	}

	if operationalAlertResolvedPublisher != nil {
		for i := range alerts {
			// Mirror the batch UPDATE onto the in-memory rows so the published
			// payload matches what a re-read would return (full row shape, same
			// as operational_alerts.publishAlert).
			alerts[i].Status = OperationalAlertStatusResolved
			alerts[i].ResolvedAt = &now
			alerts[i].LastEventAt = now
			alerts[i].UpdatedAt = now
			operationalAlertResolvedPublisher(alerts[i])
		}
	}
	return nil
}
