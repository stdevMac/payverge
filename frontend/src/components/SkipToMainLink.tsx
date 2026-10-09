"use client";

import { useEffect, useState } from "react";
import { getChromeTranslation } from "@/i18n/operatorChromeCatalog";
import { chromeCopyLocale, useChromeLocale } from "@/i18n/useChromeLocale";

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

/** Locale-aware skip link: guest 21-locale catalog on diner routes only (#519). */
export function SkipToMainLink() {
  const detected = useChromeLocale();
  const operatorLabel = getChromeTranslation(
    "common.skipToMainContent",
    chromeCopyLocale(detected),
  ) as string;
  const [label, setLabel] = useState(operatorLabel);

  useEffect(() => {
    if (typeof window === "undefined" || !isGuestPublicPath(window.location.pathname)) {
      setLabel(operatorLabel);
      return;
    }
    let cancelled = false;
    void import("@/i18n/skipToMainContentCatalog").then((mod) => {
      if (cancelled) return;
      setLabel(mod.skipToMainContentLabel(detected) ?? operatorLabel);
    });
    return () => {
      cancelled = true;
    };
  }, [detected, operatorLabel]);
  return (
    <a
      href="#main-content"
      className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-[9999] focus:bg-brand focus:text-white focus:px-4 focus:py-2 focus:rounded-lg focus:text-sm focus:font-medium"
    >
      {label}
    </a>
  );
}
