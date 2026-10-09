import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";
import {
  OPERATOR_LOCALE_COOKIE,
  hasDedicatedLocalePage,
  resolveOperatorRequestLocale,
  stripOperatorLocalePrefix,
  type OperatorRequestLocale,
} from "@/utils/requestLocale";
import {
  GUEST_LOCALE_COOKIE,
  guestPathLocaleFromPathname,
  hasExplicitGuestLocalePreference,
  resolveGuestRequestLocale,
} from "@/i18n/guestLocaleResolver";
import {
  EDGE_PUBLIC_PREFIXES,
  matchesPublicPrefix,
} from "@/utils/publicPaths";
import { contentSecurityPolicyFor } from "@/lib/security/csp";
import {
  lookupGuestTable,
  matchGuestTableRoute,
} from "@/lib/guest/lookupGuestTable";
import {
  lookupStorefront,
  matchStorefrontRoute,
} from "@/lib/storefront/lookupStorefront";
import { guestTableNotFoundHtml } from "@/lib/guest/tableNotFoundHtml";
import { getServerApiUrl } from "@/lib/serverApiUrl";
import { getServerHome } from "@/lib/instance/serverHome";
import { isGuestPath } from "@/utils/guestPath";
import {
  defaultLocale,
  isGuestLocale,
  normalizeGuestLangParam,
  type StorefrontLocale,
} from "@/i18n/localeRegistry";

function resolveGuestStorefrontLocale(request: NextRequest): string | null {
  const { pathname } = request.nextUrl;
  if (!isGuestProviderPath(pathname)) {
    return null;
  }
  // PG-21: ?lang= → guest cookie → Accept-Language so SSR matches hydration.
  // Same path for /scan, /t, /b — a saved diner locale must survive QR-entry.
  // Never reads the operator payverge_locale cookie.
  const resolved = resolveGuestRequestLocale({
    langParam:
      request.nextUrl.searchParams.get("lang") ||
      guestPathLocale(request.nextUrl.pathname),
    guestCookie: request.cookies.get(GUEST_LOCALE_COOKIE)?.value,
    acceptLanguage: request.headers.get("accept-language"),
  });
  return resolved.locale;
}

function isGuestProviderPath(pathname: string): boolean {
  // One predicate with SimpleTranslationProvider (src/utils/guestPath.ts) so
  // the SSR <html lang> and the client lang effect agree on guest paths.
  return isGuestPath(pathname);
}

/** "/", "/es" or "/es-ar" (any trailing slash): the instance home. */
function isInstanceRoot(pathname: string): boolean {
  const stripped = pathname.replace(/\/+$/, "") || "/";
  return stripOperatorLocalePrefix(stripped) === "/";
}

function guestPathLocale(pathname: string): StorefrontLocale | null {
  return guestPathLocaleFromPathname(pathname);
}

/**
 * Bare /b and /t inherit the venue default language when the diner has not
 * named a locale via path, ?lang=, or the guest cookie (#861 / #877).
 * Accept-Language must not pin an Argentine QR to English.
 */
function applyVenueDefaultLocale(
  request: NextRequest,
  locale: string,
  venueDefault: string | undefined,
): string {
  if (
    hasExplicitGuestLocalePreference({
      langParam: request.nextUrl.searchParams.get("lang"),
      pathname: request.nextUrl.pathname,
      guestCookie: request.cookies.get(GUEST_LOCALE_COOKIE)?.value,
    })
  ) {
    return locale;
  }
  return normalizeGuestLangParam(venueDefault) ?? locale;
}

// The business dashboard renders all of its sub-views as in-page tabs
// keyed by `?tab=…` on /business/:id/dashboard. Direct URLs like
// /business/:id/menu (an intuitive deep-link) have no matching Next.js
// route file and fall through to 404. This middleware rewrites those
// three-segment URLs to the canonical dashboard with the tab in the
// query string, so bookmarks and shared links keep working.
//
// We do NOT touch cookies here. Auth cookies are host-only on the API
// origin (SEC-5 / #302 / #314 rejects a parent COOKIE_DOMAIN), so on a
// split-origin deployment the frontend never sees `session_token`, and even
// same-origin installs keep AuthGate + the backend as the only boundary.
// An edge cookie-presence bounce would redirect every logged-in user —
// including real admins — to /dashboard or /staff/login. Keeping this
// middleware concerned only with route rewriting + CSP avoids that
// regression while still fixing the 404 deep-links.
//
// Routes that already own a real page under /business/:id/ (dashboard,
// bills) are exempted; any other final segment becomes the `?tab=` value
// on the dashboard. The dashboard itself validates the tab against
// `validTabs` and falls back to "overview" for unknown segments — so
// garbage paths still resolve cleanly instead of 404'ing.
//
// `crm` is intentionally NOT exempted: its standalone page duplicated the
// dashboard's CRM tab with no in-app navigation, so it was removed and the
// path now rewrites to /business/:id/dashboard?tab=crm like every other tab.
const PASSTHROUGH_BUSINESS_SEGMENTS = new Set(["dashboard", "bills"]);

// Single-segment paths under /business/ that are NOT public redirect
// shims — these must keep hitting their own routes (e.g. /business/register).
const RESERVED_BUSINESS_SLUGS = new Set(["register"]);

// Bare section roots that have only sub-routes return 404 today.
// Redirect them to the canonical landing page so typed URLs and external
// shares don't dead-end. Locale roots `/es` and `/es-ar` rewrite to the
// instance home ("/") with the path locale (see finishMiddleware).
const BARE_PATH_REDIRECTS: Record<string, string> = {
  "/business": "/business/register",
  "/staff": "/staff/login",
};

function pathLocaleFromPathname(
  pathname: string,
): OperatorRequestLocale | null {
  if (pathname === "/es-ar" || pathname.startsWith("/es-ar/")) return "es-AR";
  if (pathname === "/es" || pathname.startsWith("/es/")) return "es";
  return null;
}

function isPrefetchRequest(request: NextRequest): boolean {
  const purpose = (request.headers.get("Purpose") || request.headers.get("Sec-Purpose") || "").toLowerCase();
  return (
    request.headers.get("Next-Router-Prefetch") === "1" ||
    request.headers.get("next-router-prefetch") === "1" ||
    purpose.includes("prefetch")
  );
}

function persistPathLocaleCookie(
  response: NextResponse,
  request: NextRequest,
): NextResponse {
  const pathLocale = pathLocaleFromPathname(request.nextUrl.pathname);
  // Prefetch of /es or /es-ar must not clobber the operator dashboard cookie
  // mid-session — that is the en→es→es-AR flip on tab change (#617).
  if (pathLocale && !isPrefetchRequest(request)) {
    response.cookies.set({
      name: OPERATOR_LOCALE_COOKIE,
      value: pathLocale,
      maxAge: 31536000,
      path: "/",
      sameSite: "lax",
      secure: request.nextUrl.protocol === "https:",
    });
  }
  return response;
}

/**
 * Strip absolute / protocol-relative open-redirect bait from common auth
 * return query params before they propagate across 307 hops (#296).
 * Relative same-origin paths are preserved untouched.
 */
function sanitizeRedirectSearchParams(url: URL): boolean {
  let mutated = false;
  for (const key of ["next", "redirect", "returnUrl", "return_url"]) {
    if (!url.searchParams.has(key)) continue;
    const raw = url.searchParams.get(key) ?? "";
    const pathPart = raw.split("?")[0] ?? "";
    const safe =
      raw.startsWith("/") &&
      !raw.startsWith("//") &&
      !pathPart.includes(":");
    if (!safe) {
      url.searchParams.delete(key);
      mutated = true;
    }
  }
  return mutated;
}

// BCP-47 tags are case-insensitive (`es-AR` == `/es-ar`), and iOS/macOS share
// sheets plus typed URLs routinely emit the uppercase region form. The App
// Router only serves the lowercase prefix, so alias any case variant of the
// locale segment onto the canonical lowercase route with a permanent 308
// before any other handling (#789). Only the locale segment is folded — the
// rest of the path keeps its casing.
const CASED_LOCALE_PREFIX = /^\/(es(?:-ar)?)(\/.*)?$/i;

function canonicalizeLocalePrefixCase(request: NextRequest): NextResponse | null {
  const { pathname } = request.nextUrl;
  const match = pathname.match(CASED_LOCALE_PREFIX);
  if (!match) return null;
  const segment = match[1];
  const lower = segment.toLowerCase();
  if (segment === lower) return null;
  const url = request.nextUrl.clone();
  url.pathname = `/${lower}${match[2] ?? ""}`;
  return NextResponse.redirect(url, 308);
}

export async function middleware(
  request: NextRequest,
): Promise<NextResponse> {
  const { pathname } = request.nextUrl;

  // /es-AR → /es-ar (any case variant of the locale prefix) before anything
  // else so cookies, rewrites, and CSP all see the canonical path (#789).
  const caseRedirect = canonicalizeLocalePrefixCase(request);
  if (caseRedirect) {
    return caseRedirect;
  }

  // Open-redirect hygiene (#296): drop external next/redirect targets at the
  // edge so /register?next=https://evil → /business/register does not keep them.
  if (sanitizeRedirectSearchParams(request.nextUrl)) {
    return persistPathLocaleCookie(
      NextResponse.redirect(request.nextUrl, 307),
      request,
    );
  }

  // /admin is NOT cookie-gated at the edge (#288 redesign / Chopper merge
  // blocker with #314): session_token is host-only on the API origin, so a
  // split-origin frontend never sees it. AuthGate + AdminLayoutClient
  // soft-redirect; AuthenticationAdminMiddleware is the real boundary.

  // Guest storefront: ?lang= → guest cookie → Accept-Language (PG-21).
  // Operator routes: path prefix / operator cookie / Accept-Language.
  const guestLocale = resolveGuestStorefrontLocale(request);
  const locale = isGuestProviderPath(pathname)
    ? guestLocale ?? "en"
    : resolveOperatorRequestLocale({
        pathname,
        explicitLocale: request.nextUrl.searchParams.get("lang"),
        persistedLocale: request.cookies.get(OPERATOR_LOCALE_COOKIE)?.value,
        acceptLanguage: request.headers.get("accept-language"),
      });

  const stripped = pathname.replace(/\/+$/, "") || "/";
  const bareTarget = BARE_PATH_REDIRECTS[stripped];
  if (bareTarget) {
    const url = request.nextUrl.clone();
    url.pathname = bareTarget;
    // 307 (temporary), not 308: these bare-root → landing mappings can change
    // as sections gain their own pages. A 308 is cached permanently by browsers
    // and would pin returning visitors to a stale target with no way to revoke.
    return persistPathLocaleCookie(NextResponse.redirect(url, 307), request);
  }

  // PG-1: do NOT blind-edge-redirect /business/<slug> → /b/<slug>. Copying the
  // path segment into /b/ 404s when the segment is not the canonical custom_url
  // (operator alias, legacy demo slug, numeric id). The Route Handler at
  // app/business/[businessId]/route.ts owns the lookup: raw fetch → 308 to
  // /b/<resolved-custom_url>, or real HTTP 404 HTML (not a soft-200 shell).

  // Instance root: nothing published → operator sign-in; the served venue
  // contributes its default language like /b/<slug> does.
  // An operator invite link (/?invite_code=… from `server invite`) belongs
  // to the sign-in, whatever "/" serves to guests.
  if (
    isInstanceRoot(stripped) &&
    request.nextUrl.searchParams.has("invite_code")
  ) {
    const url = request.nextUrl.clone();
    url.pathname = "/dashboard";
    return persistPathLocaleCookie(NextResponse.redirect(url, 307), request);
  }
  if (isInstanceRoot(stripped) && getServerApiUrl()) {
    const home = await getServerHome();
    if (home?.mode === "empty") {
      const url = request.nextUrl.clone();
      // Query survives (e.g. ?invite_code= from an operator invite link).
      url.pathname = "/dashboard";
      return persistPathLocaleCookie(NextResponse.redirect(url, 307), request);
    }
    if (home?.mode === "venue" && home.primary) {
      const lookup = await lookupStorefront(home.primary.custom_url);
      const rootLocale =
        lookup.kind === "found"
          ? applyVenueDefaultLocale(request, locale, lookup.defaultLanguage)
          : locale;
      return finishMiddleware(request, pathname, rootLocale);
    }
  }

  // Unknown /t/:code must be HTTP 404 before the App Router streams a 200
  // shell (notFound() digest arrives after headers). Empty API URL is owned
  // by the page/layout default export so unit tests stay fetch-free.
  const tableRoute = matchGuestTableRoute(stripped);
  if (tableRoute && getServerApiUrl()) {
    return resolveGuestTableResponse(request, tableRoute, locale);
  }

  // Unknown /b/:slug must be HTTP 404 before the App Router streams a 200
  // "Business Not Found" shell (notFound() digest arrives after headers).
  const storefrontRoute = matchStorefrontRoute(stripped);
  if (storefrontRoute && getServerApiUrl()) {
    return resolveStorefrontResponse(request, storefrontRoute, locale);
  }

  return finishMiddleware(request, pathname, locale);
}

async function resolveGuestTableResponse(
  request: NextRequest,
  tableRoute: { tableCode: string; surface: "table" | "menu" },
  locale: string,
): Promise<NextResponse> {
  const lookup = await lookupGuestTable(tableRoute.tableCode);
  if (lookup.kind === "not_found") {
    const pageLocale: StorefrontLocale = isGuestLocale(locale)
      ? (locale as StorefrontLocale)
      : defaultLocale;
    const title =
      tableRoute.surface === "menu" ? "Menu | Payverge" : "Table | Payverge";
    return persistPathLocaleCookie(
      new NextResponse(guestTableNotFoundHtml(pageLocale, title), {
        status: 404,
        headers: {
          "Content-Type": "text/html; charset=utf-8",
          "Cache-Control":
            "private, no-cache, no-store, max-age=0, must-revalidate",
          "X-Robots-Tag": "noindex, nofollow",
        },
      }),
      request,
    );
  }
  const resolvedLocale =
    lookup.kind === "found"
      ? applyVenueDefaultLocale(request, locale, lookup.defaultLanguage)
      : locale;
  return finishMiddleware(request, request.nextUrl.pathname, resolvedLocale);
}

async function resolveStorefrontResponse(
  request: NextRequest,
  storefrontRoute: { slug: string },
  locale: string,
): Promise<NextResponse> {
  const lookup = await lookupStorefront(storefrontRoute.slug);
  if (lookup.kind === "not_found") {
    const pageLocale: StorefrontLocale = isGuestLocale(locale)
      ? (locale as StorefrontLocale)
      : defaultLocale;
    return persistPathLocaleCookie(
      new NextResponse(guestTableNotFoundHtml(pageLocale, "Business Not Found | Payverge"), {
        status: 404,
        headers: {
          "Content-Type": "text/html; charset=utf-8",
          "Cache-Control":
            "private, no-cache, no-store, max-age=0, must-revalidate",
          "X-Robots-Tag": "noindex, nofollow",
        },
      }),
      request,
    );
  }
  const resolvedLocale =
    lookup.kind === "found"
      ? applyVenueDefaultLocale(request, locale, lookup.defaultLanguage)
      : locale;
  return finishMiddleware(request, request.nextUrl.pathname, resolvedLocale);
}

function finishMiddleware(
  request: NextRequest,
  pathname: string,
  locale: string,
): NextResponse {
  const businessMatch = pathname.match(/^\/business\/([^/]+)\/([^/]+)\/?$/);
  if (businessMatch) {
    const [, businessId, segment] = businessMatch;
    if (
      !RESERVED_BUSINESS_SLUGS.has(businessId) &&
      !PASSTHROUGH_BUSINESS_SEGMENTS.has(segment)
    ) {
      const url = request.nextUrl.clone();
      url.pathname = `/business/${businessId}/dashboard`;
      url.searchParams.set("tab", segment);
      return persistPathLocaleCookie(NextResponse.redirect(url, 307), request);
    }
  }

  // Per-request nonce for CSP script-src. The policy itself is built from
  // the live runtime config (PUBLIC_URL, API_URL, MEDIA_ORIGINS, ...), see
  // src/lib/security/csp.ts; nothing deployment-specific is baked in.
  const nonce = Buffer.from(crypto.randomUUID()).toString("base64");
  const cspHeader = contentSecurityPolicyFor(nonce);

  const requestHeaders = new Headers(request.headers);
  requestHeaders.set("x-payverge-locale", locale);
  requestHeaders.set("x-nonce", nonce);

  // Locale-prefixed marketing URLs without a dedicated page rewrite to the
  // English base while keeping the /es|/es-ar URL + locale header (#11/#12).
  // /app is the PWA launch path — it is not a public marketing prefix (AuthGate
  // still owns it) but /es/app and /es-ar/app must rewrite instead of 404 (#914).
  const pathLocale = pathLocaleFromPathname(pathname);
  if (pathLocale) {
    const unprefixed = stripOperatorLocalePrefix(pathname);
    const isPwaLaunch =
      unprefixed === "/app" || unprefixed.startsWith("/app/");
    if (
      !hasDedicatedLocalePage(pathLocale, unprefixed) &&
      (isPwaLaunch || matchesPublicPrefix(unprefixed, EDGE_PUBLIC_PREFIXES))
    ) {
      const url = request.nextUrl.clone();
      url.pathname = unprefixed;
      // Carry the path locale in the rewritten URL too, not only in the
      // x-payverge-locale request header. When the rewrite origin differs
      // from the server's own origin (request.nextUrl reports localhost
      // while `next start -H`/HOSTNAME binds another host), Next proxies the
      // rewrite as a fresh request: overridden request headers are dropped
      // and middleware runs again on the bare path, so /es rendered English
      // under a canonical that advertises it as Spanish. ?lang= survives
      // both paths; an explicit ?lang= on the original URL still wins.
      if (!url.searchParams.has("lang")) {
        url.searchParams.set("lang", pathLocale.toLowerCase());
      }
      const response = NextResponse.rewrite(url, {
        request: { headers: requestHeaders },
      });
      response.headers.set("Content-Security-Policy", cspHeader);
      return persistPathLocaleCookie(response, request);
    }
  }

  const response = NextResponse.next({
    request: {
      headers: requestHeaders,
    },
  });

  response.headers.set("Content-Security-Policy", cspHeader);

  return persistPathLocaleCookie(response, request);
}

export const config = {
  // Node.js runtime: the CSP and locale logic read the runtime environment
  // (PUBLIC_URL, MEDIA_ORIGINS, ...) per request instead of build-time
  // inlined values, so one image serves any deployment.
  runtime: "nodejs",
  matcher: [
    // Every page path except static assets, images, the same-origin API
    // proxy (/api/*) and uploads (/media/*). Keeping the proxies out of
    // middleware means request bodies stream straight to the route handler
    // instead of being buffered (and size-capped) for middleware.
    "/((?!_next/static|_next/image|api/|media/|favicon.ico|.*\\.(?:svg|png|jpg|jpeg|gif|webp|ico)$).*)",
  ],
};
