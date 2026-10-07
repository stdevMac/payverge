import { asDollars } from "@/types/money";
import type { Timeseries } from "@/api/analytics";
import {
  mergeTimeseriesByCurrency,
  seriesHasActivity,
} from "./mergeTimeseries";

function series(
  buckets: Array<{
    date: string;
    revenue: number;
    transactions?: number;
  }>,
): Timeseries {
  return {
    buckets: buckets.map((bucket) => ({
      date: bucket.date,
      revenue: asDollars(bucket.revenue),
      tips: asDollars(0),
      bills: bucket.transactions ?? 0,
      transactions: bucket.transactions ?? 0,
      average_ticket: asDollars(0),
    })),
    range: { from: buckets[0]?.date ?? "", to: buckets.at(-1)?.date ?? "" },
  };
}

describe("mergeTimeseriesByCurrency", () => {
  it("sums same-currency venues on the same day", () => {
    const merged = mergeTimeseriesByCurrency([
      {
        currency: "USD",
        series: series([
          { date: "2026-08-01", revenue: 100, transactions: 2 },
          { date: "2026-08-02", revenue: 40, transactions: 1 },
        ]),
      },
      {
        currency: "USD",
        series: series([{ date: "2026-08-01", revenue: 25, transactions: 1 }]),
      },
    ]);

    expect(merged).toHaveLength(1);
    expect(merged[0].currency).toBe("USD");
    expect(merged[0].points).toEqual([
      { date: "2026-08-01", value: 125 },
      { date: "2026-08-02", value: 40 },
    ]);
  });

  it("does not add USD and AED into one series", () => {
    const merged = mergeTimeseriesByCurrency([
      {
        currency: "USD",
        series: series([{ date: "2026-08-01", revenue: 100, transactions: 1 }]),
      },
      {
        currency: "AED",
        series: series([{ date: "2026-08-01", revenue: 50, transactions: 1 }]),
      },
    ]);

    expect(merged.map((entry) => entry.currency)).toEqual(["AED", "USD"]);
    expect(merged[0].points).toEqual([{ date: "2026-08-01", value: 50 }]);
    expect(merged[1].points).toEqual([{ date: "2026-08-01", value: 100 }]);
  });

  it("treats idle days as gaps instead of plotted zeros", () => {
    const merged = mergeTimeseriesByCurrency([
      {
        currency: "USD",
        series: series([
          { date: "2026-08-01", revenue: 0, transactions: 0 },
          { date: "2026-08-02", revenue: 80, transactions: 2 },
        ]),
      },
    ]);

    expect(merged[0].points).toEqual([
      { date: "2026-08-01", value: null },
      { date: "2026-08-02", value: 80 },
    ]);
    expect(seriesHasActivity(merged)).toBe(true);
  });

  it("skips missing series and reports no activity for an empty merge", () => {
    expect(mergeTimeseriesByCurrency([{ currency: "USD", series: null }])).toEqual(
      [],
    );
    expect(seriesHasActivity([])).toBe(false);
  });
});
