// src/components/dashboard/weekOverWeekDelta.ts
//
// Week-over-week delta from a daily analytics series. The dashboard overview's
// tips / avg-ticket tiles show a "% vs prior 7 days" figure computed CLIENT-SIDE
// from the fetched timeseries (there is no per-metric growth_rate on the
// dashboard summary the way RevenuePanel gets one from the sales endpoint).
//
// Windows: last7 = final 7 daily buckets; prior7 = the 7 immediately before.
// Needs >= 14 buckets. A zero prior-window sum yields null (no meaningful
// percent, avoids Infinity), and so does an under-length series.
import type { TimeseriesBucket } from "@/api/analytics";

const WINDOW = 7;

export function weekOverWeekDelta(
  buckets: readonly TimeseriesBucket[],
  select: (bucket: TimeseriesBucket) => number,
): number | null {
  if (!buckets || buckets.length < WINDOW * 2) return null;

  const last7 = buckets.slice(buckets.length - WINDOW);
  const prior7 = buckets.slice(buckets.length - WINDOW * 2, buckets.length - WINDOW);

  const sum = (window: readonly TimeseriesBucket[]) =>
    window.reduce((acc, b) => acc + (select(b) || 0), 0);

  const priorSum = sum(prior7);
  if (priorSum === 0) return null;

  return ((sum(last7) - priorSum) / priorSum) * 100;
}
