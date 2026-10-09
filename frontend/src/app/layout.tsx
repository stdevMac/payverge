// RootLayout.tsx
import localFont from "next/font/local";
import type { Metadata } from "next";
import { headers } from "next/headers";
import { Providers } from "./providers";
import "./globals.css";

import { AppQueryProvider } from "@/providers/AppQueryProvider";
import { Toaster } from "react-hot-toast";
import { SimpleTranslationProvider } from "@/i18n/OperatorLocaleProvider";
import FloatingLanguageSwitcherLazy from "@/components/FloatingLanguageSwitcherLazy";
import { SkipToMainLink } from "@/components/SkipToMainLink";
import { CookieConsentProvider } from "@/contexts/CookieConsentContext";
import { CookieConsent } from "@/components/CookieConsent";
import WithMaintenance from "@/components/WithMaintenance";
import { buildSiteStructuredData } from "@/lib/seo/organization";
import { getSiteUrl, serializePublicEnvScript } from "@/config/publicConfig";
import { getTwitterHandle, isSeoIndexingEnabled } from "@/config/serverConfig";
import { serializeJsonLd } from "@/lib/seo/jsonLd";
import {
  DEFAULT_SITE_DESCRIPTION,
  OG_IMAGE_URL,
} from "@/lib/seo/openGraphImages";
import {
  resolveHtmlLang,
  resolveHtmlDir,
  resolveOperatorLocale,
} from "@/i18n/htmlLangDir";
import { pwaManifestHref } from "@/pwa/manifestHref";
import { getServerInstanceInfo } from "@/lib/instance/serverInstance";
import InstanceProvider from "@/components/instance/InstanceProvider";
import { getServerHome } from "@/lib/instance/serverHome";
import DemoBanner from "@/components/demo/DemoBanner";
import {
  brandRgbChannels,
  legalNameOf,
  serializeInstanceScript,
  siteNameOf,
} from "@/lib/instance/instanceInfo";

// Self-hosted (OFL) so a build never reaches Google Fonts: the same latin
// subset files public/fonts serves to print windows and the preview frame.
// Only latin is loaded (and preloaded) here; globals.css puts the latin-ext
// faces at the front of --font-inter / --font-title so pl, tr, cs, ro … glyphs
// still render in DM Sans / DM Serif Display.
const dmSerif = localFont({
  src: [
    {
      path: "../../public/fonts/dm-serif-display/dm-serif-display-regular-latin.woff2",
      weight: "400",
      style: "normal",
    },
    {
      path: "../../public/fonts/dm-serif-display/dm-serif-display-italic-latin.woff2",
      weight: "400",
      style: "italic",
    },
  ],
  variable: "--font-dm-serif-display",
  display: "swap",
  fallback: ["Georgia", "serif"],
});

const dmSans = localFont({
  src: "../../public/fonts/dm-sans/dm-sans-latin.woff2",
  weight: "400 700",
  style: "normal",
  variable: "--font-dm-sans",
  display: "swap",
  fallback: ["system-ui", "sans-serif"],
});

const DEFAULT_DESCRIPTION = DEFAULT_SITE_DESCRIPTION;

function buildMetadata(
  locale: "en" | "es" | "es-ar",
  siteUrl: string = getSiteUrl(),
  // COMPANY_NAME → PRODUCT_NAME from GET /api/v1/instance (siteNameOf); the
  // upstream name by default.
  siteName: string = siteNameOf(null),
): Metadata {
  // These are the instance-wide defaults every route inherits. The site is
  // the operator's (a venue, a group of venues), not the upstream product, so
  // the defaults carry the instance name and neutral copy, never product
  // marketing.
  const name = siteName;
  // Canonical origin and indexing policy are runtime settings (PUBLIC_URL,
  // SEO_INDEXING, SEO_TWITTER_HANDLE) so one image serves any deployment. A
  // fresh self-hosted instance is noindex until SEO_INDEXING=true.
  const indexable = isSeoIndexingEnabled();
  const twitterHandle = getTwitterHandle();
  // Distinguish Argentine Spanish for og:locale so an es-AR page doesn't
  // advertise itself as es_ES (Spain) while <html lang> says es-AR.
  const ogLocale =
    locale === "es-ar" ? "es_AR" : locale === "es" ? "es_ES" : "en_US";
  return {
    title: name,
    description: DEFAULT_DESCRIPTION,
    authors: [{ name }],
    creator: name,
    publisher: name,
    metadataBase: new URL(siteUrl),
    // Do not set a site-root canonical here — soft metadata fallbacks would
    // inherit the site root and de-index other pages (#18). The home page
    // sets its own canonical.
    openGraph: {
      type: "website",
      locale: ogLocale,
      url: siteUrl,
      title: name,
      description: DEFAULT_DESCRIPTION,
      siteName: name,
      images: [
        {
          url: OG_IMAGE_URL,
          width: 1200,
          height: 630,
          alt: name,
        },
      ],
    },
    twitter: {
      card: "summary_large_image",
      title: name,
      description: DEFAULT_DESCRIPTION,
      images: [OG_IMAGE_URL],
      ...(twitterHandle ? { creator: `@${twitterHandle}` } : {}),
    },
    robots: {
      index: indexable,
      follow: indexable,
      googleBot: {
        index: indexable,
        follow: indexable,
        "max-video-preview": -1,
        "max-image-preview": "large",
        "max-snippet": -1,
      },
    },
    icons: {
      icon: [
        { url: "/favicon.ico" },
        { url: "/favicon-16x16.png", sizes: "16x16", type: "image/png" },
        { url: "/favicon-32x32.png", sizes: "32x32", type: "image/png" },
      ],
      apple: [
        { url: "/apple-touch-icon.png", sizes: "180x180", type: "image/png" },
      ],
    },
    manifest: pwaManifestHref(locale),
    other: {
      "payverge-build": `${process.env.NEXT_PUBLIC_RELEASE_SHA ?? "dev"}@${process.env.NEXT_PUBLIC_VERSION ?? "unknown"}`,
    },
  };
}

export async function generateMetadata(): Promise<Metadata> {
  const requestHeaders = await headers();
  const localeHeader = requestHeaders.get("x-payverge-locale");
  // Collapse to the operator metadata tier (en/es/es-AR). A guest storefront
  // ?lang= flow (e.g. ?lang=ar) degrades to en here — guest pages own their own
  // SEO via generateMetadata in the /b and /t routes. es-AR shares the Spanish
  // metadata copy but keeps its own og:locale (es_AR), hence the lowercase form.
  const operatorLocale = resolveOperatorLocale(localeHeader);
  const locale: "en" | "es" | "es-ar" =
    operatorLocale === "es-AR"
      ? "es-ar"
      : operatorLocale === "es"
        ? "es"
        : "en";
  const instance = await getServerInstanceInfo();
  return buildMetadata(locale, getSiteUrl(), siteNameOf(instance));
}

export default async function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const requestHeaders = await headers();
  const localeHeader = requestHeaders.get("x-payverge-locale");
  // <html lang>/<dir> are derived from the registry so SSR first paint is
  // correct even for guest storefront ?lang= flows (e.g. /b/slug?lang=ar emits
  // lang="ar" dir="rtl") — no longer LTR-until-hydration. es-ar normalises to
  // es-AR; unknown codes default to en/ltr.
  const htmlLang = resolveHtmlLang(localeHeader);
  const htmlDir = resolveHtmlDir(localeHeader);
  // SimpleTranslationProvider + metadata stay on the operator tier (en/es/es-AR);
  // a guest-only storefront code (ar, fr, …) degrades to en here so the operator
  // dashboard provider is never seeded with strings it doesn't ship.
  const locale = resolveOperatorLocale(localeHeader);
  const nonce = requestHeaders.get("x-nonce") || "";

  // Runtime public config (whitelisted keys only, JSON-escaped against
  // </script>) must execute before any client module reads getPublicConfig(),
  // so it is the first body script. instrumentation-client may still run
  // earlier (async main chunk) and waits for it via onPublicEnvReady.
  const publicEnvScript = serializePublicEnvScript();
  // GET /api/v1/instance, seeded for useInstance() so gated UI renders in its
  // final state on first paint (null → the hook fetches it itself).
  const instance = await getServerInstanceInfo();
  const instanceScript = serializeInstanceScript(instance);
  const brandRgb = brandRgbChannels(instance);
  // Structured data derives from the operator-configured PUBLIC_URL, not user
  // input; serializeJsonLd sanitizes via JSON.stringify. The nonce attribute
  // is required so these inline scripts pass the nonce-based CSP.
  // Both nodes carry the instance identity, never the upstream brand. When
  // the site is a single venue (GET /api/v1/home mode "venue") the venue's
  // Restaurant node at "/" is the entity, so no competing Organization is
  // emitted for the same origin.
  const home = await getServerHome();
  const structuredData = buildSiteStructuredData(getSiteUrl(), {
    name: siteNameOf(instance),
    legalName: legalNameOf(instance),
    logoUrl: instance?.logo_url,
    twitterHandle: getTwitterHandle(),
    homeMode: home?.mode,
  });
  const orgJsonLd = structuredData.organization
    ? serializeJsonLd(structuredData.organization)
    : null;
  const siteJsonLd = serializeJsonLd(structuredData.website);
  const buildInfo = `window.__PAYVERGE_BUILD__=${JSON.stringify({
    sha: process.env.NEXT_PUBLIC_RELEASE_SHA ?? "dev",
    version: process.env.NEXT_PUBLIC_VERSION ?? "unknown",
    builtAt: process.env.NEXT_PUBLIC_BUILD_TIMESTAMP ?? "unknown",
  })};`;

  return (
    <html
      lang={htmlLang}
      dir={htmlDir}
      className={`${dmSerif.variable} ${dmSans.variable}`}
      style={
        brandRgb
          ? ({ "--pv-brand-rgb": brandRgb } as React.CSSProperties)
          : undefined
      }
      suppressHydrationWarning
    >
      <body className="font-sans antialiased" suppressHydrationWarning={true}>
        {/* Next's Metadata API exclusively owns the document head. Keeping
            these scripts in an explicit root head made streamed route metadata
            hydrate nondeterministically: the server streamed a title after the
            head had closed, and the client could remove it without installing
            it in document.head.
            JSON-LD is valid in body and all inline scripts retain the request
            nonce required by the CSP. */}
        <script
          nonce={nonce}
          suppressHydrationWarning
          dangerouslySetInnerHTML={{ __html: publicEnvScript }}
        />
        <script
          nonce={nonce}
          suppressHydrationWarning
          dangerouslySetInnerHTML={{ __html: instanceScript }}
        />
        {orgJsonLd ? (
          <script
            nonce={nonce}
            suppressHydrationWarning
            type="application/ld+json"
            dangerouslySetInnerHTML={{ __html: orgJsonLd }}
          />
        ) : null}
        <script
          nonce={nonce}
          suppressHydrationWarning
          type="application/ld+json"
          dangerouslySetInnerHTML={{ __html: siteJsonLd }}
        />
        <script
          nonce={nonce}
          suppressHydrationWarning
          dangerouslySetInnerHTML={{ __html: buildInfo }}
        />
        {/* Toaster defaults raised to match the premium-fintech surface:
            top-center reads more like a Stripe/Linear notification banner
            than the top-right cluster, and the brand-tinted border + warm
            background keeps the toast from clashing with the cream canvas.
            Success/error get explicit colors so green is reserved for "the
            money moved" affirmations and red is reserved for hard failures. */}
        {/* eslint-disable no-restricted-syntax -- runtime style object passed
            to react-hot-toast; tokens here mirror tailwind warm/ink/brand
            scales and cannot live as class names because the library
            consumes inline styles. */}
        <Toaster
          position="top-center"
          gutter={10}
          toastOptions={{
            duration: 3500,
            style: {
              background: "#ffffff",
              color: "#1c1917", // ink-950
              border: "1px solid rgba(0,0,0,0.06)",
              boxShadow: "0 10px 30px rgba(28, 25, 23, 0.10)",
              borderRadius: "12px",
              padding: "12px 14px",
              fontSize: "0.9rem",
              fontWeight: 500,
              maxWidth: "440px",
            },
            success: {
              iconTheme: { primary: "#1a6b6a", secondary: "#ffffff" },
            }, // brand teal
            error: { iconTheme: { primary: "#be123c", secondary: "#ffffff" } }, // rose-700
          }}
        />
        {/* eslint-enable no-restricted-syntax */}
        <InstanceProvider value={instance}>
          <SimpleTranslationProvider initialLocale={locale}>
            <SkipToMainLink />
            <DemoBanner />
            <CookieConsentProvider>
              <WithMaintenance>
                {/* React Query only. wagmi mounts in the (shop) layout so diner
                  routes never download the wallet stack. */}
                <AppQueryProvider>
                  <Providers>{children}</Providers>
                </AppQueryProvider>
              </WithMaintenance>
              <FloatingLanguageSwitcherLazy />
              {/* Global consent banner. Renders only on first visit (no stored
                choice) and after client hydration — see CookieConsent.tsx. The
                provider also wraps the floating switcher so the footer/banner
                share one consent context. */}
              <CookieConsent />
            </CookieConsentProvider>
          </SimpleTranslationProvider>
        </InstanceProvider>
      </body>
    </html>
  );
}
