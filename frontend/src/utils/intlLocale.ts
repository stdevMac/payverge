import { localeRegistry, type LocaleCode } from "@/i18n/localeRegistry";

// Our canonical locale codes (en, es, es-AR, fr, pt, ar, hi, de, …) are already
// valid BCP-47 tags, so the registry is the source of truth for which inputs map
// to themselves. BCP-47 tags are case-insensitive ("es-AR" == "es-ar"), but our
// codes carry a fixed canonical casing — map a lowercased input back to that
// canonical code, mirroring the byLower pattern used across the locale registry.
const canonicalByLower = new Map<string, LocaleCode>(
  (Object.keys(localeRegistry) as LocaleCode[]).map((code) => [
    code.toLowerCase(),
    code,
  ]),
);

// Legacy prefix mapping kept for inputs that are NOT a canonical registry code
// (e.g. a regional "pt-BR" the dashboards might receive). Preserves the original
// AccountingDashboard/FiscalDashboard behavior of collapsing to a base tag, and
// the historical "everything else → en-US" default.
const LEGACY_BASE_TAGS = ["es", "fr", "pt", "ar", "hi"] as const;

/**
 * Resolve one of the app's locale codes to a BCP-47 tag suitable for Intl
 * date/number/currency formatting.
 *
 * - Canonical registry codes (including "es-AR") map to themselves, so Argentine
 *   Spanish formats with the Argentine convention instead of being collapsed to
 *   neutral "es" (the bug the old binary es-ES/en-US ternary caused).
 * - Other Spanish/French/Portuguese/Arabic/Hindi inputs collapse to their base
 *   tag (legacy dashboard behavior).
 * - Everything else (including "en" and unknown input) defaults to "en-US".
 *
 * This is LOCALE-tag resolution only — it never touches currency amounts,
 * cents↔dollars conversion, or rounding. Callers pass the resolved tag to Intl.
 */
export function intlLocaleFor(locale: string): string {
  const raw = (locale || "").trim();
  if (!raw) return "en-US";

  const lower = raw.toLowerCase();

  // English (and any "en-*") is the app's default display convention.
  if (lower === "en" || lower.startsWith("en-")) return "en-US";

  // Canonical registry code (preserves es-AR and every other shippable locale).
  const canonical = canonicalByLower.get(lower);
  if (canonical) return canonical;

  // Non-canonical regional variant: collapse to a known base tag if we have one.
  for (const base of LEGACY_BASE_TAGS) {
    if (lower === base || lower.startsWith(`${base}-`)) return base;
  }

  return "en-US";
}
