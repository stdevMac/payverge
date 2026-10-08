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
// Task 28 reviews: BusinessPageEditor loads enabled plugins — mock to avoid XHR noise.
jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    business: {
      getBusinessPlugins: jest.fn().mockResolvedValue({ plugins: [] }),
    },
  },
}));
// Stub DesignCustomization so the merged tab's "design" half is findable.
jest.mock("@/components/business/DesignCustomization", () => ({ __esModule: true, default: () => <div data-testid="design-panel" /> }));
// Stub the banner uploader so the merged tab's "visuals" half is findable.
jest.mock("@/components/business/BannerImageUploader", () => ({ __esModule: true, default: () => <div data-testid="banner-uploader" /> }));

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

describe("BusinessPageEditor — merged Look & feel tab (IA-2)", () => {
  beforeEach(() => {
    resetTestUrl("/business/1/dashboard?tab=business-page");
  });

  it("renders exactly one 'Look & feel' tab and no 'Visuals' tab", async () => {
    render(<BusinessPageEditor businessId={1} />);
    // Desktop strip is a WAI-ARIA tablist (role="tab"). Assert at least one
    // Look & feel tab, zero Visuals/Design tabs.
    const lookAndFeel = await screen.findAllByRole("tab", { name: /look & feel/i });
    expect(lookAndFeel.length).toBeGreaterThan(0);
    expect(screen.queryByRole("tab", { name: /^visuals$/i })).toBeNull();
    expect(screen.queryByRole("tab", { name: /^design$/i })).toBeNull();
  });

  it("the merged tab shows BOTH design controls and the banner/gallery sections", async () => {
    render(<BusinessPageEditor businessId={1} />);
    const tab = (await screen.findAllByRole("tab", { name: /look & feel/i }))[0];
    fireEvent.click(tab);
    await waitFor(() => expect(screen.getByTestId("design-panel")).toBeInTheDocument());
    // The former Visuals content (banner uploader) now lives under the same tab.
    expect(screen.getByTestId("banner-uploader")).toBeInTheDocument();
  });
});
