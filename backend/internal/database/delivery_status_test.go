package database

import "testing"

// TestDeliveryStatusIsValid locks in the allow-list used to reject arbitrary
// delivery-status strings on write/filter paths.
func TestDeliveryStatusIsValid(t *testing.T) {
	valid := []DeliveryStatus{
		DeliveryStatusPending, DeliveryStatusConfirmed, DeliveryStatusPreparing,
		DeliveryStatusReady, DeliveryStatusAssigned, DeliveryStatusPickedUp,
		DeliveryStatusInTransit, DeliveryStatusNearby, DeliveryStatusDelivered,
		DeliveryStatusCancelled, DeliveryStatusFailed,
	}
	for _, s := range valid {
		if !s.IsValid() {
			t.Errorf("expected %q to be valid", s)
		}
	}

	invalid := []DeliveryStatus{"", "shipped", "foobar", "Delivered", "in transit"}
	for _, s := range invalid {
		if s.IsValid() {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}
