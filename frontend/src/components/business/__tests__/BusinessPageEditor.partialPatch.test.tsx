/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return { useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }), getTranslation: actual.getTranslation };
});
const mockToast = { showSuccess: jest.fn(), showError: jest.fn() };
jest.mock("@/contexts/ToastContext", () => ({ useToast: () => mockToast }));
jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: () => ({ hasAccess: true, loading: false }) }));
jest.mock("@/components/business/DesignCustomization", () => ({ __esModule: true, default: () => <div data-testid="design-panel" /> }));
jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    business: { getBusinessPlugins: jest.fn().mockResolvedValue({ plugins: [] }) },
  },
}));
jest.mock("@/api/publicBusiness", () => ({
  getBusinessByCustomUrl: jest.fn().mockResolvedValue(null),
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getBusiness: jest.fn().mockResolvedValue({
      name: "Demo", custom_url: "demo", business_page_enabled: true,
      phone: "+15550001111", tax_rate: 7, google_reviews_enabled: true,
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

it("sends only fields this surface owns — no name/phone/tax/google_reviews_enabled round-trip", async () => {
  render(<BusinessPageEditor businessId={1} />);

  const welcome = await screen.findByRole("textbox", { name: /welcome message/i });
  fireEvent.change(welcome, { target: { value: "Hi there" } });
  fireEvent.click(screen.getByRole("button", { name: /save changes/i }));

  await waitFor(() => expect(businessApi.updateBusiness).toHaveBeenCalled());
  const payload = (businessApi.updateBusiness as jest.Mock).mock.calls.at(-1)[1];

  expect(payload.welcome_message).toContain("Hi there");
  expect(payload).not.toHaveProperty("name");
  // Phase 4 (INT-1): phone is now an editor-owned Contact field, so it IS sent.
  expect(payload).toHaveProperty("phone");
  expect(payload).not.toHaveProperty("tax_rate");
  expect(payload).not.toHaveProperty("google_reviews_enabled");
});
