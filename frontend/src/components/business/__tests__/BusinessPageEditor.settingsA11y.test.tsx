/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { resetTestUrl } from "@/test/nextNavigationMock";
import {
  announcedSwitchName,
  labelledByTargetsHaveText,
} from "@/components/ui/namedControl";

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

const mockToast = { showSuccess: jest.fn(), showError: jest.fn() };
jest.mock("@/contexts/ToastContext", () => ({ useToast: () => mockToast }));
jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({ hasAccess: true, loading: false }),
}));
jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    business: {
      getBusinessPlugins: jest.fn().mockResolvedValue({ plugins: [] }),
    },
  },
}));
jest.mock("@/components/business/DesignCustomization", () => ({
  __esModule: true,
  default: () => <div data-testid="design-panel" />,
}));
jest.mock("@/components/business/BannerImageUploader", () => ({
  __esModule: true,
  default: () => <div data-testid="banner-uploader" />,
}));
jest.mock("@/components/business/GalleryImageUploader", () => ({
  __esModule: true,
  default: () => <div data-testid="gallery-uploader" />,
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getBusiness: jest.fn().mockResolvedValue({
      name: "Demo",
      custom_url: "demo",
      business_page_enabled: true,
      show_gallery: false,
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

describe("BusinessPageEditor setting switches (#446)", () => {
  beforeEach(() => {
    resetTestUrl("/business/1/dashboard?tab=business-page");
  });

  it("names the Look & feel Photo Gallery switch by purpose and state", async () => {
    render(<BusinessPageEditor businessId={1} />);
    const tab = (await screen.findAllByRole("tab", { name: /look & feel/i }))[0];
    fireEvent.click(tab);
    await waitFor(() =>
      expect(screen.getByTestId("design-panel")).toBeInTheDocument(),
    );

    const gallery = await screen.findByRole("switch", {
      name: "Photo Gallery",
    });
    expect(announcedSwitchName(gallery)).toBe("Photo Gallery, off");
    expect(labelledByTargetsHaveText(gallery)).toBe(true);
  });
});
