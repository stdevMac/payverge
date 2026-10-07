package services

import "time"

// ReservationApprovalDeadline is the moment a pending (manual-approval)
// reservation request auto-declines if the business never acts:
// min(created+24h, reservation-2h), but never sooner than created+30m so a
// same-day request always gets a real review window.
// Mirrored in frontend/src/components/business/ReservationApprovalQueue.tsx
// (approvalDeadline) — keep both in sync.
func ReservationApprovalDeadline(createdAt, reservationTime time.Time) time.Time {
	deadline := reservationTime.Add(-2 * time.Hour)
	if cap := createdAt.Add(24 * time.Hour); cap.Before(deadline) {
		deadline = cap
	}
	if floor := createdAt.Add(30 * time.Minute); deadline.Before(floor) {
		deadline = floor
	}
	return deadline
}

// ReservationApprovalReminderTime is when the operator gets a "still
// unactioned" nudge email for a pending request: the halfway point of the
// request's own approval window. A standard 24h window nudges at 12h; a
// same-day 6h window nudges at 3h; even the 30m floor window nudges at 15m,
// when the request is genuinely urgent. Requests actioned before this moment
// never generate an email.
func ReservationApprovalReminderTime(createdAt, reservationTime time.Time) time.Time {
	deadline := ReservationApprovalDeadline(createdAt, reservationTime)
	return createdAt.Add(deadline.Sub(createdAt) / 2)
}
