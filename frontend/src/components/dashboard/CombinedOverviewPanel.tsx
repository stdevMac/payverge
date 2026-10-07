"use client";

import React, { useState } from "react";
import type { Business } from "@/api/business";
import { formatCurrency as formatCurrencyIntl } from "@/api/currency";
import { useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { intlLocaleFor } from "@/utils/intlLocale";
import { localDateKey } from "@/lib/localDate";
import { useCombinedAnalyticsTimeseries } from "@/hooks/useCombinedAnalyticsTimeseries";
import { seriesHasActivity } from "./mergeTimeseries";
import { TrendChart } from "./charts/TrendChart";

interface CombinedOverviewPanelProps {
  businesses: Business[];
  todayHasActivity: boolean;
  statsLoading: boolean;
  t: (key: string) => string;
}

const PERIODS = [
  { key: "7d", days: 7, labelKey: "overview.panel.period7d" },
  { key: "30d", days: 30, labelKey: "overview.panel.period30d" },
  { key: "90d", days: 90, labelKey: "overview.panel.period90d" },
] as const;
type PeriodKey = (typeof PERIODS)[number]["key"];

export function CombinedOverviewPanel({
  businesses,
  todayHasActivity,
  statsLoading,
  t,
}: CombinedOverviewPanelProps) {
  const { locale } = useSimpleLocale();
  const intlLocale = intlLocaleFor(locale);
  const [period, setPeriod] = useState<PeriodKey>("30d");

  const days = PERIODS.find((entry) => entry.key === period)!.days;
  const now = Date.now();
  const from = localDateKey(new Date(now - days * 864e5));
  const to = localDateKey(new Date(now - 864e5));

  const { seriesByCurrency, loading, error } = useCombinedAnalyticsTimeseries(
    businesses,
    { from, to, enabled: businesses.length > 0 },
  );
  const hasHistoricalActivity = seriesHasActivity(seriesByCurrency);
  const showTodayEmpty =
    !statsLoading && !todayHasActivity && !loading;

  return (
    <div className="mt-7" data-testid="combined-overview-panel">
      <div className="mb-4 flex items-center justify-between gap-3">
        <h3 className="text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
          {t("overview.panel.trendTitle")}
        </h3>
        <div className="flex items-center gap-1 rounded-full bg-warm-100 p-1">
          {PERIODS.map((entry) => (
            <button
              key={entry.key}
              type="button"
              onClick={() => setPeriod(entry.key)}
              aria-pressed={period === entry.key}
              className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors ${
                period === entry.key
                  ? "bg-brand text-white"
                  : "text-ink-500 hover:text-ink-800"
              }`}
            >
              {t(entry.labelKey)}
            </button>
          ))}
        </div>
      </div>

      {error ? (
        <p className="py-6 text-center text-sm text-ink-500">
          {t("overview.panel.loadError")}
        </p>
      ) : (
        <div className="space-y-4">
          {(loading || seriesByCurrency.length === 0 ? [null] : seriesByCurrency).map(
            (entry) => {
              const currency = entry?.currency ?? "USD";
              const points = loading || !entry ? [] : entry.points;
              const fmt = (amount: number) =>
                formatCurrencyIntl(amount, currency, undefined, intlLocale);
              return (
                <section
                  key={currency}
                  data-testid="combined-trend"
                  className="rounded-2xl border border-warm-200 bg-white p-4"
                >
                  {seriesByCurrency.length > 1 ? (
                    <p className="mb-2 text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
                      {currency}
                    </p>
                  ) : null}
                  <TrendChart
                    points={points}
                    formatValue={fmt}
                    ariaLabel={t("overview.panel.trendTitle")}
                    emptyLabel={
                      showTodayEmpty
                        ? t("overview.noActivityToday")
                        : t("overview.panel.noData")
                    }
                    locale={locale}
                  />
                </section>
              );
            },
          )}
        </div>
      )}

      {showTodayEmpty && hasHistoricalActivity ? (
        <p
          data-testid="portfolio-no-activity-today"
          className="mt-4 max-w-xl text-sm leading-6 text-ink-500"
        >
          {t("overview.noActivityToday")}
        </p>
      ) : null}
    </div>
  );
}
