package database

import "fmt"

// ErrInvalidStatusTransition is returned when an order status change violates
// the allowed state machine transitions.
var ErrInvalidStatusTransition = fmt.Errorf("invalid order status transition")

// ErrDeliveryLinkedCancel is returned when a delivery-linked order is
// cancelled outside the delivery lifecycle. The lifecycle (services.
// DeliveryService.RejectDeliveryByOrder → cancelDeliveryLinked) must own the
// cancel so the delivery leg goes terminal, the driver is released, the
// unpaid bill is closed, and the guest is emailed. Handlers map this to 409.
var ErrDeliveryLinkedCancel = fmt.Errorf("delivery-linked orders must be cancelled through the delivery lifecycle")

// validOrderTransitions defines the allowed from → to state machine edges.
// Cancellation is allowed from any non-terminal state.
var validOrderTransitions = map[OrderStatus][]OrderStatus{
	OrderStatusPending:        {OrderStatusApproved, OrderStatusOrderCancelled},
	OrderStatusApproved:       {OrderStatusInKitchen, OrderStatusOrderCancelled},
	OrderStatusInKitchen:      {OrderStatusOrderReady, OrderStatusOrderCancelled},
	OrderStatusOrderReady:     {OrderStatusOrderDelivered, OrderStatusOrderCancelled},
	OrderStatusOrderDelivered: {}, // terminal
	OrderStatusOrderCancelled: {}, // terminal
}

// ValidateTransition returns nil if the transition from → to is allowed,
// or ErrInvalidStatusTransition otherwise.
func ValidateTransition(from, to OrderStatus) error {
	allowed, ok := validOrderTransitions[from]
	if !ok {
		return fmt.Errorf("%w: unknown current status %q", ErrInvalidStatusTransition, from)
	}
	for _, s := range allowed {
		if s == to {
			return nil
		}
	}
	return fmt.Errorf("%w: cannot transition from %q to %q", ErrInvalidStatusTransition, from, to)
}
