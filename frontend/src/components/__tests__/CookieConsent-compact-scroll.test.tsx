/** @jest-environment jsdom */
import React from "react";
import { render, screen, act, within } from "@testing-library/react";
import { CookieConsent } from "../CookieConsent";
import { CookieConsentProvider } from "@/contexts/CookieConsentContext";

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

// #800 part 2. Reserving page-bottom space (already shipped) only clears
// bottom-of-page CTAs. The banner is 328px tall at 390x844 — 39% of the
// viewport — so scrolling any /pricing plan card to the top of the viewport
// still parks its Start-trial / Talk-to-us button under the banner. Measured
// on live payverge.io/pricing at 390x844, banner rect top 516 vs CTA bottoms
// 522 / 545 / 545 => covered. Clamping the description to two lines and
// tightening the policy link once the visitor scrolls shortens the banner.
// The purpose and the /privacy-policy link stay visible.

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

function scrollTo(y: number) {
  act(() => {
    Object.defineProperty(window, "scrollY", {
      value: y,
      writable: true,
      configurable: true,
    });
    window.dispatchEvent(new Event("scroll"));
  });
}

describe("CookieConsent compaction on scroll (#800)", () => {
  beforeEach(() => {
    localStorage.clear();
    Object.defineProperty(window, "scrollY", {
      value: 0,
      writable: true,
      configurable: true,
    });
  });

  it("lands expanded: the description and policy link are on the banner the visitor arrives at", async () => {
    renderBanner();
    await screen.findByText(/We value your privacy/i);
    expect(
      screen.getByTestId("cookie-consent-description"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /Read our Privacy Policy/i }),
    ).toBeInTheDocument();
  });

  it("keeps the description and privacy link once the visitor scrolls", async () => {
    renderBanner();
    await screen.findByText(/We value your privacy/i);

    scrollTo(200);

    const description = screen.getByTestId("cookie-consent-description");
    expect(description).toBeInTheDocument();
    expect(description.className).toMatch(/line-clamp-2/);
    const link = screen.getByRole("link", {
      name: /Read our Privacy Policy/i,
    });
    expect(link).toHaveAttribute("href", "/privacy-policy");
    expect(link.className).toMatch(/\bmt-1\b/);
    expect(link.className).not.toMatch(/\bmt-2\b/);
  });

  it("keeps the title and all three actions in the compact form", async () => {
    renderBanner();
    await screen.findByText(/We value your privacy/i);
    scrollTo(200);

    // The dialog still announces itself and still offers every choice, so
    // Decline stays exactly as reachable as Accept.
    expect(screen.getByRole("dialog")).toHaveAccessibleName(
      /We value your privacy/i,
    );
    expect(screen.getByText(/We value your privacy/i)).toBeInTheDocument();

    const actions = screen.getByTestId("cookie-consent-actions");
    const buttons = within(actions).getAllByRole("button");
    expect(buttons).toHaveLength(3);
    expect(buttons[0]).toHaveAccessibleName(/Accept all/i);
    expect(buttons[1]).toHaveAccessibleName(/Customize/i);
    expect(buttons[2]).toHaveAccessibleName(/Decline non-essential/i);
  });

  it("keeps the #355 unclipped stacked-action layout while compact", async () => {
    renderBanner();
    await screen.findByText(/We value your privacy/i);
    scrollTo(200);

    const card = screen.getByTestId("cookie-consent-card");
    const actions = screen.getByTestId("cookie-consent-actions");
    expect(actions.className).toMatch(/flex-col/);
    expect(actions.className).not.toMatch(/grid-cols-2/);
    within(actions)
      .getAllByRole("button")
      .forEach((btn) => expect(btn.className).toMatch(/w-full/));
    expect(card.className).not.toMatch(/24dvh|200px|32dvh|45dvh|520/);
    expect(card.className).not.toMatch(/overflow-y-auto/);
    expect(
      screen
        .getByRole("button", { name: /Accept all/i })
        .closest("[class*='overflow-y-auto']"),
    ).toBeNull();
  });

  it("Customize re-expands the description and policy link from the compact form", async () => {
    renderBanner();
    await screen.findByText(/We value your privacy/i);
    scrollTo(200);
    const compactDescription = screen.getByTestId(
      "cookie-consent-description",
    );
    expect(compactDescription).toBeInTheDocument();
    expect(compactDescription.className).toMatch(/line-clamp-2/);
    expect(
      screen.getByRole("link", { name: /Read our Privacy Policy/i }),
    ).toHaveAttribute("href", "/privacy-policy");

    act(() => screen.getByRole("button", { name: /Customize/i }).click());

    const expandedDescription = screen.getByTestId(
      "cookie-consent-description",
    );
    expect(expandedDescription).toBeInTheDocument();
    expect(expandedDescription.className).not.toMatch(/line-clamp-2/);
    const link = screen.getByRole("link", {
      name: /Read our Privacy Policy/i,
    });
    expect(link).toHaveAttribute("href", "/privacy-policy");
    expect(link.className).toMatch(/\bmt-2\b/);
    expect(
      screen.getByText(/Required for the site to function/i),
    ).toBeInTheDocument();
  });

  it("re-expands when the visitor returns to the top", async () => {
    renderBanner();
    await screen.findByText(/We value your privacy/i);
    scrollTo(200);
    const compactDescription = screen.getByTestId(
      "cookie-consent-description",
    );
    expect(compactDescription).toBeInTheDocument();
    expect(compactDescription.className).toMatch(/line-clamp-2/);
    expect(screen.getByRole("link")).toHaveAttribute(
      "href",
      "/privacy-policy",
    );

    scrollTo(0);

    const expandedDescription = screen.getByTestId(
      "cookie-consent-description",
    );
    expect(expandedDescription).toBeInTheDocument();
    expect(expandedDescription.className).not.toMatch(/line-clamp-2/);
    expect(screen.getByRole("link").className).toMatch(/\bmt-2\b/);
  });

  it("ignores sub-threshold scroll jitter", async () => {
    renderBanner();
    await screen.findByText(/We value your privacy/i);

    scrollTo(12);

    expect(
      screen.getByTestId("cookie-consent-description"),
    ).toBeInTheDocument();
  });

  it("still records the choice from the compact form", async () => {
    renderBanner();
    await screen.findByText(/We value your privacy/i);
    scrollTo(200);

    act(() => screen.getByRole("button", { name: /Accept all/i }).click());

    expect(
      screen.queryByText(/We value your privacy/i),
    ).not.toBeInTheDocument();
    expect(document.documentElement.dataset.cookieBanner).toBeUndefined();
  });
});
