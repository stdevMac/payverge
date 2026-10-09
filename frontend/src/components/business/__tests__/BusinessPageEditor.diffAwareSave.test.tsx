/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});
const mockToast = { showSuccess: jest.fn(), showError: jest.fn() };
jest.mock("@/contexts/ToastContext", () => ({ useToast: () => mockToast }));
jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({ hasAccess: true, loading: false }),
}));
jest.mock("@/components/business/DesignCustomization", () => ({
  __esModule: true,
  default: () => <div data-testid="design-panel" />,
}));
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
      name: "Demo",
      custom_url: "demo",
      business_page_enabled: true,
      design_settings: { primary_color: "#111111" },
      welcome_message: "",
    }),
    getBusinessGalleryImages: jest.fn().mockResolvedValue([
      {
        id: 42,
        business_id: 1,
        image_url: "https://cdn.example.com/a.jpg",
        caption: "Original",
        display_order: 0,
        is_active: true,
      },
    ]),
    getBusinessOperatingHours: jest.fn().mockResolvedValue([
      { id: 1, day_of_week: 1, open_time: "09:00", close_time: "17:00", is_closed: false },
    ]),
    getBusinessSpecialFeatures: jest.fn().mockResolvedValue([
      { id: 7, title: "Wifi", description: "Free", icon: "wifi", display_order: 0 },
    ]),
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

describe("BusinessPageEditor — diff-aware section saves (Stream 9 #2)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("PUTs only the profile section when only welcome message changes", async () => {
    render(<BusinessPageEditor businessId={1} />);
    const welcome = await screen.findByRole("textbox", { name: /welcome message/i });
    fireEvent.change(welcome, { target: { value: "Just a typo fix" } });
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));

    await waitFor(() => expect(businessApi.updateBusiness).toHaveBeenCalledTimes(1));
    expect(businessApi.updateBusinessGalleryImages).not.toHaveBeenCalled();
    expect(businessApi.updateBusinessDesignSettings).not.toHaveBeenCalled();
    expect(businessApi.updateBusinessOperatingHours).not.toHaveBeenCalled();
    expect(businessApi.updateBusinessSpecialFeatures).not.toHaveBeenCalled();
  });

  it("does not call any section endpoints when nothing is dirty", async () => {
    render(<BusinessPageEditor businessId={1} />);
    // Wait for load to finish so Save is present (dirty=false → still mounted).
    await screen.findByRole("textbox", { name: /welcome message/i });
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));

    // Give microtasks a tick; nothing should fire.
    await waitFor(() => {
      expect(businessApi.updateBusiness).not.toHaveBeenCalled();
    });
    expect(businessApi.updateBusinessGalleryImages).not.toHaveBeenCalled();
  });
});
