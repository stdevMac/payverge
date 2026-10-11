import { intlLocaleFor } from "./intlLocale";

export type BusinessDateTimeOptions = Omit<
  Intl.DateTimeFormatOptions,
  "timeZone"
>;

/**
 * Named presentation presets (L8-2 / N-5).
 * Dashboard call sites must pass one of these instead of ad-hoc option objects
 * so the same instant renders consistently across tabs.
 *
 * - DATE_SHORT: "Jul 8, 2026" / "8 jul 2026"
 * - DATE_MONTH_DAY: "Jul 8" / "8 jul" (compact, no year — overview "next on")
 * - DATE_TIME_SHORT: "Jul 8, 2026, 8:53 PM" (always includes year)
 * - DATE_LONG: full weekday + long date + short time
 * - TIME_SHORT: "8:53 PM" / "20:53"
 * - RELATIVE: marker for call sites that intentionally use relative elapsed
 *   labels (humanizeDurationMinutes / localized timeAgo), not absolute times.
 */
export const DATE_SHORT: BusinessDateTimeOptions = {
  year: "numeric",
  month: "short",
  day: "numeric",
};

/** Compact month+day without year (overview next-arrival when not today). */
export const DATE_MONTH_DAY: BusinessDateTimeOptions = {
  month: "short",
  day: "numeric",
};

export const DATE_TIME_SHORT: BusinessDateTimeOptions = {
  year: "numeric",
  month: "short",
  day: "numeric",
  // Force 24h so es/en never mix "2:25" (ambiguous) with "20:53" (MIN-1).
  hour: "2-digit",
  minute: "2-digit",
  hourCycle: "h23",
};

export const DATE_LONG: BusinessDateTimeOptions = {
  dateStyle: "full",
  timeStyle: "short",
};

/** Clock time only — paired with DATE_MONTH_DAY on overview cards. */
export const TIME_SHORT: BusinessDateTimeOptions = {
  hour: "numeric",
  minute: "2-digit",
};

/** Sentinel: relative "time ago" paths — do not pass to formatBusinessDateTime. */
export const RELATIVE = { kind: "relative" as const };

/**
 * Resolve an explicitly supplied business IANA timezone. Operational screens
 * must never silently inherit the browser timezone: if business data is absent
 * or malformed, UTC is the stable and auditable fallback.
 */
export function resolveBusinessTimeZone(timeZone: string | null): string {
  if (!timeZone) return "UTC";
  try {
    new Intl.DateTimeFormat("en", { timeZone }).format(0);
    return timeZone;
  } catch {
    return "UTC";
  }
}

export function formatBusinessDateTime(
  value: string | number | Date,
  locale: string,
  timeZone: string | null,
  options: BusinessDateTimeOptions = DATE_TIME_SHORT,
): string {
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return new Intl.DateTimeFormat(intlLocaleFor(locale), {
    ...options,
    timeZone: resolveBusinessTimeZone(timeZone),
  }).format(date);
}

export function formatBusinessTime(
  value: string | number | Date,
  locale: string,
  timeZone: string | null,
  options: BusinessDateTimeOptions = TIME_SHORT,
): string {
  return formatBusinessDateTime(value, locale, timeZone, options);
}

/**
 * Calendar day key (YYYY-MM-DD) for an instant in the business timezone.
 * Prefer this over device-local date keys when comparing "is today" for
 * reservation/ops UI that is anchored to the restaurant day.
 */
export function businessDateKey(
  value: string | number | Date,
  timeZone: string | null,
): string {
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: resolveBusinessTimeZone(timeZone),
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(date);
  const y = parts.find((p) => p.type === "year")?.value;
  const m = parts.find((p) => p.type === "month")?.value;
  const d = parts.find((p) => p.type === "day")?.value;
  if (!y || !m || !d) return "";
  return `${y}-${m}-${d}`;
}

/** True when `value` falls on the same business-local calendar day as `now`. */
export function isBusinessLocalToday(
  value: string | number | Date,
  timeZone: string | null,
  now: Date = new Date(),
): boolean {
  const key = businessDateKey(value, timeZone);
  if (!key) return false;
  return key === businessDateKey(now, timeZone);
}

/**
 * Hour of day (0–23) for an instant in the business timezone.
 * Used for greetings and other venue-local time-of-day UI.
 */
export function businessLocalHour(
  value: string | number | Date = new Date(),
  timeZone: string | null = null,
): number {
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return 0;
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: resolveBusinessTimeZone(timeZone),
    hour: "numeric",
    hourCycle: "h23",
  }).formatToParts(date);
  const hour = Number(parts.find((p) => p.type === "hour")?.value ?? "0");
  return Number.isFinite(hour) ? hour : 0;
}

