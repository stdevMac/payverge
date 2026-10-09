package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestReservationAlertRouting pins the alert type + priority routing for
// guest-created reservations: pending approval requests are urgent (they have
// a deadline), auto-confirmed bookings are urgent only when the party arrives
// within 2 hours, otherwise normal.
func TestReservationAlertRouting(t *testing.T) {
	now := time.Now()

	pending := &database.TableReservation{Status: "pending", ReservationTime: now.Add(48 * time.Hour)}
	alertType, priority := reservationAlertRoute(pending, now)
	assert.Equal(t, database.OperationalAlertTypeReservationApproval, alertType)
	assert.Equal(t, database.OperationalAlertPriorityUrgent, priority)

	imminent := &database.TableReservation{Status: "confirmed", ReservationTime: now.Add(90 * time.Minute)}
	alertType, priority = reservationAlertRoute(imminent, now)
	assert.Equal(t, database.OperationalAlertTypeReservationNew, alertType)
	assert.Equal(t, database.OperationalAlertPriorityUrgent, priority)

	future := &database.TableReservation{Status: "confirmed", ReservationTime: now.Add(72 * time.Hour)}
	alertType, priority = reservationAlertRoute(future, now)
	assert.Equal(t, database.OperationalAlertTypeReservationNew, alertType)
	assert.Equal(t, database.OperationalAlertPriorityNormal, priority)

	// Boundary: exactly 2 hours out is still urgent (<=).
	boundary := &database.TableReservation{Status: "confirmed", ReservationTime: now.Add(2 * time.Hour)}
	alertType, priority = reservationAlertRoute(boundary, now)
	assert.Equal(t, database.OperationalAlertTypeReservationNew, alertType)
	assert.Equal(t, database.OperationalAlertPriorityUrgent, priority)
}
