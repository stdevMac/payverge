import { resolveErrorBoundaryCopyForLocale } from "@/i18n/errorBoundaryCopy";
import { resolveGuestEntryLocale } from "@/i18n/guestLocaleResolver";
import {
  getLocaleDirection,
  type StorefrontLocale,
} from "@/i18n/localeRegistry";
import { getServerApiUrl } from "@/lib/serverApiUrl";

/**
 * PG-1 — `/business/<slug>` is a pure protocol shim, not a render surface.
 *
 * Implemented as a Route Handler (not a Server Component page) so the HTTP
 * status is authoritative:
 *   - known business  → 308 Permanent Redirect to `/b/<custom_url>`
 *   - unknown / error → 404 with a real HTML body (never soft-200 marketing)
 *
 * A Server Component that only calls `notFound()` was observed to stream a
 * 200 shell with the root layout title on production (headers flushed before
 * the notFound digests). Route Handlers set status before any body is sent.
 */
export const dynamic = "force-dynamic";
export const revalidate = 0;

// Route files may only export handlers and config, so this stays unexported.
const SLUG_LOOKUP_TIMEOUT_MS = 5000;

type SlugLookup = {
  customUrl: string | null;
};

async function fetchCustomUrl(slug: string): Promise<SlugLookup> {
  // Resolve per request so tests/env injection and runtime config stay live.
  const apiUrl = getServerApiUrl();
  if (!slug || !apiUrl) return { customUrl: null };
  try {
    const res = await fetch(`${apiUrl}/business/${slug}`, {
      cache: "no-store",
      headers: { Accept: "application/json" },
      signal: AbortSignal.timeout(SLUG_LOOKUP_TIMEOUT_MS),
    });
    if (!res.ok) return { customUrl: null };
    const data = (await res.json()) as {
      custom_url?: string;
      business?: { custom_url?: string };
    } | null;
    const customUrl =
      (typeof data?.custom_url === "string" ? data.custom_url : "") ||
      (typeof data?.business?.custom_url === "string"
        ? data.business.custom_url
        : "");
    return { customUrl: customUrl.length > 0 ? customUrl : null };
  } catch {
    return { customUrl: null };
  }
}

function escapeHtmlText(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function escapeHtmlAttribute(value: string): string {
  return escapeHtmlText(value).replace(/"/g, "&quot;").replace(/'/g, "&#39;");
}

function notFoundHtml(locale: StorefrontLocale): string {
  // Minimal, indexable-as-404 document. Matches root not-found intent without
  // streaming through the App Router page tree (which soft-200'd).
  //
  // The home link is root-relative on purpose: any origin derived from
  // `request.url` is the container's internal bind address behind Caddy /
  // Cloudflare (observed in production as https://0.0.0.0:3000), so an
  // absolute href here shipped a dead link. Relative resolves against
  // whatever public origin the visitor actually used.
  const direction = getLocaleDirection(locale);
  const copy = resolveErrorBoundaryCopyForLocale(locale);
  const title = escapeHtmlText(copy.notFoundTitle);
  return `<!DOCTYPE html>
<html lang="${escapeHtmlAttribute(locale)}" dir="${escapeHtmlAttribute(direction)}">
<head>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1"/>
  <meta name="robots" content="noindex, nofollow"/>
  <title>${title} — Payverge</title>
  <style>
    body{font-family:system-ui,sans-serif;margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;background:#faf9f6;color:#1c1917}
    main{max-width:28rem;padding:2rem;text-align:center}
    h1{font-size:1.5rem;margin:0 0 .5rem}
    p{color:#57534e;margin:0 0 1.5rem}
    a{color:#1a6b6a;font-weight:600}
  </style>
</head>
<body>
  <main>
    <h1>${title}</h1>
    <p>${escapeHtmlText(copy.notFoundBody)}</p>
    <p><a href="/">${escapeHtmlText(copy.goHome)}</a></p>
  </main>
</body>
</html>`;
}

export async function GET(
  request: Request,
  context: { params: Promise<{ businessId: string }> },
) {
  const { businessId } = await context.params;
  // Reserved single-segment routes under /business/ (register, …) never reach
  // this dynamic segment — they have their own app routes.
  const lookup = await fetchCustomUrl(businessId);

  // Location is root-relative (RFC 7231 §7.1.2 allows it; every browser
  // resolves it against the request URI). `Response.redirect` is deliberately
  // NOT used: it requires an absolute URL, and the only origin available here
  // is `request.url`, which behind Caddy/Cloudflare is the container's internal
  // bind address. That shipped `Location: https://0.0.0.0:3000/b/<slug>` to
  // production — an unreachable host for every visitor.
  const redirectTo = lookup.customUrl
    ? `/b/${encodeURIComponent(lookup.customUrl)}`
    : null;

  if (redirectTo) {
    return new Response(null, {
      status: 308,
      headers: {
        Location: redirectTo,
        "Cache-Control": "public, max-age=0, must-revalidate",
      },
    });
  }

  // Only the query string is read from `request.url` — never its origin, which
  // behind Caddy/Cloudflare is the container's internal bind address.
  const { locale } = resolveGuestEntryLocale({
    langParam: new URL(request.url).searchParams.get("lang"),
    acceptLanguage: request.headers.get("accept-language"),
  });

  return new Response(notFoundHtml(locale), {
    status: 404,
    headers: {
      "Content-Type": "text/html; charset=utf-8",
      "Cache-Control":
        "private, no-cache, no-store, max-age=0, must-revalidate",
      "X-Robots-Tag": "noindex, nofollow",
    },
  });
}
