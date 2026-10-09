/** @jest-environment jsdom */
import {
  CONSENT_CHANGE_EVENT,
  CONSENT_STORAGE_KEY,
  clearConsent,
  readConsent,
  hasAnalyticsConsent,
  initAnalyticsIfConsented,
  onConsentChange,
  writeConsent,
} from "./consentGate";

describe("consentGate", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("returns null when no choice is stored (decline-by-default)", () => {
    expect(readConsent()).toBeNull();
    expect(hasAnalyticsConsent()).toBe(false);
  });

  it("does NOT run the analytics loader before opt-in", () => {
    const loader = jest.fn();
    initAnalyticsIfConsented(loader);
    expect(loader).not.toHaveBeenCalled();
  });

  it("runs the analytics loader once analytics consent is granted", () => {
    localStorage.setItem(
      CONSENT_STORAGE_KEY,
      JSON.stringify({
        version: 1,
        analytics: true,
        marketing: false,
        decidedAt: new Date().toISOString(),
      }),
    );
    expect(hasAnalyticsConsent()).toBe(true);
    const loader = jest.fn();
    initAnalyticsIfConsented(loader);
    expect(loader).toHaveBeenCalledTimes(1);
  });

  it("treats analytics:false as no consent", () => {
    localStorage.setItem(
      CONSENT_STORAGE_KEY,
      JSON.stringify({
        version: 1,
        analytics: false,
        marketing: false,
        decidedAt: new Date().toISOString(),
      }),
    );
    expect(hasAnalyticsConsent()).toBe(false);
    const loader = jest.fn();
    initAnalyticsIfConsented(loader);
    expect(loader).not.toHaveBeenCalled();
  });

  it("stops later analytics initialization after consent is withdrawn", () => {
    localStorage.setItem(
      CONSENT_STORAGE_KEY,
      JSON.stringify({
        version: 1,
        analytics: true,
        marketing: true,
        decidedAt: new Date().toISOString(),
      }),
    );
    expect(hasAnalyticsConsent()).toBe(true);

    localStorage.setItem(
      CONSENT_STORAGE_KEY,
      JSON.stringify({
        version: 1,
        analytics: false,
        marketing: false,
        decidedAt: new Date().toISOString(),
      }),
    );
    const loader = jest.fn();
    initAnalyticsIfConsented(loader);
    expect(loader).not.toHaveBeenCalled();
  });

  it("treats corrupt JSON as no consent (fail closed)", () => {
    localStorage.setItem(CONSENT_STORAGE_KEY, "{not json");
    expect(readConsent()).toBeNull();
    expect(hasAnalyticsConsent()).toBe(false);
  });

  it("dispatches a consent-change event from writeConsent and clearConsent", () => {
    const listener = jest.fn();
    const unsubscribe = onConsentChange(listener);

    writeConsent({ analytics: true, marketing: false });
    writeConsent({ analytics: false, marketing: false });
    clearConsent();
    expect(listener).toHaveBeenCalledTimes(3);

    unsubscribe();
    writeConsent({ analytics: true, marketing: false });
    expect(listener).toHaveBeenCalledTimes(3);
    expect(CONSENT_CHANGE_EVENT).toBe("payverge:consent-change");
  });
});
