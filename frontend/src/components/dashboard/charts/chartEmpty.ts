/**
 * Shared empty-series detection for analytics charts.
 * An all-zero series must not render fabricated axes (chart.js autoscales 0..1
 * and prints $0 / $1 ticks for "nothing").
 */
export function isAllZeroSeries(
  values: Array<number | null | undefined> | undefined | null,
): boolean {
  if (!values || values.length === 0) return true;
  return values.every((v) => v == null || v === 0 || !Number.isFinite(v));
}

/**
 * Collapse consecutive points that share the same formatted x label so admin
 * charts do not draw duplicate month ticks (e.g. two "Jan" from different years
 * when the backend only sends short month names).
 */
export function dedupePointsByLabel<T extends { month: string }>(
  points: T[],
): T[] {
  const seen = new Set<string>();
  const out: T[] = [];
  for (const p of points) {
    const key = p.month;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(p);
  }
  return out;
}
