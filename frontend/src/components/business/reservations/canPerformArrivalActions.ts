/**
 * Temporal gate for host arrival actions (seat / check-in / no-show).
 *
 * Status-only gating let operators mark future-dated rows as seated or
 * no-show (L1-13). Arrival actions are only meaningful once the reservation
 * is within the grace window before its start (default: same local day or
 * within graceMinutes of the slot).
 */
export function canPerformArrivalActions(
  reservation: { reservation_time: string | Date; status?: string },
  now: Date = new Date(),
  graceMinutes = 0,
): boolean {
  const start = new Date(reservation.reservation_time);
  if (Number.isNaN(start.getTime())) {
    return false;
  }
  const earliest = start.getTime() - Math.max(0, graceMinutes) * 60_000;
  return earliest <= now.getTime();
}

/** Statuses that expose seat / no-show in the operator UI. */
export function isArrivalActionStatus(status: string): boolean {
  return status === "pending" || status === "confirmed";
}

/**
 * Shared early-arrival grace for SEATING only, mirrored by the backend
 * (`services.EarlySeatingGraceMinutes`). Guests routinely show up ahead of
 * their slot; with a zero grace a 14:45 arrival for a 15:00 booking could not
 * be seated at all — the host button was hidden and the API rejected it.
 */
export const EARLY_SEATING_GRACE_MINUTES = 30;

/**
 * Seat / check-in affordance: correct status AND within the early-seating
 * grace window before the slot.
 */
export function canSeatNow(
  reservation: { reservation_time: string | Date; status: string },
  now: Date = new Date(),
): boolean {
  return (
    isArrivalActionStatus(reservation.status) &&
    canPerformArrivalActions(reservation, now, EARLY_SEATING_GRACE_MINUTES)
  );
}

/**
 * No-show affordance. Deliberately does NOT get the early-seating grace: a
 * guest cannot be a no-show before the reservation has even started.
 */
export function canMarkNoShowNow(
  reservation: { reservation_time: string | Date; status: string },
  now: Date = new Date(),
): boolean {
  return (
    isArrivalActionStatus(reservation.status) &&
    canPerformArrivalActions(reservation, now, 0)
  );
}
