"use client";

import React from "react";
import { RefreshCw, TrendingDown, TrendingUp } from "lucide-react";
import PeriodTabs from "../shared/PeriodTabs";
import { PremiumPanel } from "../premium";

export type AccountingDashboardTab =
  | "overview"
  | "entries"
  | "payroll"
  | "invoices"
  | "reports"
  | "outstanding";

export type FoodCostPeriod = "day" | "week" | "month";

const foodCostPeriodOptions: readonly FoodCostPeriod[] = [
  "day",
  "week",
  "month",
];

export interface AccountingDashboardProps {
  businessId: string;
  initialTab?: AccountingDashboardTab;
  /** Fired when the operator selects an inner tab so the parent can mirror it
   *  to the dashboard URL (`?tab=accounting&sub=<inner>`), keeping the sidebar
   *  active-nav indicator truthful. */
  onTabChange?: (tab: AccountingDashboardTab) => void;
  /** Whether the viewer may MUTATE payroll (create / mark-paid / delete / void).
   *  Owner-only by default; the page passes `!isStaffUser`. Reads stay available
   *  to managers. Mirrors the backend payroll:write permission — the backend is
   *  the real boundary; this only avoids showing controls that would 403. */
  canManagePayroll?: boolean;
  /** Business IANA timezone; threaded into FiscalDashboard so receipt/credential
   *  dates render on the business calendar day, not the operator's device TZ. */
  businessTimezone?: string | null;
  /** Business address ISO country; gates AFIP identity capture on bill drawers. */
  country?: string | null;
  /**
   * NEW-15: true when the operator opened this screen via `?tab=fiscal`
   * (sidebar "Facturas" / "Invoices"). The content is still the accounting
   * shell on the invoices sub-tab, but the page header must acknowledge the
   * fiscal entry point instead of only saying "Contabilidad" / "Accounting".
   */
  fiscalEntry?: boolean;
}

// Category lists live in categories.ts (shared by EntriesTab filters + create form).

export const accountingPanelClass = "overflow-hidden";
export const accountingPanelHeaderClass =
  "flex flex-wrap items-center justify-between gap-3 border-b border-warm-200/80 bg-gradient-to-r from-warm-50/80 via-white to-brand/5 px-5 py-4";
export const accountingPanelTitleClass = "text-base font-semibold text-ink-950";
export const accountingSecondaryButtonClass =
  "inline-flex h-9 items-center gap-1.5 rounded-xl border border-warm-200/90 bg-white/85 px-3 text-sm font-medium text-ink-700 shadow-sm shadow-warm-900/5 transition-all duration-200 hover:-translate-y-0.5 hover:border-brand/25 hover:bg-brand/5 hover:text-brand-700 disabled:cursor-not-allowed disabled:opacity-50";
export const accountingPrimaryButtonClass =
  "inline-flex h-9 items-center gap-1.5 rounded-xl bg-brand px-3 text-sm font-semibold text-white shadow-cta-glow transition-all duration-200 hover:-translate-y-0.5 hover:bg-brand-dark disabled:cursor-not-allowed disabled:opacity-60";

function pad(value: number): string {
  return value.toString().padStart(2, "0");
}

export function formatDateInput(date: Date): string {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

// Anchor a YYYY-MM-DD calendar date at 12:00 UTC (noon), not 00:00 UTC.
// The backend windows manual entries in the business-local timezone
// (parseDateRange → time.ParseInLocation), while occurred_at is stored
// verbatim. A UTC-midnight timestamp lands on the *previous* calendar day
// for any business west of UTC (e.g. Argentina UTC-3 → 21:00 the day
// before), so the entry falls outside the intended day's window. Noon UTC
// resolves to the same calendar day for every real business timezone
// (UTC-11 … UTC+14 all keep the date), so the entry is bucketed on the day
// the operator picked regardless of their timezone. See R3-AC-1.
export function toIsoDate(value: string): string {
  return `${value}T12:00:00Z`;
}

function titleCaseCategory(category: string): string {
  if (!category) return "";
  return category
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

export function formatCategoryLabel(
  category: string,
  t?: (key: string) => string,
): string {
  if (!category) return "";
  if (t) {
    const translated = t(`categories.${category}`);
    if (translated && !translated.includes("categories.")) return translated;
  }
  return titleCaseCategory(category);
}

// foodCostTone maps a food-cost ratio (0..1) to a trend color proxy.
// <30% good (up), 30-35% caution (null), >35% bad (down).
export function foodCostTone(pct: number): "up" | "down" | null {
  if (pct <= 0) return null;
  if (pct < 0.3) return "up";
  if (pct <= 0.35) return null;
  return "down";
}

interface FoodCostKpiCardProps {
  label: string;
  value: string;
  trend?: "up" | "down" | null;
  hint: string;
  estimatedCogs: string;
  totalRevenue: string;
  /** When false, show empty state instead of proud 0% (PV-LIVE-20260720-005). */
  hasMappedData?: boolean;
  selectedPeriod: FoodCostPeriod;
  onPeriodChange: (period: FoodCostPeriod) => void;
  loading?: boolean;
  error?: boolean;
  onRetry?: () => void;
  labels: {
    periodSelector: string;
    cogs: string;
    totalRevenue: string;
    loading: string;
    error: string;
    retry: string;
    empty: string;
    periods: Record<FoodCostPeriod, string>;
  };
}

export function FoodCostKpiCard({
  label,
  value,
  trend,
  hint,
  estimatedCogs,
  totalRevenue,
  hasMappedData = true,
  selectedPeriod,
  onPeriodChange,
  loading = false,
  error = false,
  onRetry,
  labels,
}: FoodCostKpiCardProps) {
  const trendColor =
    trend === "up"
      ? "text-emerald-600"
      : trend === "down"
        ? "text-rose-600"
        : "text-ink-400";
  const TrendIcon =
    trend === "up" ? TrendingUp : trend === "down" ? TrendingDown : null;

  const header = (
    // Header mirrors the LaborCostCard analysis-card idiom: title block
    // left (title + descriptive subtitle), period pills right.
    <div className="flex flex-wrap items-start justify-between gap-2">
      <div className="min-w-0">
        <p className="text-sm font-semibold text-ink-900">{label}</p>
        <p className="text-[11px] text-ink-500">{hint}</p>
      </div>
      <PeriodTabs
        ariaLabel={labels.periodSelector}
        value={selectedPeriod}
        onChange={onPeriodChange}
        options={foodCostPeriodOptions.map((period) => ({
          key: period,
          label: labels.periods[period],
        }))}
      />
    </div>
  );

  // A swallowed fetch used to assert a false "0%". Surface the failure with an
  // inline retry instead. See R3-AC.
  if (error) {
    return (
      <PremiumPanel className="p-5" withTexture={false}>
        {header}
        <div className="mt-4 flex flex-col items-center justify-center gap-3 rounded-2xl border border-rose-200 bg-rose-50 px-4 py-6 text-center">
          <p className="text-sm text-rose-700">{labels.error}</p>
          {onRetry ? (
            <button
              type="button"
              onClick={onRetry}
              className="inline-flex items-center gap-1.5 rounded-lg border border-rose-300 bg-white px-3 py-1.5 text-sm font-medium text-rose-700 transition-colors hover:bg-rose-100"
            >
              <RefreshCw className="h-3.5 w-3.5" />
              {labels.retry}
            </button>
          ) : null}
        </div>
      </PremiumPanel>
    );
  }

  if (loading) {
    return (
      <PremiumPanel className="p-5" withTexture={false}>
        {header}
        <div
          aria-label={labels.loading}
          className="mt-4 h-24 animate-pulse rounded-2xl bg-warm-100"
        />
      </PremiumPanel>
    );
  }

  // Mirror LaborCostCard: zero recipe-mapped revenue is not a healthy 0% KPI.
  if (!hasMappedData) {
    return (
      <PremiumPanel className="p-5" withTexture={false} data-testid="food-cost-empty">
        {header}
        <p className="mt-4 rounded-2xl border border-warm-200 bg-warm-50/80 px-4 py-6 text-center text-sm text-ink-600">
          {labels.empty}
        </p>
      </PremiumPanel>
    );
  }

  return (
    <PremiumPanel className="p-5" withTexture={false}>
      {header}
      <div className="mt-4 flex items-end justify-between gap-2">
        {/* Value is pre-formatted by the caller; keep neutral ink (never auto-emerald
            for a bare number). Trend icon carries the health signal when present. */}
        <p
          className="text-2xl font-semibold tabular-nums text-ink-900"
          data-testid="food-cost-kpi-value"
        >
          {value}
        </p>
        {TrendIcon ? <TrendIcon className={`h-4 w-4 ${trendColor}`} /> : null}
      </div>
      <dl className="mt-3 space-y-2 border-t border-warm-200/80 pt-3">
        <div className="flex items-start justify-between gap-2">
          <dt className="min-w-0 text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
            {labels.cogs}
          </dt>
          <dd className="shrink-0 text-right text-sm font-semibold text-ink-900">
            {estimatedCogs}
          </dd>
        </div>
        <div className="flex items-start justify-between gap-2">
          <dt className="min-w-0 text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
            {labels.totalRevenue}
          </dt>
          <dd className="shrink-0 text-right text-sm font-semibold text-ink-900">
            {totalRevenue}
          </dd>
        </div>
      </dl>
    </PremiumPanel>
  );
}
