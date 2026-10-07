/** @jest-environment jsdom */
/**
 * #947 / #863 — the z-[10000] cookie sheet covers guest table CTAs
 * (Ver Menú / Llamar al mozo) and the storefront cart FAB on first visit
 * because those surfaces never scroll enough for the #800 compact-on-scroll
 * rule to fire. Guest chrome must land compact so the measured banner
 * height can lift the dock/FAB instead of sitting on the diner's actions.
 */
import React from "react";
import fs from "fs";
import path from "path";
import { render, screen, within } from "@testing-library/react";
import { CookieConsent } from "../CookieConsent";
import { CookieConsentProvider } from "@/contexts/CookieConsentContext";

let mockPathname = "/t/FV214XU12D";

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
  usePathname: () => mockPathname,
  useParams: () => ({}),
}));

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

describe("CookieConsent guest chrome overlap (#947 / #863)", () => {
  beforeEach(() => {
    localStorage.clear();
    delete document.documentElement.dataset.cookieBanner;
    document.documentElement.style.removeProperty("--cookie-banner-height");
    mockPathname = "/t/FV214XU12D";
  });

  it("lands compact on a guest table path so Ver Menú is not under the 328px sheet", async () => {
    renderBanner();
    await screen.findByText(/We value your privacy/i);
    expect(
      screen.getByTestId("cookie-consent-description"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /Read our Privacy Policy/i }),
    ).toHaveAttribute("href", "/privacy-policy");
    const actions = screen.getByTestId("cookie-consent-actions");
    expect(within(actions).getAllByRole("button")).toHaveLength(3);
  });

  it("lands compact on a locale-prefixed storefront so Pedir is not covered", async () => {
    mockPathname = "/es-ar/b/parrilla-quebracho-azul";
    renderBanner();
    await screen.findByText(/We value your privacy/i);
    expect(
      screen.getByTestId("cookie-consent-description"),
    ).toBeInTheDocument();
    expect(screen.getByRole("link")).toHaveAttribute(
      "href",
      "/privacy-policy",
    );
  });

  it("compact mode shows the privacy link", async () => {
    mockPathname = "/t/FV214XU12D";
    renderBanner();
    await screen.findByText(/We value your privacy/i);
    expect(screen.getByRole("link")).toHaveAttribute(
      "href",
      "/privacy-policy",
    );
    expect(
      screen.getByTestId("cookie-consent-description"),
    ).toBeInTheDocument();
  });

  it("still lands expanded on marketing so /pricing keeps full disclosure", async () => {
    mockPathname = "/pricing";
    renderBanner();
    await screen.findByText(/We value your privacy/i);
    expect(
      screen.getByTestId("cookie-consent-description"),
    ).toBeInTheDocument();
  });

  it("PersistentGuestNav sits on --cookie-banner-height, not viewport bottom", () => {
    const nav = fs.readFileSync(
      path.join(process.cwd(), "src/components/navigation/PersistentGuestNav.tsx"),
      "utf8",
    );
    expect(nav).toMatch(
      /bottom-\[var\(--cookie-banner-height,0px\)\]/,
    );
    expect(nav).not.toMatch(/fixed bottom-0 left-0 right-0 z-50/);
  });

  it("storefront cart FAB offsets by --cookie-banner-height instead of a magic bottom-24", () => {
    const menu = fs.readFileSync(
      path.join(
        process.cwd(),
        "src/components/business-page/PublicMenuDisplay.tsx",
      ),
      "utf8",
    );
    expect(menu).toMatch(
      /fixed right-4 md:right-8 bottom-\[calc\(var\(--cookie-banner-height,0px\)\+6rem\)\]/,
    );
    expect(menu).not.toMatch(/fixed bottom-24 md:bottom-8 right-4 md:right-8/);
  });

  it("guest table landing and AiWaiter FAB include --cookie-banner-height in their dock offset", () => {
    const landing = fs.readFileSync(
      path.join(process.cwd(), "src/components/guest/GuestTableView.tsx"),
      "utf8",
    );
    const waiter = fs.readFileSync(
      path.join(process.cwd(), "src/components/guest/AiWaiter.tsx"),
      "utf8",
    );
    expect(landing).toMatch(
      /var\(--guest-nav-height,5\.5rem\)\+var\(--cookie-banner-height,0px\)/,
    );
    expect(waiter).toMatch(
      /var\(--guest-nav-height,5\.5rem\)\+var\(--cookie-banner-height,0px\)/,
    );
  });
});
