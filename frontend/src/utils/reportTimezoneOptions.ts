import { getTimezoneLabel, TIMEZONE_OPTIONS } from "@/utils/timezones";

/**
 * Platform geodefaults / canonical zones that must always appear in report
 * timezone pickers even if a short curated list is used as a fallback.
 * Mirrors backend/internal/geodefaults (AR → America/Argentina/Buenos_Aires).
 */
const PLATFORM_CANONICAL_TIMEZONES = [
  "UTC",
  "America/Argentina/Buenos_Aires",
  "America/New_York",
  "America/Sao_Paulo",
  "America/Mexico_City",
  "Europe/Madrid",
  "Europe/London",
  "Asia/Dubai",
  "Asia/Tokyo",
] as const;

export interface ReportTimezoneOption {
  value: string;
  label: string;
}

/**
 * Full IANA timezone identifiers for report (daily/weekly) pickers.
 * Prefers `Intl.supportedValuesOf("timeZone")` when available; falls back to
 * the curated TIMEZONE_OPTIONS list + platform canonical zones.
 * Always includes `businessTimezone` so Select selectedKeys can match the
 * default from resolveReportTimezone (L6-33).
 */
export function listIanaTimezoneIds(
  businessTimezone?: string | null,
): string[] {
  const ids = new Set<string>();

  try {
    const supported = (
      Intl as unknown as { supportedValuesOf?: (key: string) => string[] }
    ).supportedValuesOf;
    if (typeof supported === "function") {
      for (const z of supported.call(Intl, "timeZone")) {
        ids.add(z);
      }
    }
  } catch {
    // ignore — fall through to curated + canonical
  }

  if (ids.size === 0) {
    for (const o of TIMEZONE_OPTIONS) {
      ids.add(o.value);
    }
  }

  for (const z of PLATFORM_CANONICAL_TIMEZONES) {
    ids.add(z);
  }

  const business = (businessTimezone ?? "").trim();
  if (business) {
    ids.add(business);
  }

  return Array.from(ids).sort((a, b) => a.localeCompare(b));
}

/** Options for Daily/Weekly report Select — full IANA + business TZ guaranteed. */
export function reportTimezoneSelectOptions(
  businessTimezone?: string | null,
): ReportTimezoneOption[] {
  return listIanaTimezoneIds(businessTimezone).map((value) => ({
    value,
    label: getTimezoneLabel(value),
  }));
}
