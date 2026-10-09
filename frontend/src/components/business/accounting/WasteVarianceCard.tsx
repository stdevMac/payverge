"use client";

import React from "react";
import type { WasteVarianceReport, WasteReason } from "@/api/wasteVariance";
import { PremiumPanel } from "../premium";
import PeriodTabs from "../shared/PeriodTabs";

export type WasteVariancePeriod = "day" | "week" | "month";

const PERIOD_OPTIONS: readonly WasteVariancePeriod[] = ["day", "week", "month"];

function varianceClass(variance: number): string {
  // Color what the operator actually reads: the 2-decimal display value.
  // A raw variance of 0.0004 renders "0.00" and must not paint an alarm.
  const displayed = Number(variance.toFixed(2));
  if (displayed > 0) return "text-rose-600";
  if (displayed < 0) return "text-emerald-600";
  return "text-ink-700";
}

export interface WasteVarianceCardProps {
  report: WasteVarianceReport | null;
  loading?: boolean;
  error?: boolean;
  onRetry?: () => void;
  currency: string;
  selectedPeriod: WasteVariancePeriod;
  onPeriodChange: (period: WasteVariancePeriod) => void;
  formatMoney: (value: number, currency: string) => string;
  labels: {
    title: string;
    subtitle: string;
    periodSelectorLabel: string;
    periods: Record<WasteVariancePeriod, string>;
    trackedLoss: string;
    reasons: Record<WasteReason, string>;
    /** Optional label for ingredients that were force-included despite being
     *  inactive/discontinued, so operators can tell them apart. See R3-AC-9. */
    inactive?: string;
    table: {
      ingredient: string;
      theoretical: string;
      actual: string;
      variance: string;
      varianceCost: string;
    };
    states: {
      empty: string;
      sparse: string;
      needsRecipe: string;
      loading: string;
      error?: string;
      retry?: string;
    };
  };
}

export default function WasteVarianceCard({
  report,
  loading = false,
  error = false,
  onRetry,
  currency,
  selectedPeriod,
  onPeriodChange,
  formatMoney,
  labels,
}: WasteVarianceCardProps) {
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

  // A swallowed waste-variance fetch used to make this card silently vanish
  // (`if (!report) return null`). Surface the failure with an inline retry.
  // See R3-AC.
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

  return (
    <PremiumPanel className="p-5" withTexture={false}>
      {header}

      {/* Tracked-loss headline */}
      <div className="mt-4">
        <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
          {labels.trackedLoss}
        </p>
        <p
          className={`text-xl font-semibold ${
            // Zero loss is a good reading — only a real loss is an alarm.
            Number(report.tracked_loss_cost.toFixed(2)) > 0
              ? "text-rose-600"
              : "text-ink-950"
          }`}
        >
          {formatMoney(report.tracked_loss_cost, currency)}
        </p>
        {report.loss_by_reason.length > 0 ? (
          <dl className="mt-2 space-y-1">
            {report.loss_by_reason.map(({ reason, cost }) => (
              <div
                key={reason}
                className="flex items-center justify-between text-sm"
              >
                <dt className="text-ink-600">{labels.reasons[reason]}</dt>
                <dd className="font-medium text-rose-600">
                  {formatMoney(cost, currency)}
                </dd>
              </div>
            ))}
          </dl>
        ) : null}
      </div>

      {/* Callouts */}
      {report.items_without_recipe > 0 ? (
        <div className="mt-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-[13px] text-amber-800">
          {labels.states.needsRecipe}
        </div>
      ) : null}

      {report.sparse ? (
        <div className="mt-3 rounded-xl border border-warm-200 bg-warm-50 px-3 py-2 text-[13px] text-ink-600">
          {labels.states.sparse}
        </div>
      ) : null}

      {/* Ingredient variance table */}
      {report.ingredients.length === 0 ? (
        <div className="mt-4 flex h-24 items-center justify-center rounded-2xl border border-warm-200 bg-warm-50 text-sm text-ink-500">
          {labels.states.empty}
        </div>
      ) : (
        <div className="mt-4 overflow-x-auto rounded-2xl border border-warm-200 bg-white">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-t border-warm-100 text-left text-[11px] uppercase tracking-[0.16em] text-ink-500">
                <th className="px-4 py-2">{labels.table.ingredient}</th>
                <th className="px-4 py-2 text-right">{labels.table.theoretical}</th>
                <th className="px-4 py-2 text-right">{labels.table.actual}</th>
                <th className="px-4 py-2 text-right">{labels.table.variance}</th>
                <th className="px-4 py-2 text-right">{labels.table.varianceCost}</th>
              </tr>
            </thead>
            <tbody>
              {report.ingredients.map((ing) => (
                <tr
                  key={ing.inventory_item_id}
                  data-testid={`wv-row-${ing.inventory_item_id}`}
                  className="border-t border-warm-100"
                >
                  <td className="px-4 py-2 font-medium text-ink-950">
                    <span className="inline-flex items-center gap-1.5">
                      {ing.name}
                      {ing.is_active === false && labels.inactive ? (
                        <span className="rounded-full border border-warm-300 bg-warm-100 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-ink-500">
                          {labels.inactive}
                        </span>
                      ) : null}
                    </span>
                  </td>
                  <td className="px-4 py-2 text-right text-ink-700">
                    {ing.has_recipe
                      ? `${ing.theoretical_usage.toFixed(2)} ${ing.unit}`
                      : "—"}
                  </td>
                  <td className="px-4 py-2 text-right text-ink-700">
                    {`${ing.actual_usage.toFixed(2)} ${ing.unit}`}
                  </td>
                  <td
                    className={`px-4 py-2 text-right font-medium ${
                      ing.has_recipe ? varianceClass(ing.variance) : "text-ink-700"
                    }`}
                  >
                    {ing.has_recipe
                      ? `${ing.variance.toFixed(2)} ${ing.unit}`
                      : "—"}
                  </td>
                  <td
                    className={`px-4 py-2 text-right font-medium ${
                      ing.has_recipe ? varianceClass(ing.variance) : "text-ink-700"
                    }`}
                  >
                    {ing.has_recipe
                      ? formatMoney(ing.variance_cost, currency)
                      : "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </PremiumPanel>
  );
}
