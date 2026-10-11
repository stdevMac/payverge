import { MetadataRoute } from 'next';
import { publicPathForLocale } from '@/i18n/metadata';
import {
  publicPageAlternates,
  PUBLIC_PAGE_LOCALES,
} from '@/i18n/publicPageRoutes';
import { localeRegistry, storefrontLocales } from '@/i18n/localeRegistry';
import { isNonIndexableStorefront } from '@/api/publicBusiness';
import { buildStorefrontLanguageAlternates } from '@/lib/seo/storefrontAlternates';
import { getServerApiUrl } from '@/lib/serverApiUrl';
import { getSiteUrl } from '@/config/publicConfig';
import { isSeoIndexingEnabled } from '@/config/serverConfig';
import { getServerHome } from '@/lib/instance/serverHome';

// Rendered per request: URLs derive from the runtime PUBLIC_URL, so one image
// serves any origin. Without `force-dynamic` Next would prerender the sitemap
// at build time and freeze the build machine's origin into every <loc>.
export const dynamic = 'force-dynamic';

// Build-time fallback so sitemap entries don't claim "modified today" on
// every fetch — that pattern gets de-prioritized by Googlebot. The env
// var is already set in layout.tsx for build-info; reuse it here.
const FALLBACK_BUILD_DATE = '2026-05-11T00:00:00Z';

function getBuildLastModified(): Date {
  const raw = process.env.NEXT_PUBLIC_BUILD_TIMESTAMP;
  if (raw) {
    const parsed = new Date(raw);
    if (!Number.isNaN(parsed.getTime())) return parsed;
  }
  return new Date(FALLBACK_BUILD_DATE);
}

// Public storefront slugs come from the backend (no auth). Bounded by the
// endpoint; a fetch failure degrades to the static sitemap (never throws).
// kind=test fixtures are omitted if flags leak through the payload.
interface StorefrontSlug {
  custom_url: string;
  updated_at?: string;
  is_demo?: boolean;
  kind?: string;
}

async function fetchStorefrontSlugs(): Promise<StorefrontSlug[]> {
  // Runtime INTERNAL_API_URL (Docker) — do not freeze NEXT_PUBLIC_ at module
  // scope or the sitemap silently emits zero /b/ URLs in compose (#612).
  const apiUrl = getServerApiUrl();
  if (!apiUrl) return [];
  try {
    const res = await fetch(`${apiUrl}/business/storefronts`, {
      next: { revalidate: 3600 },
    });
    if (!res.ok) return [];
    const data = await res.json();
    return Array.isArray(data?.storefronts) ? data.storefronts : [];
  } catch {
    // Explicit LIMIT: degrade to the static sitemap on any fetch/parse failure
    // rather than throwing the whole sitemap or emitting silent garbage.
    return [];
  }
}

/**
 * Public, indexable non-venue pages. Each ships real `/es` + `/es-ar` URLs
 * (middleware rewrites the prefix to the English base) so it carries the
 * en/es/es-AR hreflang cluster. The legal pages are absent on purpose: they
 * render the generic template, which is `noindex` (lib/instance/legalPage).
 */
const LOCALIZED_PUBLIC_ROUTES = ['/business/register'] as const;

function localizedEntries(
  siteUrl: string,
  route: string,
  lastModified: Date,
): MetadataRoute.Sitemap {
  return PUBLIC_PAGE_LOCALES.filter(
    (locale) => localeRegistry[locale].publishable,
  ).map((locale) => ({
    url: `${siteUrl}${publicPathForLocale(locale, route)}`,
    lastModified,
    changeFrequency: 'monthly' as const,
    priority: 0.3,
    alternates: { languages: publicPageAlternates(locale, route).languages },
  }));
}

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  // Same gate as robots.ts: a private-by-default install (SEO_INDEXING unset)
  // disallows crawling, so it must not enumerate venue storefront slugs here.
  if (!isSeoIndexingEnabled()) return [];
  const SITE_URL = getSiteUrl();
  const buildLastModified = getBuildLastModified();

  // The instance root: the primary venue (canonical "/"), the venue
  // directory, or nothing when no venue is published ("/" redirects).
  const home = await getServerHome();
  const primarySlug =
    home?.mode === 'venue' && home.primary ? home.primary.custom_url : '';
  const homeEntries: MetadataRoute.Sitemap = [];
  if (primarySlug) {
    const languages = buildStorefrontLanguageAlternates(
      SITE_URL,
      primarySlug,
      storefrontLocales,
      '/',
    );
    homeEntries.push({
      url: `${SITE_URL}/`,
      lastModified: buildLastModified,
      changeFrequency: 'daily' as const,
      priority: 1,
      ...(languages ? { alternates: { languages } } : {}),
    });
  } else if (home?.mode === 'directory') {
    homeEntries.push({
      url: `${SITE_URL}/`,
      lastModified: buildLastModified,
      changeFrequency: 'weekly' as const,
      priority: 1,
    });
  }

  const publicEntries = LOCALIZED_PUBLIC_ROUTES.flatMap((route) =>
    localizedEntries(SITE_URL, route, buildLastModified),
  );

  const storefrontSlugs = await fetchStorefrontSlugs();
  // hreflang cluster mirrors /b/[customUrl] generateMetadata: path prefixes
  // for es/es-AR, ?lang=<code> for other guest locales, lang-less x-default.
  // The primary venue is listed once, at "/" (its canonical).
  const storefrontEntries: MetadataRoute.Sitemap = storefrontSlugs
    .filter(
      (s) =>
        Boolean(s.custom_url) &&
        !isNonIndexableStorefront(s) &&
        s.custom_url.toLowerCase() !== primarySlug.toLowerCase(),
    )
    .map((s) => {
      const languages = buildStorefrontLanguageAlternates(
        SITE_URL,
        s.custom_url,
        storefrontLocales,
      );
      return {
        url: `${SITE_URL}/b/${s.custom_url}`,
        lastModified: s.updated_at ? new Date(s.updated_at) : buildLastModified,
        changeFrequency: 'weekly' as const,
        priority: 0.8,
        ...(languages ? { alternates: { languages } } : {}),
      };
    });

  return [...homeEntries, ...storefrontEntries, ...publicEntries];
}
