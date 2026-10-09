/**
 * Shared admin currency formatter (Q-3), USD by default with an optional
 * `currency` override (fiscal pages format ARS receipts). Consolidates three copy-paste
 * `formatCurrency` helpers in the admin pages. Accepts the server's
 * machine-string amounts or raw numbers; `fractionDigits` defaults to 2
 * (detail rows) and is set to 0 for whole-dollar KPI tiles.
 *
 * Behavior note vs the three originals: they passed `num` straight to
 * `.format()`, so a non-numeric string yielded `$NaN`. This version hardens
 * that single edge to `$0.00` via `Number.isFinite(num) ? num : 0`. All real
 * numeric inputs (the only ones the admin pages actually pass) format
 * identically; only the never-hit NaN path changes.
 */
export const formatAdminCurrency = (
  amount: string | number | null | undefined,
  options?: { fractionDigits?: number; currency?: string; compact?: boolean },
): string => {
  const currency = options?.currency ?? "USD";
  if (amount == null && currency === "USD") {
    return options?.fractionDigits === 0 ? "$0" : "$0.00";
  }
  const num =
    amount == null ? 0 : typeof amount === "string" ? parseFloat(amount) : amount;
  const digits = options?.fractionDigits ?? 2;
  const safe = Number.isFinite(num) ? num : 0;
  // KPI tiles are narrow: a lifetime total like "ARS 38,336,450" wraps inside
  // the digits. From a million up, `compact` renders "ARS 38.3M" instead.
  if (options?.compact && Math.abs(safe) >= 1_000_000) {
    return new Intl.NumberFormat("en-US", {
      style: "currency",
      currency,
      notation: "compact",
      maximumFractionDigits: 1,
    }).format(safe);
  }
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency,
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(safe);
};
