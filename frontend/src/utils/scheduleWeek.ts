import { localDateKey } from "@/lib/localDate";
import {
  businessDateKey,
  resolveBusinessTimeZone,
} from "@/utils/businessTime";
import { intlLocaleFor } from "@/utils/intlLocale";

// Week-key helpers shared by the staff `MyScheduleView` and the operator
// `ScheduleBuilder`. A schedule row is keyed by its `week_start` date, and the
// backend floors the `?week=` value to UTC midnight WITHOUT snapping it to a
// week-start day (see backend parseWeekValue). So both surfaces MUST derive the
// same YYYY-MM-DD key for a given calendar week, or staff would query a date the
// operator never keyed and see nothing.

// 0 = Sunday .. 6 = Saturday. Backend ScheduleSettings default is 1 (Monday).
export const DEFAULT_WEEK_START_DAY = 1;

const WEEKDAY_INDEX: Record<string, number> = {
  Sun: 0,
  Mon: 1,
  Tue: 2,
  Wed: 3,
  Thu: 4,
  Fri: 5,
  Sat: 6,
};

/** Coerce a stored week_start_day (0 is Sunday — never treat as falsy). */
export function resolveWeekStartDay(value: unknown): number {
  if (value === null || value === undefined || value === "") {
    return DEFAULT_WEEK_START_DAY;
  }
  const n = Number(value);
  if (Number.isInteger(n) && n >= 0 && n <= 6) return n;
  return DEFAULT_WEEK_START_DAY;
}

/**
 * Long weekday name for a stored week_start_day (0=Sun..6=Sat).
 *
 * #664: the anchor is Sunday 2024-01-07 constructed with Date.UTC, so it MUST
 * be formatted in UTC. A device behind UTC (America/Buenos_Aires) otherwise
 * reads the midnight-UTC Monday as Sunday — "La semana empieza el: domingo"
 * while the grid correctly starts LUN.
 */
export function weekStartDayName(day: number, locale: string): string {
  const startDay = resolveWeekStartDay(day);
  return new Intl.DateTimeFormat(intlLocaleFor(locale), {
    weekday: "long",
    timeZone: "UTC",
  }).format(new Date(Date.UTC(2024, 0, 7 + startDay)));
}

/** JS weekday (0=Sun..6=Sat) of `reference` in `timeZone`, or device-local. */
export function weekdayInTimeZone(
  reference: Date,
  timeZone?: string | null,
): number {
  if (!timeZone) return reference.getDay();
  const wd = new Intl.DateTimeFormat("en-US", {
    timeZone: resolveBusinessTimeZone(timeZone),
    weekday: "short",
  }).format(reference);
  return WEEKDAY_INDEX[wd] ?? reference.getDay();
}

/**
 * The YYYY-MM-DD date of the week-start (aligned to `weekStartDay`) for the
 * calendar week that contains `reference`. Pass `timeZone` (venue IANA) so the
 * grid follows the restaurant day, not the operator device.
 */
export function weekStartKey(
  reference: Date = new Date(),
  weekStartDay: number = DEFAULT_WEEK_START_DAY,
  timeZone?: string | null,
): string {
  const startDay = resolveWeekStartDay(weekStartDay);
  if (timeZone) {
    const dateKey = businessDateKey(reference, timeZone);
    const [y, m, d] = dateKey.split("-").map(Number);
    const dow = weekdayInTimeZone(reference, timeZone);
    const back = (dow - startDay + 7) % 7;
    const start = new Date(Date.UTC(y, m - 1, d - back));
    const sy = start.getUTCFullYear();
    const sm = String(start.getUTCMonth() + 1).padStart(2, "0");
    const sd = String(start.getUTCDate()).padStart(2, "0");
    return `${sy}-${sm}-${sd}`;
  }
  const dow = reference.getDay(); // 0=Sun..6=Sat (local)
  const back = (dow - startDay + 7) % 7;
  const start = new Date(
    reference.getFullYear(),
    reference.getMonth(),
    reference.getDate() - back,
  );
  return localDateKey(start);
}

/** Shift a YYYY-MM-DD week key by N weeks (calendar-safe across DST/month ends). */
export function shiftWeekKey(weekKey: string, weeks: number): string {
  const [y, m, d] = weekKey.split("-").map(Number);
  const base = new Date(y, m - 1, d + weeks * 7);
  return localDateKey(base);
}

/** The seven YYYY-MM-DD day keys of the week starting at `weekKey`. */
export function weekDayKeys(weekKey: string): string[] {
  const [y, m, d] = weekKey.split("-").map(Number);
  return Array.from({ length: 7 }, (_, i) => localDateKey(new Date(y, m - 1, d + i)));
}
