import { formatCurrency } from "@/api/currency";

/**
 * Unicode bidi isolates (U+2068 FIRST STRONG ISOLATE / U+2069 POP DIRECTIONAL
 * ISOLATE). Wrapping every guest amount keeps the currency token, digits, and
 * leading minus as one isolated run inside RTL prose (NEW-14). Without this,
 * Arabic negatives re-order into broken shapes like "$-10.20 US".
 */
export const BIDI_FSI = "\u2068";
export const BIDI_PDI = "\u2069";

function isolateBidi(formatted: string): string {
  return `${BIDI_FSI}${formatted}${BIDI_PDI}`;
}

/**
 * Formats a money amount for a guest-facing surface using the diner's active
 * storefront locale for grouping/decimal/symbol placement, while keeping the
 * business's configured currency code.
 *
 * `amount` is a whole currency unit (dollars), matching the money wire
 * contract — callers convert cents -> dollars before calling. The currency
 * code is always the business's currency, never the diner's choice; only the
 * locale (formatting) varies, so this never changes a money value.
 *
 * Storefront locale codes are canonical BCP-47, but underscores (e.g. `es_AR`)
 * are defensively normalized to hyphens so `Intl.NumberFormat` never throws on
 * them. Empty/undefined locales fall back to en-US, matching operator/admin
 * formatting.
 *
 * Every result is wrapped in FSI/PDI so RTL locales keep the amount as a single
 * bidi-isolated run (including negatives).
 */
export function formatGuestCurrency(
  amount: number,
  currencyCode: string,
  guestLocale?: string,
): string {
  return isolateBidi(
    formatCurrency(
      amount,
      currencyCode,
      undefined,
      normalizeGuestLocale(guestLocale),
    ),
  );
}

/**
 * Convert a default-currency amount into the diner's display currency using
 * the same cents-rounded multiply as {@link convertAmount} / CurrencyPrice.
 * When currencies match or no rate is available, the source amount stays in
 * `fromCurrency` so the magnitude is never mislabeled.
 */
export function convertGuestAmount(
  amount: number,
  fromCurrency: string,
  toCurrency: string,
  rate?: number | null,
): { amount: number; currency: string } {
  const from = (fromCurrency || "").trim().toUpperCase();
  const to = (toCurrency || from).trim().toUpperCase();
  if (!from || from === to || rate == null || !Number.isFinite(rate)) {
    return { amount, currency: from || to };
  }
  return {
    amount: Math.round(amount * rate * 100) / 100,
    currency: to,
  };
}

/**
 * Format a tax/service-fee percent for a guest surface using the diner locale.
 * Returns the numeric token only (no `%`) so callers can interpolate into
 * existing `{rate}%` message strings. Never uses toFixed — mill-percents
 * (8.875) stay intact and de/fr get a comma decimal.
 */
export function formatGuestRate(
  rate: number,
  guestLocale?: string,
  maxDecimals = 4,
): string {
  if (!Number.isFinite(rate)) {
    return "0";
  }
  const locale = normalizeGuestLocale(guestLocale);
  const abs = Math.abs(rate);
  const options: Intl.NumberFormatOptions = {
    style: "decimal",
    minimumFractionDigits: 0,
    maximumFractionDigits: maxDecimals,
    useGrouping: false,
  };
  let formatted: string;
  try {
    formatted = new Intl.NumberFormat(locale, options).format(abs);
  } catch {
    formatted = new Intl.NumberFormat("en-US", options).format(abs);
  }
  return rate < 0 ? `-${formatted}` : formatted;
}

/** Convert default→display (when a rate exists) then format for the diner locale. */
export function formatConvertedGuestCurrency(
  amount: number,
  fromCurrency: string,
  displayCurrency: string,
  locale?: string,
  rate?: number | null,
): string {
  const converted = convertGuestAmount(
    amount,
    fromCurrency,
    displayCurrency,
    rate,
  );
  return formatGuestCurrency(converted.amount, converted.currency, locale);
}

/**
 * Normalizes a storefront locale code to a BCP-47 tag safe for `Intl`.
 * Exported for guest surfaces that build a raw `Intl.NumberFormat` directly
 * (e.g. display-rate conversion, currency-less spend totals) and need the same
 * locale handling as {@link formatGuestCurrency} — underscores (`es_AR`) become
 * hyphens, and empty/undefined falls back to en-US.
 */
export function normalizeGuestLocale(guestLocale?: string): string {
  const trimmed = guestLocale?.trim();
  if (!trimmed) {
    return "en-US";
  }
  return trimmed.replace(/_/g, "-");
}
