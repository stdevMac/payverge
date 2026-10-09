import type { Metadata } from "next";
import type { Locale } from "@/i18n/localeRegistry";

/**
 * Shared Open Graph / Twitter card images.
 *
 * SEO-4: Next.js does NOT deep-merge `openGraph`. A page or layout that
 * declares its own `openGraph` object replaces the root layout's wholesale,
 * so any page that set `{ title, description, url }` silently dropped the
 * root's `images` and shipped a bare text preview.
 *
 * REV-5: the same wholesale replace also drops root `siteName` / `type` /
 * `locale`. Use `siteOpenGraphDefaults` so page-level openGraph blocks keep
 * those site-wide fields plus images.
 *
 * Spread these into any `openGraph` / `twitter` block that overrides the root.
 *
 * Root-relative on purpose: Next resolves them against the root layout's
 * `metadataBase` (the runtime PUBLIC_URL), so a self-hosted origin never
 * advertises another deployment's share cards.
 */
export const OG_IMAGE_URL = "/share-card.png";
const TWITTER_IMAGE_URL = OG_IMAGE_URL;

/** Neutral site description: the operator's site, not product marketing. */
export const DEFAULT_SITE_DESCRIPTION =
  "Menus, ordering, reservations and payments in one place.";

// The card itself (app/share-card.png/route.ts) renders the instance name,
// logo and brand color at request time, so one URL serves every deployment.
export const defaultOpenGraphImages = [
  {
    url: OG_IMAGE_URL,
    width: 1200,
    height: 630,
    alt: "Share card",
  },
] as const;

const defaultTwitterImages = [TWITTER_IMAGE_URL] as const;

/** Site-wide OG defaults that page-level openGraph blocks must re-declare. */
export const SITE_OG_SITE_NAME = "Payverge";
export const SITE_OG_TYPE = "website" as const;
const SITE_OG_LOCALE_EN = "en_US";

type SiteOpenGraphInput = {
  url: string;
  title?: string;
  description?: string;
  locale?: string;
  alternateLocale?: string[];
  imageAlt?: string;
};

/**
 * Build a page openGraph object that preserves site-wide defaults (siteName,
 * type, locale, images) while setting the page url (and optional title/desc).
 * REV-5 — Next.js replaces root openGraph wholesale, so callers must spread
 * this rather than only `{ url, images }`.
 */
export function siteOpenGraphDefaults(
  input: SiteOpenGraphInput,
): NonNullable<Metadata["openGraph"]> {
  return {
    type: SITE_OG_TYPE,
    siteName: SITE_OG_SITE_NAME,
    locale: input.locale ?? SITE_OG_LOCALE_EN,
    url: input.url,
    images: input.imageAlt
      ? defaultOpenGraphImages.map((image) => ({ ...image, alt: input.imageAlt }))
      : [...defaultOpenGraphImages],
    ...(input.title ? { title: input.title } : {}),
    ...(input.description ? { description: input.description } : {}),
    ...(input.alternateLocale ? { alternateLocale: input.alternateLocale } : {}),
  };
}

/** Page-owned Twitter card so the root English default does not leak (#643). */
export function siteTwitterDefaults(input: {
  title?: string;
  description?: string;
}): NonNullable<Metadata["twitter"]> {
  return {
    card: "summary_large_image",
    images: [...defaultTwitterImages],
    ...(input.title ? { title: input.title } : {}),
    ...(input.description ? { description: input.description } : {}),
  };
}

const ogLocaleFor: Record<Locale, string> = {
  en: "en_US",
  es: "es_ES",
  "es-AR": "es_AR",
};

/** Derive a page-owned OG block from title + canonical so homepage OG cannot leak. */
export function pageOpenGraphFromCanonical(input: {
  title: string;
  url: string;
  description?: string;
  locale?: Locale;
}): NonNullable<Metadata["openGraph"]> {
  return siteOpenGraphDefaults({
    url: input.url,
    title: input.title,
    description: input.description,
    locale: input.locale ? ogLocaleFor[input.locale] : SITE_OG_LOCALE_EN,
  });
}
