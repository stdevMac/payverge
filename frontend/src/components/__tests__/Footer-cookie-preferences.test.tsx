/** @jest-environment jsdom */
import React from "react";
import { render, screen, act, waitFor } from "@testing-library/react";
import { Footer } from "../Footer";
import { CookieConsent } from "../CookieConsent";
import { CookieConsentProvider } from "@/contexts/CookieConsentContext";
import { CONSENT_STORAGE_KEY } from "@/lib/analytics/consentGate";

jest.mock("@/i18n/OperatorLocaleProvider", () => {
  const actual = jest.requireActual("@/i18n/OperatorLocaleProvider");
  return {
    ...actual,
    useSimpleLocale: () => ({ locale: "en", setLocale: () => {} }),
  };
});

// Footer pulls in analytics click tracking; stub it to a no-op.
jest.mock("@/hooks/useAnalytics", () => ({
  useClickTracking: () => () => {},
}));

describe("Footer cookie-preferences affordance", () => {
  beforeEach(() => localStorage.clear());

  it("renders a Cookie preferences control", () => {
    render(
      <CookieConsentProvider>
        <Footer />
      </CookieConsentProvider>,
    );
    expect(
      screen.getByRole("button", { name: /Cookie preferences/i }),
    ).toBeInTheDocument();
  });

  it("clicking it clears the stored decision (so the banner can reopen)", () => {
    localStorage.setItem(
      CONSENT_STORAGE_KEY,
      JSON.stringify({
        version: 1,
        analytics: true,
        marketing: false,
        decidedAt: new Date().toISOString(),
      }),
    );
    render(
      <CookieConsentProvider>
        <Footer />
      </CookieConsentProvider>,
    );
    act(() => {
      screen.getByRole("button", { name: /Cookie preferences/i }).click();
    });
    expect(localStorage.getItem(CONSENT_STORAGE_KEY)).toBeNull();
  });

  it("moves focus into the remounted dialog after Cookie preferences (#437)", async () => {
    localStorage.setItem(
      CONSENT_STORAGE_KEY,
      JSON.stringify({
        version: 1,
        analytics: true,
        marketing: false,
        decidedAt: new Date().toISOString(),
      }),
    );
    render(
      <CookieConsentProvider>
        <Footer />
        <CookieConsent />
      </CookieConsentProvider>,
    );
    const opener = screen.getByRole("button", { name: /Cookie preferences/i });
    act(() => {
      opener.dispatchEvent(
        new MouseEvent("mousedown", { bubbles: true, cancelable: true }),
      );
      opener.click();
    });
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveAttribute("aria-modal", "true");
    await waitFor(() => {
      expect(dialog.contains(document.activeElement)).toBe(true);
    });
    expect(document.activeElement).not.toBe(opener);
  });
});
