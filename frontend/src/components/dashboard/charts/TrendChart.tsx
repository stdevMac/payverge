// src/components/dashboard/charts/TrendChart.tsx
import React from "react";
import {
  Chart as ChartJS, CategoryScale, LinearScale, PointElement, LineElement, Tooltip,
} from "chart.js";
import { Line } from "react-chartjs-2";
import { ANALYTICS_TEAL, trendLineOptions } from "./analyticsTheme";
import { formatTrendLabel } from "./chartDates";
import { isAllZeroSeries } from "./chartEmpty";

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip);

interface TrendChartProps {
  // `value` is nullable so no-data days render as GAPS in the line rather than
  // plotted $0 points (which produced misleading cliffs, especially at the
  // end of the range). Paired with `spanGaps: false` on the dataset below.
  points: { date: string; value: number | null }[];
  formatValue?: (n: number) => string;
  height?: number;
  ariaLabel: string;
  /**
   * Optional, already-localized empty-state caption. When omitted we render a
   * locale-neutral em-dash placeholder instead of the previous untranslated
   * "{ariaLabel}: no data" English string.
   */
  emptyLabel?: string;
  /**
   * Locale for x-axis labels — ISO "YYYY-MM-DD" bucket dates render as short
   * localized dates ("Jun 3" / "4 ago"); other label shapes pass through.
   * Required so Spanish operator UI never falls back to English months (L4-10).
   */
  locale: string;
}

function ChartEmpty({
  ariaLabel,
  emptyLabel,
}: {
  ariaLabel: string;
  emptyLabel?: string;
}) {
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

// Minimal chart.js line: one teal series, straight segments, zero baseline,
// faint y-grid, no points until hover. All-zero series → empty state (no axes).
export function TrendChart({ points, formatValue = (n) => String(n), height = 220, ariaLabel, emptyLabel, locale }: TrendChartProps) {
  if (!points || points.length === 0) {
    return <ChartEmpty ariaLabel={ariaLabel} emptyLabel={emptyLabel} />;
  }
  // An all-zero series used to autoscale to 0..1 and print a fabricated $1.00
  // axis. Treat it as empty instead.
  if (isAllZeroSeries(points.map((p) => p.value))) {
    return <ChartEmpty ariaLabel={ariaLabel} emptyLabel={emptyLabel} />;
  }
  const data = {
    labels: points.map((p) => formatTrendLabel(p.date, locale)),
    datasets: [
      {
        data: points.map((p) => p.value),
        borderColor: ANALYTICS_TEAL,
        backgroundColor: "transparent",
        borderWidth: 1.5,
        fill: false,
        // Leave null buckets as breaks in the line instead of bridging them
        // with a straight segment down to a phantom zero.
        spanGaps: false,
      },
    ],
  };
  return (
    <div style={{ height }} role="img" aria-label={ariaLabel}>
      <Line data={data} options={trendLineOptions(formatValue)} />
    </div>
  );
}
