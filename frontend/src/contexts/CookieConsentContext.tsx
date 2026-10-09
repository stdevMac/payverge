"use client";

import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import {
  type CookieConsent,
  readConsent,
  writeConsent,
  clearConsent,
  initAnalyticsIfConsented,
} from "@/lib/analytics/consentGate";

export type ConsentCategory = "essential" | "analytics" | "marketing";

interface CookieConsentContextValue {
  // The persisted decision, or null when the visitor has not decided yet.
  consent: CookieConsent | null;
  // True once the client effect has read localStorage. SSR + first paint are
  // `false` so the banner never flashes on the server-rendered HTML.
  ready: boolean;
  // Essential is always granted. analytics/marketing follow the stored choice.
  hasConsent: (category: ConsentCategory) => boolean;
  // Grant everything (analytics + marketing).
  accept: () => void;
  // Reject all non-essential categories (still a recorded decision).
  decline: () => void;
  // Persist a granular choice (used by the "Save preferences" path).
  save: (choice: { analytics: boolean; marketing: boolean }) => void;
  // Forget the decision so the banner reopens (footer "Cookie preferences").
  reset: () => void;
  /** Remember the footer control so the banner can restore focus on close. */
  setOpener: (el: HTMLElement | null) => void;
  openerRef: React.MutableRefObject<HTMLElement | null>;
  /**
   * True while a surface has asked the banner to step aside (see
   * useSuppressCookieBanner). The decision is still pending; the banner just
   * waits until the surface closes.
   */
  bannerSuppressed: boolean;
  /** Hide the banner until the returned release function runs. */
  suppressBanner: () => () => void;
}

const CookieConsentContext = createContext<CookieConsentContextValue | undefined>(
  undefined,
);

export function CookieConsentProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const [consent, setConsent] = useState<CookieConsent | null>(null);
  const [ready, setReady] = useState(false);
  const openerRef = React.useRef<HTMLElement | null>(null);
  // Count of open surfaces that asked the banner to step aside. A count, not
  // a flag, so two overlapping modals cannot unhide it early.
  const [suppressors, setSuppressors] = useState(0);
  const suppressBanner = useCallback(() => {
    let released = false;
    setSuppressors((n) => n + 1);
    return () => {
      if (released) return;
      released = true;
      setSuppressors((n) => Math.max(0, n - 1));
    };
  }, []);
  const setOpener = useCallback((el: HTMLElement | null) => {
    openerRef.current = el;
  }, []);

  // Read the persisted choice once on the client. Keeping this in an effect
  // (rather than a lazy useState initializer) guarantees server and first
  // client render agree (both null/false) so hydration is stable and the
  // banner only appears after mount.
  useEffect(() => {
    setConsent(readConsent());
    setReady(true);
  }, []);

  // Whenever analytics consent is (or becomes) granted, run the analytics
  // gate. Today initAnalyticsIfConsented is a documented no-op because no
  // loader is wired; this is the single place a future PostHog bootstrap
  // re-runs after a late opt-in without a page reload.
  useEffect(() => {
    if (!ready) return;
    initAnalyticsIfConsented(() => {
      // FUTURE: posthog.init(...) — see lib/analytics/consentGate.ts.
    });
  }, [ready, consent]);

  const hasConsent = useCallback(
    (category: ConsentCategory): boolean => {
      if (category === "essential") return true;
      if (!consent) return false;
      return consent[category] === true;
    },
    [consent],
  );

  const save = useCallback(
    (choice: { analytics: boolean; marketing: boolean }) => {
      setConsent(writeConsent(choice));
    },
    [],
  );

  const accept = useCallback(() => {
    save({ analytics: true, marketing: true });
  }, [save]);

  const decline = useCallback(() => {
    save({ analytics: false, marketing: false });
  }, [save]);

  const reset = useCallback(() => {
    clearConsent();
    setConsent(null);
  }, []);

  const value = useMemo<CookieConsentContextValue>(
    () => ({
      consent,
      ready,
      hasConsent,
      accept,
      decline,
      save,
      reset,
      setOpener,
      openerRef,
      bannerSuppressed: suppressors > 0,
      suppressBanner,
    }),
    [
      consent,
      ready,
      hasConsent,
      accept,
      decline,
      save,
      reset,
      setOpener,
      suppressors,
      suppressBanner,
    ],
  );

  return (
    <CookieConsentContext.Provider value={value}>
      {children}
    </CookieConsentContext.Provider>
  );
}

export function useCookieConsent(): CookieConsentContextValue {
  const ctx = useContext(CookieConsentContext);
  if (ctx === undefined) {
    // Safe no-op fallback so a stray consumer outside the provider (e.g. a
    // unit-rendered component) doesn't crash. Mirrors useSimpleLocale's
    // defensive default.
    return {
      consent: null,
      ready: false,
      hasConsent: (c) => c === "essential",
      accept: () => {},
      decline: () => {},
      save: () => {},
      reset: () => {},
      setOpener: () => {},
      openerRef: { current: null },
      bannerSuppressed: false,
      suppressBanner: () => () => {},
    };
  }
  return ctx;
}

/**
 * Keep the consent banner out of the way while `active` is true.
 *
 * A modal (for example the guest cart bottom sheet) hides and blocks
 * everything outside it, and the banner is fixed to the same bottom edge at
 * z-[10000]. Left visible, it covers the modal's primary action on a phone,
 * and a tap on it counts as a click outside the modal: the modal closes and
 * the choice is never recorded. Surfaces that own the bottom of the screen
 * call this with their open state; the banner comes back when they close.
 */
export function useSuppressCookieBanner(active: boolean): void {
  const { suppressBanner } = useCookieConsent();
  useEffect(() => {
    if (!active) return;
    return suppressBanner();
  }, [active, suppressBanner]);
}
