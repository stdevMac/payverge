"use client";

import { getSafeApiErrorMessage } from "@/utils/apiError";

import React, { useState, useEffect, useCallback } from "react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { localDateKey } from "@/lib/localDate";
import { TrendingDown, DollarSign, Award } from "lucide-react";
import { analyticsApi, TipAnalytics } from "@/api/analytics";
import { getBusiness } from "@/api/business";
import { formatCurrency as formatCurrencyIntl } from "@/api/currency";
import { intlLocaleFor } from "@/utils/intlLocale";
import PeriodTabs from "@/components/business/shared/PeriodTabs";
import { MetricStat } from "@/components/dashboard/charts/MetricStat";
import { TrendChart } from "@/components/dashboard/charts/TrendChart";
import { RankedBarList } from "@/components/dashboard/charts/RankedBarList";
import { HourOfDayChart } from "@/components/dashboard/charts/HourOfDayChart";
import { useAnalyticsTimeseries } from "@/hooks/useAnalyticsTimeseries";

interface ServiceTipsPanelProps {
  businessId: string;
  /** Fix 7: currency threaded from the dashboard page (no re-fetch). */
  currency?: string;
  /** Fix 4: shared analytics period (controlled) + change reporter. */
  period?: string;
  onPeriodChange?: (period: string) => void;
}

// Canonical display order for the currency-neutral tip-distribution bucket keys
// emitted by the backend (backend/internal/analytics/service.go). The keys are
// currency-neutral so the frontend can render currency-correct labels for any
// business currency; this list also imposes a deterministic order (we do not
// rely on JS object-key iteration order or on parsing the key string).
const TIP_BUCKET_ORDER = ["0_5", "5_10", "10_20", "20_50", "50_plus"] as const;

// Canonical period keys — `as const` so the period state and PeriodTabs<K>
// instantiate on the narrow key union instead of string.
const TIP_PERIOD_KEYS = [
  "today",
  "yesterday",
  "week",
  "month",
  "quarter",
] as const;
type TipPeriodKey = (typeof TIP_PERIOD_KEYS)[number];

const isTipPeriodKey = (v: string | undefined): v is TipPeriodKey =>
  !!v && (TIP_PERIOD_KEYS as readonly string[]).includes(v);

// Trend-chart lookback (in completed days) for each period. The `to` bound is
// always yesterday, so `from = yesterday - span`. today/yesterday keep a 1-day
// span so the "Daily tip trend" isn't a single point; week=7, month=30,
// quarter=90. Unknown keys fall back to 30 (see default at the call site).
const TIP_PERIOD_SPAN_DAYS: Record<TipPeriodKey, number> = {
  today: 1,
  yesterday: 1,
  week: 7,
  month: 30,
  quarter: 90,
};

// Maps a neutral bucket key to a localized, currency-correct label using the
// business currency (e.g. "$0.00–$5.00" for USD, "AED 0.00–AED 5.00" for AED).
// The top bucket renders as "<formatted>+". Unknown keys fall back to the raw
// key so nothing silently disappears from the chart.
function tipBucketLabel(
  key: string,
  formatCurrency: (n: number) => string,
): string {
  switch (key) {
    case "0_5":
      return `${formatCurrency(0)}–${formatCurrency(5)}`;
    case "5_10":
      return `${formatCurrency(5)}–${formatCurrency(10)}`;
    case "10_20":
      return `${formatCurrency(10)}–${formatCurrency(20)}`;
    case "20_50":
      return `${formatCurrency(20)}–${formatCurrency(50)}`;
    case "50_plus":
      return `${formatCurrency(50)}+`;
    default:
      return key;
  }
}

// Outer component: handles data fetching, loading/error/empty early returns,
// and all non-visual wiring. Passes everything into the inner body so that
// useAnalyticsTimeseries is always called unconditionally (rules-of-hooks).
export default function ServiceTipsPanel({
  businessId,
  currency: currencyProp,
  period: periodProp,
  onPeriodChange,
}: ServiceTipsPanelProps) {
  // Translation setup — ported verbatim from TipReports.tsx
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);

  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  const tString = (key: string): string => {
    const fullKey = `businessDashboard.dashboard.tipReports.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  const [tipData, setTipData] = useState<TipAnalytics | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [localPeriod, setLocalPeriod] = useState<TipPeriodKey>("week");
  const period: TipPeriodKey = isTipPeriodKey(periodProp) ? periodProp : localPeriod;
  const setPeriod = (key: TipPeriodKey) => {
    setLocalPeriod(key);
    onPeriodChange?.(key);
  };
  // Business currency — prefer the threaded prop (Fix 7); only fetch when absent.
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
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [businessId, currencyProp]);

  const fetchTipAnalytics = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await analyticsApi.getTipAnalytics(businessId, period);
      setTipData(data);
    } catch (err) {
      console.error("ServiceTipsPanel - API Error:", err);
      setError(getSafeApiErrorMessage(err, "An error occurred"));
    } finally {
      setLoading(false);
    }
  }, [businessId, period]);

  useEffect(() => {
    fetchTipAnalytics();
  }, [fetchTipAnalytics, period]);

  const formatCurrency = (amount: number) =>
    formatCurrencyIntl(amount, businessCurrency, undefined, intlLocaleFor(locale));

  const formatPercentage = (value: number) => `${value.toFixed(1)}%`;

  const getTipRateLabel = (rate: number) => {
    if (rate >= 20) return tString("tipRateLabels.excellent");
    if (rate >= 15) return tString("tipRateLabels.good");
    if (rate >= 10) return tString("tipRateLabels.average");
    return tString("tipRateLabels.belowAverage");
  };

  // Keys from the canonical tuple, labels from the `periods.<key>` i18n entries.
  const periods = TIP_PERIOD_KEYS.map((key) => ({
    key,
    label: tString(`periods.${key}`),
  }));

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

  if (!tipData) {
    return (
      <div className="text-center py-12">
        <div className="w-16 h-16 bg-warm-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-warm-100">
          <DollarSign className="w-8 h-8 text-ink-400" />
        </div>
        <p className="text-ink-600 tracking-wide">
          {tString("noData")}
        </p>
      </div>
    );
  }

  // Derived from tip_distribution. The backend emits currency-neutral bucket
  // keys ("0_5", "5_10", …, "50_plus"); we order them by the canonical bucket
  // sequence rather than parsing the key string, then append any unrecognized
  // keys (forward-compat) after the known buckets in insertion order.
  const tipDistributionEntries = Object.entries(
    tipData.tip_distribution || {},
  ).sort(([a], [b]) => {
    const rank = (key: string) => {
      const i = TIP_BUCKET_ORDER.indexOf(key as (typeof TIP_BUCKET_ORDER)[number]);
      return i === -1 ? TIP_BUCKET_ORDER.length : i;
    };
    return rank(a) - rank(b);
  });

  return (
    <ServiceTipsBody
      businessId={businessId}
      tipData={tipData}
      period={period}
      setPeriod={setPeriod}
      periods={periods}
      tipDistributionEntries={tipDistributionEntries}
      formatCurrency={formatCurrency}
      formatPercentage={formatPercentage}
      getTipRateLabel={getTipRateLabel}
      tString={tString}
      locale={locale}
    />
  );
}

/** Staff-facing tipper label — CRM name only; never raw wallet hex. */
export function tipperDisplayName(
  tipper: { guest_name?: string; payer_address: string },
  anonymousLabel: string,
): string {
  const name = tipper.guest_name?.trim();
  if (name && !/^0x[a-fA-F0-9]{6,}$/i.test(name)) return name;
  return anonymousLabel;
}

// Inner component: useAnalyticsTimeseries is always called here (no early returns
// above it), satisfying rules-of-hooks. Rendered only when tipData is non-null.
function ServiceTipsBody({
  businessId,
  tipData,
  period,
  setPeriod,
  periods,
  tipDistributionEntries,
  formatCurrency,
  formatPercentage,
  getTipRateLabel,
  tString,
  locale,
}: {
  businessId: string;
  tipData: TipAnalytics;
  period: TipPeriodKey;
  setPeriod: (p: TipPeriodKey) => void;
  periods: { key: TipPeriodKey; label: string }[];
  tipDistributionEntries: [string, number][];
  formatCurrency: (n: number) => string;
  formatPercentage: (n: number) => string;
  getTipRateLabel: (rate: number) => string;
  tString: (key: string) => string;
  locale: string;
}) {
  // Trend of COMPLETED days only: ending the range at the in-progress day
  // renders a misleading cliff-to-zero final point (worst right after
  // midnight, and off-by-a-bucket across browser/business timezones).
  // "Today" already has its own live KPI tiles.
  //
  // Audit M1: the window must follow the selected period, not hardcode 30 days.
  // Sub-day periods (today/yesterday) still show a short completed-days trend
  // (min 1 day) rather than a single empty point. Unknown keys default to 30.
  const spanDays = TIP_PERIOD_SPAN_DAYS[period] ?? 30;
  const today = new Date();
  const from = localDateKey(new Date(today.getTime() - spanDays * 864e5));
  const to = localDateKey(new Date(today.getTime() - 864e5));
  const { series } = useAnalyticsTimeseries(businessId, { from, to });
  const tipPoints = (series?.buckets ?? []).map((b) => ({ date: b.date, value: b.tips as number }));

  const distItems = tipDistributionEntries.map(([bucketKey, count]) => ({
    label: tipBucketLabel(bucketKey, formatCurrency),
    value: count as number,
  }));

  const hourItems = Object.entries(tipData.hourly_tips ?? {}).map(([h, v]) => ({
    hour: parseInt(h, 10),
    value: v as number,
  }));

  const dc = tipData.daily_comparison;

  return (
    <div className="space-y-6">
      {/* Header with canonical PeriodTabs */}
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
        </div>
      </div>

      {/* Key Metrics */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <MetricStat
          label={tString("metrics.totalTips")}
          value={formatCurrency(tipData.total_tips || 0)}
        />
        <MetricStat
          label={tString("metrics.averageTip")}
          value={formatCurrency(tipData.average_tip || 0)}
        />
        <MetricStat
          label={tString("metrics.tipRate")}
          value={formatPercentage(tipData.average_tip_rate)}
          subtext={getTipRateLabel(tipData.average_tip_rate)}
        />
        <MetricStat
          label={tString("dailyComparison.today")}
          value={formatCurrency(dc?.today ?? 0)}
          delta={
            dc
              ? {
                  percent: dc.change_percentage,
                  comparedToLabel: tString("dailyComparison.yesterday"),
                }
              : undefined
          }
        />
      </div>

      {/* Daily Tip Trend */}
      <section className="rounded-2xl border border-warm-200 bg-white p-4">
        <h3 className="mb-3 text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
          {tString("trend.title")}
        </h3>
        <TrendChart
          points={tipPoints}
          formatValue={(n) => formatCurrency(n)}
          ariaLabel={tString("trend.title")}
          emptyLabel={tString("noData")}
          locale={locale}
        />
      </section>

      {/* Distribution + Hourly */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <section className="rounded-2xl border border-warm-200 bg-white p-4">
          <h3 className="mb-3 text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
            {tString("tipDistribution.title")}
          </h3>
          <RankedBarList
            items={distItems}
            formatValue={(n) => `${n}`}
            ariaLabel={tString("tipDistribution.title")}
            emptyLabel={tString("noData")}
          />
        </section>
        <section className="rounded-2xl border border-warm-200 bg-white p-4">
          <h3 className="mb-3 text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
            {tString("hourlyTips.title")}
          </h3>
          <HourOfDayChart
            hours={hourItems}
            formatValue={(n) => formatCurrency(n)}
            ariaLabel={tString("hourlyTips.title")}
            emptyLabel={tString("noData")}
          />
        </section>
      </div>

      {/* Top Tippers — CRM guest names only; never raw wallet hex (#253). */}
      {tipData.top_tippers && tipData.top_tippers.length > 0 && (
        <div
          className="bg-white border border-warm-200 rounded-lg shadow-sm"
          data-testid="top-tippers"
        >
          <div className="border-b border-warm-200 p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-warm-50 rounded-xl flex items-center justify-center border border-warm-100">
                <Award className="w-5 h-5 text-ink-600" />
              </div>
              <div>
                <h3 className="text-lg text-ink-900 tracking-wide">
                  {tString("topTippers.title")}
                </h3>
              </div>
            </div>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-warm-50">
                <tr>
                  <th className="px-4 py-3 text-left text-xs font-medium text-ink-600 uppercase tracking-wider">
                    {tString("topTippers.rank")}
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-ink-600 uppercase tracking-wider">
                    {tString("topTippers.customer")}
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-ink-600 uppercase tracking-wider">
                    {tString("topTippers.totalTips")}
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-ink-600 uppercase tracking-wider">
                    {tString("topTippers.tipCount")}
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-ink-600 uppercase tracking-wider">
                    {tString("topTippers.averageTip")}
                  </th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-warm-200">
                {tipData.top_tippers.slice(0, 10).map((tipper, index) => {
                  const label = tipperDisplayName(
                    tipper,
                    tString("topTippers.anonymousGuest"),
                  );
                  return (
                    <tr
                      key={`${tipper.payer_address}-${index}`}
                      className="hover:bg-warm-50"
                    >
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <span className="font-semibold text-lg text-ink-700">
                            #{index + 1}
                          </span>
                          {index < 3 && (
                            <Award className="w-4 h-4 text-ink-400" />
                          )}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-sm font-medium text-ink-800">
                          {label}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="font-semibold text-ink-700">
                          {formatCurrency(tipper.total_tips)}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-ink-600">{tipper.tip_count}</div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="font-semibold text-ink-700">
                          {formatCurrency(tipper.average_tip)}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}
