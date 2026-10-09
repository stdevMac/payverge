/**
 * Trend-chart x-axis labels arrive as raw ISO day strings ("2026-06-03")
 * from analytics bucket endpoints. Operators should read "Jun 3" (locale-
 * aware), not ISO. Anything that isn't a plain ISO day (hour buckets, week
 * labels, already-formatted strings) passes through untouched.
 */
const ISO_DAY = /^(\d{4})-(\d{2})-(\d{2})$/;

export function formatTrendLabel(label: string, locale: string): string {
  const match = ISO_DAY.exec(label);
  if (!match) return label;
  const [, year, month, day] = match;
  // Construct as a LOCAL date — new Date("YYYY-MM-DD") parses as UTC and
  // shifts the day for operators west of Greenwich.
  const date = new Date(Number(year), Number(month) - 1, Number(day));
  if (Number.isNaN(date.getTime())) return label;
  try {
    return new Intl.DateTimeFormat(locale, {
      month: "short",
      day: "numeric",
    }).format(date);
  } catch {
    return label;
  }
}
