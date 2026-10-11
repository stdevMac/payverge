// src/components/dashboard/charts/HourOfDayChart.tsx
import React from "react";
import { Chart as ChartJS, CategoryScale, LinearScale, BarElement, Tooltip } from "chart.js";
import { Bar } from "react-chartjs-2";
import { ANALYTICS_BAR_HUE, chartBaseOptions, ANALYTICS_INK_300 } from "./analyticsTheme";
import { isAllZeroSeries } from "./chartEmpty";

ChartJS.register(CategoryScale, LinearScale, BarElement, Tooltip);

interface HourOfDayChartProps {
  hours: { hour: number; value: number }[];
  formatValue?: (n: number) => string;
  height?: number;
  ariaLabel: string;
  /**
   * Optional, already-localized empty-state caption. When omitted we render a
   * locale-neutral em-dash placeholder instead of the previous untranslated
   * "{ariaLabel}: no data" English string.
   */
  emptyLabel?: string;
}

// Compact hour-of-day bars, replacing the low-density per-hour card grids.
// All-zero series → empty state (no fabricated 0..1 axis).
export function HourOfDayChart({ hours, formatValue = (n) => String(n), height = 200, ariaLabel, emptyLabel }: HourOfDayChartProps) {
  if (!hours || hours.length === 0 || isAllZeroSeries(hours.map((h) => h.value))) {
    return (
      <p
        role="status"
        aria-label={ariaLabel}
        data-testid="chart-empty"
        className="py-10 text-center text-sm text-ink-500"
      >
        {emptyLabel ?? "—"}
      </p>
    );
  }
  const sorted = [...hours].sort((a, b) => a.hour - b.hour);
  const data = {
    labels: sorted.map((h) => `${h.hour}:00`),
    datasets: [{ data: sorted.map((h) => h.value), backgroundColor: ANALYTICS_BAR_HUE, borderRadius: 2 }],
  };
  const options = {
    ...chartBaseOptions,
    scales: {
      x: { grid: { display: false }, ticks: { font: { size: 10 }, autoSkip: true, maxRotation: 0 } },
      y: {
        beginAtZero: true,
        grid: { color: ANALYTICS_INK_300, drawTicks: false },
        border: { display: false },
        // precision 0: all-zero series otherwise prints fractional 0..1 ticks.
        ticks: { font: { size: 10 }, precision: 0, callback: (v: number | string) => formatValue(Number(v)) },
      },
    },
  };
  return (
    <div style={{ height }} role="img" aria-label={ariaLabel}>
      <Bar data={data} options={options as any} />
    </div>
  );
}
