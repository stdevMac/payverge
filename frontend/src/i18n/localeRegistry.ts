import {
  defaultLocale,
  localeRegistry,
  type LocaleCode
} from './generated/locales';

export { defaultLocale, localeRegistry };
export type { LocaleCode };

type LocaleRegistry = typeof localeRegistry;
type LocaleWithFlag<Flag extends 'guestLocale' | 'operatorLocale' | 'publishable'> = {
  [L in LocaleCode]: LocaleRegistry[L][Flag] extends true ? L : never;
}[LocaleCode];
export type GuestLocale = LocaleWithFlag<'guestLocale'>;
export type OperatorLocale = LocaleWithFlag<'operatorLocale'>;

// Locales whose registry entry declares the `guestStorefront` required surface,
// meaning a matching `guest-messages/{code}.json` ships in the bundle and the
// guest UI dropdown can offer the locale to end customers. Distinct from
// GuestLocale (which marks any locale we'd accept as a menu-translation
// target — backend-only concept, may have no UI strings yet).
export type StorefrontLocale = {
  [L in LocaleCode]: 'guestStorefront' extends LocaleRegistry[L]['requiredSurfaces'][number]
    ? L
    : never;
}[LocaleCode];

// `Locale` historically meant "operator dashboard locale" — the narrow set of
// codes (en/es/es-AR) for which the operator UI ships fully translated
// messages, emails, and prompts. Code that handles the
// operator dashboard cookie, routes, and Record<Locale, ...> message maps
// expects this narrower set, so the alias preserves that contract.
//
// For the broader "any code in the canonical registry" set (which now
// includes 12 guest-only menu-translation targets) use `LocaleCode`. For
// "any locale shippable to guests" use `GuestLocale` or `StorefrontLocale`.
export type Locale = OperatorLocale;

const allLocaleCodes = Object.keys(localeRegistry) as LocaleCode[];
const localeFlagByCode: Partial<Record<LocaleCode, string>> = {
  en: '🇺🇸',
  es: '🇪🇸',
  'es-AR': '🇦🇷',
  fr: '🇫🇷',
  de: '🇩🇪',
  it: '🇮🇹',
  pt: '🇵🇹',
  zh: '🇨🇳',
  ja: '🇯🇵',
  ko: '🇰🇷',
  ar: '🇸🇦',
  ru: '🇷🇺',
  hi: '🇮🇳',
  th: '🇹🇭',
  nl: '🇳🇱',
  tr: '🇹🇷',
  vi: '🇻🇳',
  pl: '🇵🇱',
  sv: '🇸🇪',
  da: '🇩🇰',
  no: '🇳🇴',
};
// Operator-only codes — what `Locale`-typed code expects to see. Kept as the
// primary export `locales` to preserve the historical API surface (callers
// like SimpleTranslationProvider iterate this to gate dashboard locale
// switching).
export const locales = allLocaleCodes.filter(
  (locale): locale is OperatorLocale => localeRegistry[locale].operatorLocale
);
export const guestLocales = allLocaleCodes.filter(
  (locale): locale is GuestLocale => localeRegistry[locale].guestLocale
);
export const storefrontLocales = allLocaleCodes.filter(
  (locale): locale is StorefrontLocale =>
    (localeRegistry[locale].requiredSurfaces as readonly string[]).includes(
      'guestStorefront'
    )
);

const operatorLocaleSet = new Set<string>(locales);
const guestLocaleSet = new Set<string>(guestLocales);

// BCP-47 tags are case-insensitive (es-AR == es-ar == ES-AR), but our canonical
// storefront codes preserve a fixed casing ("es-AR"). The guest `?lang=` query
// param (used for the SEO/share snippet and first-paint language) and the route
// `/es-ar/` prefix must resolve identically, so map a lowercased input back to
// the canonical storefront code via this lookup — mirroring the `byLower`
// pattern in resolveGuestInitialLanguage and pathSegmentToCanonical.
const storefrontLocaleByLower = new Map<string, StorefrontLocale>(
  storefrontLocales.map((locale) => [locale.toLowerCase(), locale])
);

const pathSegmentToLocale = new Map<string, Locale>(
  locales.map((locale) => [localeRegistry[locale].pathSegment, locale])
);

// isSupportedLocale narrows to the operator-locale set — that's the
// historical behavior callers depend on (cookies, dashboard routes). For the
// broader "any registry entry" check use `allLocaleCodes` membership.
export function isSupportedLocale(
  value: string | null | undefined
): value is Locale {
  return typeof value === 'string' && operatorLocaleSet.has(value);
}

// isGuestLocale accepts any locale in the guest-locale tier — the 21-locale
// set that covers diner-facing menu translations. Use this (not
// isSupportedLocale) when persisting a user's preferred language across the
// full guest surface.
export function isGuestLocale(
  value: string | null | undefined
): value is GuestLocale {
  return typeof value === 'string' && guestLocaleSet.has(value);
}

export function canonicalToPathSegment(locale: Locale): string {
  return localeRegistry[locale].pathSegment;
}

export function pathSegmentToCanonical(segment: string): Locale | undefined {
  return pathSegmentToLocale.get(segment.toLowerCase());
}

// Normalize an incoming guest `?lang=` query value to a canonical storefront
// locale, case-insensitively (BCP-47 tags are case-insensitive). Returns the
// canonical code (e.g. "es-AR" for "es-ar"/"ES-AR"/"es-AR") when the lowercased
// input matches a storefront locale, else null. Used by both the server
// generateMetadata (SERP/share snippet) and the client first-paint resolution
// so the two agree — a lowercase "es-ar" used to fail an exact membership check
// and silently fall back to the business default.
export function normalizeGuestLangParam(
  raw: string | null | undefined
): StorefrontLocale | null {
  if (typeof raw !== 'string' || raw.length === 0) {
    return null;
  }
  return storefrontLocaleByLower.get(raw.toLowerCase()) ?? null;
}

export function getLocaleDisplayName(locale: LocaleCode): string {
  return localeRegistry[locale].nativeName;
}

export function getLocaleFlag(locale: LocaleCode): string {
  return localeFlagByCode[locale] ?? locale.toUpperCase();
}

// Compact chrome label (sidebar / guest-style pills). Flags are not languages;
// ISO-style codes stay visible when the trigger has to stay small.
export function getLocaleCodeLabel(
  locale: string | null | undefined,
): string {
  return locale ? locale.toUpperCase() : 'EN';
}

export function getLocaleDirection(locale: LocaleCode): 'ltr' | 'rtl' {
  return localeRegistry[locale].direction;
}

// Generic Latin-American Spanish tags that should land on our LatAm variant
// (es-AR) rather than Peninsular Spanish. Country-specific codes (es-MX, es-CL,
// …) intentionally fall through to the neutral `es` base for now — es-AR is the
// only LatAm Spanish we ship, and imposing voseo on every LatAm country is a
// product decision for when more variants exist.
const LATAM_GENERIC_SPANISH = new Set(['es-419']);

// BCP-47 tags are case-insensitive (es-AR == es-ar == ES-AR), but our canonical
// codes preserve a fixed casing ("es-AR"). Browsers usually send canonical case,
// but not always — match case-insensitively via a lowercase→canonical lookup so
// a lowercase "es-ar" or uppercase "ES-419" still resolves.
const operatorLocaleByLower = new Map<string, Locale>(
  locales.map((locale) => [locale.toLowerCase(), locale])
);

export function resolveLocaleForBrowser(
  browserLanguages: readonly string[]
): Locale {
  // Honour the browser's preference ORDER: the first entry that yields a
  // supported locale wins. Within a single entry we try, in order of
  // specificity: exact operator match, then the LatAm-generic → es-AR mapping
  // (so "es-419" lands on es-AR, not Peninsular es), then the base language
  // ("es-MX" -> es). The LatAm check sits between exact and base so a generic
  // "es-419" still beats its own "es" base — without letting a later, lower
  // preference (e.g. "en") jump ahead of an earlier Spanish tag.
  for (const browserLanguage of browserLanguages) {
    const lower = browserLanguage.trim().toLowerCase();
    if (!lower) continue;

    const exact = operatorLocaleByLower.get(lower);
    if (exact) {
      return exact;
    }

    if (LATAM_GENERIC_SPANISH.has(lower) && isSupportedLocale('es-AR')) {
      return 'es-AR';
    }

    const baseMatch = operatorLocaleByLower.get(lower.split('-')[0]);
    if (baseMatch) {
      return baseMatch;
    }
  }

  return defaultLocale;
}

// Pick the initial guest-storefront language for a diner, choosing only among
// the languages the business has actually enabled. Layered: a saved per-business
// preference wins, then the browser's language list (Argentine/LatAm Spanish
// preferring es-AR when offered, any Spanish degrading to es-AR rather than
// leaking to English), then the business default, then English. Unlike the
// operator dashboard, the guest tier had no browser detection at all — a diner
// scanning a QR with an Argentine phone used to get English.
export function resolveGuestInitialLanguage(opts: {
  saved?: string | null;
  browserLanguages?: readonly string[];
  enabled: readonly string[];
  businessDefault?: string | null;
}): string {
  // Case-insensitive lookup into the business's enabled set (BCP-47 tags are
  // case-insensitive; enabled codes carry canonical casing like "es-AR").
  const byLower = new Map(opts.enabled.map((code) => [code.toLowerCase(), code]));
  const resolve = (code: string | null | undefined): string | undefined =>
    typeof code === 'string' ? byLower.get(code.toLowerCase()) : undefined;

  // Keep a known storefront regional locale (es-AR) when only the base
  // language (es) is enabled. Menu rows fall back es-AR → es on the backend;
  // collapsing chrome to generic es — or discarding the pick and leaking to
  // EN — is the storefront bug in issue #386.
  const keepRegionalStorefront = (
    code: string | null | undefined,
  ): StorefrontLocale | undefined => {
    const storefront = normalizeGuestLangParam(code);
    if (!storefront || !storefront.includes('-')) return undefined;
    const base = storefront.split('-')[0]?.toLowerCase();
    if (base && byLower.has(base)) {
      return storefront;
    }
    return undefined;
  };

  // 1. A saved per-business preference wins if the business still offers it
  //    or it is a regional variant of an enabled language.
  const saved = resolve(opts.saved) ?? keepRegionalStorefront(opts.saved);
  if (saved) {
    return saved;
  }

  // 2. Browser languages, matched against what the business actually offers.
  for (const raw of opts.browserLanguages ?? []) {
    const lower = raw.trim().toLowerCase();
    if (!lower) continue;

    const exact = byLower.get(lower); // exact, e.g. "es-AR" or "pt"
    if (exact) return exact;

    if (LATAM_GENERIC_SPANISH.has(lower) && byLower.has('es-ar')) {
      return byLower.get('es-ar')!;
    }

    const regional = keepRegionalStorefront(raw);
    if (regional) return regional;

    const base = lower.split('-')[0];
    if (base === 'es') {
      // Any Spanish browser: prefer neutral es, else the Argentine menu if
      // that's the only Spanish on offer — never leak a Spanish speaker to
      // English when the business has a Spanish variant available.
      if (byLower.has('es')) return byLower.get('es')!;
      if (byLower.has('es-ar')) return byLower.get('es-ar')!;
      continue;
    }

    const baseMatch = byLower.get(base);
    if (baseMatch) return baseMatch; // e.g. "pt-BR" -> "pt"
  }

  // 3. Business default, else English.
  const fallback = resolve(opts.businessDefault);
  if (fallback) {
    return fallback;
  }
  return 'en';
}

/** Locales to try when reading stored menu translations, most specific first. */
export function guestMenuTranslationFallbackChain(code: string): string[] {
  const canonical = normalizeGuestLangParam(code) ?? code.trim();
  if (!canonical) return [];
  const chain = [canonical];
  const base = canonical.split('-')[0];
  if (base && base !== canonical && isGuestLocale(base)) {
    chain.push(base);
  }
  return chain;
}

/** Language the table/storefront menu should request from the guest API. */
export function resolveGuestMenuLanguage(opts: {
  saved?: string | null;
  currentLanguage?: string | null;
  businessDefault?: string | null;
}): string {
  // Live provider/URL locale wins over a stale per-business localStorage
  // value so `?lang=es-AR` still fetches es-AR after an older EN visit.
  return (
    normalizeGuestLangParam(opts.currentLanguage) ??
    normalizeGuestLangParam(opts.saved) ??
    normalizeGuestLangParam(opts.businessDefault) ??
    'en'
  );
}

/** After the first menu locale is applied, follow later provider changes. */
export function nextGuestMenuLanguage(
  currentLanguage: string | undefined,
  selectedLanguage: string | undefined,
): string | null {
  const current = normalizeGuestLangParam(currentLanguage);
  if (!current) return null;
  if (!selectedLanguage) return current;
  if (current === selectedLanguage) return null;
  return current;
}
