import { getSiteUrl } from '@/config/publicConfig';
import { publicPathForLocale } from './metadata';
import { type Locale } from './localeRegistry';

// Locales whose public instance pages (`/`, `/business/register`,
// `/staff/login`) carry a URL of their own: `en` unprefixed, `es` + `es-AR`
// prefixed (middleware rewrites `/es/*` and `/es-ar/*` to the English base).
export const PUBLIC_PAGE_LOCALES: Locale[] = ['en', 'es', 'es-AR'];

/** Locale-aware href for a public path (`/es/...`, `/es-ar/...`, or unprefixed). */
export function localizedPublicHref(locale: Locale, route: string): string {
  return publicPathForLocale(locale, route);
}

/**
 * Self-referencing canonical plus the full reciprocal hreflang cluster for a
 * public page. Without this, an es-AR page emits no canonical and no language
 * alternates, so crawlers can't cluster it with its English sibling.
 * x-default points at the English base.
 */
export function publicPageAlternates(
  locale: Locale,
  route: string,
): { canonical: string; languages: Record<string, string> } {
  // Runtime PUBLIC_URL, read per call so one image serves any origin.
  const SITE_URL = getSiteUrl();
  const languages: Record<string, string> = {};
  for (const pageLocale of PUBLIC_PAGE_LOCALES) {
    languages[pageLocale] =
      `${SITE_URL}${publicPathForLocale(pageLocale, route)}`;
  }
  languages['x-default'] = `${SITE_URL}${publicPathForLocale('en', route)}`;
  return {
    canonical: `${SITE_URL}${publicPathForLocale(locale, route)}`,
    languages,
  };
}
