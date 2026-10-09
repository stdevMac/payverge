package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/money"
	operational_alerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
)

// businessGuestCurrency resolves the currency guests see for a business:
// display currency when set, else the pricing (default) currency, else USD —
// matching the tracking page's display_currency ?? default_currency fallback.
func businessGuestCurrency(business *database.Business) string {
	if business == nil {
		return "USD"
	}
	if c := strings.TrimSpace(business.DisplayCurrency); c != "" {
		return c
	}
	if c := strings.TrimSpace(business.DefaultCurrency); c != "" {
		return c
	}
	return "USD"
}

// formatDeliveryMoney renders a stored "cents" amount (major units ×100 for
// ALL currencies) with the currency's real number of decimals ("USD 17.00",
// "JPY 1700", "BHD 1.500"), mirroring formatTelegramMoney and the provider
// boundary (money.MajorUnitString) so zero-decimal currencies never grow fake
// cents in guest-facing copy.
func formatDeliveryMoney(cents int64, currency string) string {
	currency = strings.TrimSpace(currency)
	if currency == "" {
		currency = "USD"
	}
	return currency + " " + money.MajorUnitString(cents, currency)
}

// DeliveryPaymentWindow is how long a guest has to pay after staff accept a
// prepay-mode order. Rejection/expiry before payment costs nobody anything —
// the system has no refunds by design. Exported so demo seed / UI copy stay
// aligned with the sweeper (never a longer "demo convenience" window).
const DeliveryPaymentWindow = 15 * time.Minute

// deliveryPaymentWindow is the historical unexported alias used inside this package.
const deliveryPaymentWindow = DeliveryPaymentWindow

// ErrDeliveryOrderNotFound — no delivery order linked to the given order.
var ErrDeliveryOrderNotFound = errors.New("delivery order not found for order")

// ErrDeliveryInvalidState — the delivery is not in a state that allows the
// requested accept/reject action (e.g. already confirmed/preparing/terminal, or
// an invalid payment mode). Handlers map this to a safe client message; the
// underlying detail is wrapped with %w so errors.Is still matches while keeping
// the raw text server-side only.
var ErrDeliveryInvalidState = errors.New("delivery is not in a state that allows this action")

// AcceptDeliveryResult tells the handler what the accept did.
type AcceptDeliveryResult struct {
	Delivery        *database.DeliveryOrder
	AwaitingPayment bool
	PaymentMode     database.DeliveryPaymentMode
}

// DeliveryByOrderID loads the delivery row linked to an order, scoped to the
// business. Used by the order-status handler to route delivery orders here.
func (s *DeliveryService) DeliveryByOrderID(businessID, orderID uint) (*database.DeliveryOrder, error) {
	var delivery database.DeliveryOrder
	err := s.db.Where("business_id = ? AND order_id = ?", businessID, orderID).First(&delivery).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrDeliveryOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	return &delivery, nil
}

// AcceptDeliveryByOrder is the Bills-queue Accept. Prepay: delivery →
// confirmed + payment window opens; the kitchen waits (order stays pending).
// COD: kitchen starts now (order → approved, delivery → preparing).
// All delivery writes are one transaction; notifications fire after commit.
//
// Reconcile branch: if delivery is already preparing AND the linked order is
// still pending (wedge from a prior failed-approval seam), this call heals the
// seam by running ApproveDeliveryLinkedOrder and returning success — no delivery
// write happens (it is already correct). confirmed + pending is not reconciled
// here: confirmed means payment is still outstanding so approval would be wrong.
func (s *DeliveryService) AcceptDeliveryByOrder(businessID, orderID uint, actor string, mode database.DeliveryPaymentMode) (*AcceptDeliveryResult, error) {
	if !mode.IsValid() {
		return nil, fmt.Errorf("invalid delivery payment mode %q: %w", mode, ErrDeliveryInvalidState)
	}
	var delivery database.DeliveryOrder
	var reconcileOnly bool
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("business_id = ? AND order_id = ?", businessID, orderID).
			First(&delivery).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrDeliveryOrderNotFound
			}
			return err
		}

		// Reconcile branch: preparing delivery + pending order = prior failed-approval.
		// Re-run the approval outside the tx (idempotent) and return success.
		if delivery.Status == database.DeliveryStatusPreparing {
			var linkedOrder database.Order
			if delivery.OrderID != nil {
				if err := tx.Select("status").First(&linkedOrder, *delivery.OrderID).Error; err == nil &&
					linkedOrder.Status == database.OrderStatusPending {
					reconcileOnly = true
					return nil // delivery row is already correct — no write needed
				}
			}
			// preparing + non-pending order (already approved or terminal) — no-op
			reconcileOnly = false
			return fmt.Errorf("delivery is %s, only pending orders can be accepted: %w", delivery.Status, ErrDeliveryInvalidState)
		}

		if delivery.Status != database.DeliveryStatusPending {
			return fmt.Errorf("delivery is %s, only pending orders can be accepted: %w", delivery.Status, ErrDeliveryInvalidState)
		}
		now := time.Now().UTC()
		if mode == database.DeliveryPaymentOnline {
			expires := now.Add(deliveryPaymentWindow)
			delivery.Status = database.DeliveryStatusConfirmed
			delivery.PaymentExpiresAt = &expires
			if err := tx.Omit(clause.Associations).Save(&delivery).Error; err != nil {
				return err
			}
			return tx.Create(&database.DeliveryStatusHistory{
				DeliveryOrderID: delivery.ID, Status: database.DeliveryStatusConfirmed,
				Notes: "Accepted — awaiting guest payment", ChangedBy: actor,
			}).Error
		}
		// COD: kitchen starts now.
		delivery.Status = database.DeliveryStatusPreparing
		if err := tx.Omit(clause.Associations).Save(&delivery).Error; err != nil {
			return err
		}
		return tx.Create(&database.DeliveryStatusHistory{
			DeliveryOrderID: delivery.ID, Status: database.DeliveryStatusPreparing,
			Notes: "Accepted — cash on delivery, kitchen started", ChangedBy: actor,
		}).Error
	})
	if err != nil {
		return nil, err
	}

	if reconcileOnly {
		// Delivery is already preparing; only the order-approval leg needs to run.
		if delivery.OrderID != nil {
			if approveErr := database.ApproveDeliveryLinkedOrder(*delivery.OrderID, actor); approveErr != nil {
				log.Printf("delivery accept reconcile: order approval failed (business=%d order=%d): %v", businessID, orderID, approveErr)
				s.emitDeliveryUpdated(&delivery)
				return nil, approveErr
			}
			s.emitOrderApproved(&delivery)
		}
		// Single delivery.updated per reconcile — a second emit here doubled
		// operator/guest toasts.
		s.emitDeliveryUpdated(&delivery)
		// DEL-SM-8: the reconcile branch must never trust the caller's mode —
		// the delivery is already preparing, so no online-payment window can be
		// issued here (a pay-link email would have no expiry behind it). Accept
		// semantics are derived from state instead: COD copy when the bill is
		// still unpaid; NO acceptance email when the bill is already settled
		// (prepay wedge — the guest paid, telling them to pay cash is wrong).
		effectiveMode := database.DeliveryPaymentCashOnDelivery
		billSettled := false
		var bill database.Bill
		if err := s.db.Select("id", "status", "paid_amount", "total_amount").
			First(&bill, delivery.BillID).Error; err == nil {
			billSettled = bill.Status == database.BillStatusPaid ||
				(bill.TotalAmount > 0 && bill.PaidAmount >= bill.TotalAmount)
		}
		if !billSettled {
			// The delivery was already preparing (prior accept committed the delivery tx
			// but the order-approval leg failed). The guest never received the acceptance
			// email during the failed attempt, so we send it now that the seam is healed.
			s.notifyDeliveryAccepted(&delivery, effectiveMode)
		}
		return &AcceptDeliveryResult{
			Delivery:        &delivery,
			AwaitingPayment: false,
			PaymentMode:     effectiveMode,
		}, nil
	}

	if mode == database.DeliveryPaymentCashOnDelivery {
		// Order approval has its own transaction (inventory + history); a
		// failure here leaves delivery=preparing with order=pending, which
		// the operator sees and can retry — never silent.
		if approveErr := database.ApproveDeliveryLinkedOrder(orderID, actor); approveErr != nil {
			log.Printf("delivery accept: order approval failed (business=%d order=%d): %v", businessID, orderID, approveErr)
			// Surface the committed preparing state to the operator UI even
			// though the order-approval leg failed — the delivery tx is done.
			s.emitDeliveryUpdated(&delivery)
			return nil, approveErr
		}
	}
	s.emitDeliveryUpdated(&delivery)
	s.notifyDeliveryAccepted(&delivery, mode)
	return &AcceptDeliveryResult{
		Delivery:        &delivery,
		AwaitingPayment: mode == database.DeliveryPaymentOnline,
		PaymentMode:     mode,
	}, nil
}

// RejectDeliveryByOrder is the Bills-queue Reject: all three legs go
// terminal, the driver (if any) is released, the unpaid bill is closed.
func (s *DeliveryService) RejectDeliveryByOrder(businessID, orderID uint, actor, reason string) error {
	// Narrow lookup — we only need the delivery ID to pass to cancelDeliveryLinked.
	var deliveryID uint
	err := s.db.Model(&database.DeliveryOrder{}).
		Where("business_id = ? AND order_id = ?", businessID, orderID).
		Limit(1).Pluck("id", &deliveryID).Error
	if err != nil {
		return err
	}
	if deliveryID == 0 {
		return ErrDeliveryOrderNotFound
	}
	return s.cancelDeliveryLinked(deliveryID, reason, actor, "rejected", database.DeliveryStatusCancelled)
}

// errExpiredButPaid is returned by cancelDeliveryLinked (kind=="expired") when
// the in-tx bill check finds the bill has been paid. The expiry sweep catches
// this sentinel and calls HandleDeliveryBillPaid to reconcile instead.
var errExpiredButPaid = errors.New("delivery expiry: bill is paid — reconcile instead of cancel")

// cancelDeliveryLinked is the single termination path: delivery terminal
// (written ONCE — no UnassignDriver resurrection), driver released, order
// cancelled (inventory restored if it was approved), unpaid bill closed.
// kind ∈ {"rejected","cancelled","expired","failed"} — selects the guest email
// copy and the status-history note. terminalStatus is the final delivery status
// to write: DeliveryStatusCancelled for cancel/reject/expiry, or
// DeliveryStatusFailed for a failed delivery (which needs the SAME downstream
// cleanup — order cancel + bill close + driver release — not just a driver
// release).
//
// For kind=="expired" only: if the bill is found paid inside the tx (payment
// committed between the sweep's pre-check and this cancel tx), the cancel is
// aborted and errExpiredButPaid is returned so the caller can reconcile.
func (s *DeliveryService) cancelDeliveryLinked(deliveryID uint, reason, actor, kind string, terminalStatus database.DeliveryStatus) error {
	var delivery database.DeliveryOrder
	var alreadyTerminal bool
	var bill database.Bill
	var haveBill bool
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Lock-order contract (DEL-PAY-02): bill FIRST, then delivery. The
		// payment-confirm path (applyConfirmedPaymentTx) takes only the bill
		// lock, so acquiring it here before deciding serializes cancel/expiry
		// against a settling payment — the PaidAmount we read below cannot be
		// invalidated by a payment committing mid-cancel. A non-locking
		// pre-read learns the bill ID without inverting the order.
		var pre database.DeliveryOrder
		if err := tx.Select("id", "bill_id").First(&pre, deliveryID).Error; err != nil {
			return fmt.Errorf("delivery order not found: %w", err)
		}
		if pre.BillID != 0 {
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Select("id", "bill_number", "business_id", "status", "paid_amount", "total_amount").
				First(&bill, pre.BillID).Error
			if err == nil {
				haveBill = true
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&delivery, deliveryID).Error; err != nil {
			return fmt.Errorf("delivery order not found: %w", err)
		}
		if delivery.Status.IsTerminal() {
			alreadyTerminal = true
			return nil // idempotent — exit early, no writes
		}

		// Expiry-specific guard: re-verify under the bill lock that the bill is
		// still unpaid. A payment committing after the sweep's pre-check but
		// before we reach here must win — "a paid order can't expire".
		if kind == "expired" {
			if delivery.Status != database.DeliveryStatusConfirmed {
				// Delivery advanced past confirmed between sweep scan and this lock —
				// treat as already handled (preparing/etc. can't be "expired").
				alreadyTerminal = true
				return nil
			}
			if haveBill && (bill.Status == database.BillStatusPaid || bill.PaidAmount >= bill.TotalAmount) {
				return errExpiredButPaid
			}
		}

		now := time.Now().UTC()
		driverID := delivery.DriverID
		delivery.Status = terminalStatus
		delivery.CancelledAt = &now
		delivery.CancelledBy = actor
		delivery.CancellationReason = reason
		delivery.DriverID = nil
		delivery.AssignedAt = nil
		delivery.PaymentExpiresAt = nil
		if err := tx.Omit(clause.Associations).Save(&delivery).Error; err != nil {
			return err
		}
		if driverID != nil {
			if err := releaseDriverTx(tx, *driverID, deliveryID); err != nil {
				return err
			}
		}
		return tx.Create(&database.DeliveryStatusHistory{
			DeliveryOrderID: deliveryID, Status: terminalStatus,
			Notes: fmt.Sprintf("Order %s: %s", kind, reason), ChangedBy: actor,
		}).Error
	})
	if errors.Is(err, errExpiredButPaid) {
		return err // surface to caller (ExpireUnpaidDeliveries) for reconcile
	}
	if err != nil {
		return err
	}

	// Already terminal on entry — skip all propagation, re-emission, and
	// notifications so a duplicate call is a true no-op (no second emails).
	if alreadyTerminal {
		return nil
	}

	// Propagate to order + bill outside the delivery tx (each is itself
	// transactional and idempotent).
	if delivery.OrderID != nil {
		if err := database.CancelDeliveryLinkedOrder(*delivery.OrderID, actor, reason); err != nil &&
			!isOrderAlreadyTerminalErr(err) {
			log.Printf("delivery cancel: order cancel failed (delivery=%d order=%d): %v", deliveryID, *delivery.OrderID, err)
		}
	}
	// Bill decision uses the snapshot taken under the bill lock: an unpaid open
	// bill is closed; a bill carrying ANY guest money is left alone and flagged
	// for refund review instead (DEL-PAY-03) — money records are never silently
	// stranded on a cancelled/failed delivery, and refunds are manual by design.
	if haveBill && bill.Status == database.BillStatusOpen && bill.PaidAmount == 0 {
		remaining := bill.TotalAmount - bill.PaidAmount
		var closeErr error
		if remaining > 0 {
			// Operator CloseBill refuses leftover money. Delivery walk-out is
			// the explicit abandon door for an unpaid cancelled delivery.
			closeErr = database.AbandonUnpaidOpenBill(bill.ID, actor, "delivery cancelled unpaid")
		} else {
			closeErr = database.CloseBill(bill.ID)
		}
		if closeErr != nil && !errors.Is(closeErr, database.ErrBillNotOpen) {
			log.Printf("delivery cancel: bill close failed (delivery=%d bill=%d): %v", deliveryID, bill.ID, closeErr)
		}
	}
	if haveBill && bill.PaidAmount > 0 {
		s.raiseDeliveryRefundReviewAlert(&delivery, &bill, kind)
	}

	s.emitDeliveryCancelled(&delivery)
	s.notifyDeliveryTerminated(&delivery, kind)
	return nil
}

// raiseDeliveryRefundReviewAlert flags guest money stranded on a delivery that
// went terminal without being delivered (rejected/cancelled/expired/failed).
// The platform never auto-refunds, so the operator must review and refund
// out-of-band. Metadata carries identifiers only — amounts never enter alert
// metadata (alerts:read is a broader audience than financial:read).
func (s *DeliveryService) raiseDeliveryRefundReviewAlert(delivery *database.DeliveryOrder, bill *database.Bill, kind string) {
	if s.db == nil || delivery == nil || bill == nil {
		return
	}
	alertSvc := operational_alerts.NewService(s.db)
	if _, err := alertSvc.UpsertAlert(context.Background(), operational_alerts.UpsertAlertInput{
		BusinessID:   delivery.BusinessID,
		AlertType:    database.OperationalAlertTypePaymentRefundReview,
		ResourceType: database.OperationalAlertResourceTypeDelivery,
		ResourceID:   delivery.ID,
		Priority:     database.OperationalAlertPriorityHigh,
		Title:        "Paid delivery cancelled — refund review needed",
		Body: fmt.Sprintf(
			"Delivery #%s is %s but the guest has paid on bill #%s. Review and refund manually — automatic refunds are never issued.",
			delivery.DeliveryNumber, kind, bill.BillNumber,
		),
		Metadata: map[string]any{
			"delivery_id":     delivery.ID,
			"delivery_number": delivery.DeliveryNumber,
			"bill_id":         bill.ID,
			"bill_number":     bill.BillNumber,
			"reason":          kind,
		},
	}); err != nil {
		log.Printf("delivery refund-review alert failed (delivery=%d bill=%d): %v", delivery.ID, bill.ID, err)
	}
}

// releaseDriverTx frees a driver inside the caller's transaction: back
// online, current delivery cleared. Fixes the permanent-busy brick (R4).
func releaseDriverTx(tx *gorm.DB, driverID, deliveryID uint) error {
	return tx.Model(&database.DeliveryDriver{}).
		Where("id = ? AND current_delivery_id = ?", driverID, deliveryID).
		Updates(map[string]interface{}{
			"status":              database.DriverStatusOnline,
			"current_delivery_id": nil,
		}).Error
}

func isOrderAlreadyTerminalErr(err error) bool {
	// database.UpdateOrderStatus returns a ValidateTransition error for
	// terminal→terminal; treat as idempotent success.
	return err != nil && errors.Is(err, database.ErrInvalidStatusTransition)
}

// notifyDeliveryAccepted sends the guest a localized email after staff accept.
// Prepay: includes the payment link and countdown. COD: confirms kitchen has started.
func (s *DeliveryService) notifyDeliveryAccepted(delivery *database.DeliveryOrder, mode database.DeliveryPaymentMode) {
	if delivery.CustomerEmail == "" {
		return
	}
	business, err := database.GetBusinessByID(delivery.BusinessID)
	if err != nil {
		log.Printf("notifyDeliveryAccepted: failed to load business %d: %v", delivery.BusinessID, err)
	}
	locale := guestNotificationLocale(delivery, business)
	var title, body string
	if mode == database.DeliveryPaymentOnline {
		payURL := fmt.Sprintf("%s/delivery/%s/pay", publicSiteBaseURL(), delivery.DeliveryNumber)
		title, body = localizedAcceptedPayMessage(delivery.DeliveryNumber, payURL, int(deliveryPaymentWindow.Minutes()), locale)
	} else {
		total := "—"
		var bill database.Bill
		if s.db != nil {
			if dbErr := s.db.Select("total_amount").First(&bill, delivery.BillID).Error; dbErr == nil {
				// DEL-GT-5: format in the business currency with its real minor
				// units — never a hardcoded "$"/2-decimals (JPY has no cents).
				total = formatDeliveryMoney(bill.TotalAmount, businessGuestCurrency(business))
			}
		}
		title, body = localizedAcceptedCODMessage(delivery.DeliveryNumber, total, locale)
	}
	s.sendDeliveryNotificationToEmail(deliveryCustomerMailOrigin(delivery, ""), delivery.CustomerEmail, delivery.CustomerName, title, body, locale)
}

// notifyDeliveryTerminated sends the guest a localized email when an order ends
// terminally. kind ∈ {"rejected","cancelled","expired"}: only "expired" has
// dedicated copy; rejected/cancelled reuse the shared cancellation path.
func (s *DeliveryService) notifyDeliveryTerminated(delivery *database.DeliveryOrder, kind string) {
	if kind == "expired" {
		if delivery.CustomerEmail == "" {
			return
		}
		business, berr := database.GetBusinessByID(delivery.BusinessID)
		if berr != nil {
			log.Printf("notifyDeliveryTerminated: failed to load business %d: %v", delivery.BusinessID, berr)
		}
		locale := guestNotificationLocale(delivery, business)
		title, body := localizedExpiredMessage(delivery.DeliveryNumber, locale)
		s.sendDeliveryNotificationToEmail(deliveryCustomerMailOrigin(delivery, ""), delivery.CustomerEmail, delivery.CustomerName, title, body, locale)
		return
	}
	s.notifyCancellation(delivery)
}

func (s *DeliveryService) emitDeliveryUpdated(delivery *database.DeliveryOrder) {
	events.GetHub().PublishJSON(delivery.BusinessID, "delivery.updated", delivery)
}

func (s *DeliveryService) emitDeliveryCancelled(delivery *database.DeliveryOrder) {
	events.GetHub().PublishJSON(delivery.BusinessID, "delivery.cancelled", delivery)
	if delivery.OrderID != nil {
		// X-7: carry BOTH `id` and `order_id` plus reason/cancelled_by so the
		// payload matches server.publishOrderCancelledEvent additively.
		events.GetHub().PublishJSON(delivery.BusinessID, "order.cancelled", map[string]interface{}{
			"id":           *delivery.OrderID,
			"order_id":     *delivery.OrderID,
			"bill_id":      delivery.BillID,
			"status":       "cancelled",
			"reason":       delivery.CancellationReason,
			"cancelled_by": delivery.CancelledBy,
		})
	}
}

// eventsPublishOrderUpdated pushes an order.updated SSE event to the hub.
// Used by the payment hook to surface kitchen-ready state after bill payment.
func eventsPublishOrderUpdated(order *database.Order) {
	if order == nil {
		return
	}
	if err := database.HydrateOrderSnapshotForWire(order); err != nil {
		log.Printf("order.updated snapshot hydrate failed: order_id=%d error=%v", order.ID, err)
		order.Items = ""
	}
	events.GetHub().PublishJSON(order.BusinessID, "order.updated", order)
}
