"use client";

// OPERATOR-TIER translation provider (catalog-free half; the
// ./SimpleTranslationProvider barrel adds getTranslation). Despite the
// historical "Simple" name,
// this is THE canonical provider for the operator dashboard, mounted at the app
// root (src/app/layout.tsx) and consumed by ~all components/business/* surfaces.
// Its `Locale` type is `OperatorLocale` = exactly { en, es, es-AR } — the three
// `operatorLocale: true` entries in the generated registry. It is NOT an
// "en/es-only fallback": es-AR reuses the es message base by design.
//
// The broader 21-locale set is the GUEST tier (guest-messages/*.json), served
// by GuestTranslationProvider for diner-facing storefront/menu — do not confuse
// the two. See OperatorTranslationProvider alias at the bottom of this file and
// the guard in __tests__/operator-provider-guard.test.ts.

import React, { createContext, useContext, useState, ReactNode } from "react";
import { isGuestPath } from "@/utils/guestPath";
import { setOperatorHtmlLang } from "./operatorHtmlLang";
import {
  canonicalToPathSegment,
  defaultLocale,
  isSupportedLocale,
  pathSegmentToCanonical,
  type Locale,
} from "./localeRegistry";
import {
  isUnprefixedOperatorAppPath,
  isUnprefixedPublicPagePath,
  OPERATOR_LOCALE_COOKIE,
  preferStoredOperatorLocale,
} from "@/utils/requestLocale";
// No message catalog here: the full operator tree lives in ./getTranslation
// and is re-exported only by the ./SimpleTranslationProvider barrel. Root
// layout chrome imports this module directly so diner routes never download
// the operator catalog (they use ./operatorChromeCatalog instead).

function persistOperatorLocaleCookie(locale: Locale): void {
  if (typeof document === "undefined" || typeof window === "undefined") return;
  const secure = window.location.protocol === "https:" ? "; Secure" : "";
  document.cookie = `${OPERATOR_LOCALE_COOKIE}=${locale}; Max-Age=31536000; Path=/; SameSite=Lax${secure}`;
}

/**
 * Record an explicit operator locale pick, without touching render state or the
 * URL. `setLocale` routes through here, but a pick that MATCHES the locale
 * already on screen never reaches `setLocale` at all — SimpleLanguageSwitcher
 * returns early — and on operator app paths the init effect below is
 * contractually forbidden from writing `locale` (see the #617 guard there).
 * So the store stayed empty on the dashboard, resolveBackendLocaleSeed read
 * that as "the operator has no opinion", and the account's saved
 * `language_selected` was applied over the English the operator had just
 * picked. Persisting the no-op pick is what makes the choice win (#617).
 */
export function persistOperatorLocaleChoice(locale: Locale): void {
  if (typeof window === "undefined") return;
  try {
    localStorage.setItem("locale", locale);
  } catch (error) {
    console.warn("Error saving locale to localStorage:", error);
  }
  persistOperatorLocaleCookie(locale);
}

// Locale RESOLUTION only: storefront and table pages keep the saved operator
// locale instead of reading a route prefix. Deliberately narrower than
// isGuestPath — the instance root ("/es") still takes its locale from the
// prefix. Whether this provider may write <html lang> uses isGuestPath.
function isStorefrontLocalePath(pathname: string): boolean {
  return pathname.startsWith("/b/") || pathname.startsWith("/t/");
}

export function resolveRuntimeLocale(
  pathname: string,
  savedLocale: string | null,
): Locale {
  if (!isStorefrontLocalePath(pathname)) {
    const firstSegment = pathname.split("/").filter(Boolean)[0];
    const routeLocale = firstSegment
      ? pathSegmentToCanonical(firstSegment)
      : undefined;
    if (routeLocale) {
      return routeLocale;
    }
    // Unprefixed public pages are English by path contract (#37).
    // Do not let a stale cookie/localStorage paint Spanish onto them.
    if (isUnprefixedPublicPagePath(pathname)) {
      return defaultLocale;
    }
  }

  return isSupportedLocale(savedLocale) ? savedLocale : defaultLocale;
}

/**
 * Navigate to the same public path under another locale prefix: rewrite the
 * `/es` / `/es-ar` prefix (#13). Returns undefined for surfaces that keep the
 * locale in a cookie instead of the URL.
 */
export function getPublicLocaleSwitchHref({
  path,
  targetLocale,
  search = "",
  hash = "",
}: {
  path: string;
  targetLocale: Locale;
  search?: string;
  hash?: string;
}): string | undefined {
  const segments = path.split("/").filter(Boolean);
  const prefixedPathSegment = segments[0] ?? "";
  const prefixedLocale = pathSegmentToCanonical(prefixedPathSegment);
  const currentLocale = prefixedLocale ?? defaultLocale;
  if (currentLocale === targetLocale) {
    return undefined;
  }

  // Only rewrite shareable public URLs. Dashboard and authenticated surfaces
  // keep a cookie preference without a path change.
  const unprefixedPath = prefixedLocale
    ? path.slice(`/${prefixedPathSegment}`.length) || "/"
    : path;
  if (!isUnprefixedPublicPagePath(unprefixedPath) && !prefixedLocale) {
    return undefined;
  }

  const segment = canonicalToPathSegment(targetLocale);
  const rewritten =
    targetLocale === defaultLocale
      ? unprefixedPath
      : `/${segment}${unprefixedPath === "/" ? "" : unprefixedPath}`;

  return `${rewritten}${search}${hash}`;
}

interface SimpleTranslationContextType {
  locale: Locale;
  setLocale: (locale: Locale) => void;
}

const SimpleTranslationContext = createContext<
  SimpleTranslationContextType | undefined
>(undefined);

export function SimpleTranslationProvider({
  children,
  initialLocale,
}: {
  children: ReactNode;
  // Locale resolved server-side from the x-payverge-locale request header. Seeds
  // the first render so SSR HTML (what crawlers and the no-JS first paint see)
  // matches the route's language instead of always defaulting to English.
  initialLocale?: Locale;
}) {
  const [locale, setLocaleState] = useState<Locale>(() => {
    if (typeof window === "undefined") return initialLocale ?? defaultLocale;
    try {
      const storedWin = preferStoredOperatorLocale({
        pathname: window.location.pathname,
        storedLocale: localStorage.getItem("locale"),
        initialLocale,
      });
      if (storedWin) return storedWin;
    } catch {
      // Storage unavailable — fall through to the request locale.
    }
    return initialLocale ?? defaultLocale;
  });

  // Initialize locale from localStorage on client side
  React.useEffect(() => {
    if (typeof window !== "undefined") {
      try {
        if (initialLocale) {
          const savedLocale = localStorage.getItem("locale");
          const storedWin = preferStoredOperatorLocale({
            pathname: window.location.pathname,
            storedLocale: savedLocale,
            initialLocale,
          });
          if (storedWin) {
            // Operator dashboard: an explicit in-session pick (English login)
            // must not flip to Accept-Language es/es-AR on tab navigation (#617).
            setLocaleState(storedWin);
            persistOperatorLocaleCookie(storedWin);
            return;
          }
          // Do not persist an Accept-Language SSR locale onto operator chrome.
          // Writing it here is what locked Argentine AL into storage so the
          // next tab's RSC painted Spanish over an English login (#617).
          if (isUnprefixedOperatorAppPath(window.location.pathname)) {
            return;
          }
          // Prefixed /es and marketing entries still plant the operator cookie
          // so unprefixed auth/register routes stay Spanish after refresh.
          localStorage.setItem("locale", initialLocale);
          persistOperatorLocaleCookie(initialLocale);
          return;
        }
        const savedLocale = localStorage.getItem("locale");
        setLocaleState(resolveRuntimeLocale(window.location.pathname, savedLocale));
      } catch (error) {
        console.warn("Error reading locale from localStorage:", error);
      }
    }
  }, [initialLocale]);

  // Keep <html lang="…"> in sync with the active locale. Without this, the
  // root layout's hardcoded `lang="en"` outlived a runtime language switch:
  // content rendered in Spanish but screen readers / Google still saw English.
  //
  // EXCEPT on guest routes (isGuestPath: instance root, /b, /t, /scan,
  // /delivery, /reservations), where GuestTranslationProvider owns <html lang>
  // and <dir> — otherwise this root-level effect runs after the child provider
  // and stomps the guest-selected language back to the operator's locale.
  // The operator locale is still recorded so the guest provider can hand the
  // document back to it on unmount.
  React.useEffect(() => {
    if (typeof document === "undefined" || typeof window === "undefined") {
      return;
    }
    setOperatorHtmlLang(locale);
    if (isGuestPath(window.location.pathname)) {
      return;
    }
    document.documentElement.lang = locale;
  }, [locale]);

  const setLocale = (newLocale: Locale) => {
    setLocaleState(newLocale);
    if (typeof window !== "undefined") {
      persistOperatorLocaleChoice(newLocale);

      // Public pages encode locale in the URL. State-only toggles would
      // leave English URLs serving Spanish (#13 / #37). Rewrite the prefix.
      const path = window.location.pathname;
      const href = getPublicLocaleSwitchHref({
        path,
        targetLocale: newLocale,
        search: window.location.search,
        hash: window.location.hash,
      });
      if (href) {
        window.location.assign(href);
      }
    }
  };

  return (
    <SimpleTranslationContext.Provider value={{ locale, setLocale }}>
      {children}
    </SimpleTranslationContext.Provider>
  );
}

// Discoverable canonical name for the operator-tier provider. Aliases the
// existing component so the 155 historical import sites need no rename while new
// code (and audits) can reference the intent-revealing name.
/** @alias */
export const OperatorTranslationProvider = SimpleTranslationProvider;

export function useSimpleLocale() {
  const context = useContext(SimpleTranslationContext);
  if (context === undefined) {
    return { locale: defaultLocale, setLocale: () => { } };
  }
  return context;
}
