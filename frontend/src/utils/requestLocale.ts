import {
  isSupportedLocale,
  locales,
  resolveLocaleForBrowser,
  type Locale,
} from "@/i18n/localeRegistry";
import {
  EDGE_PUBLIC_PREFIXES,
  matchesPublicPrefix,
} from "@/utils/publicPaths";

export type RequestLocale = "en" | "es" | "es-ar";

export type OperatorRequestLocale = "en" | "es" | "es-AR";

export const OPERATOR_LOCALE_COOKIE = "payverge_locale";

/**
 * Unprefixed auth/register surfaces honor the operator locale cookie.
 * They are public for AuthGate, but they are not English-stable marketing URLs.
 */
const OPERATOR_AUTH_PUBLIC_PREFIXES: readonly string[] = [
  "/staff/login",
  "/staff/accept-invitation",
  "/forgot-password",
  "/reset-password",
  "/register",
  "/business/register",
  "/verify-email",
];

function explicitPathLocale(pathname: string): OperatorRequestLocale | null {
  if (pathname === "/es-ar" || pathname.startsWith("/es-ar/")) {
    return "es-AR";
  }
  if (pathname === "/es" || pathname.startsWith("/es/")) {
    return "es";
  }
  return null;
}

const operatorLocaleByLower = new Map<string, OperatorRequestLocale>(
  locales.map((locale) => [locale.toLowerCase(), locale]),
);

function canonicalOperatorLocale(
  value: string | null | undefined,
): OperatorRequestLocale | null {
  return value ? (operatorLocaleByLower.get(value.toLowerCase()) ?? null) : null;
}

function acceptedOperatorLocale(
  acceptLanguage: string | null | undefined,
): OperatorRequestLocale | null {
  if (!acceptLanguage) return null;

  const candidates = acceptLanguage
    .split(",")
    .map((part) => {
      const [rawTag, ...params] = part.trim().split(";");
      const qParam = params.find((param) => param.trim().startsWith("q="));
      const quality = qParam ? Number(qParam.trim().slice(2)) : 1;
      return {
        tag: rawTag.toLowerCase(),
        quality: Number.isFinite(quality) ? quality : 0,
      };
    })
    .filter(({ quality }) => quality > 0)
    .sort((a, b) => b.quality - a.quality);

  return candidates.length > 0
    ? resolveLocaleForBrowser(candidates.map(({ tag }) => tag))
    : null;
}

/**
 * Unprefixed public pages (`/`, legal pages) are English by path contract (#37).
 * Cookie / Accept-Language / ?lang= must not paint Spanish onto them — those
 * preferences apply on dashboard / auth surfaces and on `/es*` paths.
 */
export function isUnprefixedPublicPagePath(pathname: string): boolean {
  if (explicitPathLocale(pathname)) return false;
  if (matchesPublicPrefix(pathname, OPERATOR_AUTH_PUBLIC_PREFIXES)) {
    return false;
  }
  return matchesPublicPrefix(pathname, EDGE_PUBLIC_PREFIXES);
}

/**
 * Unprefixed operator app surfaces (/dashboard, /business/:id, …).
 * An explicit stored locale must not be overwritten by Accept-Language SSR.
 */
export function isUnprefixedOperatorAppPath(pathname: string): boolean {
  if (explicitPathLocale(pathname)) return false;
  if (pathname === "/business/register" || pathname.startsWith("/business/register/")) {
    return false;
  }
  if (pathname === "/staff/login" || pathname.startsWith("/staff/login/")) {
    return false;
  }
  if (pathname === "/staff/accept-invitation" || pathname.startsWith("/staff/accept-invitation/")) {
    return false;
  }
  return (
    pathname === "/dashboard" ||
    pathname.startsWith("/dashboard/") ||
    pathname.startsWith("/business/") ||
    pathname.startsWith("/admin") ||
    pathname.startsWith("/account") ||
    pathname.startsWith("/staff/")
  );
}

/**
 * After hydration on operator app paths, honor the stored operator pick over
 * an Accept-Language-derived initialLocale so English login does not flip
 * to es/es-AR mid-session (#617).
 */
export function preferStoredOperatorLocale({
  pathname,
  storedLocale,
  initialLocale,
}: {
  pathname: string;
  storedLocale: string | null | undefined;
  initialLocale: string | null | undefined;
}): Locale | null {
  if (!isUnprefixedOperatorAppPath(pathname)) return null;
  if (!isSupportedLocale(storedLocale)) return null;
  if (storedLocale === initialLocale) return null;
  return storedLocale;
}

/** Resolve the operator locale used by middleware for SSR and first hydration. */
export function resolveOperatorRequestLocale({
  pathname,
  explicitLocale,
  persistedLocale,
  acceptLanguage,
}: {
  pathname: string;
  explicitLocale?: string | null;
  persistedLocale?: string | null;
  acceptLanguage?: string | null;
}): OperatorRequestLocale {
  const pathLocale = explicitPathLocale(pathname);
  if (pathLocale) return pathLocale;

  // Path is the SEO source of truth for unprefixed public marketing URLs.
  if (isUnprefixedPublicPagePath(pathname)) {
    return "en";
  }

  // Operator chrome (/dashboard, /business/:id, …): cookie and ?lang= only.
  // Accept-Language SSR is what flipped English login to es then es-AR on
  // every tab RSC when the operator cookie had not been planted yet (#617).
  if (isUnprefixedOperatorAppPath(pathname)) {
    return (
      canonicalOperatorLocale(explicitLocale) ??
      canonicalOperatorLocale(persistedLocale) ??
      "en"
    );
  }

  return (
    canonicalOperatorLocale(explicitLocale) ??
    canonicalOperatorLocale(persistedLocale) ??
    acceptedOperatorLocale(acceptLanguage) ??
    "en"
  );
}

export function getRequestLocale(pathname: string): RequestLocale {
  if (pathname === "/es-ar" || pathname.startsWith("/es-ar/")) {
    return "es-ar";
  }
  if (pathname === "/es" || pathname.startsWith("/es/")) {
    return "es";
  }
  return "en";
}

/** Strip a leading /es or /es-ar prefix; returns the unprefixed path. */
export function stripOperatorLocalePrefix(pathname: string): string {
  if (pathname === "/es-ar") return "/";
  if (pathname.startsWith("/es-ar/")) {
    return pathname.slice("/es-ar".length) || "/";
  }
  if (pathname === "/es") return "/";
  if (pathname.startsWith("/es/")) {
    return pathname.slice("/es".length) || "/";
  }
  return pathname;
}

/**
 * True when a locale-prefixed path already has a dedicated App Router page
 * (the staff home trees). Everything else may be rewritten
 * to the English base while keeping the prefixed URL (#11 / #12).
 */
export function hasDedicatedLocalePage(
  _locale: OperatorRequestLocale,
  unprefixedPath: string,
): boolean {
  const path = unprefixedPath === "" ? "/" : unprefixedPath;
  return path === "/staff/home" || path.startsWith("/staff/home/");
}
