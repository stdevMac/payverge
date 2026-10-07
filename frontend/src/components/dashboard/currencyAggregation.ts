// src/components/dashboard/currencyAggregation.ts
//
// Cross-business dashboard money must NOT be summed across currencies. A
// USD business's revenue and an AED business's revenue are different units;
// adding them under one symbol (the old behavior, which labeled the sum with
// businesses[0].default_currency) produces a meaningless figure. This helper
// buckets per-business summaries by their default_currency and sums only
// within a bucket. "Active bills" is a currency-agnostic count, so it is also
// summed per bucket here; the page renders the global active-bills count once.
import type { DashboardSummary } from "@/api/analytics";
import { asDollars } from "@/types/money";

export interface BusinessSummaryEntry {
  /** The business's default_currency (ISO code) or undefined → defaults to USD. */
  currency?: string;
  summary: DashboardSummary;
}

export interface CurrencyBucket {
  currency: string;
  summary: DashboardSummary;
}

const DEFAULT_CURRENCY = "USD";

function emptySummary(): DashboardSummary {
  return {
    today: {
      revenue: asDollars(0),
      tips: asDollars(0),
      transactions: 0,
      bills: 0,
    },
    week: {
      revenue: asDollars(0),
      tips: asDollars(0),
      transactions: 0,
      bills: 0,
      unique_customers: 0,
      average_ticket: asDollars(0),
    },
    live: { active_bills: 0 },
    top_items: [],
  };
}

/**
 * Group per-business dashboard summaries by currency, summing money fields
 * only within each currency bucket (no FX conversion, no cross-currency sum).
 * Each bucket's week.average_ticket is derived as revenue / bills.
 * Buckets are returned sorted by currency code for deterministic rendering.
 */
export function groupSummariesByCurrency(
  entries: readonly BusinessSummaryEntry[],
): CurrencyBucket[] {
  const byCurrency = new Map<string, DashboardSummary>();

  for (const { currency, summary } of entries) {
    const code = currency || DEFAULT_CURRENCY;
    const agg = byCurrency.get(code) ?? emptySummary();

    agg.today.revenue = asDollars(agg.today.revenue + summary.today.revenue);
    agg.today.tips = asDollars(agg.today.tips + summary.today.tips);
    agg.today.transactions += summary.today.transactions;
    agg.today.bills += summary.today.bills;

    agg.week.revenue = asDollars(agg.week.revenue + summary.week.revenue);
    agg.week.tips = asDollars(agg.week.tips + summary.week.tips);
    agg.week.transactions += summary.week.transactions;
    agg.week.bills += summary.week.bills;
    agg.week.unique_customers += summary.week.unique_customers;

    agg.live.active_bills += summary.live.active_bills;

    byCurrency.set(code, agg);
  }

  const buckets: CurrencyBucket[] = [];
  for (const [currency, summary] of byCurrency) {
    if (summary.week.bills > 0) {
      summary.week.average_ticket = asDollars(
        summary.week.revenue / summary.week.bills,
      );
    }
    buckets.push({ currency, summary });
  }

  buckets.sort((a, b) => a.currency.localeCompare(b.currency));
  return buckets;
}
