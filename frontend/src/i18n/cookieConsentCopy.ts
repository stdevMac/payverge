/**
 * Provider-free cookie-banner copy for all storefront locales (NEW-3).
 *
 * CookieConsent sits in the root layout above GuestTranslationProvider, so
 * it cannot use useGuestTranslation(). Resolve locale from the guest cookie /
 * ?lang= / path, then load the slim cookie-consent catalog for that locale.
 * English is bundled statically; the other storefront locales load on demand.
 */

import {
  GUEST_LOCALE_COOKIE,
  resolveGuestRequestLocale,
} from "./guestLocaleResolver";
import { defaultLocale } from "./localeRegistry";

import enCookies from "./cookie-consent/en.json";

export type CookieBannerCopy = {
  title: string;
  description: string;
  acceptAll: string;
  declineAll: string;
  customize: string;
  essentialLabel: string;
  essentialDescription: string;
  analyticsLabel: string;
  analyticsDescription: string;
  marketingLabel: string;
  marketingDescription: string;
  savePreferences: string;
  privacyLink: string;
  closeLabel: string;
  preferences: string;
};

type CookieTree = {
  banner: Omit<CookieBannerCopy, "preferences">;
  footer: { preferences: string };
};

type CookieTreeModule = { default: CookieTree };

const enTree = enCookies as CookieTree;

const loadedCopy = new Map<string, CookieBannerCopy>();

const cookieConsentLoaders: Record<string, () => Promise<CookieTreeModule>> = {
  ar: () => import("./cookie-consent/ar.json"),
  da: () => import("./cookie-consent/da.json"),
  de: () => import("./cookie-consent/de.json"),
  es: () => import("./cookie-consent/es.json"),
  "es-AR": () => import("./cookie-consent/es-AR.json"),
  fr: () => import("./cookie-consent/fr.json"),
  hi: () => import("./cookie-consent/hi.json"),
  it: () => import("./cookie-consent/it.json"),
  ja: () => import("./cookie-consent/ja.json"),
  ko: () => import("./cookie-consent/ko.json"),
  nl: () => import("./cookie-consent/nl.json"),
  no: () => import("./cookie-consent/no.json"),
  pl: () => import("./cookie-consent/pl.json"),
  pt: () => import("./cookie-consent/pt.json"),
  ru: () => import("./cookie-consent/ru.json"),
  sv: () => import("./cookie-consent/sv.json"),
  th: () => import("./cookie-consent/th.json"),
  tr: () => import("./cookie-consent/tr.json"),
  vi: () => import("./cookie-consent/vi.json"),
  zh: () => import("./cookie-consent/zh.json"),
};

function flattenCookies(tree: CookieTree): CookieBannerCopy {
  return {
    ...tree.banner,
    preferences: tree.footer.preferences,
  };
}

function treeFromModule(mod: CookieTreeModule): CookieTree | undefined {
  const tree =
    mod && typeof mod.default === "object" && mod.default
      ? mod.default
      : (mod as unknown as CookieTree);
  if (!tree?.banner || typeof tree.footer?.preferences !== "string") {
    return undefined;
  }
  return tree;
}

/** True for diner surfaces that own the 21-locale guest tier. */
export function isGuestSurfacePath(pathname: string): boolean {
  return /^\/(t|b|scan)(\/|$)/i.test(pathname);
}

function readCookie(name: string): string | null {
  if (typeof document === "undefined") return null;
  const match = document.cookie
    .split(";")
    .map((c) => c.trim())
    .find((c) => c.startsWith(`${name}=`));
  if (!match) return null;
  return decodeURIComponent(match.slice(name.length + 1));
}

/**
 * Locale for the cookie banner:
 * - guest surfaces → guest cookie / ?lang= / Accept-Language
 * - operator/marketing → operator SimpleTranslation locale (passed in)
 */
export function resolveCookieConsentLocale(
  operatorLocale: string,
  pathname?: string,
): string {
  const path =
    pathname ??
    (typeof window !== "undefined" ? window.location.pathname : "");
  if (!isGuestSurfacePath(path)) {
    return operatorLocale || defaultLocale;
  }
  const langParam =
    typeof window !== "undefined"
      ? new URLSearchParams(window.location.search).get("lang")
      : null;
  const guestCookie = readCookie(GUEST_LOCALE_COOKIE);
  return resolveGuestRequestLocale({
    langParam,
    guestCookie,
    acceptLanguage:
      typeof navigator !== "undefined" ? navigator.language : null,
  }).locale;
}

/** Sync English banner copy. Used when a locale is unknown or fails to load. */
function englishCookieConsentCopy(): CookieBannerCopy {
  return flattenCookies(enTree);
}

/**
 * Load banner copy for the resolved locale. Unknown locales and failed loads
 * fall back to English.
 */
export async function loadCookieConsentCopy(
  operatorLocale: string,
  pathname?: string,
): Promise<{ locale: string; copy: CookieBannerCopy }> {
  const locale = resolveCookieConsentLocale(operatorLocale, pathname);
  const cached = loadedCopy.get(locale);
  if (cached) {
    return { locale, copy: cached };
  }

  const englishResult = (): { locale: string; copy: CookieBannerCopy } => {
    const existing = loadedCopy.get(defaultLocale);
    if (existing) return { locale: defaultLocale, copy: existing };
    const copy = englishCookieConsentCopy();
    loadedCopy.set(defaultLocale, copy);
    return { locale: defaultLocale, copy };
  };

  if (locale === defaultLocale) {
    return englishResult();
  }

  const loader = cookieConsentLoaders[locale];
  if (!loader) {
    return englishResult();
  }

  try {
    const tree = treeFromModule(await loader());
    if (!tree) return englishResult();
    const copy = flattenCookies(tree);
    loadedCopy.set(locale, copy);
    return { locale, copy };
  } catch {
    return englishResult();
  }
}

/** Required dotted keys under guest-messages.*.cookies (for locale guards). */
export const COOKIE_CONSENT_GUEST_KEYS = [
  "cookies.banner.title",
  "cookies.banner.description",
  "cookies.banner.acceptAll",
  "cookies.banner.declineAll",
  "cookies.banner.customize",
  "cookies.banner.essentialLabel",
  "cookies.banner.essentialDescription",
  "cookies.banner.analyticsLabel",
  "cookies.banner.analyticsDescription",
  "cookies.banner.marketingLabel",
  "cookies.banner.marketingDescription",
  "cookies.banner.savePreferences",
  "cookies.banner.privacyLink",
  "cookies.banner.closeLabel",
  "cookies.footer.preferences",
] as const;
