/** @jest-environment jsdom */
/**
 * #721 — the 404 home CTA must keep the request locale, so an es / es-AR
 * visitor who lands on a dead URL is not bounced onto the English home.
 *
 * The contact CTA is a mailto: to the deployment's SUPPORT_EMAIL and is hidden
 * when none is configured (there is no /contact page to fall back to).
 *
 * The real component + the real `@/i18n/publicPageRoutes` are exercised; only
 * the translation lookup and next/link|next/image chrome are stubbed.
 */
import React from "react";
import { render, screen } from "@testing-library/react";

let mockLocale = "en";
const mockBrandLinks: { contactEmail?: string } = {};

jest.mock("@/config/brand", () => ({
  get brandLinks() {
    return mockBrandLinks;
  },
}));

jest.mock("@/i18n/OperatorLocaleProvider", () => ({
  useSimpleLocale: () => ({ locale: mockLocale }),
}));

jest.mock("@/i18n/operatorChromeCatalog", () => ({
  getChromeTranslation: (key: string) => key,
}));

jest.mock("next/link", () => {
  const ReactActual = require("react");
  const MockLink = ReactActual.forwardRef(
    ({ children, href, ...props }: any, ref: any) =>
      ReactActual.createElement("a", { href, ref, ...props }, children),
  );
  MockLink.displayName = "MockNextLink";
  return MockLink;
});

jest.mock("next/image", () => {
  const ReactActual = require("react");
  const MockImage = ReactActual.forwardRef(
    ({ src, alt, className }: any, ref: any) =>
      ReactActual.createElement("img", { src, alt, className, ref }),
  );
  MockImage.displayName = "MockNextImage";
  return MockImage;
});

import NotFoundClient from "../NotFoundClient";

function hrefs(): string[] {
  return screen.getAllByRole("link").map((el) => el.getAttribute("href") || "");
}

describe("NotFoundClient recovery CTAs keep the locale prefix (#721)", () => {
  afterEach(() => {
    mockLocale = "en";
    delete mockBrandLinks.contactEmail;
  });

  it("leaves the CTAs unprefixed on English", () => {
    mockLocale = "en";
    render(<NotFoundClient />);

    expect(hrefs()).toEqual(["/"]);
  });

  it("prefixes the CTAs with /es for Spanish", () => {
    mockLocale = "es";
    render(<NotFoundClient />);

    expect(hrefs()).toEqual(["/es"]);
  });

  it("prefixes the CTAs with /es-ar for Argentine Spanish", () => {
    mockLocale = "es-AR";
    render(<NotFoundClient />);

    expect(hrefs()).toEqual(["/es-ar"]);
  });

  it("offers a mailto contact CTA only when SUPPORT_EMAIL is configured", () => {
    mockBrandLinks.contactEmail = "help@venue.example";
    render(<NotFoundClient />);

    expect(hrefs()).toEqual(["/", "mailto:help@venue.example"]);
  });
});
