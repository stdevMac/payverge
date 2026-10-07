/**
 * Resolve the default timezone for daily/weekly email report plugins.
 * Preference order: saved config → business profile timezone → UTC.
 */
export function resolveReportTimezone(
  configTimezone: string | null | undefined,
  businessTimezone: string | null | undefined,
): string {
  const config = (configTimezone ?? "").trim();
  if (config) return config;
  const business = (businessTimezone ?? "").trim();
  if (business) return business;
  return "UTC";
}
