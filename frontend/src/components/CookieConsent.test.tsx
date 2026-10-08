/** @jest-environment jsdom */
import React from "react";
import { render, screen, act, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CookieConsent } from "./CookieConsent";
import { CookieConsentProvider } from "@/contexts/CookieConsentContext";
import { CONSENT_STORAGE_KEY } from "@/lib/analytics/consentGate";

// "/" is the venue page (guest chrome, which lands compact); these tests
// cover the full banner that non-guest pages show.
jest.mock("next/navigation", () => ({
  useRouter: () => ({
    push: jest.fn(),
    replace: jest.fn(),
    prefetch: jest.fn(),
    back: jest.fn(),
    forward: jest.fn(),
    refresh: jest.fn(),
  }),
  useSearchParams: () => new URLSearchParams(),
  usePathname: () => "/privacy-policy",
  useParams: () => ({}),
}));

// Drive the banner in English so we can assert on copy.
jest.mock("@/i18n/OperatorLocaleProvider", () => {
  const actual = jest.requireActual("@/i18n/OperatorLocaleProvider");
  return {
    ...actual,
    useSimpleLocale: () => ({ locale: "en", setLocale: () => {} }),
  };
});

function renderBanner() {
  return render(
    <CookieConsentProvider>
      <CookieConsent />
    </CookieConsentProvider>,
  );
}

describe("CookieConsent banner", () => {
  beforeEach(() => localStorage.clear());

  it("shows on first visit (no stored consent)", async () => {
    renderBanner();
    expect(
      await screen.findByText(/We value your privacy/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /Accept all/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /Decline non-essential/i }),
    ).toBeInTheDocument();
  });

  it("Decline hides the banner and persists analytics=false", async () => {
    renderBanner();
    const declineBtn = await screen.findByRole("button", {
      name: /Decline non-essential/i,
    });
    act(() => declineBtn.click());
    expect(
      screen.queryByText(/We value your privacy/i),
    ).not.toBeInTheDocument();
    const stored = JSON.parse(localStorage.getItem(CONSENT_STORAGE_KEY)!);
    expect(stored.analytics).toBe(false);
  });

  it("Accept hides the banner and persists analytics=true", async () => {
    renderBanner();
    const acceptBtn = await screen.findByRole("button", {
      name: /Accept all/i,
    });
    act(() => acceptBtn.click());
    expect(
      screen.queryByText(/We value your privacy/i),
    ).not.toBeInTheDocument();
    const stored = JSON.parse(localStorage.getItem(CONSENT_STORAGE_KEY)!);
    expect(stored.analytics).toBe(true);
  });

  it("stores separate analytics and marketing choices", async () => {
    renderBanner();
    const customize = await screen.findByRole("button", { name: /Customize/i });
    act(() => customize.click());
    const analytics = screen.getByRole("checkbox", { name: /Analytics/i });
    const marketing = screen.getByRole("checkbox", { name: /Marketing/i });
    act(() => analytics.click());
    expect(analytics).toBeChecked();
    expect(marketing).not.toBeChecked();
    act(() =>
      screen.getByRole("button", { name: /Save preferences/i }).click(),
    );
    const stored = JSON.parse(localStorage.getItem(CONSENT_STORAGE_KEY)!);
    expect(stored.analytics).toBe(true);
    expect(stored.marketing).toBe(false);
  });

  it("does NOT render when a decision is already stored", async () => {
    localStorage.setItem(
      CONSENT_STORAGE_KEY,
      JSON.stringify({
        version: 1,
        analytics: true,
        marketing: false,
        decidedAt: new Date().toISOString(),
      }),
    );
    renderBanner();
    // Allow the hydration effect to run.
    await act(async () => {});
    expect(
      screen.queryByText(/We value your privacy/i),
    ).not.toBeInTheDocument();
  });

  // Issue 355: Accept all must be fully visible at 390px (no max-h clip)
  // and desktop must be a slim bottom bar, not a modal on pricing/hero CTAs.
  it("shows stacked full-width actions without clipping Accept all", async () => {
    renderBanner();
    await screen.findByText(/We value your privacy/i);
    const dialog = screen.getByRole("dialog");
    const card = screen.getByTestId("cookie-consent-card");
    const actions = screen.getByTestId("cookie-consent-actions");
    const accept = screen.getByRole("button", { name: /Accept all/i });

    // 16px side padding; extra bottom gap so Decline clears the home indicator.
    expect(dialog.className).toMatch(/(?:^|\s)p-4(?:\s|$)/);
    expect(dialog.className).toMatch(/safe-area-inset-bottom/);
    expect(dialog.className).toMatch(/1\.75rem/);
    expect(card.className).toMatch(/(?:^|\s)p-4(?:\s|$)/);

    // The old 200px/24dvh cap clipped Accept all; do not bring it back.
    expect(card.className).not.toMatch(/24dvh|200px|32dvh|45dvh|520/);
    expect(card.className).not.toMatch(/overflow-y-auto/);
    expect(accept.closest("[class*='overflow-y-auto']")).toBeNull();

    // Three full-width stacked actions on mobile: Accept all, Customize, Decline.
    expect(actions.className).toMatch(/flex-col/);
    expect(actions.className).not.toMatch(/grid-cols-2/);
    const buttons = within(actions).getAllByRole("button");
    expect(buttons).toHaveLength(3);
    expect(buttons[0]).toHaveAccessibleName(/Accept all/i);
    expect(buttons[1]).toHaveAccessibleName(/Customize/i);
    expect(buttons[2]).toHaveAccessibleName(/Decline non-essential/i);
    buttons.forEach((btn) => {
      expect(btn.className).toMatch(/w-full/);
    });

    // Desktop is a flush bottom bar, not a centered max-w-3xl modal.
    expect(card.className).toMatch(/md:max-w-none/);
    expect(card.className).toMatch(/md:rounded-none/);
    expect(card.className).not.toMatch(/max-w-3xl/);
    expect(actions.className).toMatch(/md:flex-row/);
  });

  // Category explainer stays collapsed until Customize so the default 390
  // sheet fits without scrolling the actions.
  it("hides the category explainer by default and shows it when customizing (RV-1)", async () => {
    renderBanner();
    await screen.findByText(/We value your privacy/i);

    // Accept / Decline must remain available in the compact default state.
    expect(
      screen.getByRole("button", { name: /Accept all/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /Decline non-essential/i }),
    ).toBeInTheDocument();

    // Descriptions only appear in the explainer <ul>, not the fieldset labels.
    expect(
      screen.queryByText(/Required for the site to function/i),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText(/Helps us understand usage/i),
    ).not.toBeInTheDocument();

    act(() => screen.getByRole("button", { name: /Customize/i }).click());

    expect(
      screen.getByText(/Required for the site to function/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Helps us understand usage/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Used to measure and improve our campaigns/i),
    ).toBeInTheDocument();
  });

  it("moves focus into the dialog and keeps Tab inside it (#437)", async () => {
    const user = userEvent.setup();
    renderBanner();
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveAttribute("aria-modal", "true");
    const accept = screen.getByRole("button", { name: /Accept all/i });
    await waitFor(() => expect(accept).toHaveFocus());
    await user.tab();
    expect(screen.getByRole("button", { name: /Customize/i })).toHaveFocus();
    await user.tab();
    expect(
      screen.getByRole("button", { name: /Decline non-essential/i }),
    ).toHaveFocus();
    await user.tab();
    expect(
      screen.getByRole("link", { name: /Read our Privacy Policy/i }),
    ).toHaveFocus();
    await user.tab();
    expect(accept).toHaveFocus();
  });
});
