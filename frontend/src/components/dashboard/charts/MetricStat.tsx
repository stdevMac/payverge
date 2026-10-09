// src/components/dashboard/charts/MetricStat.tsx
import React from "react";
import { Sparkline } from "./Sparkline";
import { DELTA_UP, DELTA_DOWN } from "./analyticsTheme";

interface MetricStatProps {
  label: string;
  value: string | number;
  period?: string;
  delta?: { percent: number; comparedToLabel?: string };
  sparklineData?: number[];
  ariaLabel?: string;
  /** Qualifier line under the value (e.g. a tip-rate grade). */
  subtext?: string;
  /**
   * Neutral note shown in place of the delta when there is no delta — e.g.
   * "No sales yet today" instead of a misleading "▼100.0% vs typical day" on a
   * zero-value tile early in the day.
   */
  neutralNote?: string;
}

// Headline number with a "compared to what?" delta. Color is never the sole
// signal — the ▲/▼ glyph + sign carries direction for accessibility.
export function MetricStat({ label, value, period, delta, sparklineData, ariaLabel, subtext, neutralNote }: MetricStatProps) {
  const hasDelta = delta != null && Number.isFinite(delta.percent);
  const isNeutral = hasDelta && delta!.percent === 0;
  const up = hasDelta && !isNeutral && delta!.percent > 0;
  return (
    <div className="rounded-2xl border border-warm-200 bg-white p-4">
      <div className="flex items-start justify-between gap-2">
        <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">{label}</p>
        {period && (
          <span className="rounded-full bg-warm-100 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-ink-500">
            {period}
          </span>
        )}
      </div>
      <p className="mt-1 text-2xl font-semibold tracking-tight text-ink-900 tabular-nums">{value}</p>
      {subtext && (
        <p data-testid="metric-subtext" className="mt-0.5 text-xs text-ink-500">
          {subtext}
        </p>
      )}
      <div className="mt-2 flex items-center justify-between gap-2">
        {hasDelta ? (
          isNeutral ? (
            <span
              data-testid="metric-delta"
              className="inline-flex items-center gap-1 text-xs font-medium tabular-nums text-ink-500"
            >
              —
              {delta!.comparedToLabel && (
                <span className="font-medium text-ink-500">{delta!.comparedToLabel}</span>
              )}
            </span>
          ) : (
          <span
            data-testid="metric-delta"
            className="inline-flex items-center gap-1 text-xs font-bold tabular-nums"
            style={{ color: up ? DELTA_UP : DELTA_DOWN }}
          >
            {up ? "▲" : "▼"} {Math.abs(delta!.percent).toFixed(1)}%
            {delta!.comparedToLabel && (
              <span className="font-medium text-ink-500">{delta!.comparedToLabel}</span>
            )}
          </span>
          )
        ) : neutralNote ? (
          <span data-testid="metric-neutral-note" className="text-xs font-medium text-ink-500">
            {neutralNote}
          </span>
        ) : (
          <span />
        )}
        {sparklineData && sparklineData.length > 1 && (
          <Sparkline data={sparklineData} ariaLabel={ariaLabel ?? `${label} trend`} />
        )}
      </div>
    </div>
  );
}
