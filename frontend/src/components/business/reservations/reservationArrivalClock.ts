/**
 * Shared late / no-show clock for operator reservations.
 * Mirrors backend services.ReservationArrivalPhase so NEXT ARRIVAL, the
 * status chip, and table Reserved cannot disagree.
 */

import { wallTimeToInstant } from "@/utils/zonedDateTime";

export const DEFAULT_NO_SHOW_GRACE_MINUTES = 15;

const NAIVE_WALL_TIME =
  /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2})(?::\d{2}(?:\.\d+)?)?$/;

/**
 * Instant for a reservation_time. Naive wall strings (no Z / offset) are
 * interpreted in the venue timezone so a device in ART cannot expire a
 * 19:00 America/New_York booking the host still has on Próximas.
 */
export function reservationInstant(
  value: string | Date,
  businessTimeZone?: string | null,
): Date {
  if (value instanceof Date) return value;
  const trimmed = String(value).trim();
  const naive = NAIVE_WALL_TIME.exec(trimmed);
  if (naive && businessTimeZone) {
    try {
      return wallTimeToInstant(naive[1], businessTimeZone);
    } catch {
      // DST gap / invalid zone — fall through to Date parse.
    }
  }
  return new Date(trimmed);
}

export type ReservationArrivalPhase =
  | "upcoming"
  | "late"
  | "expired"
  | "other";

function normalizeNoShowGraceMinutes(graceMinutes?: number | null): number {
  if (graceMinutes == null || graceMinutes < 0) {
    return DEFAULT_NO_SHOW_GRACE_MINUTES;
  }
  return graceMinutes;
}

export function reservationArrivalPhase(
  reservation: { reservation_time: string | Date; status: string },
  now: Date = new Date(),
  graceMinutes?: number | null,
  businessTimeZone?: string | null,
): ReservationArrivalPhase {
  if (reservation.status !== "pending" && reservation.status !== "confirmed") {
    return "other";
  }
  const start = reservationInstant(
    reservation.reservation_time,
    businessTimeZone,
  ).getTime();
  if (Number.isNaN(start)) return "other";
  if (now.getTime() < start) return "upcoming";
  const cutoff =
    start + normalizeNoShowGraceMinutes(graceMinutes) * 60_000;
  if (now.getTime() < cutoff) return "late";
  return "expired";
}

/** Operator chip: Late from start until the sweeper persists no_show. */
export function reservationDisplayStatus(
  reservation: { reservation_time: string | Date; status: string },
  now: Date = new Date(),
  graceMinutes?: number | null,
  businessTimeZone?: string | null,
): string {
  const phase = reservationArrivalPhase(
    reservation,
    now,
    graceMinutes,
    businessTimeZone,
  );
  if (phase === "late" || phase === "expired") {
    return "late";
  }
  return reservation.status;
}

/**
 * NEXT ARRIVAL candidate: pending/confirmed/waitlist that are still in the
 * future, or pending/confirmed still inside the no-show grace. Past-grace
 * Confirmed is not an arrival (it ages to no-show).
 */
export function isNextArrivalCandidate(
  reservation: { reservation_time: string | Date; status: string },
  now: Date,
  graceMinutes?: number | null,
  businessTimeZone?: string | null,
): boolean {
  if (!["pending", "confirmed", "waitlist"].includes(reservation.status)) {
    return false;
  }
  const start = reservationInstant(
    reservation.reservation_time,
    businessTimeZone,
  ).getTime();
  if (Number.isNaN(start)) return false;
  if (reservation.status === "waitlist") {
    return start > now.getTime();
  }
  const cutoff =
    start + normalizeNoShowGraceMinutes(graceMinutes) * 60_000;
  return cutoff > now.getTime();
}

export function isActiveArrivalStatus(status: string): boolean {
  return ["pending", "confirmed", "waitlist"].includes(status);
}
