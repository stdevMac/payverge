"use client";

import React, { useMemo } from "react";
import type { Locale } from "@/i18n/localeRegistry";
import {
  useProfitLoss,
  useSummary,
  useTimeseries,
} from "@/hooks/accounting/useAccountingQueries";
import { useCostHealth } from "@/hooks/accounting/useCostHealth";
import { combineCostHealthCoverage } from "./dishRecipeCoverage";
import { useDishRecipeCoverage } from "./useDishRecipeCoverage";
import { PremiumPanel } from "../premium";
import {
  formatCategoryLabel,
  type AccountingDashboardTab,
} from "./accountingShared";
import CategoryBreakdown from "./CategoryBreakdown";
import NeedsAttentionStrip from "./NeedsAttentionStrip";
import RecentActivity, { type ActivityItem } from "./RecentActivity";
import { formatDay } from "@/components/business/accounting/format";
import { Metric, isAllZeroSeries } from "@/components/ui/Metric";

export type OverviewTabProps = {
  businessId: string | number;
  start: string;
  end: string;
  locale: Locale;
  t: (key: string, params?: Record<string, string | number>) => string;
  tWith: (key: string, replacements: Record<string, string | number>) => string;
  fmtMoney: (value: number, currency: string) => string;
  activityItems: ActivityItem[];
  formatOccurredAt: (iso: string) => string;
  onSubTabChange?: (tab: AccountingDashboardTab) => void;
};

function OverviewKpiTile({
  label,
  value,
  currency,
  formatMoney,
  series,
}: {
  label: string;
  value: number;
  currency: string;
  formatMoney: (value: number, currency: string) => string;
  series?: number[];
}) {
  // All-zero series used to draw a flat sparkline against a fabricated 0..1
  // scale — Metric suppresses that and keeps tone honest for zeros.
  const liveSeries =
    series && series.length > 1 && !isAllZeroSeries(series) ? series : undefined;
  return (
    <PremiumPanel className="px-4 py-3" withTexture={false}>
      <Metric
        label={label}
        value={value}
        state="ok"
        format={(n) => formatMoney(n, currency)}
        series={liveSeries}
        size="md"
      />
    </PremiumPanel>
  );
}

function formatPct(
  fraction: number | null | undefined,
  emptyLabel: string,
): string {
  if (fraction == null || !Number.isFinite(fraction)) return emptyLabel;
  return `${Math.round(fraction * 100)}%`;
}

export default function OverviewTab({
  businessId,
  start,
  end,
  locale,
  t,
  tWith,
  fmtMoney,
  activityItems,
  formatOccurredAt,
  onSubTabChange,
}: OverviewTabProps) {

  const range = useMemo(() => ({ start, end }), [start, end]);
  const { data: summary } = useSummary(businessId, range);
  const { data: timeseries } = useTimeseries(businessId, range);
  // Net P/L + estimated COGS come from the P&L statement (single definition with
  // Reports). Summary keeps component totals only — no second derived net.
  const { data: profitLoss } = useProfitLoss(businessId, range, false);
  const pl = profitLoss?.current;

  const reportingCurrency = summary?.currency || pl?.currency || "USD";
  const incomeSeries = useMemo(
    () => (timeseries?.series ?? []).map((p) => p.income),
    [timeseries],
  );
  const expenseSeries = useMemo(
    () => (timeseries?.series ?? []).map((p) => p.expense),
    [timeseries],
  );
  const payrollSeries = useMemo(
    () => (timeseries?.series ?? []).map((p) => p.payroll),
    [timeseries],
  );

  const netPnl = Number(pl?.net) || 0;
  const estimatedCogs = Number(pl?.cogs) || 0;
  const collectionGap = summary?.collection_gap ?? 0;
  const netTone = netPnl < 0 ? "danger" : "accent";

  // Collection-gap deep-link: pending fiscal receipts are the closest
  // Invoices filter to "open / uncollected work" (billed−collected has no
  // 1:1 unpaid-bill UI). Honored via AccountingTab invoicesInitialFilter.
  const collectionGapHref =
    "?tab=accounting&sub=outstanding";
  const analyticsHref = "?tab=analytics";
  const formatCategory = (category: string) =>
    formatCategoryLabel(category, t);

  // Compact Cost health strip — one server response with shared-denominator
  // food/labor/prime + status (never invent prime from food alone; never green 329%).
  // Full cards live on Analytics (CostAnalyticsSection).
  const { data: costHealth } = useCostHealth(businessId, range);
  const { data: dishCoverage } = useDishRecipeCoverage(businessId);
  const foodPct = costHealth?.foodPct ?? null;
  const laborPct = costHealth?.laborPct ?? null;
  const primePct = costHealth?.primePct ?? null;
  const costStatus = costHealth?.status ?? "insufficient_data";
  const costReason = costHealth?.reason;
  const recipeCoveragePct = costHealth?.recipeCoveragePct ?? null;
  const coverage = combineCostHealthCoverage(
    costHealth?.lowCoverage === true,
    dishCoverage,
  );
  const lowCoverage = coverage.lowCoverage;
  const coverageKind = coverage.kind;
  const coveragePctLabel =
    recipeCoveragePct != null && Number.isFinite(recipeCoveragePct)
      ? Math.round(recipeCoveragePct * 100)
      : null;

  const notEnoughData = t("overview.costHealthNotEnoughData");
  const implausibleLabel = t("overview.costHealthImplausible");
  const costEmptyLabel =
    costStatus === "implausible" ? implausibleLabel : notEnoughData;
  // Thin recipe mapping makes a low food% look "healthy" — amber, never emerald.
  const costPctClass =
    costStatus === "implausible"
      ? "text-rose-700"
      : lowCoverage
        ? "text-amber-800"
        : "text-ink-950";

  return (
    <div className="space-y-6">
      <NeedsAttentionStrip
        businessId={businessId}
        start={start}
        end={end}
        t={t}
        tWith={tWith}
        fmtMoney={fmtMoney}
      />

      <p className="text-xs font-medium text-ink-500">
        {tWith("overview.currencyNote", { currency: reportingCurrency })}
      </p>

      {/* 4 KPI tiles — Bill income · Other income · Expenses · Payroll */}
      <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
        <OverviewKpiTile
          label={t("overview.kpi.billIncome")}
          value={summary?.auto_income_total ?? 0}
          currency={reportingCurrency}
          formatMoney={fmtMoney}
          series={incomeSeries}
        />
        <OverviewKpiTile
          label={t("overview.kpi.otherIncome")}
          value={summary?.manual_income_total ?? 0}
          currency={reportingCurrency}
          formatMoney={fmtMoney}
        />
        <OverviewKpiTile
          label={t("overview.kpi.expenses")}
          value={summary?.expense_total ?? 0}
          currency={reportingCurrency}
          formatMoney={fmtMoney}
          series={expenseSeries}
        />
        <OverviewKpiTile
          label={t("overview.kpi.payroll")}
          value={summary?.payroll_total ?? 0}
          currency={reportingCurrency}
          formatMoney={fmtMoney}
          series={payrollSeries}
        />
      </div>

      {/* Hero row — Net P&L + Collection gap */}
      <div className="grid gap-3 md:grid-cols-2">
        <PremiumPanel
          tone={netTone === "danger" ? "urgent" : "accent"}
          className="px-5 py-4"
          withTexture={false}
          data-testid="overview-net-pnl"
          data-tone={netTone}
        >
          <Metric
            label={t("overview.kpi.netProfitLoss")}
            value={netPnl}
            state="ok"
            format={(n) => fmtMoney(n, reportingCurrency)}
            size="lg"
          />
          {lowCoverage ? (
            <p
              className="mt-2 text-[11px] leading-tight text-amber-800"
              data-testid="overview-net-cogs-caveat"
              data-coverage-kind={coverageKind}
            >
              {coverageKind === "dish" && dishCoverage
                ? tWith("overview.kpi.netCogsDishCaveat", {
                    mapped: dishCoverage.mapped,
                    total: dishCoverage.total,
                  })
                : coveragePctLabel != null
                  ? tWith("overview.kpi.netCogsCaveat", {
                      coverage: coveragePctLabel,
                    })
                  : t("overview.kpi.netCogsCaveatUnknown")}
            </p>
          ) : (
            <p className="mt-2 text-[11px] leading-tight text-ink-500">
              {t("overview.kpi.netIncludesEstimatedCogs")}
            </p>
          )}
        </PremiumPanel>

        <PremiumPanel
          as="a"
          href={collectionGapHref}
          tone="accent"
          interactive
          className="px-5 py-4"
          withTexture={false}
          data-testid="overview-collection-gap"
        >
          <Metric
            label={t("overview.kpi.collectionGap")}
            value={collectionGap}
            state="ok"
            format={(n) => fmtMoney(n, reportingCurrency)}
            size="lg"
          />
          {/* Gap is range-created leftover remainings; Outstanding is as-of end (#222 / #770). */}
          <p className="mt-1 text-[11px] leading-tight text-ink-500">
            {t("overview.kpi.collectionGapHint")}
          </p>
          <p
            className="mt-0.5 text-[11px] leading-tight text-ink-500"
            data-testid="overview-collection-gap-scope"
          >
            {tWith("overview.kpi.collectionGapScope", {
              start: formatDay(start, locale),
              end: formatDay(end, locale),
            })}
          </p>
        </PremiumPanel>
      </div>

      {/* Compact Cost health line → Analytics */}
      <PremiumPanel
        as="a"
        href={analyticsHref}
        interactive
        className="px-5 py-4"
        withTexture={false}
        data-testid="overview-cost-health"
      >
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
              {t("overview.costHealth")}
            </p>
            <div
              className="mt-2 flex flex-wrap items-baseline gap-x-4 gap-y-1 text-sm text-ink-800"
              data-cost-status={costStatus}
              data-cost-reason={costReason ?? undefined}
              data-low-coverage={lowCoverage ? "true" : "false"}
              data-coverage-kind={coverageKind}
              data-dish-mapped={dishCoverage?.mapped ?? undefined}
              data-dish-total={dishCoverage?.total ?? undefined}
            >
              <span>
                <span className="text-ink-500">{t("foodCost.cardLabel")}</span>{" "}
                <span
                  className={`font-semibold tabular-nums ${costPctClass}`}
                  data-testid="cost-health-food"
                >
                  {formatPct(foodPct, costEmptyLabel)}
                </span>
              </span>
              <span>
                <span className="text-ink-500">{t("laborCost.laborPctLabel")}</span>{" "}
                <span
                  className={`font-semibold tabular-nums ${costPctClass}`}
                  data-testid="cost-health-labor"
                >
                  {formatPct(laborPct, costEmptyLabel)}
                </span>
              </span>
              <span>
                <span className="text-ink-500">{t("laborCost.primeCostLabel")}</span>{" "}
                <span
                  className={`font-semibold tabular-nums ${
                    costStatus === "implausible" ? "text-rose-700" : costPctClass
                  }`}
                  data-testid="cost-health-prime"
                >
                  {formatPct(primePct, costEmptyLabel)}
                </span>
              </span>
            </div>
            {coverageKind === "dish" && dishCoverage ? (
              <p
                className="mt-2 text-[11px] leading-tight text-amber-800"
                data-testid="cost-health-coverage-caveat"
                data-coverage-kind="dish"
              >
                {tWith("overview.costHealthDishCoverageCaveat", {
                  mapped: dishCoverage.mapped,
                  total: dishCoverage.total,
                })}
              </p>
            ) : coverageKind === "sales" && coveragePctLabel != null ? (
              <p
                className="mt-2 text-[11px] leading-tight text-amber-800"
                data-testid="cost-health-coverage-caveat"
                data-coverage-kind="sales"
              >
                {tWith("overview.costHealthCoverageCaveat", {
                  coverage: coveragePctLabel,
                })}
              </p>
            ) : null}
          </div>
          <span className="shrink-0 text-sm font-medium text-brand-700">
            {t("overview.viewInAnalytics")}
          </span>
        </div>
      </PremiumPanel>

      {/* Income / expense category breakdowns */}
      <div className="grid gap-3 lg:grid-cols-2">
        <CategoryBreakdown
          title={t("overview.breakdown.incomeTitle")}
          items={summary?.income_breakdown ?? []}
          currency={reportingCurrency}
          formatMoney={fmtMoney}
          formatCategory={formatCategory}
          emptyLabel={t("overview.breakdown.empty")}
          viewAllLabel={t("overview.viewAll")}
          onViewAll={
            onSubTabChange ? () => onSubTabChange("entries") : undefined
          }
        />
        <CategoryBreakdown
          title={t("overview.breakdown.expenseTitle")}
          items={summary?.expense_breakdown ?? []}
          currency={reportingCurrency}
          formatMoney={fmtMoney}
          formatCategory={formatCategory}
          emptyLabel={t("overview.breakdown.empty")}
          viewAllLabel={t("overview.viewAll")}
          onViewAll={
            onSubTabChange ? () => onSubTabChange("entries") : undefined
          }
        />
      </div>

      {/* P&L composition strip — same five inputs Reports sums into Net */}
      <PremiumPanel className="p-5" withTexture={false} data-testid="overview-pnl-composition">
        <h3 className="mb-4 text-lg font-semibold text-ink-950">
          {t("overview.reconciliation.title")}
        </h3>
        <p className="mb-3 text-xs text-ink-500">
          {t("overview.reconciliation.pnlHint")}
        </p>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
          <div className="rounded-2xl border border-warm-200/80 bg-warm-50/70 p-4">
            <p className="text-xs font-semibold uppercase tracking-[0.16em] text-ink-500">
              {t("overview.reconciliation.revenue")}
            </p>
            <p
              className="mt-2 text-xl font-semibold tabular-nums text-ink-950"
              data-testid="overview-pnl-revenue"
            >
              {fmtMoney(
                Number(pl?.revenue) || summary?.auto_income_total || 0,
                reportingCurrency,
              )}
            </p>
          </div>
          <div className="rounded-2xl border border-warm-200/80 bg-warm-50/70 p-4">
            <p className="text-xs font-semibold uppercase tracking-[0.16em] text-ink-500">
              {t("overview.reconciliation.otherIncome")}
            </p>
            <p
              className="mt-2 text-xl font-semibold tabular-nums text-ink-950"
              data-testid="overview-pnl-other-income"
            >
              {fmtMoney(
                Number(pl?.other_income) || summary?.manual_income_total || 0,
                reportingCurrency,
              )}
            </p>
          </div>
          <div className="rounded-2xl border border-warm-200/80 bg-warm-50/70 p-4">
            <p className="text-xs font-semibold uppercase tracking-[0.16em] text-ink-500">
              {t("overview.reconciliation.cogs")}
            </p>
            <p
              className="mt-2 text-xl font-semibold tabular-nums text-ink-950"
              data-testid="overview-pnl-cogs"
            >
              {fmtMoney(estimatedCogs, reportingCurrency)}
            </p>
            <p
              className={`mt-1 text-[11px] leading-tight ${
                lowCoverage ? "text-amber-800" : "text-ink-500"
              }`}
              data-testid="overview-pnl-cogs-hint"
            >
              {coverageKind === "dish" && dishCoverage
                ? tWith("overview.reconciliation.cogsDishHint", {
                    mapped: dishCoverage.mapped,
                    total: dishCoverage.total,
                  })
                : lowCoverage && coveragePctLabel != null
                  ? tWith("overview.reconciliation.cogsCoverageHint", {
                      coverage: coveragePctLabel,
                    })
                  : t("overview.reconciliation.cogsHint")}
            </p>
          </div>
          <div className="rounded-2xl border border-warm-200/80 bg-warm-50/70 p-4">
            <p className="text-xs font-semibold uppercase tracking-[0.16em] text-ink-500">
              {t("overview.reconciliation.labor")}
            </p>
            <p
              className="mt-2 text-xl font-semibold tabular-nums text-ink-950"
              data-testid="overview-pnl-labor"
            >
              {fmtMoney(
                Number(pl?.labor) || summary?.payroll_total || 0,
                reportingCurrency,
              )}
            </p>
          </div>
          <div className="rounded-2xl border border-warm-200/80 bg-warm-50/70 p-4">
            <p className="text-xs font-semibold uppercase tracking-[0.16em] text-ink-500">
              {t("overview.reconciliation.opex")}
            </p>
            <p
              className="mt-2 text-xl font-semibold tabular-nums text-ink-950"
              data-testid="overview-pnl-opex"
            >
              {fmtMoney(
                Number(pl?.opex) || summary?.expense_total || 0,
                reportingCurrency,
              )}
            </p>
          </div>
          <div className="rounded-2xl border border-warm-200/80 bg-warm-50/70 p-4">
            <p className="text-xs font-semibold uppercase tracking-[0.16em] text-ink-500">
              {t("overview.reconciliation.billed")}
            </p>
            <p className="mt-2 text-xl font-semibold tabular-nums text-ink-950">
              {fmtMoney(summary?.billed_total || 0, reportingCurrency)}
            </p>
            <p className="mt-1 text-[11px] text-ink-500">
              {t("overview.reconciliation.collected")}:{" "}
              {fmtMoney(summary?.collected_total || 0, reportingCurrency)}
            </p>
            <p className="mt-0.5 text-[11px] text-ink-500">
              {t("overview.reconciliation.paidRuns")}:{" "}
              {summary?.payroll_summary?.paid_runs || 0}
            </p>
          </div>
        </div>
      </PremiumPanel>

      {/* Recent activity — full-width (rail removed) */}
      <RecentActivity
        items={activityItems}
        emptyLabel={t("activity.empty")}
        title={t("activity.title")}
        formatOccurredAt={formatOccurredAt}
      />
    </div>
  );
}
