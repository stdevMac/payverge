"use client";
// src/components/dashboard/BusinessOverviewPanel.tsx
//
// Expanded accordion body for a single business row. On open it lazily fetches
// the daily analytics timeseries (via useAnalyticsTimeseries — React Query,
// cached; the shared hook's 5-min staleTime is accepted here, more than the
// ~60s the spec suggested but functionally identical for toggling rows within a
// session, and editing the shared default would regress RevenuePanel). Period
// tabs (7d/30d/90d) re-window the from/to range. Renders a revenue TrendChart
// hero, tips + avg-ticket MetricStats with client-computed week-over-week
// deltas, today's paid-bill count, and a deep link into the business analytics tab.
import React, { useState } from "react";
import Link from "next/link";
import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import { ArrowRight } from "lucide-react";
import type { Business } from "@/api/business";
import type { DashboardSummary } from "@/api/analytics";
import { formatCurrency as formatCurrencyIntl } from "@/api/currency";
import { useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { intlLocaleFor } from "@/utils/intlLocale";
import { localDateKey } from "@/lib/localDate";
import { getBusinessDashboardPath } from "@/utils/businessUrl";
import { useAnalyticsTimeseries } from "@/hooks/useAnalyticsTimeseries";
import { TrendChart } from "./charts/TrendChart";
import { MetricStat } from "./charts/MetricStat";
import { weekOverWeekDelta } from "./weekOverWeekDelta";

interface BusinessOverviewPanelProps {
  business: Business;
  summary: DashboardSummary | null;
  isOpen: boolean;
  t: (key: string) => string;
}

const PERIODS = [
  { key: "7d", days: 7, labelKey: "overview.panel.period7d" },
  { key: "30d", days: 30, labelKey: "overview.panel.period30d" },
  { key: "90d", days: 90, labelKey: "overview.panel.period90d" },
] as const;
type PeriodKey = (typeof PERIODS)[number]["key"];

export function BusinessOverviewPanel({ business, summary, isOpen, t }: BusinessOverviewPanelProps) {
  const { locale } = useSimpleLocale();
  const intlLocale = intlLocaleFor(locale);
  const reduceMotion = useReducedMotion();
  const [period, setPeriod] = useState<PeriodKey>("30d");

  const days = PERIODS.find((p) => p.key === period)!.days;
  const now = Date.now();
  const from = localDateKey(new Date(now - days * 864e5));
  const to = localDateKey(new Date(now - 864e5));

  // Fetch only while open — the hook disables on an undefined id.
  const { series, loading, error } = useAnalyticsTimeseries(
    isOpen ? business.id : undefined,
    { from, to },
  );
  const buckets = series?.buckets ?? [];
  const currency = business.default_currency || "USD";
  const fmt = (n: number) => formatCurrencyIntl(n, currency, undefined, intlLocale);

  // Trend points: null out no-activity days so the line breaks instead of
  // plunging to a phantom $0 (matches RevenuePanel's treatment).
  const revenuePoints = buckets.map((b) => ({
    date: b.date,
    value: (b.transactions ?? 0) > 0 ? (b.revenue as number) : null,
  }));
  const tipsSpark = buckets.map((b) => b.tips as number);
  const avgTicketSpark = buckets.map((b) => b.average_ticket as number);

  const tipsDelta = weekOverWeekDelta(buckets, (b) => b.tips as number);
  const avgTicketDelta = weekOverWeekDelta(buckets, (b) => b.average_ticket as number);

  const body = (
    <div className="border-t border-warm-100 bg-warm-50/40 p-5">
      {/* Header: trend title + period tabs */}
      <div className="mb-4 flex items-center justify-between gap-3">
        <h3 className="text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
          {t("overview.panel.trendTitle")}
        </h3>
        <div className="flex items-center gap-1 rounded-full bg-warm-100 p-1">
          {PERIODS.map((p) => (
            <button
              key={p.key}
              type="button"
              onClick={() => setPeriod(p.key)}
              aria-pressed={period === p.key}
              className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors ${
                period === p.key ? "bg-brand text-white" : "text-ink-500 hover:text-ink-800"
              }`}
            >
              {t(p.labelKey)}
            </button>
          ))}
        </div>
      </div>

      {/* Revenue hero */}
      {error ? (
        <p className="py-6 text-center text-sm text-ink-500">{t("overview.panel.loadError")}</p>
      ) : (
        <section className="rounded-2xl border border-warm-200 bg-white p-4">
          <TrendChart
            points={loading ? [] : revenuePoints}
            formatValue={fmt}
            ariaLabel={t("overview.panel.trendTitle")}
            emptyLabel={t("overview.panel.noData")}
            locale={locale}
          />
        </section>
      )}

      {/* Metric tiles */}
      <div className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-3">
        <MetricStat
          label={t("overview.panel.tips")}
          value={fmt((summary?.today.tips as number) ?? 0)}
          delta={tipsDelta != null ? { percent: tipsDelta, comparedToLabel: t("overview.panel.wow") } : undefined}
          sparklineData={tipsSpark.length > 1 ? tipsSpark : undefined}
          ariaLabel={t("overview.panel.tips")}
        />
        <MetricStat
          label={t("overview.panel.averageTicket")}
          value={fmt((summary?.week.average_ticket as number) ?? 0)}
          delta={
            avgTicketDelta != null
              ? { percent: avgTicketDelta, comparedToLabel: t("overview.panel.wow") }
              : undefined
          }
          sparklineData={avgTicketSpark.length > 1 ? avgTicketSpark : undefined}
          ariaLabel={t("overview.panel.averageTicket")}
        />
        <div data-testid="business-today-bills">
          <MetricStat
            label={t("overview.todayPaidBills")}
            value={summary ? summary.today.bills : 0}
          />
        </div>
      </div>

      {/* Deep link into full analytics */}
      <div className="mt-4">
        <Link
          href={`${getBusinessDashboardPath(business)}?tab=analytics`}
          className="inline-flex items-center gap-1 text-sm font-semibold text-brand hover:text-brand-dark"
        >
          {t("overview.panel.openFullAnalytics")}
          <ArrowRight className="h-4 w-4" />
        </Link>
      </div>
    </div>
  );

  // Reduced-motion: render/unmount without the height animation.
  if (reduceMotion) {
    return isOpen ? body : null;
  }

  return (
    <AnimatePresence initial={false}>
      {isOpen && (
        <motion.div
          key="panel"
          initial={{ height: 0, opacity: 0 }}
          animate={{ height: "auto", opacity: 1 }}
          exit={{ height: 0, opacity: 0 }}
          transition={{ duration: 0.2, ease: "easeOut" }}
          className="overflow-hidden"
        >
          {body}
        </motion.div>
      )}
    </AnimatePresence>
  );
}
