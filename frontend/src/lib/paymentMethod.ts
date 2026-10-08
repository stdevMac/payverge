/**
 * Canonical payment-method vocabulary — FE mirror of
 * `backend/internal/reporting/paymentmethod.go`.
 *
 * Both payment ledgers (payments + alternative_payments) and every plugin
 * settlement name collapse to one of these six Methods. The Payment History
 * filter options are generated from the server's `available_methods` list
 * (which is itself built from this set intersected with rows present for the
 * business+window) — never from a hardcoded "crypto / manual / Online" array.
 */

export const PAYMENT_METHODS = [
  "crypto",
  "cross_chain",
  "card",
  "cash",
  "wallet",
  "other",
] as const;

export type PaymentMethod = (typeof PAYMENT_METHODS)[number];

const KNOWN = new Set<string>(PAYMENT_METHODS);

/** True when `value` is a member of the canonical set. */
export function isPaymentMethod(value: string | null | undefined): value is PaymentMethod {
  return !!value && KNOWN.has(value);
}

/**
 * Map a raw `payment_method` column value (either source table) to the
 * canonical Method. Mirrors `reporting.Canonicalize`.
 */
export function canonicalizePaymentMethod(
  raw: string | null | undefined,
  sourceTable: string = "payments",
): PaymentMethod {
  const s = (raw ?? "")
    .trim()
    .toLowerCase()
    .replace(/-/g, "_");

  switch (s) {
    case "crypto":
    case "usdc":
    case "usdc_payment":
      return "crypto";
    case "cross_chain":
    case "cross_chain_payment":
    case "crosschain":
      return "cross_chain";
    case "card":
    case "stripe":
    case "mercadopago":
      return "card";
    case "cash":
      return "cash";
    case "wallet":
    case "venmo":
    case "paypal":
      return "wallet";
    case "other":
    case "plugin":
    case "manual":
      return "other";
    case "":
      return sourceTable.toLowerCase().includes("alternative") ? "other" : "crypto";
    default:
      // Legacy currency codes (usd, eur, …) and unknown plugin names.
      return "other";
  }
}

/**
 * Filter option list for Payment History. Always starts with `all`, then only
 * the methods the server reported as present for the business+window. Never
 * invents a method that would match zero rows.
 */
export function methodFilterOptions(
  available: readonly string[] | null | undefined,
  labelFor: (method: PaymentMethod | "all") => string,
): { key: string; label: string }[] {
  const opts: { key: string; label: string }[] = [
    { key: "all", label: labelFor("all") },
  ];
  if (!available || available.length === 0) {
    return opts;
  }
  // Preserve canonical order; drop unknown / empty server values.
  for (const m of PAYMENT_METHODS) {
    if (available.includes(m)) {
      opts.push({ key: m, label: labelFor(m) });
    }
  }
  return opts;
}

/**
 * Parse a committed filter key. Returns null for "all" / empty / unknown so
 * the client omits the method query param rather than sending a dead key.
 */
export function parseMethodFilter(
  key: string | null | undefined,
): PaymentMethod | null {
  if (!key || key === "all") return null;
  return isPaymentMethod(key) ? key : null;
}
