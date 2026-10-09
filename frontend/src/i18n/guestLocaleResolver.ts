import {
  defaultLocale,
  normalizeGuestLangParam,
  storefrontLocales,
  type StorefrontLocale,
} from "./localeRegistry";

export interface ResolvedGuestLocale {
  locale: StorefrontLocale;
  isExplicit: boolean;
}

/**
 * Cross-surface guest locale preference cookie (PG-12 / PG-21).
 * Distinct from the operator `payverge_locale` cookie so dashboard language
 * never leaks into diner SSR chrome.
 */
export const GUEST_LOCALE_COOKIE = "payverge_guest_locale";

/**
 * Path prefixes `/es` and `/es-ar` are explicit diner locale, same as `?lang=`.
 * Bare `/b/{slug}` and `/t/{code}` have no path locale (#860 / #861).
 */
export function guestPathLocaleFromPathname(
  pathname: string,
): StorefrontLocale | null {
  if (pathname === "/es-ar" || pathname.startsWith("/es-ar/")) return "es-AR";
  if (pathname === "/es" || pathname.startsWith("/es/")) return "es";
  return null;
}

/** True when the diner named a locale via `?lang=`, `/es` path, or guest cookie. */
export function hasExplicitGuestLocalePreference(opts: {
  langParam?: string | null;
  pathname?: string | null;
  guestCookie?: string | null;
}): boolean {
  return Boolean(
    normalizeGuestLangParam(opts.langParam) ||
      guestPathLocaleFromPathname(opts.pathname ?? "") ||
      normalizeGuestLangParam(opts.guestCookie),
  );
}

/** Resolve a guest `?lang=` once for middleware and server page bootstrap. */
export function resolveGuestLocale(
  raw: string | null | undefined,
): ResolvedGuestLocale {
  const normalized = normalizeGuestLangParam(raw);
  return normalized
    ? { locale: normalized, isExplicit: true }
    : { locale: defaultLocale, isExplicit: false };
}

/**
 * Full guest request locale for middleware + SSR storefront bootstrap (PG-21).
 * Priority: explicit `?lang=` → guest cookie (user pick) → Accept-Language → en.
 * Never reads the operator `payverge_locale` cookie.
 *
 * `isExplicit` is true when the locale came from `?lang=` or the guest cookie
 * so server-seeded messages/html lang match hydration and client effects do
 * not thrash the first paint.
 */
export function resolveGuestRequestLocale(opts: {
  langParam?: string | null;
  guestCookie?: string | null;
  acceptLanguage?: string | null;
}): ResolvedGuestLocale {
  const fromQuery = normalizeGuestLangParam(opts.langParam);
  if (fromQuery) {
    return { locale: fromQuery, isExplicit: true };
  }

  const fromCookie = normalizeGuestLangParam(opts.guestCookie);
  if (fromCookie) {
    return { locale: fromCookie, isExplicit: true };
  }

  return resolveGuestEntryLocale({
    langParam: null,
    acceptLanguage: opts.acceptLanguage,
  });
}

/**
 * Accept-Language / browser-language fallback for guest locale resolution.
 * Callers that also have a diner cookie (`/scan`, `/t`, `/b`) must go through
 * `resolveGuestRequestLocale` so the saved locale wins over the browser.
 * Marks isExplicit only for query params so client hydration does not thrash.
 */
export function resolveGuestEntryLocale(opts: {
  langParam?: string | null;
  acceptLanguage?: string | null;
  browserLanguages?: readonly string[];
}): ResolvedGuestLocale {
  const fromQuery = normalizeGuestLangParam(opts.langParam);
  if (fromQuery) {
    return { locale: fromQuery, isExplicit: true };
  }

  const browserList =
    opts.browserLanguages && opts.browserLanguages.length > 0
      ? opts.browserLanguages
      : parseAcceptLanguageHeader(opts.acceptLanguage);

  const enabled = storefrontLocales as readonly string[];
  const byLower = new Map(enabled.map((code) => [code.toLowerCase(), code]));

  for (const raw of browserList) {
    const lower = raw.trim().toLowerCase();
    if (!lower) continue;
    const exact = byLower.get(lower);
    if (exact) {
      return { locale: exact as StorefrontLocale, isExplicit: false };
    }
    // LatAm generic Spanish → es-AR when available
    if (lower === "es-419" && byLower.has("es-ar")) {
      return { locale: "es-AR" as StorefrontLocale, isExplicit: false };
    }
    const base = byLower.get(lower.split("-")[0]);
    if (base) {
      return { locale: base as StorefrontLocale, isExplicit: false };
    }
  }

  return { locale: defaultLocale, isExplicit: false };
}

/** Parse an Accept-Language header into ordered language tags (quality ignored beyond order). */
export function parseAcceptLanguageHeader(
  header: string | null | undefined,
): string[] {
  if (!header || !header.trim()) return [];
  return header
    .split(",")
    .map((part) => part.trim().split(";")[0]?.trim() ?? "")
    .filter(Boolean);
}

/** Client-side: persist the diner's guest locale for SSR on the next request. */
export function writeGuestLocaleCookie(locale: StorefrontLocale): void {
  if (typeof document === "undefined") return;
  const secure =
    typeof window !== "undefined" && window.location?.protocol === "https:"
      ? "; Secure"
      : "";
  document.cookie = `${GUEST_LOCALE_COOKIE}=${encodeURIComponent(locale)}; Max-Age=31536000; Path=/; SameSite=Lax${secure}`;
}

/** Client-side: read the guest locale cookie (if valid). */
export function readGuestLocaleCookie(): StorefrontLocale | null {
  if (typeof document === "undefined") return null;
  const match = document.cookie
    .split("; ")
    .find((row) => row.startsWith(`${GUEST_LOCALE_COOKIE}=`));
  if (!match) return null;
  const raw = decodeURIComponent(match.slice(GUEST_LOCALE_COOKIE.length + 1));
  return normalizeGuestLangParam(raw);
}

/**
 * Build a storefront/table `?lang=` href without dropping the active hash
 * (`#menu`, `#delivery`, `#reservations`). Locale transitions must keep the
 * guest on the same tab for refresh, share, and back/forward.
 */
export function buildGuestLangHref(
  pathname: string,
  search: string,
  lang: string,
  hash = "",
): string {
  const rawSearch = search.startsWith("?") ? search.slice(1) : search;
  const params = new URLSearchParams(rawSearch);
  params.set("lang", lang);
  const qs = params.toString();
  const normalizedHash = !hash
    ? ""
    : hash.startsWith("#")
      ? hash
      : `#${hash}`;
  return `${pathname}${qs ? `?${qs}` : ""}${normalizedHash}`;
}
