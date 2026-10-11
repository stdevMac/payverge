// src/components/dashboard/charts/AdminBarChart.tsx
import React from "react";
import {
  Chart as ChartJS, CategoryScale, LinearScale, BarElement, Tooltip,
} from "chart.js";
import type { ChartOptions } from "chart.js";
import { Bar } from "react-chartjs-2";
import { ANALYTICS_INK_300, chartBaseOptions } from "./analyticsTheme";
import type { MonthlyGrowth } from "@/api/admin";
import { dedupePointsByLabel, isAllZeroSeries } from "./chartEmpty";

ChartJS.register(CategoryScale, LinearScale, BarElement, Tooltip);

interface AdminBarChartProps {
  points: MonthlyGrowth[];
  ariaLabel: string;
  barColor: string;
  height?: number;
  /** Force integer ticks (e.g. business counts can't be fractional). */
  integerTicks?: boolean;
  /** Localized empty-state caption when there is nothing to chart. */
  emptyLabel?: string;
}

// Growth / churn bars keyed on MonthlyGrowth.count, rounded bars, faint y-grid.
// barColor is passed in because churn uses a danger red with no theme constant. (P-3)
// Empty / all-zero series → empty state (no blank panel with a fake 0..1 axis).
// Labels are deduped so the x-axis never double-prints the same month string.
export function AdminBarChart({
  points,
  ariaLabel,
  barColor,
  height = 240,
  integerTicks = false,
  emptyLabel,
}: AdminBarChartProps) {
  const deduped = dedupePointsByLabel(points ?? []);
  const counts = deduped.map((p) => p.count);

  if (deduped.length === 0 || isAllZeroSeries(counts)) {
    return (
      <p
        role="status"
        aria-label={ariaLabel}
        data-testid="chart-empty"
        className="flex h-full min-h-[160px] items-center justify-center text-sm text-ink-500"
      >
        {emptyLabel ?? "—"}
      </p>
    );
  }

  const data = {
    labels: deduped.map((p) => p.month),
    datasets: [{ data: counts, backgroundColor: barColor, borderRadius: 4 }],
  };
  const options = {
    ...chartBaseOptions,
    scales: {
      x: { grid: { display: false }, ticks: { font: { size: 12 } } },
      y: {
        beginAtZero: true,
        ...(integerTicks
          ? { ticks: { font: { size: 12 }, precision: 0, stepSize: 1 } }
          : { ticks: { font: { size: 12 } } }),
        grid: { color: ANALYTICS_INK_300, drawTicks: false },
        border: { display: false },
      },
    },
  };
  return (
    <div style={{ height }} role="img" aria-label={ariaLabel}>
      <Bar data={data} options={options as ChartOptions<"bar">} />
    </div>
  );
}
