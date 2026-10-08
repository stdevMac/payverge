package database

import (
	"errors"
	"fmt"
	"time"

	"github.com/stdevmac/payverge/backend/internal/txhash"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReversePluginPayment finds a Payment by TxHash and reverses its effect on the
// associated bill:
//   - Decrements bill.PaidAmount by payment.Amount
//   - Decrements bill.TipAmount by payment.TipAmount
//   - Sets payment.Status to "reversed"
//   - Recomputes bill status from the remaining paid amount
//
// Returns nil if no payment is found for the given txHash or if the payment is
// already terminally reversed or refunded (idempotent).
func ReversePluginPayment(txHash string) error {
	txHash = txhash.NormalizeReference(txHash)
	if txHash == "" {
		return fmt.Errorf("txHash must not be empty")
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// Lock the payment row first to prevent concurrent reversals.
		var payment Payment
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("tx_hash = ?", txHash).
			First(&payment).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// Nothing to reverse — idempotent.
				return nil
			}
			return fmt.Errorf("failed to look up payment: %w", err)
		}

		// Already terminally reversed/refunded — idempotent. The operator in-app
		// refund path (RefundBillPayment) sets PaymentStatusRefunded and ALREADY
		// decremented the revenue aggregate; a subsequent provider 'refunded'
		// webhook must be a no-op here or it double-decrements the books.
		//
		// refund_pending is also skipped: the manual refund flow has called the
		// PSP and owns the reversal (it finalizes via RefundBillPayment). If the
		// inbound 'refunded' webhook reversed it here first, that finalize would
		// then 409 the operator after the money already moved, and the audit /
		// credit-note enrichment on the manual path would be skipped.
		if payment.Status == PaymentStatusReversed ||
			payment.Status == PaymentStatusRefunded ||
			payment.Status == PaymentStatusRefundPending {
			return nil
		}

		// Lock the bill row before modifying amounts.
		var bill Bill
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&bill, payment.BillID).Error; err != nil {
			return fmt.Errorf("failed to lock bill %d: %w", payment.BillID, err)
		}

		// Reverse the amounts (floor at zero to guard against double-reversal data
		// corruption that somehow bypassed the status check above).
		newPaid := bill.PaidAmount - payment.Amount
		if newPaid < 0 {
			newPaid = 0
		}
		newTip := bill.TipAmount - payment.TipAmount
		if newTip < 0 {
			newTip = 0
		}

		billUpdates, err := pluginReversalBillUpdates(tx, &bill, newPaid, newTip)
		if err != nil {
			return err
		}

		if err := updateBillLifecycleTx(tx, bill.ID, billUpdates); err != nil {
			return fmt.Errorf("failed to update bill %d after reversal: %w", bill.ID, err)
		}

		// Mark the payment as reversed.
		now := time.Now()
		if err := tx.Model(&Payment{}).Where("id = ?", payment.ID).Updates(map[string]interface{}{
			"status":      PaymentStatusReversed,
			"updated_at":  now,
			"reversed_at": &now,
		}).Error; err != nil {
			return fmt.Errorf("failed to mark payment %d as reversed: %w", payment.ID, err)
		}

		recognizedBillDelta := int64(0)
		if bill.PaidAmount > 0 && newPaid == 0 {
			recognizedBillDelta = -1
		}
		if _, _, err := incrementBusinessRevenueAggregateTx(tx, bill.BusinessID, -payment.Amount, -payment.TipAmount, recognizedBillDelta); err != nil {
			return fmt.Errorf("failed to update revenue aggregate for reversal: %w", err)
		}

		return nil
	})
}

// PartialReversePluginPayment applies a strict partial provider refund against a
// local plugin Payment row. Unlike ReversePluginPayment it does not zero the
// whole payment: it decrements bill paid amounts and shrinks the payment's
// remaining Amount (and TipAmount when the refund exceeds the bill portion).
//
// Idempotency for a given webhook is owned by webhook_events; this function is
// intentionally re-runnable for sequential partials of the same capture (each
// call reduces the remaining payment amount). When remaining bill+tip reaches
// zero the payment is marked refunded.
//
// refundedCents is the portion refunded by the provider in this event (not
// cumulative). Values <= 0 are rejected.
func PartialReversePluginPayment(txHash string, refundedCents int64) error {
	txHash = txhash.NormalizeReference(txHash)
	if txHash == "" {
		return fmt.Errorf("txHash must not be empty")
	}
	if refundedCents <= 0 {
		return fmt.Errorf("refundedCents must be positive")
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var payment Payment
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("tx_hash = ?", txHash).
			First(&payment).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return fmt.Errorf("failed to look up payment: %w", err)
		}

		if payment.Status == PaymentStatusReversed ||
			payment.Status == PaymentStatusRefunded ||
			payment.Status == PaymentStatusRefundPending {
			return nil
		}

		remainingCapture := payment.Amount + payment.TipAmount
		if remainingCapture <= 0 {
			return nil
		}
		// Cap at remaining capture so overstated provider amounts cannot drive
		// bill paid below zero beyond the payment's residual.
		apply := refundedCents
		if apply > remainingCapture {
			apply = remainingCapture
		}

		// Prefer reducing bill portion first, then tip (matches typical PSP
		// partial refunds of the goods amount before gratuity).
		billPortion := apply
		tipPortion := int64(0)
		if billPortion > payment.Amount {
			tipPortion = billPortion - payment.Amount
			billPortion = payment.Amount
		}

		var bill Bill
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&bill, payment.BillID).Error; err != nil {
			return fmt.Errorf("failed to lock bill %d: %w", payment.BillID, err)
		}

		newPaid := bill.PaidAmount - billPortion
		if newPaid < 0 {
			newPaid = 0
		}
		newTip := bill.TipAmount - tipPortion
		if newTip < 0 {
			newTip = 0
		}

		billUpdates, err := pluginReversalBillUpdates(tx, &bill, newPaid, newTip)
		if err != nil {
			return err
		}
		if err := updateBillLifecycleTx(tx, bill.ID, billUpdates); err != nil {
			return fmt.Errorf("failed to update bill %d after partial refund: %w", bill.ID, err)
		}

		newPaymentAmount := payment.Amount - billPortion
		newPaymentTip := payment.TipAmount - tipPortion
		paymentUpdates := map[string]interface{}{
			"amount":     newPaymentAmount,
			"tip_amount": newPaymentTip,
			"updated_at": time.Now(),
		}
		if newPaymentAmount+newPaymentTip <= 0 {
			now := time.Now()
			paymentUpdates["status"] = PaymentStatusRefunded
			paymentUpdates["reversed_at"] = &now
		}
		if err := tx.Model(&Payment{}).Where("id = ?", payment.ID).Updates(paymentUpdates).Error; err != nil {
			return fmt.Errorf("failed to update payment %d after partial refund: %w", payment.ID, err)
		}

		recognizedBillDelta := int64(0)
		if bill.PaidAmount > 0 && newPaid == 0 {
			recognizedBillDelta = -1
		}
		if _, _, err := incrementBusinessRevenueAggregateTx(tx, bill.BusinessID, -billPortion, -tipPortion, recognizedBillDelta); err != nil {
			return fmt.Errorf("failed to update revenue aggregate for partial refund: %w", err)
		}

		return nil
	})
}

// pluginReversalBillUpdates returns the bill columns a provider reversal
// (refund, chargeback, dispute withdrawal or reinstatement) writes once the
// paid amount moves to newPaid. Money columns always move. The lifecycle:
//   - a closed or voided bill stays terminal: it is a finished service record
//     and must never re-occupy its table;
//   - a paid bill reopens to open/partial so the money now owed is visible,
//     unless another check is already active on its table or counter, in
//     which case it stays paid (reopening would trip the one-active-check
//     unique index and fail the webhook on every redelivery);
//   - open and partial bills recompute open/partial/paid from newPaid.
func pluginReversalBillUpdates(tx *gorm.DB, bill *Bill, newPaid, newTip int64) (map[string]interface{}, error) {
	updates := map[string]interface{}{
		"paid_amount": newPaid,
		"tip_amount":  newTip,
	}
	if bill.Status == BillStatusClosed || bill.Status == BillStatusVoided {
		return updates, nil
	}
	newStatus := BillStatusOpen
	if newPaid > billPaymentAmountTolerance {
		if newPaid >= bill.TotalAmount-billPaymentAmountTolerance {
			newStatus = BillStatusPaid
		} else {
			newStatus = BillStatusPartial
		}
	}
	reopening := newStatus == BillStatusOpen || newStatus == BillStatusPartial
	if reopening && bill.Status == BillStatusPaid {
		taken, err := billSlotTakenByAnotherActiveBillTx(tx, bill)
		if err != nil {
			return nil, err
		}
		if taken {
			return updates, nil
		}
	}
	updates["status"] = newStatus
	if reopening {
		updates["closed_at"] = nil
		updates["closed_by_staff_id"] = nil
	}
	return updates, nil
}

// billSlotTakenByAnotherActiveBillTx reports whether another open or partial
// check already holds bill's table or counter (the slots guarded by
// idx_bills_active_per_table / idx_bills_active_per_counter).
func billSlotTakenByAnotherActiveBillTx(tx *gorm.DB, bill *Bill) (bool, error) {
	active := []BillStatus{BillStatusOpen, BillStatusPartial}
	q := tx.Model(&Bill{}).Where("id <> ? AND status IN ?", bill.ID, active)
	switch {
	case bill.TableID != 0:
		q = q.Where("table_id = ?", bill.TableID)
	case bill.CounterID != nil:
		q = q.Where("counter_id = ?", *bill.CounterID)
	default:
		return false, nil
	}
	var ids []uint
	if err := q.Limit(1).Pluck("id", &ids).Error; err != nil {
		return false, fmt.Errorf("failed to check active check on bill %d slot: %w", bill.ID, err)
	}
	return len(ids) > 0, nil
}

// lockPluginPaymentForProviderDelta locks the payment row for txHash. found is
// false when no row exists.
func lockPluginPaymentForProviderDelta(tx *gorm.DB, txHash string) (Payment, bool, error) {
	var payment Payment
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("tx_hash = ?", txHash).
		First(&payment).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return payment, false, nil
	}
	if err != nil {
		return payment, false, fmt.Errorf("failed to look up payment: %w", err)
	}
	return payment, true, nil
}

// reducePluginPaymentTx takes up to want cents out of payment (bill portion
// first, then tip) and out of its bill. It returns the bill and tip portions
// actually taken. finalStatus is applied when nothing of the payment remains.
func reducePluginPaymentTx(tx *gorm.DB, payment *Payment, want int64, paymentExtra map[string]interface{}, finalStatus PaymentStatus) (int64, int64, error) {
	remaining := payment.Amount + payment.TipAmount
	if want > remaining {
		want = remaining
	}
	if want < 0 {
		want = 0
	}
	billPortion := want
	tipPortion := int64(0)
	if billPortion > payment.Amount {
		tipPortion = billPortion - payment.Amount
		billPortion = payment.Amount
	}

	if want > 0 {
		var bill Bill
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&bill, payment.BillID).Error; err != nil {
			return 0, 0, fmt.Errorf("failed to lock bill %d: %w", payment.BillID, err)
		}
		newPaid := bill.PaidAmount - billPortion
		if newPaid < 0 {
			newPaid = 0
		}
		newTip := bill.TipAmount - tipPortion
		if newTip < 0 {
			newTip = 0
		}
		billUpdates, err := pluginReversalBillUpdates(tx, &bill, newPaid, newTip)
		if err != nil {
			return 0, 0, err
		}
		if err := updateBillLifecycleTx(tx, bill.ID, billUpdates); err != nil {
			return 0, 0, fmt.Errorf("failed to update bill %d after provider reversal: %w", bill.ID, err)
		}
		recognizedBillDelta := int64(0)
		if bill.PaidAmount > 0 && newPaid == 0 {
			recognizedBillDelta = -1
		}
		if _, _, err := incrementBusinessRevenueAggregateTx(tx, bill.BusinessID, -billPortion, -tipPortion, recognizedBillDelta); err != nil {
			return 0, 0, fmt.Errorf("failed to update revenue aggregate for provider reversal: %w", err)
		}
	}

	now := time.Now()
	updates := map[string]interface{}{
		"amount":     payment.Amount - billPortion,
		"tip_amount": payment.TipAmount - tipPortion,
		"updated_at": now,
	}
	for k, v := range paymentExtra {
		updates[k] = v
	}
	if want > 0 && payment.Amount-billPortion+payment.TipAmount-tipPortion <= 0 {
		updates["status"] = finalStatus
		updates["reversed_at"] = &now
	}
	if err := tx.Model(&Payment{}).Where("id = ?", payment.ID).Updates(updates).Error; err != nil {
		return 0, 0, fmt.Errorf("failed to update payment %d after provider reversal: %w", payment.ID, err)
	}
	return billPortion, tipPortion, nil
}

// ApplyPluginRefundCumulative applies a provider's CUMULATIVE refunded total
// (Stripe charge.amount_refunded, MercadoPago transaction_amount_refunded) to
// the plugin payment for txHash. Only the delta over what was already applied
// (provider_refunded_cents) is reversed, so a replayed or reordered webhook is
// a no-op and a later larger total reverses only the difference. It returns
// the cents reversed by this call.
//
// A refunded, reversed or refund_pending payment is left alone: the operator
// in-app refund (RefundBillPayment) and the full provider reversal already
// settled the books, and refund_pending belongs to the manual refund flow.
func ApplyPluginRefundCumulative(txHash string, cumulativeCents int64) (int64, error) {
	txHash = txhash.NormalizeReference(txHash)
	if txHash == "" {
		return 0, fmt.Errorf("txHash must not be empty")
	}
	if cumulativeCents <= 0 {
		return 0, nil
	}
	var applied int64
	err := db.Transaction(func(tx *gorm.DB) error {
		payment, found, err := lockPluginPaymentForProviderDelta(tx, txHash)
		if err != nil || !found {
			return err
		}
		if payment.Status == PaymentStatusReversed ||
			payment.Status == PaymentStatusRefunded ||
			payment.Status == PaymentStatusRefundPending {
			return nil
		}
		delta := cumulativeCents - payment.ProviderRefundedCents
		if delta <= 0 {
			return nil
		}
		billPortion, tipPortion, err := reducePluginPaymentTx(tx, &payment, delta,
			map[string]interface{}{"provider_refunded_cents": cumulativeCents},
			PaymentStatusRefunded)
		applied = billPortion + tipPortion
		return err
	})
	return applied, err
}

// SetPluginDisputedCents moves the plugin payment for txHash to a provider
// dispute withdrawal of targetCents. Raising the target reverses only the
// additional amount (bill portion first, then tip); lowering it (funds
// reinstated, dispute won) restores what was withdrawn, tip first, and returns
// a fully reversed payment to confirmed. Repeating the current target is a
// no-op. It returns the signed change applied to the payment (negative when
// money was withdrawn).
func SetPluginDisputedCents(txHash string, targetCents int64) (int64, error) {
	txHash = txhash.NormalizeReference(txHash)
	if txHash == "" {
		return 0, fmt.Errorf("txHash must not be empty")
	}
	if targetCents < 0 {
		targetCents = 0
	}
	var change int64
	err := db.Transaction(func(tx *gorm.DB) error {
		payment, found, err := lockPluginPaymentForProviderDelta(tx, txHash)
		if err != nil || !found {
			return err
		}
		if payment.Status == PaymentStatusRefundPending {
			return nil
		}
		// A legacy full reversal or a refund with no dispute tracking is not
		// ours to undo. A row that still carries a disputed amount can be
		// reinstated even after a provider refund zeroed its remainder: the
		// refund covered the undisputed part, the dispute withdrew the rest.
		if (payment.Status == PaymentStatusReversed || payment.Status == PaymentStatusRefunded) &&
			payment.ProviderDisputedCents == 0 {
			return nil
		}
		current := payment.ProviderDisputedCents
		switch {
		case targetCents > current:
			billPortion, tipPortion, err := reducePluginPaymentTx(tx, &payment, targetCents-current, nil, PaymentStatusReversed)
			if err != nil {
				return err
			}
			taken := billPortion + tipPortion
			if taken == 0 {
				return nil
			}
			change = -taken
			return tx.Model(&Payment{}).Where("id = ?", payment.ID).Updates(map[string]interface{}{
				"provider_disputed_cents":     current + taken,
				"provider_disputed_tip_cents": payment.ProviderDisputedTipCents + tipPortion,
			}).Error
		case targetCents < current:
			restore := current - targetCents
			tipRestore := restore
			if tipRestore > payment.ProviderDisputedTipCents {
				tipRestore = payment.ProviderDisputedTipCents
			}
			billRestore := restore - tipRestore

			var bill Bill
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				First(&bill, payment.BillID).Error; err != nil {
				return fmt.Errorf("failed to lock bill %d: %w", payment.BillID, err)
			}
			newPaid := bill.PaidAmount + billRestore
			newTip := bill.TipAmount + tipRestore
			// A bill staff closed or voided meanwhile stays terminal; the
			// restored money is still recorded against it.
			billUpdates, err := pluginReversalBillUpdates(tx, &bill, newPaid, newTip)
			if err != nil {
				return err
			}
			if err := updateBillLifecycleTx(tx, bill.ID, billUpdates); err != nil {
				return fmt.Errorf("failed to update bill %d after dispute reinstatement: %w", bill.ID, err)
			}
			recognizedBillDelta := int64(0)
			if bill.PaidAmount == 0 && newPaid > 0 {
				recognizedBillDelta = 1
			}
			if _, _, err := incrementBusinessRevenueAggregateTx(tx, bill.BusinessID, billRestore, tipRestore, recognizedBillDelta); err != nil {
				return fmt.Errorf("failed to update revenue aggregate for dispute reinstatement: %w", err)
			}

			paymentUpdates := map[string]interface{}{
				"amount":                      payment.Amount + billRestore,
				"tip_amount":                  payment.TipAmount + tipRestore,
				"provider_disputed_cents":     targetCents,
				"provider_disputed_tip_cents": payment.ProviderDisputedTipCents - tipRestore,
				"updated_at":                  time.Now(),
			}
			if payment.Status == PaymentStatusReversed || payment.Status == PaymentStatusRefunded {
				paymentUpdates["status"] = PaymentStatusConfirmed
				paymentUpdates["reversed_at"] = nil
			}
			if err := tx.Model(&Payment{}).Where("id = ?", payment.ID).Updates(paymentUpdates).Error; err != nil {
				return fmt.Errorf("failed to update payment %d after dispute reinstatement: %w", payment.ID, err)
			}
			change = restore
		}
		return nil
	})
	return change, err
}
