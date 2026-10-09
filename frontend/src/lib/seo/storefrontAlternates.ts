import { guestPublicUrl } from "./guestUrls";

/**
 * hreflang alternates for a storefront URL, shared by
 * `app/b/[customUrl]/page.tsx` (generateMetadata, per-business supported
 * languages from the API payload) and `app/sitemap.ts` (storefront entries,
 * using the static storefront locale registry — the sitemap slug endpoint
 * does not carry per-business languages).
 *
 * es / es-AR use path prefixes (`/es/b/{slug}`) so a localized storefront
 * can self-canonicalize (#864). Other guest locales keep `?lang=<code>`.
 */
export function buildStorefrontLanguageAlternates(
  baseUrl: string,
  customUrl: string,
  languageCodes: readonly string[],
  // The storefront's canonical path: `/b/{slug}`, or `/` for the venue the
  // instance serves at its root (PRIMARY_VENUE / single published venue).
  pagePath: string = `/b/${customUrl}`,
): Record<string, string> | undefined {
  const codes = languageCodes.filter(Boolean);
  if (codes.length === 0 || !baseUrl) return undefined;
  const path = pagePath;
  const languages: Record<string, string> = {};
  for (const code of codes) {
    const lower = code.toLowerCase();
    if (lower === "es" || lower === "es-ar") {
      languages[code] = guestPublicUrl(code, path, baseUrl);
    } else {
      languages[code] = `${baseUrl}${path}?lang=${code}`;
    }
  }
  languages["x-default"] = `${baseUrl}${path}`;
  return languages;
}
