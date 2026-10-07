/**
 * Metric — money / percent / count primitive with honest zero/empty/implausible states.
 *
 * Findings 14, 15, 18: a metric must know "zero" vs "nothing". Success-green is
 * never applied to zero or empty; insufficient shows a reason, not a number;
 * all-zero series never fabricate a chart axis.
 *
 * Tone is **derived** from `state` and the sign of `value`. Callers cannot pass
 * `tone="success"` with `value={0}` — `tone` is not an input prop (TypeScript
 * impossibility via omission from the public props union).
 */

import React from "react";
import { Sparkline } from "@/components/dashboard/charts/Sparkline";
import { isAllZeroSeries as chartIsAllZero } from "@/components/dashboard/charts/chartEmpty";

export type MetricState = "ok" | "empty" | "insufficient" | "implausible";

/** Derived presentation tone — never accepted as an unconstrained input. */
export type MetricTone = "success" | "danger" | "warn" | "neutral" | "accent";

type MetricFormat =
  | "money"
  | "percent"
  | "count"
  | ((n: number) => string);

type MetricBase = {
  format?: MetricFormat;
  series?: Array<number | null | undefined>;
  /** Localized caption when series is empty / all-zero. */
  seriesEmptyLabel?: string;
  className?: string;
  size?: "sm" | "md" | "lg";
  label?: string;
  currency?: string;
  locale?: string;
  "data-testid"?: string;
};

/**
 * Discriminated union on `state`. `tone` is intentionally absent — derived only.
 * That makes `tone="success"` + `value={0}` a compile-time impossibility.
 */
export type MetricProps =
  | (MetricBase & {
      state: "empty";
      value?: number | null;
      reason?: string;
    })
  | (MetricBase & {
      state: "insufficient";
      reason: string;
      value?: number | null;
    })
  | (MetricBase & {
      state: "implausible";
      reason?: string;
      value?: number | null;
    })
  | (MetricBase & {
      state?: "ok";
      value: number;
      reason?: undefined;
    });

const TONE_CLASS: Record<MetricTone, string> = {
  success: "text-emerald-700",
  danger: "text-rose-700",
  warn: "text-amber-700",
  neutral: "text-ink-500",
  accent: "text-ink-900",
};

const SIZE_CLASS = {
  sm: "text-base font-semibold tabular-nums",
  md: "text-xl font-semibold tabular-nums",
  lg: "text-2xl font-semibold tabular-nums",
} as const;

/** True when there is no series, or every point is null/undefined/0. */
export function isAllZeroSeries(
  series: Array<number | null | undefined> | undefined | null,
): boolean {
  return chartIsAllZero(series);
}

/**
 * Tone is derived from state + sign. Zero and empty are never success.
 * Negative money/count → danger (matches OverviewTab net P&L).
 */
export function deriveMetricTone(
  state: MetricState,
  value: number | null | undefined,
): MetricTone {
  if (state === "empty" || state === "insufficient") return "neutral";
  if (state === "implausible") return "danger";
  // state === "ok"
  if (value == null || !Number.isFinite(value)) return "neutral";
  if (value < 0) return "danger";
  if (value === 0) return "neutral";
  return "accent";
}

function formatValue(
  value: number,
  format: MetricFormat | undefined,
  currency: string | undefined,
  locale: string | undefined,
): string {
  if (typeof format === "function") return format(value);
  switch (format) {
    case "percent":
      return `${Math.round(value * 100)}%`;
    case "count":
      return new Intl.NumberFormat(locale ?? "en").format(value);
    case "money":
    default: {
      try {
        return new Intl.NumberFormat(locale ?? "en", {
          style: "currency",
          currency: currency ?? "USD",
        }).format(value);
      } catch {
        return `$${value.toFixed(2)}`;
      }
    }
  }
}

export function Metric(props: MetricProps) {
  const {
    format,
    series,
    seriesEmptyLabel,
    className = "",
    size = "md",
    label,
    currency,
    locale,
    "data-testid": dataTestId,
  } = props;

  const state: MetricState = props.state ?? "ok";
  const value = "value" in props ? props.value : null;
  const reason = "reason" in props ? props.reason : undefined;

  const tone = deriveMetricTone(state, value);
  const toneClass = TONE_CLASS[tone];
  const sizeClass = SIZE_CLASS[size];

  let display: string;
  if (state === "empty") {
    display = reason?.trim() || "—";
  } else if (state === "insufficient") {
    // Never invent a percentage/money figure when the denominator is missing.
    display = reason?.trim() || "—";
  } else if (state === "implausible") {
    // Surface the impossible ratio (never clamp to a green band). Prefer the
    // number when format+value are available; fall back to the reason string.
    if (
      typeof value === "number" &&
      Number.isFinite(value) &&
      format != null
    ) {
      display = formatValue(value, format, currency, locale);
    } else {
      display = reason?.trim() || "—";
    }
  } else {
    // ok
    const n = typeof value === "number" && Number.isFinite(value) ? value : 0;
    display = formatValue(n, format, currency, locale);
  }

  const showSeriesEmpty =
    series != null && isAllZeroSeries(series) && Boolean(seriesEmptyLabel);
  const showSeries =
    series != null && !isAllZeroSeries(series) && series.length > 1;

  const numericSeries = showSeries
    ? (series as Array<number | null | undefined>).map((v) =>
        typeof v === "number" && Number.isFinite(v) ? v : 0,
      )
    : [];

  return (
    <div className={className}>
      {label ? (
        <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
          {label}
        </p>
      ) : null}
      <p
        data-testid={dataTestId}
        data-tone={tone}
        data-state={state}
        className={`${label ? "mt-1" : ""} ${sizeClass} ${toneClass}`.trim()}
      >
        {display}
      </p>
      {showSeries ? (
        <div data-testid="metric-series" className="mt-2">
          <Sparkline
            data={numericSeries}
            ariaLabel={label ? `${label} trend` : "trend"}
          />
        </div>
      ) : null}
      {showSeriesEmpty ? (
        <p
          data-testid="metric-series-empty"
          role="status"
          className="mt-2 text-xs text-ink-500"
        >
          {seriesEmptyLabel}
        </p>
      ) : null}
    </div>
  );
}
