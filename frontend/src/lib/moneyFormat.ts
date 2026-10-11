/**
 * Shared money display convention for Payverge surfaces (PG-20 / PG-8 / L6-37).
 *
 * Wire contract: amounts here are **whole currency units** (dollars). Callers
 * convert int64 cents → dollars before `formatMoneyAmount`, or use
 * `formatMoneyFromCents` which divides by 100 once.
 *
 * Surface conventions (deliberate, not content-language drift):
 * - **guest**: diner's active storefront locale + business currency code
 *   (via {@link formatGuestCurrency}).
 * - **operator**: operator UI locale (en / es) + currency code; falls back to
 *   en-US when locale is omitted so admin/dashboard stays consistent.
 */
import { formatCurrency } from "@/api/currency";
import {
  formatGuestCurrency,
  normalizeGuestLocale,
} from "@/utils/guestCurrencyFormatter";

/**
 * Root D / PG-15.2 — guest surface requires an explicit locale so an omitted
 * locale cannot silently fall back to en-US (S-4). Operator locale stays
 * optional and defaults to en-US for admin/dashboard consistency.
 */
export type FormatMoneyOptions =
  | {
      surface: "guest";
      /** BCP-47 diner language — required for guest surfaces. */
      locale: string;
    }
  | {
      surface: "operator";
      /** BCP-47 operator locale (en / es). Defaults to en-US when omitted. */
      locale?: string;
    };

export function formatMoneyAmount(
  amountDollars: number,
  currencyCode: string,
  options: FormatMoneyOptions,
): string {
  const code = (currencyCode || "USD").toUpperCase();
  if (options.surface === "guest") {
    return formatGuestCurrency(amountDollars, code, options.locale);
  }
  const locale = options.locale?.trim()
    ? normalizeGuestLocale(options.locale)
    : "en-US";
  return formatCurrency(amountDollars, code, undefined, locale);
}

/** Format integer cents as a display string (divides by 100 once). */
export function formatMoneyFromCents(
  amountCents: number,
  currencyCode: string,
  options: FormatMoneyOptions,
): string {
  const dollars = Number.isFinite(amountCents) ? amountCents / 100 : 0;
  return formatMoneyAmount(dollars, currencyCode, options);
}
