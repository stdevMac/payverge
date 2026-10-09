// Per-spec menu price formatting. Pure: no DOM.
// Rules (spec §2.1): nested bare numerals by default; symbol off | locale;
// decimals strip-zeros | always; zero-decimal currencies never invent .00.

import type { MenuPriceFormatSpec } from "./types";

/**
 * Format a menu price for print.
 *
 * @param price  Amount in dollars (frontend wire shape). Do NOT re-divide.
 * @param currency  ISO 4217 code (e.g. "USD", "JPY").
 * @param spec  Locale-aware price presentation.
 * @param locale  BCP 47 locale for numeral / symbol placement (e.g. "en", "es", "ja").
 */
export function formatMenuPrice(
  price: number,
  currency: string,
  spec: MenuPriceFormatSpec,
  locale: string,
): string {
  const currencyCode = currency.trim().toUpperCase() || "USD";
  const fractionDigits = resolveCurrencyFractionDigits(locale, currencyCode);

  const { minimumFractionDigits, maximumFractionDigits } =
    resolveFractionDigits(price, fractionDigits, spec.decimals);

  if (spec.symbol === "off") {
    return new Intl.NumberFormat(locale, {
      style: "decimal",
      minimumFractionDigits,
      maximumFractionDigits,
    }).format(price);
  }

  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency: currencyCode,
    currencyDisplay: "narrowSymbol",
    minimumFractionDigits,
    maximumFractionDigits,
  }).format(price);
}

/** Prefer Intl's currency-aware fraction digits over hardcoding 2. */
function resolveCurrencyFractionDigits(
  locale: string,
  currency: string,
): number {
  try {
    const resolved = new Intl.NumberFormat(locale, {
      style: "currency",
      currency,
    }).resolvedOptions();
    // maximumFractionDigits is the currency's minor-unit count (0 for JPY/KRW).
    if (typeof resolved.maximumFractionDigits === "number") {
      return resolved.maximumFractionDigits;
    }
  } catch {
    // Invalid currency code — fall through to decimal default.
  }
  return 2;
}

function resolveFractionDigits(
  price: number,
  currencyFractionDigits: number,
  decimals: MenuPriceFormatSpec["decimals"],
): { minimumFractionDigits: number; maximumFractionDigits: number } {
  // Zero-decimal currencies (JPY, KRW, …): never show fake decimals.
  if (currencyFractionDigits === 0) {
    return { minimumFractionDigits: 0, maximumFractionDigits: 0 };
  }

  if (decimals === "always") {
    return {
      minimumFractionDigits: currencyFractionDigits,
      maximumFractionDigits: currencyFractionDigits,
    };
  }

  // strip-zeros: whole amounts → no decimals; fractional → full minor units.
  if (isWholeInMinorUnits(price, currencyFractionDigits)) {
    return { minimumFractionDigits: 0, maximumFractionDigits: 0 };
  }

  return {
    minimumFractionDigits: currencyFractionDigits,
    maximumFractionDigits: currencyFractionDigits,
  };
}

function isWholeInMinorUnits(price: number, fractionDigits: number): boolean {
  if (!Number.isFinite(price)) return true;
  const factor = 10 ** fractionDigits;
  // Round to the currency's minor unit, then check remainder.
  const minor = Math.round(price * factor);
  return minor % factor === 0;
}
