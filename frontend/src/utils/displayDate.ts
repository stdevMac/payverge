/**
 * Canonical operator-facing date: "Jul 2, 2026" (en) / "2 jul 2026" (es).
 *
 * Never use bare toLocaleDateString(locale) for display dates — it renders
 * numeric m/d/yyyy vs d/m/yyyy depending on locale, which is ambiguous the
 * moment en and es operators look at the same account.
 */
export function formatDisplayDate(
  value: string | Date | null | undefined,
  locale: string,
): string {
  if (!value) return "--";
  const d = typeof value === "string" ? new Date(value) : value;
  if (Number.isNaN(d.getTime())) return "--";
  return d.toLocaleDateString(locale, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}
