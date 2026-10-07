"use client";

import React, {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  useEffect,
  ReactNode,
} from "react";
import { reportMissingTranslation } from "./missingTranslationReporter";
import enGuestMessages from "./guest-messages/en.json";
// sentenceCaseLeaf lives in a server-safe module so the operator getTranslation
// lookup can run during server generateMetadata(). Imported for internal use
// here and re-exported below so existing client importers keep their path.
import { sentenceCaseLeaf } from "./sentenceCaseLeaf";

import { loadApiErrorCatalog } from "./apiErrors";
import { getLocaleDirection, getLocaleFlag, localeRegistry, storefrontLocales, type StorefrontLocale } from "./localeRegistry";
import { writeGuestLocaleCookie } from "./guestLocaleResolver";
import { restoreOperatorHtmlAttributes } from "./operatorHtmlLang";

// GUEST_SUPPORTED_LANGUAGES is derived from the canonical locale registry —
// every locale that declares the `guestStorefront` required surface must ship
// a matching `guest-messages/{code}.json` bundle (enforced by
// `npm run i18n:validate`). Adding a new storefront language is a registry
// edit + new JSON bundle; nothing here changes.
export const GUEST_SUPPORTED_LANGUAGES = Object.fromEntries(
  storefrontLocales.map((code) => {
    const entry = localeRegistry[code];
    return [
      code,
      {
        name: entry.nativeName,
        flag: getLocaleFlag(code),
      },
    ];
  }),
) as Record<StorefrontLocale, { name: string; flag: string }>;

export type GuestLanguageCode = StorefrontLocale;

interface GuestTranslationContextType {
  currentLanguage: GuestLanguageCode;
  setLanguage: (language: GuestLanguageCode) => void;
  t: (key: string, params?: Record<string, string | number>) => string;
  availableLanguages: typeof GUEST_SUPPORTED_LANGUAGES;
  businessId?: number;
  setBusinessId: (id: number) => void;
}

export const GuestTranslationContext = createContext<
  GuestTranslationContextType | undefined
>(undefined);

interface GuestTranslationProviderProps {
  children: ReactNode;
  businessId?: number;
  initialLanguage?: GuestLanguageCode;
  initialMessages?: Record<string, unknown>;
  preferInitialLanguage?: boolean;
}

// Translation storage - will be loaded dynamically
const translations: Record<GuestLanguageCode, Record<string, any>> = {
  en: enGuestMessages,
} as any;

// Load translation function
const loadTranslations = async (
  language: GuestLanguageCode,
): Promise<Record<string, any>> => {
  if (translations[language]) {
    return translations[language];
  }

  try {
    const response = await import(`./guest-messages/${language}.json`);
    translations[language] = response.default;
    return translations[language];
  } catch (error) {
    console.warn(
      `Failed to load translations for ${language}, falling back to English`,
    );
    if (language !== "en") {
      return loadTranslations("en");
    }
    return {};
  }
};

// Matches an ICU plural/select block: a `{{ ... }}` wrapper whose body begins
// with `<word>, plural,` or `<word>, select,`. The captured group is the block
// body, modelled as a run of non-brace text interleaved with single-brace form
// options (`one {item}`, `other {items}`), terminated by the closing `}}`. This
// brace-aware body (rather than a lazy `[\s\S]*?`) keeps the final form option's
// closing `}` inside the body so form extraction stays correct.
const ICU_PLURAL_BLOCK_RE =
  /\{\{\s*\w+\s*,\s*(?:plural|select)\s*,((?:[^{}]|\{[^{}]*\})*)\}\}/g;
// Extracts the `other { ... }` form (preferred) or, failing that, the first
// `<keyword> { ... }` form option from inside an ICU plural/select body.
const ICU_OTHER_FORM_RE = /\bother\s*\{([^{}]*)\}/;
const ICU_ANY_FORM_RE = /\b\w+\s*\{([^{}]*)\}/;

// GUEST-4: The guest runtime has NO ICU plural/select engine — it only does
// flat `{word}` interpolation. If a translation value carries ICU plural syntax
// (e.g. `{{count, plural, one {item} other {items}}}`), the `{word}` regex below
// cannot resolve it and the raw `{{...}}` braces would leak verbatim into the
// guest UI. This guard collapses any such block to its `other` form (the runtime
// can't count-select, so it deterministically picks the plural form) BEFORE the
// flat `{word}` pass, so no `{{...}}` can ever render to a guest. It is a safety
// net; the validator and tests keep new ICU keys out of the bundles to begin with.
const stripIcuBlocks = (value: string): string =>
  value.replace(ICU_PLURAL_BLOCK_RE, (_match, body: string) => {
    const preferred = body.match(ICU_OTHER_FORM_RE);
    if (preferred) return preferred[1].trim();
    const any = body.match(ICU_ANY_FORM_RE);
    if (any) return any[1].trim();
    // No recognizable form option — drop the block rather than leak braces.
    return "";
  });

// Translation function with parameter support.
//
// Uses ICU-compatible {identifier} syntax for the simple interpolation the
// runtime supports. ICU plural/select blocks (`{{count, plural, ...}}`) are NOT
// natively resolvable — stripIcuBlocks (the GUEST-4 guard) collapses them to a
// single brace-free form so guests never see raw `{{...}}`. When a param is
// missing we preserve the original `{word}` token so regressions surface.
export const translateKey = (
  translations: Record<string, any>,
  key: string,
  params?: Record<string, any>,
): string => {
  const keys = key.split(".");
  let value: any = translations;

  for (const k of keys) {
    if (value && typeof value === "object" && k in value) {
      value = value[k];
    } else {
      return key; // Return key if translation not found
    }
  }

  if (typeof value !== "string") {
    return key;
  }

  // GUEST-4: collapse any unresolved ICU plural/select block to a brace-free
  // form before the flat-placeholder pass, so no `{{...}}` leaks to a guest.
  const resolved = stripIcuBlocks(value);

  // Replace parameters in the translation
  if (params) {
    return resolved.replace(/\{\{(\w+)\}\}|\{(\w+)\}/g, (match: string, doubleKey: string, singleKey: string) => {
      const paramKey = doubleKey || singleKey;
      return params[paramKey] !== undefined && params[paramKey] !== null
        ? params[paramKey].toString()
        : match;
    });
  }

  return resolved;
};

export function GuestTranslationProvider({
  children,
  businessId: initialBusinessId,
  initialLanguage = "en",
  initialMessages,
  preferInitialLanguage = false,
}: GuestTranslationProviderProps) {
  const [currentLanguage, setCurrentLanguage] =
    useState<GuestLanguageCode>(initialLanguage);
  const [currentTranslations, setCurrentTranslations] = useState<
    Record<string, any>
  >(() =>
    initialMessages ?? (initialLanguage === "en" ? enGuestMessages : {}),
  );
  // English is the canonical fallback layer — under-translated locales
  // resolve missing keys against this map before falling through to a
  // sentence-cased leaf. Loaded once on mount so the first non-EN
  // session doesn't pay an extra round-trip on each missing key.
  const [englishTranslations, setEnglishTranslations] = useState<
    Record<string, any>
  >(enGuestMessages);
  const [businessId, setBusinessId] = useState<number | undefined>(
    initialBusinessId,
  );

  // Load translations when language changes
  useEffect(() => {
    loadTranslations(currentLanguage).then(setCurrentTranslations);
  }, [currentLanguage]);

  // Guest API-error catalogs for the 18 non-operator locales are not in the
  // main bundle. Start the load as soon as the diner's language is known so
  // a later toast can read the catalog synchronously.
  useEffect(() => {
    void loadApiErrorCatalog(currentLanguage);
  }, [currentLanguage]);

  // Eagerly load the English manifest as the universal fallback layer
  // so missing keys never block on a dynamic import at render time.
  useEffect(() => {
    loadTranslations("en").then(setEnglishTranslations);
  }, []);

  // Sync <html lang> + dir to the selected guest language. The operator
  // dashboard cookie also writes to <html lang>; if a guest visits a public
  // /b/[slug] page after being signed into the dashboard, lang stays on
  // the operator's locale unless this effect overrides it. Direction is driven
  // by the locale registry's `direction` field (getLocaleDirection) — so any
  // future RTL storefront locale (he/fa/ur) lights up automatically without
  // touching this component. Today Arabic is the only rtl entry.
  useEffect(() => {
    if (typeof document === "undefined") return;
    document.documentElement.lang = currentLanguage;
    document.documentElement.dir = getLocaleDirection(currentLanguage);
  }, [currentLanguage]);

  // Hand <html lang> and <dir> back to the operator tier when this provider
  // unmounts (e.g. an in-SPA nav from an Arabic storefront to the dashboard).
  // The operator provider writes lang only when its own locale changes and
  // never writes dir, so without this lang="ar" dir="rtl" stayed on <html>,
  // mirroring the operator/marketing UI until a full reload. A guest-to-guest
  // nav mounts the next provider after this cleanup, which sets its own.
  useEffect(() => restoreOperatorHtmlAttributes, []);

  // Sync with business language preference if available
  useEffect(() => {
    if (preferInitialLanguage) return;
    if (businessId) {
      const savedLanguage = localStorage.getItem(
        `guest-language-${businessId}`,
      ) as GuestLanguageCode;
      if (
        savedLanguage &&
        savedLanguage in GUEST_SUPPORTED_LANGUAGES &&
        savedLanguage !== currentLanguage
      ) {
        setCurrentLanguage(savedLanguage);
      }
    }
  }, [businessId, currentLanguage, preferInitialLanguage]);

  const setLanguage = useCallback((language: GuestLanguageCode) => {
    setCurrentLanguage(language);
    // PG-12 / PG-21: persist cross-surface so reservations micro-pages and
    // the next SSR request agree with the diner's pick (distinct from the
    // operator payverge_locale cookie).
    writeGuestLocaleCookie(language);
    if (businessId) {
      localStorage.setItem(`guest-language-${businessId}`, language);
    }
    // Always notify listeners (menu refetch, skip-link chrome) even before a
    // business id is attached — the table layout mounts the provider with
    // businessId undefined until the menu page hydrates.
    window.dispatchEvent(
      new CustomEvent("guestLanguageChange", {
        detail: { language, businessId },
      }),
    );
  }, [businessId]);

  const t = useCallback((key: string, params?: Record<string, string | number>): string => {
    // Fast path: current locale has the key.
    const primary = translateKey(currentTranslations, key, params);
    if (primary !== key) {
      return primary;
    }

    // Already on English and still missing — go straight to the leaf
    // fallback and report once. Skipping the EN lookup here avoids
    // double-reporting (a missing EN key would otherwise trip both
    // branches).
    if (currentLanguage === "en") {
      reportMissingTranslation({
        locale: currentLanguage,
        key,
        fallbackUsed: "leaf",
      });
      return sentenceCaseLeaf(key);
    }

    // Translation missing from the active locale — try English.
    const englishValue = translateKey(englishTranslations, key, params);
    if (englishValue !== key) {
      reportMissingTranslation({
        locale: currentLanguage,
        key,
        fallbackUsed: "english",
      });
      return englishValue;
    }

    // Both the active locale and English are missing this key. Surface
    // a humanised version of the leaf so guests never see a raw dotted
    // identifier in the UI.
    reportMissingTranslation({
      locale: currentLanguage,
      key,
      fallbackUsed: "leaf",
    });
    return sentenceCaseLeaf(key);
  }, [currentTranslations, englishTranslations, currentLanguage]);

  const value = useMemo(
    () => ({
      currentLanguage,
      setLanguage,
      t,
      availableLanguages: GUEST_SUPPORTED_LANGUAGES,
      businessId,
      setBusinessId,
    }),
    [currentLanguage, setLanguage, t, businessId, setBusinessId],
  );

  return (
    <GuestTranslationContext.Provider value={value}>
      {children}
    </GuestTranslationContext.Provider>
  );
}

export function useGuestTranslation() {
  const context = useContext(GuestTranslationContext);
  if (context === undefined) {
    throw new Error(
      "useGuestTranslation must be used within a GuestTranslationProvider",
    );
  }
  return context;
}
