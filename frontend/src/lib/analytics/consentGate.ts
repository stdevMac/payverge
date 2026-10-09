// Single source of truth for the persisted cookie-consent choice and the
// analytics gate. This module is intentionally framework-agnostic (no React)
// so it can be called from anywhere — including a future PostHog bootstrap.
//
// === FUTURE POSTHOG HOOKUP (the one and only integration point) ===
// PostHog is NOT initialized in the app yet. When it is added, do NOT call
// posthog.init() at module load. Instead call it through initAnalyticsIfConsented:
//
//   import posthog from "posthog-js";
//   initAnalyticsIfConsented(() => {
//     posthog.init(getPublicConfig().posthogKey, {
//       api_host: getPublicConfig().posthogHost,
//       persistence: "localStorage", // never set non-essential cookies pre-consent
//       autocapture: true,
//     });
//   });
//
// Re-run this (e.g. from CookieConsentProvider) after the user opts in so a
// late "Accept" still boots analytics without a page reload. The runtime CSP
// (src/lib/security/csp.ts) already adds the configured POSTHOG_HOST to
// connect-src, so no CSP change is required for the PostHog hookup.

export const CONSENT_STORAGE_KEY = "payverge_cookie_consent";

export const CONSENT_CHANGE_EVENT = "payverge:consent-change";

export interface CookieConsent {
  version: 1;
  analytics: boolean;
  marketing: boolean;
  decidedAt: string;
}

const CONSENT_VERSION = 1 as const;

// Returns the stored choice, or null when the visitor has not decided yet OR
// the stored value is unparseable/stale-version. Null => decline-by-default.
export function readConsent(): CookieConsent | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = window.localStorage.getItem(CONSENT_STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<CookieConsent>;
    if (
      !parsed ||
      parsed.version !== CONSENT_VERSION ||
      typeof parsed.analytics !== "boolean" ||
      typeof parsed.marketing !== "boolean"
    ) {
      return null;
    }
    return {
      version: CONSENT_VERSION,
      analytics: parsed.analytics,
      marketing: parsed.marketing,
      decidedAt: typeof parsed.decidedAt === "string" ? parsed.decidedAt : "",
    };
  } catch {
    // Fail closed: any read/parse error is treated as "no consent".
    return null;
  }
}

export function hasAnalyticsConsent(): boolean {
  return readConsent()?.analytics === true;
}

// Subscribe to writeConsent/clearConsent. Returns an unsubscribe function.
export function onConsentChange(cb: () => void): () => void {
  if (typeof window === "undefined") {
    return () => {};
  }
  window.addEventListener(CONSENT_CHANGE_EVENT, cb);
  return () => {
    window.removeEventListener(CONSENT_CHANGE_EVENT, cb);
  };
}

function dispatchConsentChange(): void {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new Event(CONSENT_CHANGE_EVENT));
}

// THE analytics gate. `loader` is the side-effecting init (e.g. posthog.init).
// It runs synchronously iff analytics consent is currently granted, and never
// otherwise. Today no caller passes a real loader, so this is a no-op in prod.
export function initAnalyticsIfConsented(loader: () => void): void {
  if (hasAnalyticsConsent()) {
    loader();
  }
}

// Persist a decision. Used by the provider; exported so tests/util callers
// share the exact serialization shape.
export function writeConsent(input: {
  analytics: boolean;
  marketing: boolean;
}): CookieConsent {
  const value: CookieConsent = {
    version: CONSENT_VERSION,
    analytics: input.analytics,
    marketing: input.marketing,
    decidedAt: new Date().toISOString(),
  };
  if (typeof window !== "undefined") {
    try {
      window.localStorage.setItem(CONSENT_STORAGE_KEY, JSON.stringify(value));
    } catch {
      // localStorage can throw (private mode / quota). Swallow — the banner
      // simply reappears next visit, which is the safe (decline) default.
    }
    dispatchConsentChange();
  }
  return value;
}

export function clearConsent(): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.removeItem(CONSENT_STORAGE_KEY);
  } catch {
    // ignore
  }
  dispatchConsentChange();
}
