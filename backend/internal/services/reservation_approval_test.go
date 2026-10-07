package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReservationApprovalDeadline(t *testing.T) {
	created := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	// Far-future booking: capped at created+24h.
	farFuture := created.AddDate(0, 0, 20)
	assert.Equal(t, created.Add(24*time.Hour), ReservationApprovalDeadline(created, farFuture))

	// Tomorrow-evening booking: reservation-2h wins over created+24h.
	tomorrow := created.Add(26 * time.Hour)
	assert.Equal(t, tomorrow.Add(-2*time.Hour), ReservationApprovalDeadline(created, tomorrow))

	// Booked 90 minutes ahead: reservation-2h is in the past -> floor at created+30m.
	soon := created.Add(90 * time.Minute)
	assert.Equal(t, created.Add(30*time.Minute), ReservationApprovalDeadline(created, soon))
}

func TestReservationApprovalReminderTime(t *testing.T) {
	created := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	// Far-future booking: 24h window -> reminder at the 12h halfway mark.
	farFuture := created.AddDate(0, 0, 20)
	assert.Equal(t, created.Add(12*time.Hour), ReservationApprovalReminderTime(created, farFuture))

	// Same-day booking 8h out: deadline is reservation-2h (6h window) -> reminder at 3h.
	sameDay := created.Add(8 * time.Hour)
	assert.Equal(t, created.Add(3*time.Hour), ReservationApprovalReminderTime(created, sameDay))

	// Last-minute booking: 30m floor window -> reminder at 15m.
	soon := created.Add(90 * time.Minute)
	assert.Equal(t, created.Add(15*time.Minute), ReservationApprovalReminderTime(created, soon))
}
