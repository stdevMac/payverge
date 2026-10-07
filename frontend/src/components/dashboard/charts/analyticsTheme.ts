// src/components/dashboard/charts/analyticsTheme.ts
/* eslint-disable no-restricted-syntax -- chart.js canvas + inline-SVG colors require hex strings, not Tailwind classes. ANALYTICS_TEAL mirrors brand.DEFAULT (#1a6b6a) in tailwind.config. */
import type { ChartOptions } from "chart.js";

// Brand-anchored palette. No rainbow: one teal for the primary series, a single
// hue for ranked bars (length encodes value), and a restrained up/down pair used
// ONLY for delta indicators. ANALYTICS_TEAL === Tailwind brand.DEFAULT.
export const ANALYTICS_TEAL = "#1a6b6a";
export const ANALYTICS_BAR_HUE = "#1a6b6a";
export const ANALYTICS_INK_300 = "#d6d3d1"; // faint gridline
export const DELTA_UP = "#047857"; // emerald-700
export const DELTA_DOWN = "#be123c"; // rose-700

// Chartjunk-free chart.js base: no legend (we direct-label), no aspect lock,
// minimal tooltip. Shared by every chart.js chart in the analytics area.
export const chartBaseOptions: ChartOptions<"line" | "bar"> = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { display: false },
    tooltip: {
      backgroundColor: "rgba(28,25,23,0.92)",
      padding: 8,
      cornerRadius: 6,
      displayColors: false,
    },
  },
};

// Options for the daily TrendChart: zero baseline, no curve, faint y-grid only.
export function trendLineOptions(formatY: (n: number) => string): ChartOptions<"line"> {
  return {
    ...chartBaseOptions,
    elements: { line: { tension: 0 }, point: { radius: 0, hoverRadius: 3 } },
    scales: {
      x: { grid: { display: false }, ticks: { font: { size: 10 }, maxRotation: 0, autoSkip: true } },
      y: {
        beginAtZero: true,
        grid: { color: ANALYTICS_INK_300, drawTicks: false },
        border: { display: false },
        // precision 0: an all-zero series otherwise autoscales 0..1 and prints
        // fractional ticks (0.1, 0.2, …) for what are integer counts/amounts.
        ticks: { font: { size: 10 }, precision: 0, callback: (v) => formatY(Number(v)) },
      },
    },
  } as ChartOptions<"line">;
}
