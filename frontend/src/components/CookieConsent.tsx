"use client";

import React, {
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import Link from "next/link";
import { Button } from "@nextui-org/react";
import { useSimpleLocale } from "@/i18n/OperatorLocaleProvider";
import {
  loadCookieConsentCopy,
  type CookieBannerCopy,
} from "@/i18n/cookieConsentCopy";
import { usePathname } from "next/navigation";
import { isGuestChromePath } from "@/utils/guestChromePath";
import { useCookieConsent } from "@/contexts/CookieConsentContext";

// Any deliberate scroll compacts the banner; 24px ignores rubber-banding and
// the scroll restoration browsers apply on reload.
const COMPACT_SCROLL_THRESHOLD_PX = 24;

// Decline-by-default consent banner. Shown only when the visitor has not yet
// made a choice (consent === null) AND after the client hydration read
// (`ready`) so it never flashes onto the SSR HTML.
//
// Copy: guest surfaces (/t/, /b/, /scan) use the 21-locale cookie-consent
// catalogs (NEW-3); marketing/operator keep the operator locale. Essential
// cookies are always-on; analytics/marketing are opt-in. The banner stays
// hidden until that copy has loaded.
export function CookieConsent() {
  const { locale } = useSimpleLocale();
  const pathname = usePathname();
  const guestChrome = isGuestChromePath(pathname);
  const { ready, consent, accept, decline, save, openerRef } =
    useCookieConsent();
  const dialogRef = useRef<HTMLDivElement>(null);
  const [customizing, setCustomizing] = useState(false);
  const [analytics, setAnalytics] = useState(false);
  const [marketing, setMarketing] = useState(false);
  const [compact, setCompact] = useState(false);
  const [copy, setCopy] = useState<CookieBannerCopy | null>(null);

  useEffect(() => {
    // Keep the previous copy on screen while a new locale loads (cached
    // locales resolve on the next microtask), so navigation never flickers
    // the banner. Only the very first load gates visibility.
    let cancelled = false;
    void loadCookieConsentCopy(locale, pathname ?? undefined).then((result) => {
      if (!cancelled) setCopy(result.copy);
    });
    return () => {
      cancelled = true;
    };
  }, [locale, pathname]);

  const bannerVisible = ready && consent === null && copy !== null;
  // Compact shortens the disclosure; it never removes the description or the
  // privacy-policy link. Guest chrome lands compact; marketing stays expanded
  // until the visitor scrolls (see the effect below). Customize forces the
  // full copy back open.
  const expanded = !compact || customizing;

  // #800 (part 2): reserving page-bottom space fixes bottom-of-page CTAs, but
  // the banner is 328px tall at 390x844 — 39% of the viewport — so mid-page
  // money CTAs are still covered wherever the visitor stops scrolling. Measured
  // at 390x844: scrolling a card to the top of the viewport puts its primary
  // button under the banner. Once the visitor scrolls, clamp the description
  // to two lines and tighten the policy link (mt-2 -> mt-1). The purpose and
  // the privacy-policy link stay on screen. Compaction is
  // driven by a real scroll event, never by the initial offset, so a restored
  // scroll position still shows the full banner until the visitor moves.
  useEffect(() => {
    if (!bannerVisible) {
      setCompact(false);
      return;
    }
    // #947 / #863: table home and storefront #menu often never scroll, so
    // compact-on-scroll never fires and the 328px z-[10000] sheet covers
    // Ver Menú / Llamar al mozo and the Pedir FAB. Land compact on guest
    // chrome; marketing still starts expanded until the visitor scrolls.
    if (guestChrome) {
      setCompact(true);
      return;
    }
    const onScroll = () => {
      setCompact((was) => {
        const next = window.scrollY > COMPACT_SCROLL_THRESHOLD_PX;
        return next === was ? was : next;
      });
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, [bannerVisible, guestChrome]);

  // Let marketing layouts reserve bottom space while the banner owns the
  // viewport edge (#34/#53) without importing CookieConsent into every page.
  // #800: publish the measured banner height as --cookie-banner-height so the
  // globals.css rule keyed on data-cookie-banner pads the page bottom by the
  // real banner size — otherwise the fixed banner permanently covers
  // bottom-of-page money CTAs (pricing Start-trial / Talk-to-us) on first
  // visit, since no scroll position ever clears them.
  useEffect(() => {
    const root = document.documentElement;
    if (!bannerVisible) {
      delete root.dataset.cookieBanner;
      root.style.removeProperty("--cookie-banner-height");
      return;
    }
    root.dataset.cookieBanner = "1";
    const setHeight = () => {
      const height = dialogRef.current?.offsetHeight ?? 0;
      root.style.setProperty("--cookie-banner-height", `${height}px`);
    };
    setHeight();
    let observer: ResizeObserver | undefined;
    if (typeof ResizeObserver !== "undefined" && dialogRef.current) {
      observer = new ResizeObserver(setHeight);
      observer.observe(dialogRef.current);
    }
    return () => {
      observer?.disconnect();
      delete root.dataset.cookieBanner;
      root.style.removeProperty("--cookie-banner-height");
    };
    // customizing and compaction change the card height; re-measure when they
    // toggle even without ResizeObserver support.
  }, [bannerVisible, customizing, compact]);

  const restoreOpener = useCallback(() => {
    openerRef.current?.focus();
  }, [openerRef]);

  useEffect(() => {
    if (!bannerVisible) return;
    const root = dialogRef.current;
    if (!root) return;
    const focusables = () =>
      Array.from(
        root.querySelectorAll<HTMLElement>(
          'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
        ),
      );
    const focusFirst = () => {
      if (!root.isConnected) return;
      if (root.contains(document.activeElement)) return;
      const firstAction = root.querySelector<HTMLElement>(
        '[data-testid="cookie-consent-actions"] button',
      );
      (firstAction ?? focusables()[0] ?? root).focus();
    };
    // Footer reopen races the opener's click-focus. Immediate + rAF +
    // delayed retries so focus lands inside after the pointer cycle.
    focusFirst();
    const raf = requestAnimationFrame(() => {
      focusFirst();
      requestAnimationFrame(focusFirst);
    });
    const t0 = window.setTimeout(focusFirst, 0);
    const t1 = window.setTimeout(focusFirst, 50);
    const t2 = window.setTimeout(focusFirst, 160);

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        decline();
        restoreOpener();
        return;
      }
      if (event.key !== "Tab") return;
      const nodes = focusables();
      if (nodes.length === 0) return;
      const first = nodes[0];
      const last = nodes[nodes.length - 1];
      const active = document.activeElement;
      if (event.shiftKey && active === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && active === last) {
        event.preventDefault();
        first.focus();
      } else if (!root.contains(active)) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => {
      cancelAnimationFrame(raf);
      window.clearTimeout(t0);
      window.clearTimeout(t1);
      window.clearTimeout(t2);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [bannerVisible, customizing, decline, restoreOpener]);

  const t = (key: string): string => {
    if (!copy) return key;
    const map: Record<string, string> = {
      "banner.title": copy.title,
      "banner.description": copy.description,
      "banner.acceptAll": copy.acceptAll,
      "banner.declineAll": copy.declineAll,
      "banner.customize": copy.customize,
      "banner.essentialLabel": copy.essentialLabel,
      "banner.essentialDescription": copy.essentialDescription,
      "banner.analyticsLabel": copy.analyticsLabel,
      "banner.analyticsDescription": copy.analyticsDescription,
      "banner.marketingLabel": copy.marketingLabel,
      "banner.marketingDescription": copy.marketingDescription,
      "banner.savePreferences": copy.savePreferences,
      "banner.privacyLink": copy.privacyLink,
      "banner.closeLabel": copy.closeLabel,
      "footer.preferences": copy.preferences,
    };
    return map[key] ?? key;
  };

  // Render nothing on the server / before hydration and once a decision exists.
  if (!bannerVisible) return null;

  return (
    <div
      ref={dialogRef}
      role="dialog"
      aria-modal="true"
      tabIndex={-1}
      aria-label={t("banner.title")}
      className="pointer-events-none fixed inset-x-0 bottom-0 z-[10000] p-4 pb-[max(1.75rem,calc(env(safe-area-inset-bottom)+0.75rem))] md:p-0 md:pb-0"
    >
      {/* Mobile 390: padded sheet with three stacked full-width actions so
          Accept all is never clipped. Desktop: flush bottom bar — not a
          centered max-w-3xl card sitting on page or hero CTAs. */}
      <div
        data-testid="cookie-consent-card"
        className={`pointer-events-auto mx-auto flex w-full flex-col rounded-2xl border border-ink-950/10 bg-white p-4 shadow-[0_10px_40px_rgba(28,25,23,0.18)] md:max-w-none md:flex-row md:items-center md:gap-6 md:rounded-none md:border-x-0 md:border-b-0 md:px-6 md:py-3 md:shadow-[0_-8px_24px_rgba(28,25,23,0.10)] ${
          customizing
            ? "max-h-[calc(100dvh-2rem)] md:max-h-[min(50dvh,28rem)] md:items-start"
            : ""
        }`}
      >
        <div
          className={`min-w-0 flex-1 ${customizing ? "overflow-y-auto" : ""}`}
        >
          <h2 className="font-serif text-base text-ink-950 md:text-sm">
            {t("banner.title")}
          </h2>
          {/* Always rendered (#800). Compact clamps the description to two
              lines and tightens the policy link; expanded copy is unchanged.
              Customize re-opens the full description. */}
          <p
            data-testid="cookie-consent-description"
            className={`mt-1 text-xs leading-snug text-ink-700 md:text-sm md:leading-snug${
              expanded ? "" : " line-clamp-2"
            }`}
          >
            {t("banner.description")}
          </p>
          <Link
            href="/privacy-policy"
            className={`${
              expanded ? "mt-2" : "mt-1"
            } inline-flex items-center min-h-[24px] py-1 text-xs font-medium text-brand underline-offset-2 hover:underline`}
          >
            {t("banner.privacyLink")}
          </Link>

          {/* Category explainer stays collapsed until Customize so the
              default 390 sheet fits without scrolling the actions. */}
          {customizing && (
            <ul className="mt-4 space-y-2 text-xs text-ink-600">
              <li>
                <span className="font-semibold text-ink-800">
                  {t("banner.essentialLabel")}:
                </span>{" "}
                {t("banner.essentialDescription")}
              </li>
              <li>
                <span className="font-semibold text-ink-800">
                  {t("banner.analyticsLabel")}:
                </span>{" "}
                {t("banner.analyticsDescription")}
              </li>
              <li>
                <span className="font-semibold text-ink-800">
                  {t("banner.marketingLabel")}:
                </span>{" "}
                {t("banner.marketingDescription")}
              </li>
            </ul>
          )}

          {customizing && (
            <fieldset className="mt-4 space-y-3 rounded-xl border border-ink-950/10 bg-warm-50 p-4">
              <legend className="px-1 text-sm font-semibold text-ink-900">
                {t("banner.customize")}
              </legend>
              <label className="flex items-center justify-between gap-4 text-sm text-ink-700">
                <span>{t("banner.essentialLabel")}</span>
                <input
                  type="checkbox"
                  checked
                  disabled
                  className="h-4 w-4 accent-brand"
                />
              </label>
              <label className="flex items-center justify-between gap-4 text-sm text-ink-700">
                <span>{t("banner.analyticsLabel")}</span>
                <input
                  type="checkbox"
                  checked={analytics}
                  onChange={(event) => setAnalytics(event.target.checked)}
                  className="h-4 w-4 accent-brand"
                />
              </label>
              <label className="flex items-center justify-between gap-4 text-sm text-ink-700">
                <span>{t("banner.marketingLabel")}</span>
                <input
                  type="checkbox"
                  checked={marketing}
                  onChange={(event) => setMarketing(event.target.checked)}
                  className="h-4 w-4 accent-brand"
                />
              </label>
            </fieldset>
          )}
        </div>

        <div
          data-testid="cookie-consent-actions"
          className="mt-3 flex w-full shrink-0 flex-col gap-2 md:mt-0 md:w-auto md:flex-row md:items-center md:gap-2"
        >
          {customizing ? (
            <Button
              radius="lg"
              size="sm"
              onPress={() => {
                save({ analytics, marketing });
                setCustomizing(false);
                restoreOpener();
              }}
              className="h-auto min-h-8 w-full bg-brand font-medium text-white md:w-auto"
            >
              {t("banner.savePreferences")}
            </Button>
          ) : (
            <Button
              radius="lg"
              size="sm"
              onPress={() => {
                setCustomizing(false);
                accept();
                restoreOpener();
              }}
              className="h-auto min-h-8 w-full bg-brand font-medium text-white md:w-auto"
            >
              {t("banner.acceptAll")}
            </Button>
          )}
          <Button
            variant="light"
            radius="lg"
            size="sm"
            onPress={() => setCustomizing((value) => !value)}
            className="h-auto min-h-8 w-full font-medium text-ink-800 md:w-auto"
          >
            {t("banner.customize")}
          </Button>
          <Button
            variant="bordered"
            radius="lg"
            size="sm"
            onPress={() => {
              setCustomizing(false);
              decline();
              restoreOpener();
            }}
            className="h-auto min-h-8 w-full whitespace-normal border-ink-950/15 font-medium leading-snug text-ink-800 md:w-auto"
          >
            {t("banner.declineAll")}
          </Button>
        </div>
      </div>
    </div>
  );
}
