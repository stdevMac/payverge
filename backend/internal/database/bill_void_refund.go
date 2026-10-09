package database

// IMP-15: bill-level void and per-payment refund.
//
// VoidBill cancels an unpaid bill (no confirmed payments yet) and flips it to
// BillStatusVoided. RefundBillPayment reverses a single confirmed payment row
// on a bill and decrements bill.paid_amount accordingly.
//
// Both helpers run inside a single transaction with row-level locking on the
// bill (and the target payment, where applicable) to serialize concurrent
// edits with AdjustBillItem / ConfirmPendingAlternativePayment / CloseBill.
// They also write a BillHistoryEvent so the existing audit timeline picks up
// the action, and they leave the comp_void_audit row to the middleware in
// server/manager_pin.go (which already wrote one before the handler ran).

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BillHistoryEventBillVoided and BillHistoryEventPaymentRefunded extend the
// existing history-event vocabulary so the bill-details modal can surface
// IMP-15 actions in the timeline alongside item voids and bill closures.
const (
	BillHistoryEventBillVoided      = "bill.voided"
	BillHistoryEventPaymentRefunded = "payment.refunded"
)

// ErrBillCannotBeVoided is returned when VoidBill is called on a bill that
// already has at least one confirmed payment (operators must refund those
// payments individually instead) or on a bill that is already voided/closed.
var ErrBillCannotBeVoided = errors.New("bill has payments or is already closed; refund payments instead of voiding")

// ErrPaymentNotRefundable is returned when RefundBillPayment is asked to
// refund a payment that is already reversed/refunded, never confirmed, or
// belongs to a different bill.
var ErrPaymentNotRefundable = errors.New("payment is not in a refundable state")

// ErrBillVoidLiveKitchenTickets is returned when VoidBill is called while expo
// still owes this check food (approved / in_kitchen / ready). Voiding would
// leave those tickets attached to a terminal bill: the line keeps plating,
// inventory is already deducted, and there is no open check left to charge
// against (#704 — same class as the merged-away bill 1132, different door).
// The same rule CloseBillWithHistory and ClearTable apply on their doors.
var ErrBillVoidLiveKitchenTickets = errors.New("bill still has live kitchen tickets; bump or cancel them before voiding")

// MarkPaymentRefundPending atomically claims a confirmed payment for an
// external provider refund. The PSP call happens outside this transaction, but
// duplicate requests will now see refund_pending instead of confirmed and will
// not call the provider a second time.
func MarkPaymentRefundPending(billID, paymentID uint) (*Payment, error) {
	var result *Payment
	err := db.Transaction(func(tx *gorm.DB) error {
		var payment Payment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&payment, paymentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("payment not found")
			}
			return fmt.Errorf("failed to lock payment: %w", err)
		}
		if payment.BillID != billID {
			return fmt.Errorf("payment does not belong to this bill")
		}
		if payment.Status != PaymentStatusConfirmed {
			return ErrPaymentNotRefundable
		}

		now := time.Now()
		if err := tx.Model(&Payment{}).
			Where("id = ? AND status = ?", payment.ID, PaymentStatusConfirmed).
			Updates(map[string]interface{}{
				"status":     PaymentStatusRefundPending,
				"updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("failed to mark payment refund pending: %w", err)
		}
		payment.Status = PaymentStatusRefundPending
		payment.UpdatedAt = now
		result = &payment
		return nil
	})
	return result, err
}

// RestorePaymentRefundPending rolls a provider-refund claim back to confirmed
// when the provider refund failed before any local ledger reversal happened.
func RestorePaymentRefundPending(paymentID uint) error {
	now := time.Now()
	if err := db.Model(&Payment{}).
		Where("id = ? AND status = ?", paymentID, PaymentStatusRefundPending).
		Updates(map[string]interface{}{
			"status":     PaymentStatusConfirmed,
			"updated_at": now,
		}).Error; err != nil {
		return fmt.Errorf("failed to restore payment refund state: %w", err)
	}
	return nil
}

// VoidBill marks an unpaid bill as voided and appends a history event. It
// refuses to operate on bills with confirmed payments — those must go through
// RefundBillPayment first (one call per payment row).
func VoidBill(billID uint, actor, reason string) (*Bill, error) {
	trimmedReason := strings.TrimSpace(reason)
	if trimmedReason == "" {
		return nil, fmt.Errorf("void reason is required")
	}

	var result *Bill
	err := db.Transaction(func(tx *gorm.DB) error {
		var bill Bill
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&bill, billID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("bill not found")
			}
			return fmt.Errorf("failed to lock bill: %w", err)
		}

		switch bill.Status {
		case BillStatusVoided:
			return ErrBillCannotBeVoided
		case BillStatusClosed:
			return ErrBillCannotBeVoided
		}

		if bill.PaidAmount > 0 {
			// Refuse to void a bill that has captured money — the operator
			// must refund the underlying payment(s) first. This keeps the
			// money-wire contract sane: PaidAmount only goes down via an
			// audited refund path, never silently zeroed by a void.
			return ErrBillCannotBeVoided
		}

		// Refuse before any write: a voided check cannot pay for food that is
		// still being cooked. Pending sends are NOT live work — they never
		// fired and never deducted inventory, and they are auto-cancelled
		// below, so they must not block a legitimate void.
		liveKitchen, err := countLiveKitchenTicketsTx(tx, bill.ID)
		if err != nil {
			return err
		}
		if liveKitchen > 0 {
			return ErrBillVoidLiveKitchenTickets
		}

		beforeStatus := bill.Status
		now := time.Now()
		updates := map[string]interface{}{
			"status":     BillStatusVoided,
			"closed_at":  &now,
			"updated_at": now,
		}
		if err := updateBillLifecycleTx(tx, bill.ID, updates); err != nil {
			return fmt.Errorf("failed to mark bill voided: %w", err)
		}

		// Return any loyalty points the guest spent on this bill. The bill is
		// unpaid (VoidBill refuses bills with captured money), so the redemption
		// never converted to a payment — the points must come back. Idempotent in
		// practice: a re-void is blocked by the status guard above, and an
		// already-undone redemption has LoyaltyPointsRedeemed == 0.
		if bill.LoyaltyPointsRedeemed > 0 && bill.LoyaltyRedeemedByCustomerID != nil {
			if err := tx.Model(&CustomerBusiness{}).
				Where("customer_id = ? AND business_id = ? AND is_active = ?",
					*bill.LoyaltyRedeemedByCustomerID, bill.BusinessID, true).
				Update("loyalty_points", gorm.Expr("loyalty_points + ?", bill.LoyaltyPointsRedeemed)).Error; err != nil {
				return fmt.Errorf("failed to restore loyalty points on void: %w", err)
			}
		}

		// B-7: voiding the bill orphans its pending orders the same way a
		// close does — cancel them in the same transaction.
		if err := cancelPendingOrdersForClosedBillTx(tx, &bill, actor); err != nil {
			return err
		}

		event := BillHistoryEvent{
			BillID:     bill.ID,
			BusinessID: bill.BusinessID,
			EventType:  BillHistoryEventBillVoided,
			Actor:      actor,
			Reason:     trimmedReason,
			Details: map[string]interface{}{
				"status_before": string(beforeStatus),
				"status_after":  string(BillStatusVoided),
				// Dollars — BillHistoryEvent.Details has no MarshalJSON conversion.
				"total_amount": float64(bill.TotalAmount) / 100.0,
			},
		}
		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("failed to record void history: %w", err)
		}

		// Re-fetch to return the canonical row with the new fields applied.
		var refreshed Bill
		if err := tx.First(&refreshed, bill.ID).Error; err != nil {
			return fmt.Errorf("failed to reload bill after void: %w", err)
		}
		result = &refreshed
		return nil
	})
	return result, err
}

// refundedBillStatusUpdates returns the lifecycle columns for a bill whose paid
// amount drops to newPaid after a refund. A closed, paid or voided bill stays terminal
// (status and closed_at untouched) so a refund never re-occupies the table; an
// open or partial bill recomputes open/partial/paid from newPaid.
func refundedBillStatusUpdates(bill *Bill, newPaid int64) map[string]interface{} {
	if bill == nil || bill.Status == BillStatusClosed || bill.Status == BillStatusPaid || bill.Status == BillStatusVoided {
		return map[string]interface{}{}
	}

	newStatus := BillStatusOpen
	if newPaid > billPaymentAmountTolerance {
		if newPaid >= bill.TotalAmount-billPaymentAmountTolerance {
			newStatus = BillStatusPaid
		} else {
			newStatus = BillStatusPartial
		}
	}

	updates := map[string]interface{}{
		"status": newStatus,
	}
	if newStatus == BillStatusOpen || newStatus == BillStatusPartial {
		updates["closed_at"] = nil
		updates["closed_by_staff_id"] = nil
	}
	return updates
}

// RefundBillPayment marks a single confirmed payment as refunded and subtracts
// its amount + tip from the parent bill. An open or partial bill reflows its
// status; a closed or paid bill stays terminal so the table is not re-occupied.
// The caller is responsible for any external refund (Stripe / MP / on-chain) —
// this function is the bookkeeping side only, matching IMP-15's strict scope.
func RefundBillPayment(billID, paymentID uint, actor, reason string) (*Bill, *Payment, error) {
	trimmedReason := strings.TrimSpace(reason)
	if trimmedReason == "" {
		return nil, nil, fmt.Errorf("refund reason is required")
	}

	var resultBill *Bill
	var resultPayment *Payment

	err := db.Transaction(func(tx *gorm.DB) error {
		var bill Bill
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&bill, billID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("bill not found")
			}
			return fmt.Errorf("failed to lock bill: %w", err)
		}

		var payment Payment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&payment, paymentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("payment not found")
			}
			return fmt.Errorf("failed to lock payment: %w", err)
		}

		if payment.BillID != bill.ID {
			return fmt.Errorf("payment does not belong to this bill")
		}

		if payment.Status != PaymentStatusConfirmed && payment.Status != PaymentStatusRefundPending {
			// Refuse pending / failed / already-reversed / already-refunded
			// payments. The frontend only offers confirmed payments anyway,
			// but defend in depth.
			return ErrPaymentNotRefundable
		}

		var splitShareForRefund *BillSplitShare
		var linkedSplitShare BillSplitShare
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("bill_id = ? AND payment_id = ? AND status = ?", bill.ID, payment.ID, BillSplitShareStatusSettled).
			Take(&linkedSplitShare).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("failed to lock linked split share for refund: %w", err)
			}
		} else {
			splitShareForRefund = &linkedSplitShare
		}

		newPaid := bill.PaidAmount - payment.Amount
		if newPaid < 0 {
			newPaid = 0
		}
		newTip := bill.TipAmount - payment.TipAmount
		if newTip < 0 {
			newTip = 0
		}

		statusUpdates := refundedBillStatusUpdates(&bill, newPaid)
		newStatus := bill.Status
		if updated, ok := billStatusUpdate(statusUpdates["status"]); ok {
			newStatus = updated
		}

		now := time.Now()
		billUpdates := map[string]interface{}{
			"paid_amount": newPaid,
			"tip_amount":  newTip,
			"updated_at":  now,
		}
		for key, value := range statusUpdates {
			billUpdates[key] = value
		}
		if err := updateBillLifecycleTx(tx, bill.ID, billUpdates); err != nil {
			return fmt.Errorf("failed to update bill after refund: %w", err)
		}

		paymentUpdates := map[string]interface{}{
			"status":      PaymentStatusRefunded,
			"updated_at":  now,
			"reversed_at": &now,
		}
		if err := tx.Model(&Payment{}).Where("id = ?", payment.ID).Updates(paymentUpdates).Error; err != nil {
			return fmt.Errorf("failed to mark payment refunded: %w", err)
		}

		if splitShareForRefund != nil {
			splitShareUpdates := map[string]interface{}{
				"status":          BillSplitShareStatusReleased,
				"released_at":     &now,
				"hold_expires_at": nil,
				"updated_at":      now,
			}
			if err := tx.Model(&BillSplitShare{}).
				Where("id = ? AND status = ?", splitShareForRefund.ID, BillSplitShareStatusSettled).
				Updates(splitShareUpdates).Error; err != nil {
				return fmt.Errorf("failed to release split share after refund: %w", err)
			}
			splitShareForRefund.Status = BillSplitShareStatusReleased
			splitShareForRefund.ReleasedAt = &now
			splitShareForRefund.HoldExpiresAt = nil
			splitShareForRefund.UpdatedAt = now
		}

		// Roll back the business-level revenue aggregate so the dashboard
		// doesn't double-count the now-reversed payment. Mirrors the
		// ReversePluginPayment path so chain reorgs and operator refunds use
		// the same accounting reset.
		recognizedBillDelta := int64(0)
		if bill.PaidAmount > 0 && newPaid == 0 {
			recognizedBillDelta = -1
		}
		if _, _, err := incrementBusinessRevenueAggregateTx(tx, bill.BusinessID, -payment.Amount, -payment.TipAmount, recognizedBillDelta); err != nil {
			return fmt.Errorf("failed to update revenue aggregate for refund: %w", err)
		}

		eventDetails := map[string]interface{}{
			"payment_id":     payment.ID,
			"payment_method": payment.PaymentMethod,
			// Dollars — BillHistoryEvent.Details has no MarshalJSON conversion.
			"amount":        float64(payment.Amount) / 100.0,
			"tip_amount":    float64(payment.TipAmount) / 100.0,
			"paid_before":   float64(bill.PaidAmount) / 100.0,
			"paid_after":    float64(newPaid) / 100.0,
			"status_before": string(bill.Status),
			"status_after":  string(newStatus),
		}
		if splitShareForRefund != nil {
			eventDetails["split_share_id"] = splitShareForRefund.ID
			eventDetails["split_share_status_after"] = string(BillSplitShareStatusReleased)
		}

		event := BillHistoryEvent{
			BillID:     bill.ID,
			BusinessID: bill.BusinessID,
			EventType:  BillHistoryEventPaymentRefunded,
			Actor:      actor,
			Reason:     trimmedReason,
			Details:    eventDetails,
		}
		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("failed to record refund history: %w", err)
		}

		var refreshedBill Bill
		if err := tx.First(&refreshedBill, bill.ID).Error; err != nil {
			return fmt.Errorf("failed to reload bill after refund: %w", err)
		}
		var refreshedPayment Payment
		if err := tx.First(&refreshedPayment, payment.ID).Error; err != nil {
			return fmt.Errorf("failed to reload payment after refund: %w", err)
		}
		resultBill = &refreshedBill
		resultPayment = &refreshedPayment
		return nil
	})

	return resultBill, resultPayment, err
}

// RefundBillAlternativePayment reverses a single confirmed non-crypto payment
// row. When the alternative payment settled a split share, the linked share is
// released and its per-share tip is reversed from the parent bill too.
func RefundBillAlternativePayment(billID, alternativePaymentID uint, actor, reason string) (*Bill, *AlternativePayment, error) {
	trimmedReason := strings.TrimSpace(reason)
	if trimmedReason == "" {
		return nil, nil, fmt.Errorf("refund reason is required")
	}

	var resultBill *Bill
	var resultPayment *AlternativePayment

	err := db.Transaction(func(tx *gorm.DB) error {
		var bill Bill
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&bill, billID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("bill not found")
			}
			return fmt.Errorf("failed to lock bill: %w", err)
		}

		var payment AlternativePayment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&payment, alternativePaymentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("alternative payment not found")
			}
			return fmt.Errorf("failed to lock alternative payment: %w", err)
		}

		if payment.BillID != bill.ID {
			return fmt.Errorf("alternative payment does not belong to this bill")
		}
		if payment.Status != AltPaymentStatusConfirmed {
			return ErrPaymentNotRefundable
		}

		var splitShareForRefund *BillSplitShare
		var linkedSplitShare BillSplitShare
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("bill_id = ? AND alternative_payment_id = ? AND status = ?", bill.ID, payment.ID, BillSplitShareStatusSettled).
			Take(&linkedSplitShare).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("failed to lock linked split share for alternative refund: %w", err)
			}
		} else {
			splitShareForRefund = &linkedSplitShare
		}

		// Reverse the tip that was booked when this payment settled. A split
		// share carries its own per-share tip; a plain (non-split) manual
		// payment carries the tip on the payment row itself. Ignoring the
		// latter left bill.TipAmount, tips analytics, and the revenue
		// aggregate crediting a tip the cash drawer already logged going out
		// on refund — books vs drawer diverged forever (R3-BP-1). Crypto
		// RefundBillPayment already reverses payment.TipAmount; mirror it.
		refundTip := payment.TipAmountCents
		if splitShareForRefund != nil {
			refundTip = splitShareForRefund.TipCents
		}

		newPaid := bill.PaidAmount - payment.Amount
		if newPaid < 0 {
			newPaid = 0
		}
		newTip := bill.TipAmount - refundTip
		if newTip < 0 {
			newTip = 0
		}

		statusUpdates := refundedBillStatusUpdates(&bill, newPaid)
		newStatus := bill.Status
		if updated, ok := billStatusUpdate(statusUpdates["status"]); ok {
			newStatus = updated
		}

		now := time.Now()
		billUpdates := map[string]interface{}{
			"paid_amount": newPaid,
			"tip_amount":  newTip,
			"updated_at":  now,
		}
		for key, value := range statusUpdates {
			billUpdates[key] = value
		}
		if err := updateBillLifecycleTx(tx, bill.ID, billUpdates); err != nil {
			return fmt.Errorf("failed to update bill after alternative refund: %w", err)
		}

		if err := tx.Model(&AlternativePayment{}).Where("id = ?", payment.ID).Updates(map[string]interface{}{
			"status":     AltPaymentStatusRefunded,
			"updated_at": now,
		}).Error; err != nil {
			return fmt.Errorf("failed to mark alternative payment refunded: %w", err)
		}
		if err := AttachCashRegisterMovementForAlternativePaymentTx(
			tx,
			bill.BusinessID,
			payment,
			CashRegisterMovementTypeCashRefund,
			alternativePaymentTotalCashCents(payment),
			CashRegisterActor{Label: "system:refund"},
			now,
		); err != nil {
			return fmt.Errorf("failed to attach cash register refund movement: %w", err)
		}

		if splitShareForRefund != nil {
			if err := tx.Model(&BillSplitShare{}).
				Where("id = ? AND status = ?", splitShareForRefund.ID, BillSplitShareStatusSettled).
				Updates(map[string]interface{}{
					"status":          BillSplitShareStatusReleased,
					"released_at":     &now,
					"hold_expires_at": nil,
					"updated_at":      now,
				}).Error; err != nil {
				return fmt.Errorf("failed to release split share after alternative refund: %w", err)
			}
			splitShareForRefund.Status = BillSplitShareStatusReleased
			splitShareForRefund.ReleasedAt = &now
			splitShareForRefund.HoldExpiresAt = nil
			splitShareForRefund.UpdatedAt = now
		}

		recognizedBillDelta := int64(0)
		if bill.PaidAmount > 0 && newPaid == 0 {
			recognizedBillDelta = -1
		}
		if _, _, err := incrementBusinessRevenueAggregateTx(tx, bill.BusinessID, -payment.Amount, -refundTip, recognizedBillDelta); err != nil {
			return fmt.Errorf("failed to update revenue aggregate for alternative refund: %w", err)
		}

		eventDetails := map[string]interface{}{
			"alternative_payment_id": payment.ID,
			"payment_method":         payment.PaymentMethod,
			// Dollars — BillHistoryEvent.Details has no MarshalJSON conversion.
			"amount":        float64(payment.Amount) / 100.0,
			"tip_amount":    float64(refundTip) / 100.0,
			"paid_before":   float64(bill.PaidAmount) / 100.0,
			"paid_after":    float64(newPaid) / 100.0,
			"status_before": string(bill.Status),
			"status_after":  string(newStatus),
		}
		if splitShareForRefund != nil {
			eventDetails["split_share_id"] = splitShareForRefund.ID
			eventDetails["split_share_status_after"] = string(BillSplitShareStatusReleased)
		}

		event := BillHistoryEvent{
			BillID:     bill.ID,
			BusinessID: bill.BusinessID,
			EventType:  BillHistoryEventPaymentRefunded,
			Actor:      actor,
			Reason:     trimmedReason,
			Details:    eventDetails,
		}
		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("failed to record alternative refund history: %w", err)
		}

		var refreshedBill Bill
		if err := tx.First(&refreshedBill, bill.ID).Error; err != nil {
			return fmt.Errorf("failed to reload bill after alternative refund: %w", err)
		}
		var refreshedPayment AlternativePayment
		if err := tx.First(&refreshedPayment, payment.ID).Error; err != nil {
			return fmt.Errorf("failed to reload alternative payment after refund: %w", err)
		}
		resultBill = &refreshedBill
		resultPayment = &refreshedPayment
		return nil
	})

	return resultBill, resultPayment, err
}

// ListCompVoidAuditForBill returns the audit-log rows the manager-PIN
// middleware persisted for a given bill. Sorted newest-first so the UI can
// drop them straight into a feed.
func ListCompVoidAuditForBill(billID uint, limit int) ([]CompVoidAudit, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	billIDStr := fmt.Sprintf("%d", billID)

	var rows []CompVoidAudit
	if err := db.
		Where("target_type IN ? AND target_id = ?", []string{"bill", "bill_item", "payment"}, billIDStr).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to list bill audit log: %w", err)
	}
	return rows, nil
}
