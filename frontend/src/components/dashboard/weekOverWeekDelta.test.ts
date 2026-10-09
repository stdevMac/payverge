// src/components/dashboard/weekOverWeekDelta.test.ts
import { weekOverWeekDelta } from "./weekOverWeekDelta";
import type { TimeseriesBucket } from "@/api/analytics";
import { asDollars } from "@/types/money";

// Build N daily buckets; `values` fills the chosen field left-to-right
// (oldest → newest). Unused fields are zeroed.
function series(values: number[]): TimeseriesBucket[] {
  return values.map((v, i) => ({
    date: `2026-05-${String(i + 1).padStart(2, "0")}`,
    revenue: asDollars(v),
    tips: asDollars(v),
    bills: 0,
    transactions: 0,
    average_ticket: asDollars(v),
  }));
}

describe("weekOverWeekDelta", () => {
  it("computes +10% when the last 7 sum is 10% above the prior 7", () => {
    // prior7 (indices 0-6) each 100 → 700; last7 (indices 7-13) each 110 → 770.
    const buckets = series([100, 100, 100, 100, 100, 100, 100, 110, 110, 110, 110, 110, 110, 110]);
    const delta = weekOverWeekDelta(buckets, (b) => b.revenue as number);
    expect(delta).not.toBeNull();
    expect(delta!).toBeCloseTo(10, 5);
  });

  it("computes a negative delta when the last 7 sum is below the prior 7", () => {
    // prior7 = 700, last7 = 630 → -10%.
    const buckets = series([100, 100, 100, 100, 100, 100, 100, 90, 90, 90, 90, 90, 90, 90]);
    const delta = weekOverWeekDelta(buckets, (b) => b.tips as number);
    expect(delta!).toBeCloseTo(-10, 5);
  });

  it("uses only the trailing 14 buckets when the series is longer", () => {
    // 21 buckets: first 7 are noise (999). prior7 = indices 7-13 (all 100 → 700),
    // last7 = indices 14-20 (all 120 → 840) → +20%.
    const buckets = series([
      999, 999, 999, 999, 999, 999, 999,
      100, 100, 100, 100, 100, 100, 100,
      120, 120, 120, 120, 120, 120, 120,
    ]);
    const delta = weekOverWeekDelta(buckets, (b) => b.average_ticket as number);
    expect(delta!).toBeCloseTo(20, 5);
  });

  it("returns null when there are fewer than 14 buckets", () => {
    expect(weekOverWeekDelta(series([1, 2, 3]), (b) => b.revenue as number)).toBeNull();
  });

  it("returns null when the prior 7 sum is zero (avoids divide-by-zero)", () => {
    // prior7 all zero, last7 all 50 → would be +Infinity%; suppress instead.
    const buckets = series([0, 0, 0, 0, 0, 0, 0, 50, 50, 50, 50, 50, 50, 50]);
    expect(weekOverWeekDelta(buckets, (b) => b.revenue as number)).toBeNull();
  });
});
