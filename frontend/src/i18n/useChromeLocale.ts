"use client";

import { useEffect, useState } from "react";
import { useSimpleLocale } from "./OperatorLocaleProvider";
import { readGuestLocaleCookie } from "./guestLocaleResolver";
import { isSupportedLocale, type Locale } from "./localeRegistry";

/** Collapse any guest/storefront tag to the operator message set. */
export function chromeCopyLocale(locale: string): Locale {
  if (isSupportedLocale(locale)) return locale;
  const short = (locale || "en").toLowerCase().split(/[-_]/)[0];
  if (short === "es") return "es";
  return "en";
}

function isGuestPublicPath(pathname: string): boolean {
  return (
    // Instance root ("/", "/es", "/es-ar"): venue page or venue directory.
    /^(?:\/(?:es|es-ar))?\/?$/i.test(pathname) ||
    pathname.startsWith("/t/") ||
    pathname.startsWith("/b/") ||
    pathname.startsWith("/scan") ||
    pathname.startsWith("/delivery") ||
    pathname.startsWith("/reservations")
  );
}

/**
 * Locale for root a11y chrome that sits above GuestTranslationProvider
 * (skip link, NextUI Dismiss/Close). Operator URL locale wins on marketing
 * and dashboard routes; guest-table in-app switches win via
 * `guestLanguageChange` and the guest cookie on diner surfaces (#26).
 */
export function useChromeLocale(): string {
  const { locale: operatorLocale } = useSimpleLocale();
  const [guestLocale, setGuestLocale] = useState<string | null>(null);

  useEffect(() => {
    if (typeof window === "undefined") return;

    if (isGuestPublicPath(window.location.pathname)) {
      const cookie = readGuestLocaleCookie();
      if (cookie) setGuestLocale(cookie);
    }

    const onGuestLanguageChange = (event: Event) => {
      const language = (event as CustomEvent<{ language?: string }>).detail
        ?.language;
      if (typeof language === "string" && language.trim()) {
        setGuestLocale(language);
      }
    };

    window.addEventListener("guestLanguageChange", onGuestLanguageChange);
    return () => {
      window.removeEventListener("guestLanguageChange", onGuestLanguageChange);
    };
  }, []);

  return guestLocale ?? operatorLocale;
}
