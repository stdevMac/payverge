import { isSupportedLocale, type Locale } from "./localeRegistry";

/**
 * OP-2 bridge: decide whether a user's backend-saved `language_selected` should
 * seed the operator render path (the `locale` key SimpleTranslationProvider
 * reads/writes).
 *
 * Returns the operator locale to apply via `setLocale`, or `null` when nothing
 * should change. Guards:
 *  - An explicit in-session pick (a non-empty `locale` value already in storage)
 *    is never clobbered — the operator's live choice wins over the stale
 *    backend value, and a value equal to the saved one is already in sync.
 *  - Only real operator-dashboard locales (en/es/es-AR) are applied; guest-only
 *    menu-translation codes (fr, ja, …) and garbage never seed the dashboard.
 */
export function resolveBackendLocaleSeed({
  saved,
  storedLocale,
}: {
  saved: string | null | undefined;
  storedLocale: string | null | undefined;
}): Locale | null {
  // An explicit in-session pick (non-empty stored `locale`) always wins.
  if (typeof storedLocale === "string" && storedLocale.length > 0) {
    return null;
  }
  if (!isSupportedLocale(saved)) {
    return null;
  }
  return saved;
}
