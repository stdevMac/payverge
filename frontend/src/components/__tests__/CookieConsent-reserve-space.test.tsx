/** @jest-environment jsdom */
/**
 * #800 — the fixed bottom consent banner covered the pricing page's
 * bottom-of-page Start-trial / Talk-to-us CTAs on first visit (no scroll
 * position ever cleared them). The banner must publish its measured height
 * as a CSS variable alongside the existing `data-cookie-banner` flag, and
 * globals.css must consume both to reserve matching space at the end of the
 * page so money CTAs can scroll clear of the banner.
 */
import React from "react";
import fs from "fs";
import path from "path";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { CookieConsent } from "../CookieConsent";
import { CookieConsentProvider } from "@/contexts/CookieConsentContext";

describe("CookieConsent reserves bottom space for page CTAs (#800)", () => {
  beforeEach(() => {
    localStorage.clear();
    delete document.documentElement.dataset.cookieBanner;
    document.documentElement.style.removeProperty("--cookie-banner-height");
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("publishes the banner height as --cookie-banner-height while visible and clears it on Accept", async () => {
    jest
      .spyOn(HTMLElement.prototype, "offsetHeight", "get")
      .mockReturnValue(132);

    render(
      <CookieConsentProvider>
        <CookieConsent />
      </CookieConsentProvider>,
    );

    await screen.findByText(/We value your privacy/i);
    expect(document.documentElement.dataset.cookieBanner).toBe("1");
    expect(
      document.documentElement.style.getPropertyValue(
        "--cookie-banner-height",
      ),
    ).toBe("132px");

    fireEvent.click(screen.getByRole("button", { name: /Accept all/i }));

    await waitFor(() =>
      expect(document.documentElement.dataset.cookieBanner).toBeUndefined(),
    );
    expect(
      document.documentElement.style.getPropertyValue(
        "--cookie-banner-height",
      ),
    ).toBe("");
  });

  it("globals.css reserves the published height under the data flag", () => {
    const css = fs.readFileSync(
      path.join(process.cwd(), "src/app/globals.css"),
      "utf8",
    );
    // The reservation rule must key off the flag the banner already sets and
    // consume the measured height variable — not a hardcoded guess.
    expect(css).toMatch(
      /html\[data-cookie-banner="1"\]\s+body\s*\{[^}]*padding-bottom:\s*var\(--cookie-banner-height/,
    );
  });
});
