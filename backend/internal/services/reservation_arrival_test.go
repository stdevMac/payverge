package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCanPerformArrivalActions_L1_13(t *testing.T) {
	now := time.Date(2026, 6, 1, 15, 0, 0, 0, time.UTC)

	t.Run("allows at or after start", func(t *testing.T) {
		assert.True(t, CanPerformArrivalActions(
			time.Date(2026, 6, 1, 14, 0, 0, 0, time.UTC), now, 0,
		))
		assert.True(t, CanPerformArrivalActions(
			time.Date(2026, 6, 1, 15, 0, 0, 0, time.UTC), now, 0,
		))
	})

	t.Run("blocks future-dated rows", func(t *testing.T) {
		assert.False(t, CanPerformArrivalActions(
			time.Date(2026, 6, 3, 20, 0, 0, 0, time.UTC), now, 0,
		))
	})

	t.Run("honors grace window", func(t *testing.T) {
		// 30 min before start with 60 min grace → allowed
		assert.True(t, CanPerformArrivalActions(
			time.Date(2026, 6, 1, 15, 30, 0, 0, time.UTC), now, 60,
		))
		// 90 min before start with 60 min grace → blocked
		assert.False(t, CanPerformArrivalActions(
			time.Date(2026, 6, 1, 16, 30, 0, 0, time.UTC), now, 60,
		))
	})
}

func TestEarlySeatingGrace_R2_B5(t *testing.T) {
	now := time.Date(2026, 6, 1, 15, 0, 0, 0, time.UTC)

	t.Run("shared default is 30 minutes", func(t *testing.T) {
		assert.Equal(t, 30, EarlySeatingGraceMinutes)
	})

	t.Run("seats a guest arriving at start minus 30 min", func(t *testing.T) {
		assert.True(t, CanSeatReservation(
			time.Date(2026, 6, 1, 15, 30, 0, 0, time.UTC), now,
		))
	})

	t.Run("does not seat at start minus 31 min", func(t *testing.T) {
		assert.False(t, CanSeatReservation(
			time.Date(2026, 6, 1, 15, 31, 0, 0, time.UTC), now,
		))
	})

	t.Run("no-show stays impossible before start", func(t *testing.T) {
		// Inside the seating grace, but the guest is not late yet.
		assert.False(t, CanMarkNoShow(
			time.Date(2026, 6, 1, 15, 30, 0, 0, time.UTC), now,
		))
		assert.False(t, CanMarkNoShow(
			time.Date(2026, 6, 1, 15, 1, 0, 0, time.UTC), now,
		))
		// At start and after it, no-show is allowed.
		assert.True(t, CanMarkNoShow(
			time.Date(2026, 6, 1, 15, 0, 0, 0, time.UTC), now,
		))
		assert.True(t, CanMarkNoShow(
			time.Date(2026, 6, 1, 14, 30, 0, 0, time.UTC), now,
		))
	})
}

func TestReservationArrivalPhaseAndAutoNoShow(t *testing.T) {
	start := time.Date(2026, 8, 13, 19, 0, 0, 0, time.UTC) // 15:00 ET
	grace := 15

	t.Run("before start is upcoming", func(t *testing.T) {
		now := start.Add(-2 * time.Hour)
		assert.Equal(t, ArrivalPhaseUpcoming, ReservationArrivalPhase(start, "confirmed", now, grace))
		assert.False(t, ShouldAutoNoShow(start, "confirmed", now, grace))
	})

	t.Run("inside grace is late", func(t *testing.T) {
		now := start.Add(10 * time.Minute)
		assert.Equal(t, ArrivalPhaseLate, ReservationArrivalPhase(start, "confirmed", now, grace))
		assert.False(t, ShouldAutoNoShow(start, "confirmed", now, grace))
		assert.Equal(t, ArrivalPhaseLate, ReservationArrivalPhase(start, "pending", now, grace))
		assert.False(t, ShouldAutoNoShow(start, "pending", now, grace), "pending stays on the approval sweeper")
	})

	t.Run("past grace is expired and auto-no-shows confirmed", func(t *testing.T) {
		now := start.Add(4*time.Hour + 48*time.Minute) // ~19:48 ET
		assert.Equal(t, ArrivalPhaseExpired, ReservationArrivalPhase(start, "confirmed", now, grace))
		assert.True(t, ShouldAutoNoShow(start, "confirmed", now, grace))
		assert.False(t, ShouldAutoNoShow(start, "pending", now, grace))
		assert.False(t, ShouldAutoNoShow(start, "seated", now, grace))
	})

	t.Run("exactly at grace cutoff expires", func(t *testing.T) {
		now := start.Add(15 * time.Minute)
		assert.Equal(t, ArrivalPhaseExpired, ReservationArrivalPhase(start, "confirmed", now, grace))
		assert.True(t, ShouldAutoNoShow(start, "confirmed", now, grace))
	})
}
