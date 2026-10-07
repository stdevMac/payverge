// Extract the symbol from a currency code via Intl. USD → "$", EUR → "€".
// Currencies without a dedicated symbol (AED, MXN in some locales) fall back
// to the code itself, which is still better than a misleading "$".
//
// `display` defaults to "narrowSymbol" (ARS → "$"), which reads naturally to
// guests paying in their own currency. Operator settings pass "symbol" so a
// peso venue sees "ARS", not a "$" that reads as dollars (USD stays "$").
export function deriveCurrencyPrefix(
  code: string,
  display: "narrowSymbol" | "symbol" = "narrowSymbol",
): string {
  try {
    const parts = new Intl.NumberFormat("en", {
      style: "currency",
      currency: code,
      currencyDisplay: display,
    }).formatToParts(0);
    const sym = parts.find((p) => p.type === "currency")?.value;
    return sym || code;
  } catch {
    return code;
  }
}
