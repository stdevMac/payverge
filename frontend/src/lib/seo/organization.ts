// Site-wide structured data: Organization and WebSite.
// Injected by src/app/layout.tsx so brand-level signals (name, logo, social
// profiles, languages) live in one place.
//
// Built per request from the runtime canonical origin (PUBLIC_URL) and the
// instance identity (GET /api/v1/instance), so one image serves any
// deployment and a self-hosted venue never claims to be the upstream product.

const ORGANIZATION_LANGUAGES = [
  "English",
  "Spanish",
  "Spanish (Argentina)",
] as const;

/** Bundled fallback logo, used when the instance sets no LOGO_URL. */
const DEFAULT_LOGO_PATH = "/images/logo.webp";

export interface OrganizationJsonLdOptions {
  /** Public name of the site (company → product); see siteNameOf. */
  name: string;
  /** Registered legal entity, emitted as legalName when it differs. */
  legalName?: string;
  /** Instance LOGO_URL: absolute http(s) or a same-origin path. */
  logoUrl?: string;
  /** Optional Twitter/X handle (no @); omitted from sameAs when empty. */
  twitterHandle?: string;
}

function trimOrigin(siteUrl: string): string {
  return siteUrl.replace(/\/+$/, "");
}

function absoluteLogo(origin: string, logoUrl: string | undefined): string {
  const logo = logoUrl?.trim();
  if (!logo) return `${origin}${DEFAULT_LOGO_PATH}`;
  return logo.startsWith("/") ? `${origin}${logo}` : logo;
}

export function buildOrganizationJsonLd(
  siteUrl: string,
  options: OrganizationJsonLdOptions,
) {
  const origin = trimOrigin(siteUrl);
  // Public brand profiles only. The former parent-brand LinkedIn company URL
  // was removed from sameAs (#36) — do not reintroduce stale parent-brand links.
  const sameAs = options.twitterHandle
    ? [`https://twitter.com/${options.twitterHandle}`]
    : [];
  const legalName =
    options.legalName && options.legalName !== options.name
      ? { legalName: options.legalName }
      : {};
  return {
    "@context": "https://schema.org",
    "@type": "Organization",
    name: options.name,
    ...legalName,
    url: origin,
    logo: absoluteLogo(origin, options.logoUrl),
    // Languages live at the Organization root (#866 / #813); es-AR is a
    // first-class locale (lang=es-AR, sitemap hreflang).
    inLanguage: ["en", "es", "es-AR"],
    availableLanguage: [...ORGANIZATION_LANGUAGES],
    sameAs,
  };
}

export function buildWebsiteJsonLd(siteUrl: string, name: string) {
  return {
    "@context": "https://schema.org",
    "@type": "WebSite",
    name,
    url: trimOrigin(siteUrl),
  } as const;
}

export interface SiteStructuredDataOptions extends OrganizationJsonLdOptions {
  /**
   * GET /api/v1/home mode. In "venue" mode the site is one restaurant whose
   * Restaurant node sits at "/", so no competing Organization is emitted for
   * the same origin.
   */
  homeMode?: string | null;
}

/** The site-wide nodes the root layout injects: [Organization?, WebSite]. */
export function buildSiteStructuredData(
  siteUrl: string,
  options: SiteStructuredDataOptions,
) {
  const { homeMode, ...org } = options;
  return {
    organization:
      homeMode === "venue" ? null : buildOrganizationJsonLd(siteUrl, org),
    website: buildWebsiteJsonLd(siteUrl, org.name),
  };
}
