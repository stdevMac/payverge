/**
 * Format a tax/service fee rate for operator UI without rounding away
 * mill-percents (e.g. NYC 8.875 must not display as 8.88).
 * Trailing zeros are stripped so 4 → "4%" and 4.50 → "4.5%".
 */
export function formatRatePercent(rate: number, maxDecimals = 4): string {
  if (!Number.isFinite(rate)) return "0%";
  const fixed = Math.abs(rate).toFixed(maxDecimals);
  const trimmed = fixed.replace(/\.?0+$/, "");
  const sign = rate < 0 ? "-" : "";
  return `${sign}${trimmed}%`;
}
