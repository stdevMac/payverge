import { intlLocaleFor } from "@/utils/intlLocale";

/**
 * Locale-aware currency formatter for accounting views.
 * Uses Intl.NumberFormat currency style; falls back to a plain
 * number + currency code when the currency code is invalid.
 */
export function formatMoney(
  amount: number,
  currency: string,
  locale = "en",
): string {
  const value = Number.isFinite(amount) ? amount : 0;
  const normalizedCurrency = currency.trim().toUpperCase();
  const intlLocale = intlLocaleFor(locale);

  try {
    return new Intl.NumberFormat(intlLocale, {
      style: "currency",
      currency: normalizedCurrency || "USD",
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(value);
  } catch {
    const formattedAmount = new Intl.NumberFormat(intlLocale, {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(value);

    return normalizedCurrency
      ? `${normalizedCurrency} ${formattedAmount}`
      : formattedAmount;
  }
}

/**
 * Format a calendar day (ISO date string or day key) for display.
 * Pins to local midnight so the displayed date matches the stored day
 * regardless of device timezone. See R3-AC-1.
 */
export function formatDay(value: string, locale: string): string {
  if (!value) return "";
  // Take only the calendar day and pin it to local midnight so the displayed
  // date matches the stored day regardless of the device timezone. Building a
  // Date from the raw ISO string (a noon-UTC instant) would let Intl re-project
  // it into the browser zone and possibly show the wrong day. See R3-AC-1.
  const dayKey = value.slice(0, 10);
  const parts = dayKey.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  const date = parts
    ? new Date(Number(parts[1]), Number(parts[2]) - 1, Number(parts[3]))
    : new Date(value);
  if (Number.isNaN(date.getTime())) return dayKey;
  return new Intl.DateTimeFormat(intlLocaleFor(locale), {
    month: "short",
    day: "numeric",
    year: "numeric",
  }).format(date);
}

/**
 * Format a payroll / reporting period as "start → end".
 * Collapses to a single day when start/end match or end is empty.
 */
export function formatPeriod(
  start: string,
  end: string,
  locale: string,
): string {
  const startDay = (start || "").slice(0, 10);
  const endDay = (end || "").slice(0, 10);
  if (!startDay) return endDay;
  if (!endDay || startDay === endDay) {
    return formatDay(startDay, locale);
  }
  return `${formatDay(startDay, locale)} → ${formatDay(endDay, locale)}`;
}
