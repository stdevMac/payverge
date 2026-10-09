package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/database"
	operational_alerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
)

// HandleDeliveryBillPaid advances a prepay delivery when its bill is paid:
// confirmed → preparing, kitchen starts (order approved), expiry cleared.
// Safe to call for any bill — silently no-ops when the bill has no delivery
// awaiting payment. The expiry sweep re-checks paid state inside its own
// transaction, so whichever of pay/expire commits first wins (spec §3.1).
func (s *DeliveryService) HandleDeliveryBillPaid(billID uint) error {
	var delivery database.DeliveryOrder
	var refundReviewBill *database.Bill
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("bill_id = ?", billID).
			First(&delivery).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errDeliveryPaymentNoop
			}
			return err
		}
		// DEL-PAY-02(c): money landed on a delivery that already went
		// cancelled/failed (cancel/expiry won the race against this payment).
		// The payment record stands — never drop money records — but the
		// operator must be told to review a refund. Delivered is normal
		// settlement, not refund review.
		if delivery.Status == database.DeliveryStatusCancelled || delivery.Status == database.DeliveryStatusFailed {
			var bill database.Bill
			if err := tx.Select("id", "bill_number", "business_id", "status", "paid_amount", "total_amount").
				First(&bill, delivery.BillID).Error; err != nil {
				return err
			}
			if bill.PaidAmount > 0 {
				refundReviewBill = &bill
			}
			return errDeliveryPaymentNoop
		}
		if delivery.Status != database.DeliveryStatusConfirmed {
			return errDeliveryPaymentNoop // preparing/in-flight/delivered: nothing to advance
		}
		// Verify the bill is fully paid inside the tx — a partial payment (crypto
		// partial, alt-payment partial) must not start the kitchen with money owed.
		var bill database.Bill
		if err := tx.Select("id", "status", "paid_amount", "total_amount").First(&bill, delivery.BillID).Error; err != nil {
			return err
		}
		if bill.Status != database.BillStatusPaid && bill.PaidAmount < bill.TotalAmount {
			return errDeliveryPaymentNoop // partial payment: kitchen waits for full payment
		}
		delivery.Status = database.DeliveryStatusPreparing
		delivery.PaymentExpiresAt = nil
		if err := tx.Omit(clause.Associations).Save(&delivery).Error; err != nil {
			return err
		}
		return tx.Create(&database.DeliveryStatusHistory{
			DeliveryOrderID: delivery.ID, Status: database.DeliveryStatusPreparing,
			Notes: "Payment received — kitchen started", ChangedBy: "system",
		}).Error
	})
	if errors.Is(err, errDeliveryPaymentNoop) {
		if refundReviewBill != nil {
			s.raiseDeliveryRefundReviewAlert(&delivery, refundReviewBill, string(delivery.Status))
		}
		return nil
	}
	if err != nil {
		return err
	}
	if err := s.ensureDeliveryLinkedOrderApproved(&delivery); err != nil {
		// Delivery already advanced to preparing; surface error so reconcile
		// can retry kitchen approval without reversing money.
		return err
	}
	s.emitDeliveryUpdated(&delivery)
	s.notifyDeliveryPaymentReceived(&delivery)
	return nil
}

// ensureDeliveryLinkedOrderApproved approves the kitchen order for a paid
// delivery. Safe to call when the order is already approved (idempotent).
// Failures leave money intact but must be retried — without this, delivery
// can sit in preparing with a pending order forever after a transient DB error.
func (s *DeliveryService) ensureDeliveryLinkedOrderApproved(delivery *database.DeliveryOrder) error {
	if delivery == nil || delivery.OrderID == nil {
		return nil
	}
	if err := database.ApproveDeliveryLinkedOrder(*delivery.OrderID, "system:payment"); err != nil {
		log.Printf("delivery payment: order approval failed (delivery=%d order=%d): %v", delivery.ID, *delivery.OrderID, err)
		return fmt.Errorf("approve delivery-linked order %d: %w", *delivery.OrderID, err)
	}
	if order := s.emitOrderApproved(delivery); order != nil {
		alertSvc := operational_alerts.NewService(database.GetDB())
		if aerr := alertSvc.CreateKitchenReadyAlert(context.Background(), *order); aerr != nil {
			log.Printf("delivery payment: kitchen alert failed (order=%d): %v", order.ID, aerr)
		}
	}
	return nil
}

var errDeliveryPaymentNoop = errors.New("no delivery awaiting payment for bill")

// ReconcilePaidDeliveries advances prepay deliveries that are still
// confirmed while their bill is fully paid. This is the durable backstop for
// post-pay HandleDeliveryBillPaid failures (log-and-continue on the payment
// path): money truth stays on the bill; kitchen advancement is retried until
// it succeeds or the delivery leaves confirmed.
//
// Unlike ExpireUnpaidDeliveries, this does not require payment_expires_at to
// have elapsed — so a paid delivery whose pay-hook failed is recovered within
// the reconciliation interval even when the payment window is still open.
func (s *DeliveryService) ReconcilePaidDeliveries(limit int) (advanced int, err error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 100
	}
	var candidates []database.DeliveryOrder
	// confirmed + paid bill: advance to preparing (missed pay-hook).
	// preparing + linked order still pending: retry kitchen approval only.
	if err := s.db.
		Select("delivery_orders.id", "delivery_orders.bill_id", "delivery_orders.order_id", "delivery_orders.business_id", "delivery_orders.status").
		Joins("JOIN bills ON bills.id = delivery_orders.bill_id").
		Joins("LEFT JOIN orders ON orders.id = delivery_orders.order_id").
		Where("(bills.status = ? OR bills.paid_amount >= bills.total_amount) AND bills.paid_amount > 0", database.BillStatusPaid).
		Where(`(
			delivery_orders.status = ?
			OR (delivery_orders.status = ? AND orders.id IS NOT NULL AND orders.status = ?)
		)`, database.DeliveryStatusConfirmed, database.DeliveryStatusPreparing, database.OrderStatusPending).
		Order("delivery_orders.updated_at ASC, delivery_orders.id ASC").
		Limit(limit).
		Find(&candidates).Error; err != nil {
		return 0, err
	}
	for _, candidate := range candidates {
		if candidate.Status == database.DeliveryStatusPreparing {
			// Kitchen approval retry only — do not re-run full paid advance.
			if appErr := s.ensureDeliveryLinkedOrderApproved(&candidate); appErr != nil {
				log.Printf("delivery paid reconcile: order approve failed (delivery=%d): %v", candidate.ID, appErr)
				continue
			}
			advanced++
			continue
		}
		if advErr := s.HandleDeliveryBillPaid(candidate.BillID); advErr != nil {
			log.Printf("delivery paid reconcile: advance failed (delivery=%d bill=%d): %v", candidate.ID, candidate.BillID, advErr)
			continue
		}
		// Re-check status; HandleDeliveryBillPaid no-ops without error when not
		// confirmed, so only count real advances.
		var after database.DeliveryOrder
		if loadErr := s.db.Select("status").First(&after, candidate.ID).Error; loadErr == nil && after.Status == database.DeliveryStatusPreparing {
			advanced++
		}
	}
	if advanced > 0 {
		log.Printf("delivery paid reconcile: advanced %d of %d candidates", advanced, len(candidates))
	}
	return advanced, nil
}

// ExpireUnpaidDeliveries cancels prepay deliveries whose payment window has
// lapsed. Paid-but-not-yet-advanced rows (payment landed between hook miss
// and sweep) are advanced instead — the sweep doubles as reconciliation.
func (s *DeliveryService) ExpireUnpaidDeliveries() error {
	now := time.Now().UTC()
	var candidates []database.DeliveryOrder
	if err := s.db.
		Select("id", "bill_id", "order_id", "business_id").
		Where("status = ? AND payment_expires_at IS NOT NULL AND payment_expires_at < ?", database.DeliveryStatusConfirmed, now).
		Limit(200).
		Find(&candidates).Error; err != nil {
		return err
	}
	// Track how many rows this run actually cancelled vs. reconciled so a stuck
	// or silently-failing sweep is visible in prod logs (audit M7).
	var swept, reconciled int
	for _, candidate := range candidates {
		var bill database.Bill
		if err := s.db.Select("id", "status", "paid_amount", "total_amount").First(&bill, candidate.BillID).Error; err != nil {
			log.Printf("delivery expiry: bill load failed (delivery=%d bill=%d): %v", candidate.ID, candidate.BillID, err)
			continue
		}
		// Divert to reconcile (advance confirmed→preparing) ONLY when the bill is
		// fully settled. A partial payment (e.g. on-chain USDC underpayment) must
		// NOT divert here — HandleDeliveryBillPaid no-ops on a partial bill, which
		// would leave the delivery stuck in 'confirmed' forever with money captured.
		// Partial bills fall through to cancelDeliveryLinked (no-refund-by-design).
		if bill.Status == database.BillStatusPaid || bill.PaidAmount >= bill.TotalAmount {
			if err := s.HandleDeliveryBillPaid(candidate.BillID); err != nil {
				log.Printf("delivery expiry: reconcile-paid failed (delivery=%d): %v", candidate.ID, err)
			} else {
				reconciled++
			}
			continue
		}
		if err := s.cancelDeliveryLinked(candidate.ID, "payment window expired", "system:expiry", "expired", database.DeliveryStatusCancelled); err != nil {
			if errors.Is(err, errExpiredButPaid) {
				// Payment committed between the sweep's pre-check and the cancel tx —
				// reconcile: advance delivery confirmed→preparing instead of cancelling.
				if reconcileErr := s.HandleDeliveryBillPaid(candidate.BillID); reconcileErr != nil {
					log.Printf("delivery expiry: reconcile-paid (in-tx race) failed (delivery=%d): %v", candidate.ID, reconcileErr)
				} else {
					reconciled++
				}
				continue
			}
			log.Printf("delivery expiry: cancel failed (delivery=%d): %v", candidate.ID, err)
			continue
		}
		swept++
	}
	// Observability: surface real work at info level; otherwise emit a
	// throttled heartbeat so an entirely silent (stuck) scheduler is still
	// detectable. The sweep runs every minute — an unthrottled idle line would
	// be ~1.4k noise lines/day, burying the signal it exists to provide.
	if swept > 0 || reconciled > 0 {
		log.Printf("delivery expiry: swept %d expired, reconciled %d paid (candidates=%d)", swept, reconciled, len(candidates))
	} else if n := expiryIdleRuns.Add(1); n%expiryHeartbeatEvery == 1 {
		log.Printf("delivery expiry: heartbeat, no unpaid deliveries to expire (candidates=%d, idle runs=%d)", len(candidates), n)
	}
	return nil
}

// expiryHeartbeatEvery throttles the idle heartbeat to roughly once an hour
// at the sweeper's 1-minute cadence (first idle run still logs immediately).
const expiryHeartbeatEvery = 60

var expiryIdleRuns atomic.Uint64

func (s *DeliveryService) emitOrderApproved(delivery *database.DeliveryOrder) *database.Order {
	if delivery.OrderID == nil {
		return nil
	}
	order, _, err := database.GetOrderByID(*delivery.OrderID)
	if err != nil {
		log.Printf("delivery payment: order reload failed (delivery=%d order=%d): %v", delivery.ID, *delivery.OrderID, err)
		return nil
	}
	eventsPublishOrderUpdated(order)
	return order
}

func (s *DeliveryService) notifyDeliveryPaymentReceived(delivery *database.DeliveryOrder) {
	if delivery.CustomerEmail == "" {
		return
	}
	business, berr := database.GetBusinessByID(delivery.BusinessID)
	if berr != nil {
		log.Printf("notifyDeliveryPaymentReceived: failed to load business %d: %v", delivery.BusinessID, berr)
	}
	locale := guestNotificationLocale(delivery, business)
	title, body := localizedPaymentReceivedMessage(delivery.DeliveryNumber, locale)
	s.sendDeliveryNotificationToEmail(deliveryCustomerMailOrigin(delivery, ""), delivery.CustomerEmail, delivery.CustomerName, title, body, locale)
}
