// src/components/dashboard/charts/AdminLineChart.tsx
import React from "react";
import {
  Chart as ChartJS, CategoryScale, LinearScale, PointElement, LineElement, Tooltip,
} from "chart.js";
import type { ChartOptions } from "chart.js";
import { Line } from "react-chartjs-2";
import { ANALYTICS_TEAL, ANALYTICS_INK_300, chartBaseOptions } from "./analyticsTheme";
import type { MonthlyGrowth } from "@/api/admin";
import { dedupePointsByLabel, isAllZeroSeries } from "./chartEmpty";

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip);

interface AdminLineChartProps {
  points: MonthlyGrowth[];
  ariaLabel: string;
  formatValue?: (n: number) => string;
  height?: number;
  /** Localized empty-state caption when there is nothing to chart. */
  emptyLabel?: string;
}

// Revenue-over-time line: teal series keyed on MonthlyGrowth.value, zero
// baseline, faint y-grid, no points until hover — mirrors the dashboard
// TrendChart but reads the admin {month,value} shape directly. (P-3)
// All-zero / empty series → empty state (no fabricated $1 axis). Labels are
// deduped so two months that stringify the same do not double-tick the x-axis.
export function AdminLineChart({
  points,
  ariaLabel,
  formatValue = (n) => String(n),
  height = 240,
  emptyLabel,
}: AdminLineChartProps) {
  const deduped = dedupePointsByLabel(points ?? []);
  const values = deduped.map((p) => p.value ?? 0);

  if (deduped.length === 0 || isAllZeroSeries(values)) {
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
    datasets: [
      {
        data: values,
        borderColor: ANALYTICS_TEAL,
        backgroundColor: "transparent",
        borderWidth: 2,
        fill: false,
        tension: 0,
      },
    ],
  };
  const options = {
    ...chartBaseOptions,
    plugins: {
      ...chartBaseOptions.plugins,
      tooltip: {
        ...chartBaseOptions.plugins?.tooltip,
        // Restore currency formatting lost in the recharts→chart.js port (MONEY-1).
        // formatValue is numerically identity-preserving: adds `$`/grouping only,
        // never re-divides by 100.
        callbacks: {
          label: (ctx: { parsed: { y: number } }) => formatValue(Number(ctx.parsed.y)),
        },
      },
    },
    elements: { point: { radius: 0, hoverRadius: 4 } },
    scales: {
      x: { grid: { display: false }, ticks: { font: { size: 12 } } },
      y: {
        beginAtZero: true,
        grid: { color: ANALYTICS_INK_300, drawTicks: false },
        border: { display: false },
        ticks: { font: { size: 12 }, callback: (v: number | string) => formatValue(Number(v)) },
      },
    },
  };
  return (
    <div style={{ height }} role="img" aria-label={ariaLabel}>
      <Line data={data} options={options as ChartOptions<"line">} />
    </div>
  );
}
