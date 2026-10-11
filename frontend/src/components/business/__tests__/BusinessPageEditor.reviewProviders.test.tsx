/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { resetTestUrl } from "@/test/nextNavigationMock";

// #225: Business Page sub-tabs are URL-backed; need a stateful navigation mock.
jest.mock("next/navigation", () =>
  require("@/test/nextNavigationMock").createStatefulNavigationMock(),
);

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});
// Stable mock return: loadData depends on showError, so a fresh jest.fn() per
// render would change loadData's identity and re-fire the load effect forever.
const mockToast = { showSuccess: jest.fn(), showError: jest.fn() };
jest.mock("@/contexts/ToastContext", () => ({ useToast: () => mockToast }));
const mockTier = { hasAccess: true, loading: false };
jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => mockTier,
}));
jest.mock("@/components/business/DesignCustomization", () => ({
  __esModule: true,
  default: () => <div data-testid="design-panel" />,
}));
jest.mock("@/components/business/BusinessPageToggle", () => ({
  BusinessPageToggle: () => <button type="button">toggle</button>,
}));
jest.mock("@/components/business/GoogleBusinessSearch", () => ({
  __esModule: true,
  default: () => <div data-testid="google-provider">Google reviews</div>,
}));

jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    business: {
      getBusinessPlugins: jest.fn(),
    },
  },
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getBusiness: jest.fn().mockResolvedValue({
      name: "Demo",
      custom_url: "demo",
      business_page_enabled: true,
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
import { pluginAPI } from "@/api/plugins";

const mockGetBusinessPlugins = pluginAPI.business
  .getBusinessPlugins as jest.Mock;

async function openReviewsTab() {
  render(<BusinessPageEditor businessId={1} />);
  const reviewsTab = await screen.findByRole(
    "tab",
    { name: /reviews/i },
    { timeout: 5000 },
  );
  fireEvent.click(reviewsTab);
  await waitFor(
    () => expect(screen.getByTestId("google-provider")).toBeInTheDocument(),
    { timeout: 5000 },
  );
}

describe("BusinessPageEditor — review providers from plugins", () => {
  beforeEach(() => {
    resetTestUrl("/business/1/dashboard?tab=business-page");
    mockGetBusinessPlugins.mockReset();
    mockGetBusinessPlugins.mockResolvedValue({ plugins: [] });
  });

  it("offers Trustpilot when the trustpilot plugin is enabled for the business", async () => {
    mockGetBusinessPlugins.mockResolvedValue({
      plugins: [
        {
          is_enabled: true,
          plugin: { name: "trustpilot", display_name: "Trustpilot" },
          config: JSON.stringify({
            business_name: "Demo Cafe",
            trustpilot_url: "https://www.trustpilot.com/review/demo",
          }),
        },
        {
          is_enabled: true,
          plugin: { name: "stripe", display_name: "Stripe" },
        },
      ],
    });

    await openReviewsTab();

    expect(mockGetBusinessPlugins).toHaveBeenCalled();
    expect(screen.getByTestId("review-provider-trustpilot")).toBeInTheDocument();
    expect(
      screen.getByTestId("review-provider-trustpilot").textContent,
    ).toMatch(/Trustpilot/i);
  });

  it("does not offer Trustpilot when the plugin is not enabled", async () => {
    mockGetBusinessPlugins.mockResolvedValue({
      plugins: [
        {
          is_enabled: false,
          plugin: { name: "trustpilot", display_name: "Trustpilot" },
        },
      ],
    });

    await openReviewsTab();

    expect(screen.queryByTestId("review-provider-trustpilot")).toBeNull();
  });
});
