/** @jest-environment jsdom */
/**
 * #808 — the footer "Dashboard" entry. /dashboard has no locale-prefixed
 * variant; Spanish visitors keep their language through the operator locale
 * cookie the prefixed page set (middleware persistPathLocaleCookie), so every
 * locale links the same /dashboard instead of the instance root, which now
 * serves the venue.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { Footer } from "../Footer";
import { CookieConsentProvider } from "@/contexts/CookieConsentContext";

let mockLocale = "en";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/SimpleTranslationProvider");
  return {
    ...actual,
    useSimpleLocale: () => ({ locale: mockLocale, setLocale: () => {} }),
  };
});

jest.mock("@/hooks/useAnalytics", () => ({
  useClickTracking: () => jest.fn(),
}));

function renderFooter(locale: string) {
  mockLocale = locale;
  return render(
    <CookieConsentProvider>
      <Footer />
    </CookieConsentProvider>,
  );
}

describe("Footer dashboard link stays locale-aware (#808)", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("keeps /dashboard on English pages", () => {
    renderFooter("en");
    const link = screen.getByRole("link", { name: "Dashboard" });
    expect(link).toHaveAttribute("href", "/dashboard");
  });

  it.each(["es", "es-AR"])("links %s visitors to /dashboard too", (locale) => {
    renderFooter(locale);
    const link = screen.getByRole("link", { name: "Panel" });
    expect(link).toHaveAttribute("href", "/dashboard");
  });
});
