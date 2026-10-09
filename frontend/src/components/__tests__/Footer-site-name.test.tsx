/** @jest-environment jsdom */
/**
 * The footer copyright names the operator running this site (COMPANY_NAME),
 * falling back to the product name — never the upstream software brand when
 * the operator set their own.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { Footer } from "../Footer";
import { CookieConsentProvider } from "@/contexts/CookieConsentContext";
import { parseInstanceInfo } from "@/lib/instance/instanceInfo";
import {
  resetInstanceCacheForTests,
  setInstanceForTests,
} from "@/hooks/useInstance";

jest.mock("@/hooks/useAnalytics", () => ({
  useClickTracking: () => jest.fn(),
}));

function renderFooter() {
  return render(
    <CookieConsentProvider>
      <Footer />
    </CookieConsentProvider>,
  );
}

describe("Footer copyright holder", () => {
  afterEach(() => resetInstanceCacheForTests());

  it("names the operator's company when one is configured", () => {
    setInstanceForTests(
      parseInstanceInfo({
        product_name: "Payverge",
        company_name: "Bodegón Mesa Larga",
        registration_mode: "invite",
        features: {},
      }),
    );
    renderFooter();
    const year = new Date().getFullYear();
    expect(
      screen.getByText(new RegExp(`© ${year} Bodegón Mesa Larga\\.`)),
    ).toBeInTheDocument();
  });

  it("falls back to the product name", () => {
    setInstanceForTests(
      parseInstanceInfo({
        product_name: "Mesa",
        registration_mode: "invite",
        features: {},
      }),
    );
    renderFooter();
    expect(screen.getByText(/© \d{4} Mesa\./)).toBeInTheDocument();
  });
});
