"use client";

import React from "react";
import type { LaborCostReport } from "@/api/laborCost";
import { Metric } from "@/components/ui/Metric";
import { PremiumPanel } from "../premium";
import PeriodTabs from "../shared/PeriodTabs";

export type LaborCostPeriod = "week" | "month";

const PERIOD_OPTIONS: readonly LaborCostPeriod[] = ["week", "month"];

function shownPct(pct: number): number {
  return Math.round(pct * 100);
}

// Labor %: <=30% healthy (emerald), <=35% caution (amber), else high (rose).
// Band on the displayed (rounded) percent so the color never disagrees with the
// label — e.g. 0.304 shows "30%" and stays emerald, not amber.
function laborPctClass(pct: number): string {
  const shown = shownPct(pct);
  if (shown <= 30) return "text-emerald-600";
  if (shown <= 35) return "text-amber-600";
  return "text-rose-600";
}

// Prime cost %: <=60% healthy (emerald), <=65% caution (amber), else high (rose).
function primePctClass(pct: number): string {
  const shown = shownPct(pct);
  if (shown <= 60) return "text-emerald-600";
  if (shown <= 65) return "text-amber-600";
  return "text-rose-600";
}

function formatPct(pct: number): string {
  return `${shownPct(pct)}%`;
}

export interface LaborCostCardProps {
  report: LaborCostReport | null;
  loading?: boolean;
  error?: boolean;
  onRetry?: () => void;
  currency: string;
  selectedPeriod: LaborCostPeriod;
  onPeriodChange: (period: LaborCostPeriod) => void;
  formatMoney: (value: number, currency: string) => string;
  labels: {
    title: string;
    subtitle: string;
    periodSelectorLabel: string;
    periods: Record<LaborCostPeriod, string>;
    laborPct: string;
    primeCost: string;
    netSales: string;
    provenance: (count: number) => string;
    target: string;
    states: {
      empty: string;
      noSales: string;
      loading: string;
      error?: string;
      retry?: string;
      /** Shown when status=implausible (ratio > 1.0) — never a green band. */
      implausible?: string;
    };
  };
}

export default function LaborCostCard({
  report,
  loading = false,
  error = false,
  onRetry,
  currency,
  selectedPeriod,
  onPeriodChange,
  formatMoney,
  labels,
}: LaborCostCardProps) {
  const header = (
    <div className="flex flex-wrap items-start justify-between gap-2">
      <div className="min-w-0">
        <p className="text-sm font-semibold text-ink-900">{labels.title}</p>
        <p className="text-[11px] text-ink-500">{labels.subtitle}</p>
      </div>
      <PeriodTabs
        ariaLabel={labels.periodSelectorLabel}
        value={selectedPeriod}
        onChange={onPeriodChange}
        options={PERIOD_OPTIONS.map((p) => ({
          key: p,
          label: labels.periods[p],
        }))}
      />
    </div>
  );

  // A swallowed labor-cost fetch used to make this card silently vanish
  // (`if (!report) return null`). Surface the failure with an inline retry
  // so the operator knows the number is missing, not zero. See R3-AC.
  if (error) {
    return (
      <PremiumPanel className="p-5" withTexture={false}>
        {header}
        <div className="mt-4 flex flex-col items-center justify-center gap-3 rounded-2xl border border-rose-200 bg-rose-50 px-4 py-6 text-center">
          <p className="text-sm text-rose-700">
            {labels.states.error ?? labels.states.empty}
          </p>
          {onRetry && labels.states.retry ? (
            <button
              type="button"
              onClick={onRetry}
              className="inline-flex items-center rounded-lg border border-rose-300 bg-white px-3 py-1.5 text-sm font-medium text-rose-700 transition-colors hover:bg-rose-100"
            >
              {labels.states.retry}
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
          aria-label={labels.states.loading}
          className="mt-4 h-24 animate-pulse rounded-2xl bg-warm-100"
        />
      </PremiumPanel>
    );
  }

  if (!report) return null;

  // has_data === false → no payroll runs in the window. Show the empty state,
  // never a misleading "0%".
  if (!report.has_data) {
    return (
      <PremiumPanel className="p-5" withTexture={false}>
        {header}
        <div className="mt-4 flex h-24 items-center justify-center rounded-2xl border border-warm-200 bg-warm-50 text-sm text-ink-500">
          {labels.states.empty}
        </div>
      </PremiumPanel>
    );
  }

  // Shared-denominator cost-health status from the server. Implausible ratios
  // (e.g. labor > revenue) must never render as healthy green percentages.
  const status = report.status;
  const isImplausible = status === "implausible";
  const isInsufficient =
    status === "insufficient_data" || (report.net_sales ?? 0) <= 0;

  // Runs exist but no sales → percentages are undefined; show "—" for them but
  // still surface the labor dollars.
  const hasSales = !isInsufficient && report.net_sales > 0;
  const primePct = report.prime_cost_pct;

  return (
    <PremiumPanel className="p-5" withTexture={false} data-cost-status={status}>
      {header}

      {/* Labor % headline — Metric owns empty/insufficient/implausible; band
          colors apply only when status is ok with real sales. */}
      <div className="mt-4">
        {isImplausible ? (
          <Metric
            label={labels.laborPct}
            state="implausible"
            value={report.labor_cost_pct}
            reason={labels.states.implausible ?? labels.states.noSales}
            format={(n) => formatPct(n)}
            size="lg"
            data-testid="labor-pct"
          />
        ) : !hasSales ? (
          <Metric
            label={labels.laborPct}
            state="insufficient"
            reason="—"
            size="lg"
            data-testid="labor-pct"
          />
        ) : (
          <>
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
              {labels.laborPct}
            </p>
            <p
              data-testid="labor-pct"
              className={`text-2xl font-semibold tabular-nums ${laborPctClass(report.labor_cost_pct)}`}
            >
              {formatPct(report.labor_cost_pct)}
            </p>
          </>
        )}
        <p className="mt-1 text-sm text-ink-600">
          {formatMoney(report.labor_cost, currency)}
          <span className="text-ink-400"> · </span>
          <span className="text-ink-500">
            {labels.netSales}: {formatMoney(report.net_sales, currency)}
          </span>
        </p>
      </div>

      {isImplausible ? (
        <div
          className="mt-3 rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-[13px] text-rose-800"
          data-testid="labor-implausible"
        >
          {labels.states.implausible ?? labels.states.noSales}
        </div>
      ) : null}

      {!hasSales && !isImplausible ? (
        <div className="mt-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-[13px] text-amber-800">
          {labels.states.noSales}
        </div>
      ) : null}

      {/* Prime cost sub-line (food + labor) — omitted when implausible */}
      <div className="mt-4 flex items-center justify-between rounded-2xl border border-warm-200 bg-warm-50 px-3 py-2">
        <span className="text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
          {labels.primeCost}
        </span>
        {hasSales && primePct != null && !isImplausible ? (
          <span
            data-testid="prime-pct"
            className={`text-base font-semibold tabular-nums ${primePctClass(primePct)}`}
          >
            {formatPct(primePct)}
          </span>
        ) : (
          <Metric
            state="insufficient"
            reason="—"
            size="sm"
            data-testid="prime-pct"
          />
        )}
      </div>

      <p className="mt-3 text-[11px] text-ink-500">
        {labels.provenance(report.payroll_run_count)}
      </p>
      <p className="text-[11px] text-ink-500">{labels.target}</p>
    </PremiumPanel>
  );
}
