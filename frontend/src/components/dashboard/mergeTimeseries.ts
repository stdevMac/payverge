// Combine per-venue daily series without summing across currencies.
// A USD day and an AED day are different units; the owner hub must keep
// them in separate buckets (same rule as groupSummariesByCurrency).
import type { Timeseries } from "@/api/analytics";

const DEFAULT_CURRENCY = "USD";

export interface TimeseriesEntry {
  currency?: string;
  series: Timeseries | null | undefined;
}

export interface CombinedCurrencySeries {
  currency: string;
  points: { date: string; value: number | null }[];
}

export function mergeTimeseriesByCurrency(
  entries: readonly TimeseriesEntry[],
): CombinedCurrencySeries[] {
  const byCurrency = new Map<
    string,
    Map<string, { revenue: number; transactions: number }>
  >();

  for (const { currency, series } of entries) {
    if (!series?.buckets?.length) continue;
    const code = currency || DEFAULT_CURRENCY;
    const dates = byCurrency.get(code) ?? new Map();
    for (const bucket of series.buckets) {
      if (!bucket.date) continue;
      const prev = dates.get(bucket.date) ?? { revenue: 0, transactions: 0 };
      dates.set(bucket.date, {
        revenue: prev.revenue + (bucket.revenue as number),
        transactions: prev.transactions + (bucket.transactions ?? 0),
      });
    }
    byCurrency.set(code, dates);
  }

  const result: CombinedCurrencySeries[] = [];
  for (const [currency, dates] of byCurrency) {
    const points = [...dates.entries()]
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([date, agg]) => ({
        date,
        // Null out idle days so the line breaks instead of plotting $0.
        value: agg.transactions > 0 ? agg.revenue : null,
      }));
    result.push({ currency, points });
  }

  result.sort((a, b) => a.currency.localeCompare(b.currency));
  return result;
}

export function seriesHasActivity(
  series: readonly CombinedCurrencySeries[],
): boolean {
  return series.some((entry) =>
    entry.points.some((point) => point.value != null && point.value !== 0),
  );
}
