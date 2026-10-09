import { formatBusinessTime, TIME_SHORT } from "./businessTime";
import { elapsedUrgency, type UrgencyTone } from "./humanizeDuration";

const NAIVE_ISO =
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(?::\d{2}(?:\.\d+)?)?$/;
const HAS_OFFSET = /(?:Z|[+-]\d{2}:\d{2})$/i;

/**
 * Parse an order timestamp as a real instant.
 *
 * Go emits RFC3339 with a `Z`/offset. A naive `YYYY-MM-DDTHH:mm[:ss]` is
 * treated as UTC so the operator device timezone cannot shift fire age by
 * the venue offset (the #662 "aging follows UTC → off by 4 hours" report).
 */
export function parseOrderInstant(value: string | number | Date): Date {
  if (value instanceof Date) return value;
  if (typeof value === "number") return new Date(value);
  const raw = value.trim();
  if (NAIVE_ISO.test(raw) && !HAS_OFFSET.test(raw)) {
    return new Date(`${raw}Z`);
  }
  return new Date(raw);
}

/**
 * Ticket fire clock in the venue IANA zone. Locale only picks 12h/24h.
 * Goes through parseOrderInstant so a naive `2026-08-19T22:12:00` is the
 * same instant as the filed `22:12Z` stamp — never device-local midnight.
 */
export function formatKdsFireTime(
  createdAt: string | number | Date,
  locale: string,
  timeZone: string | null,
): string {
  const instant = parseOrderInstant(createdAt);
  if (Number.isNaN(instant.getTime())) return String(createdAt);
  return formatBusinessTime(instant, locale, timeZone, TIME_SHORT);
}

/**
 * Minutes the ticket has been up. Instant-based so EN / es / es-AR agree, and
 * so a 22:12Z fire is 5 minutes old at 22:17Z in America/New_York — not 4h 5m
 * from comparing the UTC wall to the venue wall.
 */
export function kdsElapsedMinutes(
  createdAt: string | number | Date,
  now: Date | number,
): number {
  const created = parseOrderInstant(createdAt);
  const nowMs = typeof now === "number" ? now : now.getTime();
  if (Number.isNaN(created.getTime()) || !Number.isFinite(nowMs)) return 0;
  return Math.max(0, Math.floor((nowMs - created.getTime()) / 60000));
}

export function kdsElapsedUrgency(minutes: number): UrgencyTone {
  return elapsedUrgency(minutes);
}
