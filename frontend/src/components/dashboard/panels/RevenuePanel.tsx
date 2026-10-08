"use client";

import { getSafeApiErrorMessage } from "@/utils/apiError";
import type { Locale } from "@/i18n/config";
import toast from "react-hot-toast";
import { runRevenueExport } from "./revenueExport";

import React, { useState, useEffect, useCallback } from "react";
import { useRouter, usePathname } from "next/navigation";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { localDateKey } from "@/lib/localDate";
import { Button } from "@nextui-org/react";
import { TrendingDown, DollarSign, Download } from "lucide-react";
import { analyticsApi, SalesData } from "@/api/analytics";
import { getBusiness } from "@/api/business";
import { formatCurrency as formatCurrencyIntl } from "@/api/currency";
import { intlLocaleFor } from "@/utils/intlLocale";
import PeriodTabs from "@/components/business/shared/PeriodTabs";
import { btnSecondaryNextUI, btnPrimary } from "@/components/ui/buttonStyles";
import { EmptyState } from "@/components/ui/EmptyState";
import { MetricStat } from "@/components/dashboard/charts/MetricStat";
import { TrendChart } from "@/components/dashboard/charts/TrendChart";
import { HourOfDayChart } from "@/components/dashboard/charts/HourOfDayChart";
import { RankedBarList } from "@/components/dashboard/charts/RankedBarList";
import { useAnalyticsTimeseries } from "@/hooks/useAnalyticsTimeseries";

interface RevenuePanelProps {
  businessId: string;
  // Fix 7: currency threaded from the dashboard page (which already holds the
  // resolved business) so the panel doesn't re-fetch getBusiness for it.
  currency?: string;
  // Fix 4: ONE shared analytics period lifted to the Dashboard and persisted in
  // the URL. When provided this panel is controlled by it (and reports changes
  // back), so the period survives panel switches. Absent → local period state.
  period?: string;
  onPeriodChange?: (period: string) => void;
}

// Canonical period keys — `as const` so the period state and PeriodTabs<K>
// instantiate on the narrow key union instead of string.
const REVENUE_PERIOD_KEYS = [
  "today",
  "yesterday",
  "week",
  "month",
  "quarter",
  "year",
] as const;
type RevenuePeriodKey = (typeof REVENUE_PERIOD_KEYS)[number];

// Trailing daily-trend window length (in days) per selected period, so the
// centerpiece trend spans roughly the same horizon as the KPI tiles instead
// of a fixed 30 days. Short periods keep a week of context for a readable line.
const TREND_WINDOW_DAYS: Record<RevenuePeriodKey, number> = {
  today: 7,
  yesterday: 7,
  week: 7,
  month: 30,
  quarter: 90,
  year: 365,
};

// Type guard: is a shared-period string one this panel can render?
const isRevenuePeriodKey = (v: string | undefined): v is RevenuePeriodKey =>
  !!v && (REVENUE_PERIOD_KEYS as readonly string[]).includes(v);

export default function RevenuePanel({
  businessId,
  currency: currencyProp,
  period: periodProp,
  onPeriodChange,
}: RevenuePanelProps) {
  const router = useRouter();
  const pathname = usePathname();

  // Translation setup — ported verbatim from SalesOverview.tsx
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);

  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  const tString = (key: string): string => {
    const fullKey = `businessDashboard.dashboard.salesOverview.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  // Pulled from the business so AED operators don't see "$" on their
  // analytics totals. Prefer the threaded prop (Fix 7); only fetch when the
  // parent didn't supply the currency. Default USD until resolved.
  const [fetchedCurrency, setFetchedCurrency] = useState<string>("USD");
  const businessCurrency = currencyProp ?? fetchedCurrency;

  useEffect(() => {
    let cancelled = false;
    if (!businessId || currencyProp !== undefined) return;
    getBusiness(businessId)
      .then((biz) => {
        if (!cancelled && biz?.default_currency) {
          setFetchedCurrency(biz.default_currency);
        }
      })
      .catch(() => {
        // Best-effort — leave USD fallback in place.
      });
    return () => {
      cancelled = true;
    };
  }, [businessId, currencyProp]);

  const [salesData, setSalesData] = useState<SalesData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // Fix 4: controlled by the shared period when supplied; local otherwise. When
  // the shared period isn't one this panel supports (e.g. it never has one), we
  // fall back to the panel default.
  const [localPeriod, setLocalPeriod] = useState<RevenuePeriodKey>("today");
  const period: RevenuePeriodKey = isRevenuePeriodKey(periodProp)
    ? periodProp
    : localPeriod;
  const setPeriod = (key: RevenuePeriodKey) => {
    setLocalPeriod(key);
    onPeriodChange?.(key);
  };

  const fetchSalesData = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await analyticsApi.getSalesAnalytics(businessId, period);
      setSalesData(data || null);
    } catch (err) {
      console.error("RevenuePanel - API Error:", err);
      setError(getSafeApiErrorMessage(err, "An error occurred"));
    } finally {
      setLoading(false);
    }
  }, [businessId, period]);

  useEffect(() => {
    fetchSalesData();
  }, [fetchSalesData]);

  const formatCurrency = (amount: number) =>
    formatCurrencyIntl(amount, businessCurrency, undefined, intlLocaleFor(locale));

  // Export functionality — ported from SalesOverview.tsx.
  // L6-7: surface 413 "range too large" (and other failures) instead of console-only.
  const handleExportData = async () => {
    const result = await runRevenueExport({
      exportFn: async () => {
        const blob = await analyticsApi.exportAnalyticsData(
          businessId,
          period,
          "csv",
          locale,
        );
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = `sales-analytics-${period}-${localDateKey()}.csv`;
        document.body.appendChild(a);
        a.click();
        window.URL.revokeObjectURL(url);
        document.body.removeChild(a);
        return blob;
      },
      locale: locale as Locale,
      fallback: tString("exportFailed"),
      onError: (message) => {
        console.error("Export failed:", message);
        toast.error(message);
      },
    });
    void result;
  };

  // Loading / error / empty early-return blocks — ported verbatim from SalesOverview.tsx
  if (loading) {
    return (
      <div className="flex items-center justify-center py-16">
        <div className="text-center">
          <div className="w-16 h-16 bg-warm-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-warm-100">
            <div className="w-8 h-8 border-2 border-warm-300 border-t-ink-900 rounded-full animate-spin"></div>
          </div>
          <p className="text-ink-600 tracking-wide">
            {tString("loading")}
          </p>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-red-50 border border-red-200 rounded-lg p-4">
        <div className="flex items-center gap-3">
          <TrendingDown className="w-5 h-5 text-red-600 flex-shrink-0" />
          <p className="text-red-800 font-medium">
            {tString("error")}: {error}
          </p>
        </div>
      </div>
    );
  }

  if (!salesData) {
    return (
      <EmptyState
        panel
        icon={DollarSign}
        title={tString("noSalesTitle")}
        subtitle={tString("noSalesBody")}
        action={
          <button
            type="button"
            className={btnPrimary}
            onClick={() => router.push(`${pathname}?tab=overview`)}
          >
            {tString("goToSetup")}
          </button>
        }
      />
    );
  }

  // Periods list — keys from the canonical tuple, labels ported verbatim
  // from SalesOverview.tsx (`periods.<key>` i18n entries).
  const periods = REVENUE_PERIOD_KEYS.map((key) => ({
    key,
    label: tString(`periods.${key}`),
  }));

  return (
    <RevenueBody
      businessId={businessId}
      salesData={salesData}
      period={period}
      setPeriod={setPeriod}
      periods={periods}
      handleExportData={handleExportData}
      formatCurrency={formatCurrency}
      tString={tString}
      locale={currentLocale}
    />
  );
}

// Inner component so hooks (useAnalyticsTimeseries) always run after
// salesData is confirmed non-null — avoids conditional hook calls.
function RevenueBody({
  businessId,
  salesData,
  period,
  setPeriod,
  periods,
  handleExportData,
  formatCurrency,
  tString,
  locale,
}: {
  businessId: string;
  salesData: SalesData;
  period: RevenuePeriodKey;
  setPeriod: (p: RevenuePeriodKey) => void;
  periods: { key: RevenuePeriodKey; label: string }[];
  handleExportData: () => void;
  formatCurrency: (n: number) => string;
  tString: (key: string) => string;
  locale: string;
}) {
  // Daily trend series for the centerpiece + sparklines.
  // Trend of COMPLETED days only: ending the range at the in-progress day
  // renders a misleading cliff-to-zero final point (worst right after
  // midnight, and off-by-a-bucket across browser/business timezones).
  // "Today" already has its own live KPI tiles.
  //
  // Window LENGTH tracks the selected period so the trend describes the same
  // horizon as the KPI tiles (was hardcoded to 30 days for every period).
  const today = new Date();
  const from = localDateKey(
    new Date(today.getTime() - TREND_WINDOW_DAYS[period] * 864e5),
  );
  const to = localDateKey(new Date(today.getTime() - 864e5));
  const { series } = useAnalyticsTimeseries(businessId, { from, to });
  const buckets = series?.buckets ?? [];
  // Backend zero-fills EVERY calendar day, so a no-data day is indistinguishable
  // from a genuine $0 day by revenue alone — use the transaction count as the
  // emptiness signal and map no-activity days to `null` GAPS. This removes the
  // end-of-range cliff-to-zero while a real 0-sales-but-had-activity day still
  // plots at 0.
  const revenuePoints = buckets.map((b) => ({
    date: b.date,
    value: (b.transactions ?? 0) > 0 ? (b.revenue as number) : null,
  }));
  // Sparkline wants a dense numeric series; keep zeros here (it has no x-axis
  // to distort and is only shown for the week view).
  const revenueSpark = buckets.map((b) => b.revenue as number);

  // Period-over-period delta: the backend already computes a period-matched
  // `growth_rate` (this-period revenue vs the prior calendar period — see
  // analytics/service.go GetPeriodReport + reporting.PriorWindow). Use it so the headline, delta, and
  // period pill all describe the SAME window instead of mixing the selected
  // period with a fixed last-7-vs-prev-7 calculation.
  const rawGrowth = salesData.growth_rate as number | undefined;
  const revenueDeltaPct =
    rawGrowth != null && Number.isFinite(rawGrowth) ? rawGrowth : undefined;

  // Comparison label must match the SELECTED period ("vs yesterday" for today,
  // "vs prior month" for month, ...), not the old fixed "vs prior 7 days".
  // getTranslation echoes the key path back on a miss — fall back to the
  // generic prior-period phrase then.
  const priorLabelCandidate = tString(`vsPrior.${period}`);
  const comparedToLabel = priorLabelCandidate.includes("vsPrior.")
    ? tString("vsPriorPeriod")
    : priorLabelCandidate;

  // The sparkline is a 30-day daily series — a meaningful "trend" only for the
  // week view. For other periods it would sit under a headline/delta scoped to
  // a different window, so suppress it there.
  const showSparkline = period === "week";

  // Map raw payment-method enum keys (crypto/card/cash/...) through the existing
  // salesOverview.paymentMethods.<key> labels with a raw-key fallback so an
  // unrecognized method still renders.
  const paymentMethodLabel = (key: string): string => {
    const label = tString(`paymentMethods.${key}`);
    // getTranslation echoes the key path back when a key is missing; treat that
    // (or an empty string) as "no translation" and fall back to the raw key.
    return label && !label.includes("paymentMethods.") ? label : key;
  };

  const paymentItems = Object.entries(salesData.payment_methods ?? {})
    .map(([label, value]) => ({ label: paymentMethodLabel(label), value: value as number }));
  const hourItems = Object.entries(salesData.hourly_breakdown ?? {})
    .map(([hour, d]) => ({ hour: parseInt(hour, 10), value: d.revenue as number }));

  return (
    <div className="space-y-6">
      {/* Header: title + canonical PeriodTabs + export Button */}
      <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-4">
        <div>
          <h2 className="text-xl font-semibold text-ink-900">
            {tString("title")}
          </h2>
          <p className="text-sm text-ink-600">
            {tString("subtitle")}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <PeriodTabs
            options={periods}
            value={period}
            onChange={setPeriod}
            ariaLabel={tString("period")}
          />
          <Button
            size="sm"
            variant="bordered"
            radius="full"
            onClick={handleExportData}
            className={btnSecondaryNextUI}
            startContent={<Download className="w-4 h-4" />}
          >
            {tString("export")}
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <MetricStat
          label={tString("metrics.totalRevenue")}
          value={formatCurrency(salesData.total_revenue as number || 0)}
          period={tString(`periods.${period}`)}
          delta={
            revenueDeltaPct != null
              ? { percent: revenueDeltaPct, comparedToLabel }
              : undefined
          }
          sparklineData={showSparkline ? revenueSpark : undefined}
          ariaLabel="revenue trend"
        />
        <MetricStat
          label={tString("metrics.transactions")}
          value={(salesData.transaction_count || 0).toString()}
        />
        <MetricStat
          label={tString("metrics.averageTicket")}
          value={formatCurrency(salesData.average_ticket as number || 0)}
        />
        <MetricStat
          label={tString("metrics.uniqueCustomers")}
          value={(salesData.unique_customers || 0).toString()}
        />
      </div>

      <section className="rounded-2xl border border-warm-200 bg-white p-4">
        <h3 className="mb-3 text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
          {tString("trend.title")}
        </h3>
        <TrendChart
          points={revenuePoints}
          formatValue={(n) => formatCurrency(n)}
          ariaLabel={tString("trend.title")}
          emptyLabel={tString("noData")}
          locale={locale}
        />
      </section>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <section className="rounded-2xl border border-warm-200 bg-white p-4">
          <h3 className="mb-3 text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
            {tString("hourlyBreakdown.title")}
          </h3>
          <HourOfDayChart
            hours={hourItems}
            formatValue={(n) => formatCurrency(n)}
            ariaLabel={tString("hourlyBreakdown.title")}
            emptyLabel={tString("noData")}
          />
        </section>
        <section className="rounded-2xl border border-warm-200 bg-white p-4">
          <h3 className="mb-3 text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
            {tString("paymentMethods.title")}
          </h3>
          <RankedBarList
            items={paymentItems}
            formatValue={(n) => `${n}`}
            ariaLabel={tString("paymentMethods.title")}
            emptyLabel={tString("noData")}
          />
        </section>
      </div>
    </div>
  );
}
