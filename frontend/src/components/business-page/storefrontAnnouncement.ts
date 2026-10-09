/**
 * Operating-exception announcement logic (plan 3.5).
 *
 * Pure date helpers so the storefront can surface an upcoming holiday closure
 * or special-hours window as a dismissible banner above the hero. All
 * comparisons happen on YYYY-MM-DD date keys computed in the BUSINESS
 * timezone — never the visitor's or server's local clock.
 */

export interface StorefrontOperatingException {
  exception_date: string; // YYYY-MM-DD
  is_closed?: boolean;
  label?: string;
  open_time?: string | null;
  close_time?: string | null;
}

const DATE_KEY_RE = /^\d{4}-\d{2}-\d{2}$/;

/** YYYY-MM-DD of `now` in the given IANA timezone (UTC on invalid/missing tz). */
export function businessDateKey(now: Date, timezone?: string): string {
  const format = (tz: string) =>
    new Intl.DateTimeFormat("en-US", {
      timeZone: tz,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
    }).formatToParts(now);
  let parts: Intl.DateTimeFormatPart[];
  try {
    parts = format(timezone || "UTC");
  } catch {
    parts = format("UTC");
  }
  const lookup = Object.fromEntries(
    parts
      .filter((part) => part.type !== "literal")
      .map((part) => [part.type, part.value]),
  ) as Record<string, string>;
  return `${lookup.year}-${lookup.month}-${lookup.day}`;
}

/** Add (or subtract) whole days to a YYYY-MM-DD key, returning a YYYY-MM-DD key. */
export function addDaysToDateKey(dateKey: string, days: number): string {
  const [year, month, day] = dateKey.split("-").map(Number);
  const shifted = new Date(Date.UTC(year, month - 1, day + days));
  const y = shifted.getUTCFullYear();
  const m = String(shifted.getUTCMonth() + 1).padStart(2, "0");
  const d = String(shifted.getUTCDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

/**
 * The nearest exception whose date is today..today+windowDays (inclusive) in
 * the business timezone. ISO date keys compare lexicographically. Returns null
 * when nothing is upcoming inside the window.
 */
export function findUpcomingException<T extends StorefrontOperatingException>(
  exceptions: T[] | undefined | null,
  timezone?: string,
  now: Date = new Date(),
  windowDays = 7,
): T | null {
  if (!exceptions || exceptions.length === 0) return null;
  const today = businessDateKey(now, timezone);
  const lastDay = addDaysToDateKey(today, windowDays);
  const upcoming = exceptions
    .filter(
      (row) =>
        typeof row.exception_date === "string" &&
        DATE_KEY_RE.test(row.exception_date) &&
        row.exception_date >= today &&
        row.exception_date <= lastDay,
    )
    .sort((a, b) => a.exception_date.localeCompare(b.exception_date));
  return upcoming[0] ?? null;
}

/** sessionStorage key for a dismissed announcement, scoped per business + exception. */
export function announcementStorageKey(
  businessId: number,
  exception: { id?: number; exception_date: string },
): string {
  return `storefront-announce-dismissed:${businessId}:${exception.id ?? exception.exception_date}`;
}
