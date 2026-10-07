package services

import "time"

// CanPerformArrivalActions is the temporal gate for seat/check-in/no-show
// transitions (audit L1-13). Future-dated rows may be confirmed or cancelled,
// but arrival actions only make sense once the reservation is within
// graceMinutes of its start (or already past it).
func CanPerformArrivalActions(reservationTime, now time.Time, graceMinutes int) bool {
	if reservationTime.IsZero() {
		return false
	}
	if graceMinutes < 0 {
		graceMinutes = 0
	}
	earliest := reservationTime.Add(-time.Duration(graceMinutes) * time.Minute)
	return !earliest.After(now)
}

// EarlySeatingGraceMinutes is the shared early-arrival window for SEATING
// only, mirrored on the frontend by EARLY_SEATING_GRACE_MINUTES in
// components/business/reservations/canPerformArrivalActions.ts. Guests
// routinely arrive ahead of their slot; with a zero grace a 14:45 arrival for
// a 15:00 booking could not be seated at all.
const EarlySeatingGraceMinutes = 30

// DefaultNoShowGraceMinutes is the host-stand clock when settings are missing.
// Mirrors reservation_settings.no_show_grace_minutes (default 15).
const DefaultNoShowGraceMinutes = 15

// ArrivalPhase is the shared late / no-show clock for confirmed (and pending)
// bookings. NEXT ARRIVAL, table Reserved, and the operator status chip must
// all use this so a 15:00 Confirmed cannot sit as "None" / Available / Confirmed
// four hours later.
type ArrivalPhase string

const (
	ArrivalPhaseUpcoming ArrivalPhase = "upcoming"
	ArrivalPhaseLate     ArrivalPhase = "late"
	ArrivalPhaseExpired  ArrivalPhase = "expired"
	ArrivalPhaseOther    ArrivalPhase = "other"
)

// NormalizeNoShowGraceMinutes clamps a missing/negative setting to the default.
func NormalizeNoShowGraceMinutes(graceMinutes int) int {
	if graceMinutes < 0 {
		return DefaultNoShowGraceMinutes
	}
	return graceMinutes
}

// NoShowCutoff is reservation start plus the configured no-show grace.
func NoShowCutoff(reservationTime time.Time, graceMinutes int) time.Time {
	return reservationTime.Add(time.Duration(NormalizeNoShowGraceMinutes(graceMinutes)) * time.Minute)
}

// ReservationArrivalPhase is the single clock for past-due Confirmed/Pending.
//   - upcoming: before start
//   - late: start <= now < start+grace (still expected; holds the table)
//   - expired: now >= start+grace (auto no-show; table is free)
func ReservationArrivalPhase(reservationTime time.Time, status string, now time.Time, graceMinutes int) ArrivalPhase {
	switch status {
	case "pending", "confirmed":
	default:
		return ArrivalPhaseOther
	}
	if reservationTime.IsZero() {
		return ArrivalPhaseOther
	}
	if now.Before(reservationTime) {
		return ArrivalPhaseUpcoming
	}
	if now.Before(NoShowCutoff(reservationTime, graceMinutes)) {
		return ArrivalPhaseLate
	}
	return ArrivalPhaseExpired
}

// ShouldAutoNoShow is true for confirmed bookings past the no-show grace.
// Pending rows stay on the approval sweeper (timeout/cancel), not no-show.
func ShouldAutoNoShow(reservationTime time.Time, status string, now time.Time, graceMinutes int) bool {
	return status == "confirmed" && ReservationArrivalPhase(reservationTime, status, now, graceMinutes) == ArrivalPhaseExpired
}

// CanSeatReservation is the temporal gate for seat/check-in transitions. It
// honors the early-arrival grace.
func CanSeatReservation(reservationTime, now time.Time) bool {
	return CanPerformArrivalActions(reservationTime, now, EarlySeatingGraceMinutes)
}

// CanMarkNoShow is the temporal gate for no-show marks. It deliberately gets
// NO early grace: a guest cannot be a no-show before the reservation has even
// started.
func CanMarkNoShow(reservationTime, now time.Time) bool {
	return CanPerformArrivalActions(reservationTime, now, 0)
}
