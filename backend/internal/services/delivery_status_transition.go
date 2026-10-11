package services

import (
	"errors"
	"fmt"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// ErrInvalidDeliveryTransition is returned by UpdateDeliveryStatus when a write
// requests a status edge that is not allowed by the delivery state machine
// (a backward move, or a forward edge that would bypass payment). It is
// distinct from ErrDeliveryStatusTerminal (transition OUT of a final state) so
// handlers and callers can tell "already done" from "illegal move".
var ErrInvalidDeliveryTransition = errors.New("invalid delivery status transition")

// validDeliveryTransitions defines the allowed from → to edges for the operator
// status-update path (UpdateDeliveryStatus). It mirrors the order state machine
// in database.validOrderTransitions.
//
// The lifecycle is forward-only and skip-tolerant within the fulfillment phase
// (an operator may jump pending→ready, ready→in_transit, etc. — intermediate
// hops are optional), but two edges are deliberately forbidden:
//
//   - Backward moves (e.g. ready→preparing): they corrupt the lifecycle
//     timeline and driver-performance KPIs.
//   - Any non-terminal advance OUT of confirmed: a prepay delivery sits at
//     "confirmed" with payment still owed. It is advanced to "preparing" ONLY
//     by the system payment path (HandleDeliveryBillPaid), which deducts
//     inventory and starts the kitchen. Letting a dispatch:write staffer jump
//     confirmed→preparing/delivered would bypass payment + inventory and email
//     a "delivered" confirmation for an unpaid order.
//
// In addition, validateOperatorFulfillmentTransition layers a payment-mode
// guard on top of this map: pending + payment_mode_stored=online cannot
// advance into fulfillment at all (must Accept → confirmed → pay first). COD
// and legacy empty mode keep the skip-tolerant pending edges below.
//
// Terminal states (delivered/cancelled/failed) are reachable from any
// non-terminal state. cancelled/failed are routed through cancelDeliveryLinked
// before reaching this map, so the entries here cover the operator-driven
// non-cancel edges plus "delivered"/"failed" as terminal destinations.
var validDeliveryTransitions = map[database.DeliveryStatus][]database.DeliveryStatus{
	database.DeliveryStatusPending: {
		database.DeliveryStatusConfirmed, database.DeliveryStatusPreparing,
		database.DeliveryStatusReady, database.DeliveryStatusAssigned,
		database.DeliveryStatusPickedUp, database.DeliveryStatusInTransit,
		database.DeliveryStatusNearby, database.DeliveryStatusDelivered,
		database.DeliveryStatusCancelled, database.DeliveryStatusFailed,
	},
	// confirmed: NO non-terminal advance — only the system payment path may
	// move it forward. Terminal cancel/fail still allowed.
	database.DeliveryStatusConfirmed: {
		database.DeliveryStatusCancelled, database.DeliveryStatusFailed,
	},
	database.DeliveryStatusPreparing: {
		database.DeliveryStatusReady, database.DeliveryStatusAssigned,
		database.DeliveryStatusPickedUp, database.DeliveryStatusInTransit,
		database.DeliveryStatusNearby, database.DeliveryStatusDelivered,
		database.DeliveryStatusCancelled, database.DeliveryStatusFailed,
	},
	database.DeliveryStatusReady: {
		database.DeliveryStatusAssigned, database.DeliveryStatusPickedUp,
		database.DeliveryStatusInTransit, database.DeliveryStatusNearby,
		database.DeliveryStatusDelivered, database.DeliveryStatusCancelled,
		database.DeliveryStatusFailed,
	},
	database.DeliveryStatusAssigned: {
		database.DeliveryStatusPickedUp, database.DeliveryStatusInTransit,
		database.DeliveryStatusNearby, database.DeliveryStatusDelivered,
		database.DeliveryStatusCancelled, database.DeliveryStatusFailed,
	},
	database.DeliveryStatusPickedUp: {
		database.DeliveryStatusInTransit, database.DeliveryStatusNearby,
		database.DeliveryStatusDelivered, database.DeliveryStatusCancelled,
		database.DeliveryStatusFailed,
	},
	database.DeliveryStatusInTransit: {
		database.DeliveryStatusNearby, database.DeliveryStatusDelivered,
		database.DeliveryStatusCancelled, database.DeliveryStatusFailed,
	},
	database.DeliveryStatusNearby: {
		database.DeliveryStatusDelivered, database.DeliveryStatusCancelled,
		database.DeliveryStatusFailed,
	},
	database.DeliveryStatusDelivered: {}, // terminal
	database.DeliveryStatusCancelled: {}, // terminal
	database.DeliveryStatusFailed:    {}, // terminal
}

// validateDeliveryTransition returns nil if from → to is an allowed edge, a
// no-op (from == to), or otherwise an ErrInvalidDeliveryTransition. Callers
// must already have rejected transitions OUT of a terminal state with the more
// specific ErrDeliveryStatusTerminal before reaching here.
func validateDeliveryTransition(from, to database.DeliveryStatus) error {
	if from == to {
		return nil // redundant no-op is allowed
	}
	allowed, ok := validDeliveryTransitions[from]
	if !ok {
		return fmt.Errorf("%w: unknown current status %q", ErrInvalidDeliveryTransition, from)
	}
	for _, s := range allowed {
		if s == to {
			return nil
		}
	}
	return fmt.Errorf("%w: cannot transition from %q to %q", ErrInvalidDeliveryTransition, from, to)
}

// deliveryRequiresAcceptBeforeFulfillment is true for online-prepay deliveries
// still at intake (pending). Guest checkout leaves them here with an open
// unpaid bill; staff must Accept (→confirmed + payment window) and the guest
// must pay (→preparing via HandleDeliveryBillPaid) before assign/status
// fulfillment. Without this guard the skip-tolerant pending→assigned/delivered
// map edges let operators mark online orders delivered while the bill stays
// open and the kitchen order stays pending.
func deliveryRequiresAcceptBeforeFulfillment(d *database.DeliveryOrder) bool {
	if d == nil {
		return false
	}
	if d.Status != database.DeliveryStatusPending {
		return false
	}
	return d.PaymentModeStored == string(database.DeliveryPaymentOnline)
}

// validateOperatorFulfillmentTransition is the operator path guard for
// UpdateDeliveryStatus and AssignDriver: structural edges from
// validateDeliveryTransition plus the online-prepay Accept/pay gate.
// Cancel/fail never reach here (they route through cancelDeliveryLinked).
func validateOperatorFulfillmentTransition(d *database.DeliveryOrder, to database.DeliveryStatus) error {
	if d == nil {
		return fmt.Errorf("%w: missing delivery", ErrInvalidDeliveryTransition)
	}
	if err := validateDeliveryTransition(d.Status, to); err != nil {
		return err
	}
	if deliveryRequiresAcceptBeforeFulfillment(d) {
		return fmt.Errorf("%w: online prepay delivery must be accepted and paid before fulfillment (cannot transition from %q to %q)",
			ErrInvalidDeliveryTransition, d.Status, to)
	}
	return nil
}
