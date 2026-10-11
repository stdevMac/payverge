/** L3-18: signed customer savings (negative when bundle costs more than sum). */
export function computeBundleSavings(
  regularTotal: number,
  bundlePrice: number,
): number {
  return (Number(regularTotal) || 0) - (Number(bundlePrice) || 0);
}

export function bundleSavingsTone(
  savings: number,
): "positive" | "negative" | "zero" {
  if (savings > 0) return "positive";
  if (savings < 0) return "negative";
  return "zero";
}

export function isBundlePriceAtOrAboveRegular(
  regularTotal: number,
  bundlePrice: number,
): boolean {
  return (Number(bundlePrice) || 0) >= (Number(regularTotal) || 0) && regularTotal > 0;
}
