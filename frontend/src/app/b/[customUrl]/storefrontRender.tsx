// Shared storefront renderer: /b/[customUrl] and the instance root ("/",
// when it serves PRIMARY_VENUE or the single published venue) render the
// same page. `pagePath` is the canonical path — `/b/{slug}` normally, `/`
// for the root venue — and drives canonical, og:url, hreflang and JSON-LD so
// the two URLs never compete as duplicate content.
import { getSiteUrl } from "@/config/publicConfig";
import { Metadata } from "next";
import { notFound } from "next/navigation";
import {
  dehydrate,
  HydrationBoundary,
  QueryClient,
  type DehydratedState,
} from "@tanstack/react-query";
import BusinessPageClient from "./BusinessPageClient";
import CustomerAuthShell from "@/components/customer/CustomerAuthShell";
import { isNonIndexableStorefront } from "@/api/publicBusiness";
import { queryKeys } from "@/api/queryKeys";
import { serializeJsonLd } from "@/lib/seo/jsonLd";
import { buildBusinessJsonLd } from "@/lib/seo/businessJsonLd";
import { buildBreadcrumbJsonLd } from "@/lib/seo/breadcrumbs";
import { buildStorefrontLanguageAlternates } from "@/lib/seo/storefrontAlternates";
import { guestOgLocale, guestPublicUrl } from "@/lib/seo/guestUrls";
import {
  fetchStorefrontBusiness,
  fetchStorefrontGoogleRating,
  fetchStorefrontMenuPayload,
} from "@/lib/storefront/serverData";
import {
  normalizeStorefrontMenuPayload,
  type StorefrontMenuQueryResult,
} from "@/lib/storefront/menuPayload";
import { resolveGuestLocaleFromRequest } from "@/i18n/guestLocale.server";
import { loadGuestMessages } from "@/i18n/guestMessages.server";
import {
  sanitizeStorefrontProse,
  storefrontProseMatchesLocale,
} from "@/lib/storefront/proseLocale";

// I18N-D1b: localized metadata for the not-found path. The guest bundles carry
// businessPage.errors.notFound + businessPage.error.notFoundBody in all 21
// locales; a dynamic import loads only the requested bundle server-side.
// Any failure falls back to the English literals.
async function notFoundMetadata(lang?: string): Promise<Metadata> {
  if (lang) {
    try {
      const messages = (await import(`@/i18n/guest-messages/${lang}.json`))
        .default as Record<string, any>;
      const title = messages?.businessPage?.errors?.notFound;
      const description = messages?.businessPage?.error?.notFoundBody;
      if (typeof title === "string" && typeof description === "string") {
        return { title: `${title} | Payverge`, description };
      }
    } catch {
      // fall through to English
    }
  }
  return {
    title: "Business Not Found | Payverge",
    description: "The requested business page could not be found.",
  };
}

export interface StorefrontRenderOptions {
  customUrl: string;
  searchParams?: Record<string, string | string[] | undefined>;
  /** Canonical path: `/b/{slug}` (default) or `/` for the root venue. */
  pagePath?: string;
}

export async function storefrontMetadata({
  customUrl,
  searchParams,
  pagePath = `/b/${customUrl}`,
}: StorefrontRenderOptions): Promise<Metadata> {
  // I18N-3: honor a validated ?lang= so the share/SERP snippet matches the
  // visitor's language; an unsupported value falls back to the default fetch.
  // LOCALE-2: normalize case-insensitively (BCP-47 tags are case-insensitive),
  // so a natural lowercase "es-ar" resolves to the canonical "es-AR" storefront
  // locale instead of silently falling back to the business default — matching
  // both the case-insensitive route prefix and the client first-paint logic.
  const sp = searchParams ?? {};
  const rawLang = Array.isArray(sp.lang) ? sp.lang[0] : sp.lang;
  // PG-21: cookie / Accept-Language join ?lang= so SERP snippet matches first paint.
  const resolvedLocale = await resolveGuestLocaleFromRequest(rawLang);
  const lang = resolvedLocale.isExplicit ? resolvedLocale.locale : undefined;
  const result = await fetchStorefrontBusiness(customUrl, lang);
  const business = result.business;

  if (!business) {
    if (result.reason === "not_found") {
      return notFoundMetadata(lang);
    }
    // Transient failure - show a neutral title so users don't see a scary
    // 'Not Found' flash while the page is actually still loading. noindex it:
    // a backend blip must never get indexed as the storefront's snippet.
    return {
      title: "Loading business… | Payverge",
      description: "Loading business page…",
      robots: { index: false, follow: false },
    };
  }

  const title = `${business.name} | Payverge`;
  const snippetLocale = resolvedLocale.locale;
  const description = sanitizeStorefrontProse(
    business.description,
    snippetLocale,
    `Visit ${business.name} - Powered by Payverge`,
  );

  const baseUrl = getSiteUrl();
  const pageUrl = guestPublicUrl(snippetLocale, pagePath, baseUrl);

  const supportedLangs = (business.supported_languages || [])
    .map((language) => language.code)
    .filter(Boolean);
  const languages = buildStorefrontLanguageAlternates(
    baseUrl,
    customUrl,
    supportedLangs,
    pagePath,
  );

  const demoStorefront = isNonIndexableStorefront(business);
  // The OG/Twitter file-convention images live on the /b/[customUrl] route;
  // the root venue points at them explicitly so its share card matches.
  const servedAtRoot = pagePath !== `/b/${customUrl}`;
  const cardBase = `${baseUrl.replace(/\/+$/, "")}/b/${encodeURIComponent(customUrl)}`;

  return {
    title,
    description,
    robots: demoStorefront
      ? { index: false, follow: false }
      : { index: true, follow: true },
    icons: business.logo
      ? {
        icon: business.logo,
        shortcut: business.logo,
        apple: business.logo,
      }
      : undefined,
    // Localized /es/b and /es-ar/b self-canonicalize (#864). Other guest
    // locales stay on the unprefixed /b/{slug} URL with ?lang= hreflang.
    alternates: {
      canonical: pageUrl,
      ...(languages ? { languages } : {}),
    },
    // OG/Twitter images are intentionally NOT set here: the file-convention
    // opengraph-image.tsx in this route generates a branded per-business card
    // (banner + logo + name + rating), replacing the old raw banner/logo URL.
    openGraph: {
      title,
      description,
      type: "website",
      url: pageUrl,
      locale: guestOgLocale(snippetLocale),
      ...(servedAtRoot
        ? { images: [{ url: `${cardBase}/opengraph-image`, width: 1200, height: 630 }] }
        : {}),
    },
    twitter: {
      card: "summary_large_image",
      title,
      description,
      ...(servedAtRoot ? { images: [`${cardBase}/twitter-image`] } : {}),
    },
  };
}

export async function StorefrontView({
  customUrl,
  searchParams,
  pagePath = `/b/${customUrl}`,
}: StorefrontRenderOptions) {
  // I18N-D1: mirror generateMetadata — a validated ?lang= is threaded into the
  // body fetch so the SSR first paint carries the translated hero/about prose
  // instead of flashing the source language and swapping client-side.
  const sp = searchParams ?? {};
  const rawLang = Array.isArray(sp.lang) ? sp.lang[0] : sp.lang;
  // PG-21: seed SSR messages from ?lang= / guest cookie / Accept-Language so
  // chrome does not flash English → Spanish on hydration.
  const resolvedLocale = await resolveGuestLocaleFromRequest(rawLang);
  const lang = resolvedLocale.locale;
  const initialMessages = await loadGuestMessages(resolvedLocale.locale);
  const contentLanguage = resolvedLocale.isExplicit ? lang : undefined;

  // SEO-0.1: fetch the menu alongside the business so the crawler HTML carries
  // the full menu (dish names/descriptions/prices are the highest-intent
  // keywords a restaurant has). Parallel and resilient — a menu failure must
  // never fail the page. The business fetch stays the FIRST issued request.
  const [result, menuPayload] = await Promise.all([
    fetchStorefrontBusiness(customUrl, contentLanguage),
    fetchStorefrontMenuPayload(customUrl, contentLanguage),
  ]);
  const business = result.business
    ? {
        ...result.business,
        description: storefrontProseMatchesLocale(
          result.business.description || "",
          lang,
        )
          ? result.business.description
          : "",
        welcome_message: storefrontProseMatchesLocale(
          result.business.welcome_message || "",
          lang,
        )
          ? result.business.welcome_message
          : "",
        about_story: storefrontProseMatchesLocale(
          result.business.about_story || "",
          lang,
        )
          ? result.business.about_story
          : "",
      }
    : result.business;

  // PARITY-4: a genuinely missing business returns a real HTTP 404 (renders
  // not-found.tsx) so crawlers don't index a soft-404. A transient/unavailable
  // failure is NOT a 404 — fall through and let the client render the soft
  // "temporarily unavailable" state so a backend blip doesn't deindex a real page.
  if (result.business == null && result.reason === "not_found") {
    notFound();
  }


  // Normalize the menu payload into the exact query-data shape the client
  // hook produces. Malformed payloads degrade to "no seed" rather than 500.
  let menuSeed: StorefrontMenuQueryResult | null = null;
  if (business && menuPayload != null) {
    try {
      menuSeed = normalizeStorefrontMenuPayload(menuPayload);
    } catch {
      menuSeed = null;
    }
  }

  const baseUrl = getSiteUrl();
  const indexable = business != null && !isNonIndexableStorefront(business);

  // Best-effort Google rating for aggregateRating. Only fetched when the
  // business actually advertises Google reviews; any failure is skipped.
  const googleRating =
    business &&
    indexable &&
    business.google_reviews_enabled &&
    business.google_place_id
      ? await fetchStorefrontGoogleRating(customUrl)
      : null;

  const jsonLd =
    business && indexable
      ? buildBusinessJsonLd(business, customUrl, baseUrl, {
          googleRating,
          locale: lang,
          pagePath,
        })
      : null;
  // The root venue is the site home itself: no Home → venue breadcrumb.
  const breadcrumbJsonLd =
    business && indexable && pagePath === `/b/${customUrl}`
      ? buildBreadcrumbJsonLd([
          { name: "Home", url: baseUrl },
          {
            name: business.name,
            url: guestPublicUrl(lang, pagePath, baseUrl),
          },
        ])
      : null;

  // SEO-0.1: seed the client menu query so the storefront does not refetch on
  // mount. The key MUST mirror useBusinessPageData exactly:
  // [...queryKeys.business.menu(String(businessId)), customUrl, currentLanguage]
  // with currentLanguage === the provider's initial language
  // (resolvedLocale.locale, passed below). setQueryData stamps dataUpdatedAt=now,
  // so the hook's 5s staleTime keeps it fresh; the 5s poll resumes after mount.
  let dehydratedState: DehydratedState | null = null;
  if (business && menuSeed) {
    const seedClient = new QueryClient();
    seedClient.setQueryData(
      [...queryKeys.business.menu(String(business.id)), customUrl, lang],
      menuSeed,
    );
    dehydratedState = dehydrate(seedClient);
  }

  const client = (
    <BusinessPageClient
      customUrl={customUrl}
      initialBusiness={business}
      initialReason={result.business ? undefined : result.reason}
      initialLanguage={resolvedLocale.locale}
      initialMessages={initialMessages}
      preferInitialLanguage={resolvedLocale.isExplicit}
      initialMenuCategories={menuSeed?.categories ?? null}
    />
  );

  return (
    <>
      {jsonLd && (
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{ __html: serializeJsonLd(jsonLd) }}
        />
      )}
      {breadcrumbJsonLd && (
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{ __html: serializeJsonLd(breadcrumbJsonLd) }}
        />
      )}
      <CustomerAuthShell>
        {dehydratedState ? (
          <HydrationBoundary state={dehydratedState}>{client}</HydrationBoundary>
        ) : (
          client
        )}
      </CustomerAuthShell>
    </>
  );
}
