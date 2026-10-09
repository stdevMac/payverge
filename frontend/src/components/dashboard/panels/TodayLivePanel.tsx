"use client";
import React, { useEffect, useState } from "react";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";
import { getBusiness } from "@/api/business";
import { formatCurrency as formatCurrencyIntl } from "@/api/currency";
import { intlLocaleFor } from "@/utils/intlLocale";
import { useLivePulse } from "@/hooks/useLivePulse";
import { localizedErrorMessage } from "@/utils/localizedError";
import { MetricStat } from "../charts/MetricStat";
import TodayLiveHero from "./TodayLiveHero";
import LiveBills from "../LiveBills";
import { PanelFailure } from "@/components/ui/AsyncState";

export default function TodayLivePanel({
  businessId,
  currency: currencyProp,
  businessTimezone: businessTimezoneProp,
  country: countryProp,
}: {
  businessId: string;
  // M5b: currency + IANA timezone are threaded down from the dashboard page so
  // this panel (and LiveBills below it) no longer each fetch getBusiness just to
  // read them — that was two duplicate fetches per analytics-tab visit and a
  // brief "$/USD" flash. When the prop is absent (standalone render) we fall
  // back to a single local fetch to stay correct.
  currency?: string;
  businessTimezone?: string | null;
  country?: string | null;
}) {
  const { locale } = useSimpleLocale();
  const tString = (key: string): string => {
    const r = getTranslation(`businessDashboard.dashboard.todayLive.${key}`, locale);
    return Array.isArray(r) ? r[0] || key : (r as string);
  };

  const [fetchedCurrency, setFetchedCurrency] = useState("USD");
  const [fetchedTimezone, setFetchedTimezone] = useState<string | null>(null);
  const [fetchedCountry, setFetchedCountry] = useState<string | null>(null);

  // Prefer the props from the parent (no fetch); fall back to the locally
  // fetched values only when the parent didn't supply them.
  const currency = currencyProp ?? fetchedCurrency;
  const businessTimezone =
    businessTimezoneProp !== undefined ? businessTimezoneProp : fetchedTimezone;
  const country =
    countryProp !== undefined ? countryProp : fetchedCountry;

  useEffect(() => {
    let cancelled = false;
    if (!businessId) return;
    // Only fetch when the parent didn't thread currency/timezone/country down.
    // On the dashboard the props are always present, so this fetch never fires.
    if (
      currencyProp !== undefined &&
      businessTimezoneProp !== undefined &&
      countryProp !== undefined
    ) {
      return;
    }
    getBusiness(businessId)
      .then((b) => {
        if (cancelled) return;
        if (b?.default_currency) setFetchedCurrency(b.default_currency);
        setFetchedTimezone(b?.timezone ?? null);
        setFetchedCountry(b?.address?.country ?? "");
      })
      .catch(() => {});
    return () => { cancelled = true; };
  }, [businessId, currencyProp, businessTimezoneProp, countryProp]);

  // Fix 5: ONE shared 10s poller feeds both the hero/metrics (summary) AND the
  // open-bills list below. Previously this panel polled getDashboardSummary and
  // LiveBills each ran its own /analytics/live-bills poll — two overlapping 10s
  // cycles per open tab. useLivePulse fetches both in a single cycle (hidden-tab
  // pause preserved) and we thread the bills down to LiveBills as props so it
  // does not run a second poller.
  const {
    summary,
    liveBills,
    capped,
    lastUpdated,
    loading,
    error: pulseError,
    refresh,
  } = useLivePulse(businessId);
  const error = pulseError ? localizedErrorMessage(pulseError, locale) : null;

  const fmt = (n: number) =>
    formatCurrencyIntl(n, currency, undefined, intlLocaleFor(locale));

  // "Compared to a typical day" excludes today from its own baseline. The week
  // window (7 days) includes today, so the prior-days baseline is the average
  // of the remaining 6 completed days: (weekTotal - todayVal) / 6. Comparing a
  // partial day against a full-day average still over-reports "down vs typical"
  // early in the day, but at minimum today no longer pollutes its own baseline.
  const deltaPct = (todayVal: number, weekTotal: number): number | undefined => {
    // A zero-value tile early in the day would compute a blunt -100% "vs
    // typical day", which reads as catastrophic rather than "the day just
    // hasn't started". Suppress the delta entirely at zero and surface a
    // neutral "no sales yet today" note instead (see neutralNote below).
    if (todayVal <= 0) return undefined;
    const priorDaysTotal = weekTotal - todayVal;
    const avg = priorDaysTotal > 0 ? priorDaysTotal / 6 : 0;
    if (!(avg > 0)) return undefined;
    const pct = ((todayVal - avg) / avg) * 100;
    // Demo / thin history routinely produces +400–500% "vs typical day" which
    // reads as broken maths, not insight (#146 / #218). Hide absurd lifts.
    if (!Number.isFinite(pct) || Math.abs(pct) > 150) return undefined;
    return pct;
  };

  const comparedLabel = tString("vsTypicalDay");
  // Neutral copy shown in the delta slot for a zero-value tile.
  const noSalesNote = tString("noSalesYet");
  const neutralFor = (todayVal: number | undefined) =>
    (todayVal ?? 0) <= 0 ? noSalesNote : undefined;

  if (loading && !summary) {
    return (
      <div className="flex items-center justify-center py-16">
        <div className="text-center">
          <div className="w-16 h-16 bg-warm-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-warm-100">
            <div className="w-8 h-8 border-2 border-warm-300 border-t-ink-900 rounded-full animate-spin"></div>
          </div>
          <p className="text-ink-600 tracking-wide">{tString("loading")}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {error && (
        <PanelFailure
          title={tString("error")}
          message={error}
          retryLabel={tString("retry")}
          onRetry={() => void refresh()}
          isRetrying={loading}
        />
      )}
      <TodayLiveHero
        revenue={
          (summary?.today.collected_revenue as number | undefined) ??
          (summary?.today.revenue as number) ??
          0
        }
        formatRevenue={fmt}
        bills={summary?.today.bills ?? 0}
        openTables={openTablesCount(summary)}
        remaining={(summary?.today.floor_remaining as number) ?? 0}
        labels={{
          todayRevenue: tString("hero.todayRevenue"),
          billsClosed: tString("hero.billsClosed"),
          openTables: tString("hero.openTables"),
          livePulse: tString("hero.livePulse"),
          remainingOnFloor: tString("remainingOnFloor"),
          noSalesYet: noSalesNote,
        }}
      />

      {/* #203: hero is recognized payments collected today — not open bill totals. */}
      <p
        className="text-xs leading-relaxed text-ink-500"
        data-testid="today-live-revenue-basis"
      >
        {tString("revenueBasis")}
      </p>

      {/* #217: do not repeat revenue / bills / open tables under the hero.
          Tips (and week context) are the only unique secondary KPI. */}
      <div className="grid grid-cols-1 gap-3 sm:max-w-sm">
        <MetricStat
          label={tString("tips")}
          value={fmt(summary?.today.tips as number ?? 0)}
          period={tString("today")}
          delta={summary ? toDelta(deltaPct(summary.today.tips as number, summary.week.tips as number), comparedLabel) : undefined}
          neutralNote={summary ? neutralFor(summary.today.tips as number) : undefined}
        />
      </div>

      <section className="rounded-2xl border border-warm-200 bg-white p-5">
        <h3 className="mb-3 text-label uppercase text-ink-500">
          {tString("openBills")}
        </h3>
        <LiveBills
          businessId={businessId}
          currency={currency}
          businessTimezone={businessTimezone}
          country={country}
          bills={liveBills}
          loading={loading}
          error={error}
          lastUpdated={lastUpdated}
          capped={capped}
          onRefresh={refresh}
        />
      </section>
    </div>
  );
}

// Only show a delta when it is finite (avoids a misleading "▲ 0%" when there is
// no week baseline yet).
function toDelta(percent: number | undefined, comparedToLabel: string) {
  return percent != null && Number.isFinite(percent)
    ? { percent, comparedToLabel }
    : undefined;
}

/** Distinct occupied tables — never raw active_bills (delivery/counter inflate it). */
function openTablesCount(summary: {
  live?: {
    open_tables?: number;
    active_bills_by_table?: Record<string | number, unknown>;
  };
} | null): number {
  if (!summary?.live) return 0;
  if (typeof summary.live.open_tables === "number") {
    return summary.live.open_tables;
  }
  const byTable = summary.live.active_bills_by_table;
  if (byTable) return Object.keys(byTable).length;
  return 0;
}
