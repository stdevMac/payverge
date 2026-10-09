// src/components/dashboard/currencyAggregation.test.ts
import { groupSummariesByCurrency } from "./currencyAggregation";
import type { DashboardSummary } from "@/api/analytics";
import { asDollars } from "@/types/money";

// Minimal helper to build a per-business summary fixture in DOLLARS.
function summary(opts: {
  todayRevenue: number;
  todayTips: number;
  todayBills: number;
  weekRevenue: number;
  weekBills: number;
  weekAvgTicket: number;
  activeBills: number;
}): DashboardSummary {
  return {
    today: {
      revenue: asDollars(opts.todayRevenue),
      tips: asDollars(opts.todayTips),
      transactions: 0,
      bills: opts.todayBills,
    },
    week: {
      revenue: asDollars(opts.weekRevenue),
      tips: asDollars(0),
      transactions: 0,
      bills: opts.weekBills,
      unique_customers: 0,
      average_ticket: asDollars(opts.weekAvgTicket),
    },
    live: { active_bills: opts.activeBills },
    top_items: [],
  };
}

describe("groupSummariesByCurrency", () => {
  it("returns one bucket for a single-currency account", () => {
    const buckets = groupSummariesByCurrency([
      {
        currency: "USD",
        summary: summary({
          todayRevenue: 100,
          todayTips: 10,
          todayBills: 4,
          weekRevenue: 700,
          weekBills: 35,
          weekAvgTicket: 20,
          activeBills: 2,
        }),
      },
    ]);

    expect(buckets).toHaveLength(1);
    expect(buckets[0].currency).toBe("USD");
    expect(buckets[0].summary.today.revenue).toBe(100);
    expect(buckets[0].summary.today.tips).toBe(10);
    expect(buckets[0].summary.week.average_ticket).toBe(20); // 700 / 35
    expect(buckets[0].summary.live.active_bills).toBe(2);
  });

  it("buckets a mixed USD + AED account by currency and never cross-sums money", () => {
    const buckets = groupSummariesByCurrency([
      {
        currency: "USD",
        summary: summary({
          todayRevenue: 100,
          todayTips: 10,
          todayBills: 5,
          weekRevenue: 500,
          weekBills: 25,
          weekAvgTicket: 20,
          activeBills: 3,
        }),
      },
      {
        currency: "USD",
        summary: summary({
          todayRevenue: 50,
          todayTips: 5,
          todayBills: 5,
          weekRevenue: 500,
          weekBills: 25,
          weekAvgTicket: 20,
          activeBills: 2,
        }),
      },
      {
        currency: "AED",
        summary: summary({
          todayRevenue: 400,
          todayTips: 40,
          todayBills: 8,
          weekRevenue: 4000,
          weekBills: 40,
          weekAvgTicket: 100,
          activeBills: 4,
        }),
      },
    ]);

    expect(buckets).toHaveLength(2);

    const usd = buckets.find((b) => b.currency === "USD")!;
    const aed = buckets.find((b) => b.currency === "AED")!;

    // USD bucket sums ONLY the two USD businesses.
    expect(usd.summary.today.revenue).toBe(150);
    expect(usd.summary.today.tips).toBe(15);
    expect(usd.summary.week.revenue).toBe(1000);
    expect(usd.summary.week.bills).toBe(50);
    expect(usd.summary.week.average_ticket).toBe(20); // 1000 / 50
    expect(usd.summary.live.active_bills).toBe(5);

    // AED bucket is the single AED business — untouched by USD figures.
    expect(aed.summary.today.revenue).toBe(400);
    expect(aed.summary.today.tips).toBe(40);
    expect(aed.summary.week.average_ticket).toBe(100); // 4000 / 40
    expect(aed.summary.live.active_bills).toBe(4);

    // The merged 550 cross-currency figure must never appear in any bucket.
    const allTodayRevenue = buckets.map((b) => b.summary.today.revenue);
    expect(allTodayRevenue).not.toContain(550);
  });

  it("falls back to USD when default_currency is missing", () => {
    const buckets = groupSummariesByCurrency([
      {
        currency: undefined,
        summary: summary({
          todayRevenue: 30,
          todayTips: 3,
          todayBills: 1,
          weekRevenue: 30,
          weekBills: 1,
          weekAvgTicket: 30,
          activeBills: 1,
        }),
      },
    ]);
    expect(buckets).toHaveLength(1);
    expect(buckets[0].currency).toBe("USD");
  });

  it("returns an empty array for no input", () => {
    expect(groupSummariesByCurrency([])).toEqual([]);
  });

  it("orders buckets deterministically by currency code", () => {
    const buckets = groupSummariesByCurrency([
      { currency: "AED", summary: summary({ todayRevenue: 1, todayTips: 0, todayBills: 1, weekRevenue: 1, weekBills: 1, weekAvgTicket: 1, activeBills: 0 }) },
      { currency: "USD", summary: summary({ todayRevenue: 1, todayTips: 0, todayBills: 1, weekRevenue: 1, weekBills: 1, weekAvgTicket: 1, activeBills: 0 }) },
    ]);
    expect(buckets.map((b) => b.currency)).toEqual(["AED", "USD"]);
  });
});
