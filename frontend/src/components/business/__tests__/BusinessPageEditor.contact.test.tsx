/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { resetTestUrl } from "@/test/nextNavigationMock";

// #225: Business Page sub-tabs are URL-backed; need a stateful navigation mock.
jest.mock("next/navigation", () =>
  require("@/test/nextNavigationMock").createStatefulNavigationMock(),
);

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return { useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }), getTranslation: actual.getTranslation };
});
// Stable mock return objects keep loadData identity stable (no transient spinner).
const mockToast = { showSuccess: jest.fn(), showError: jest.fn() };
jest.mock("@/contexts/ToastContext", () => ({ useToast: () => mockToast }));
const mockTier = { hasAccess: true, loading: false };
jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: () => mockTier }));
jest.mock("@/components/business/DesignCustomization", () => ({ __esModule: true, default: () => <div data-testid="design-panel" /> }));
// Reviews tab loads enabled plugins — mock to avoid AggregateError/XHR noise.
jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    business: {
      getBusinessPlugins: jest.fn().mockResolvedValue({ plugins: [] }),
    },
  },
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getBusiness: jest.fn().mockResolvedValue({
      name: "Demo", custom_url: "demo", business_page_enabled: true,
      phone: "", website: "",
      address: { street: "", city: "", state: "", postal_code: "", country: "" },
      design_settings: { primary_color: "#111111" },
    }),
    getBusinessGalleryImages: jest.fn().mockResolvedValue([]),
    getBusinessOperatingHours: jest.fn().mockResolvedValue([]),
    getBusinessSpecialFeatures: jest.fn().mockResolvedValue([]),
    updateBusiness: jest.fn().mockResolvedValue({}),
    updateBusinessOperatingHours: jest.fn().mockResolvedValue({}),
    updateBusinessSpecialFeatures: jest.fn().mockResolvedValue({}),
    updateBusinessGalleryImages: jest.fn().mockResolvedValue({}),
    updateBusinessDesignSettings: jest.fn().mockResolvedValue({}),
  },
  checkCustomURLAvailability: jest.fn().mockResolvedValue({ available: true }),
}));

import BusinessPageEditor from "@/components/business/BusinessPageEditor";
import { businessApi } from "@/api/business";

describe("BusinessPageEditor — editor-owned Contact (INT-1)", () => {
  beforeEach(() => {
    resetTestUrl("/business/1/dashboard?tab=business-page");
  });

  it("opens Contact when landed via ?section=contact", async () => {
    resetTestUrl("/business/1/dashboard?tab=business-page&section=contact");
    render(<BusinessPageEditor businessId={1} />);
    const contactTab = (
      await screen.findAllByRole("tab", { name: /contact/i })
    )[0];
    expect(contactTab).toHaveAttribute("aria-selected", "true");
    expect(
      await screen.findByRole("textbox", { name: /phone/i }),
    ).toBeInTheDocument();
  });

  it("persists phone/website/address through updateBusiness on Save", async () => {
    render(<BusinessPageEditor businessId={1} />);
    // Contact is a DEDICATED tab (last in the strip) — navigate to it first.
    // The desktop strip is a WAI-ARIA tablist (role="tab", not "button").
    const contactTab = (await screen.findAllByRole("tab", { name: /contact/i }))[0];
    fireEvent.click(contactTab);
    const phone = await screen.findByRole("textbox", { name: /phone/i });
    fireEvent.change(phone, { target: { value: "+15551234567" } });
    const website = screen.getByRole("textbox", { name: /website/i });
    fireEvent.change(website, { target: { value: "https://demo.example" } });
    const street = screen.getByRole("textbox", { name: /street/i });
    fireEvent.change(street, { target: { value: "123 Main St" } });

    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));
    await waitFor(() => expect(businessApi.updateBusiness).toHaveBeenCalled());
    const payload = (businessApi.updateBusiness as jest.Mock).mock.calls.at(-1)[1];
    expect(payload.phone).toBe("+15551234567");
    expect(payload.website).toBe("https://demo.example");
    expect(payload.address).toEqual(expect.objectContaining({ street: "123 Main St" }));
  });
});
