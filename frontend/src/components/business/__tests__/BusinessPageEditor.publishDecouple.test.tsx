/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return { useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }), getTranslation: actual.getTranslation };
});
// Stable mock return objects: loadData closes over showError, so a fresh
// jest.fn() per render would change loadData's identity and re-fire the load
// effect (transient spinner mid-assertion). Module-level consts keep it stable.
const mockToast = { showSuccess: jest.fn(), showError: jest.fn() };
jest.mock("@/contexts/ToastContext", () => ({ useToast: () => mockToast }));
const mockTier = { hasAccess: true, loading: false };
jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: () => mockTier }));
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
    getBusiness: jest.fn().mockResolvedValue({ name: "Demo", custom_url: "demo", business_page_enabled: true, design_settings: { primary_color: "#111111" } }),
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

describe("BusinessPageEditor — edit/publish decoupling (IA-7)", () => {
  it("Save payload does NOT carry business_page_enabled (publish is the sole writer)", async () => {
    render(<BusinessPageEditor businessId={1} />);
    const welcome = await screen.findByRole("textbox", { name: /welcome message/i });
    fireEvent.change(welcome, { target: { value: "Hi" } });
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));

    await waitFor(() => expect(businessApi.updateBusiness).toHaveBeenCalled());
    const payload = (businessApi.updateBusiness as jest.Mock).mock.calls.at(-1)[1];
    expect(payload).not.toHaveProperty("business_page_enabled");
    // Content fields still sent — Save is still a real content save.
    expect(payload.welcome_message).toContain("Hi");
  });

  it("shows a Published status chip when the page is enabled", async () => {
    render(<BusinessPageEditor businessId={1} />);
    expect(await screen.findByText(/published/i)).toBeInTheDocument();
  });
});
