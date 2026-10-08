/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import BusinessPageEditor from "../BusinessPageEditor";
import { businessApi } from "@/api/business";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

const mockShowSuccess = jest.fn();
const mockShowError = jest.fn();

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: jest.fn(),
}));

jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({
    showSuccess: mockShowSuccess,
    showError: mockShowError,
  }),
}));

jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    business: { getBusinessPlugins: jest.fn().mockResolvedValue({ plugins: [] }) },
  },
}));
jest.mock("@/api/publicBusiness", () => ({
  getBusinessByCustomUrl: jest.fn().mockResolvedValue(null),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => {
    const translations: Record<string, string> = {
      "businessSettings.businessPage.button.preview": "Preview page",
      "businessSettings.businessPage.button.save": "Save Changes",
      "businessSettings.businessPage.title": "Business Page Settings",
      "businessSettings.businessPage.enableDescription": "Create a public page",
    };
    return translations[key] || key;
  },
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getBusiness: jest.fn(),
    getBusinessGalleryImages: jest.fn(),
    getBusinessOperatingHours: jest.fn(),
    getBusinessSpecialFeatures: jest.fn(),
    updateBusiness: jest.fn(),
    updateBusinessOperatingHours: jest.fn(),
    updateBusinessSpecialFeatures: jest.fn(),
    updateBusinessGalleryImages: jest.fn(),
  },
}));

jest.mock("../BusinessPageToggle", () => ({
  BusinessPageToggle: () => <div data-testid="business-page-toggle" />,
}));

jest.mock("../BannerImageUploader", () => function MockBannerImageUploader() {
  return <div data-testid="banner-uploader" />;
});
jest.mock("../CustomURLInput", () => function MockCustomURLInput() {
  return <div data-testid="custom-url-input" />;
});
jest.mock("../GalleryImageUploader", () => function MockGalleryImageUploader() {
  return <div data-testid="gallery-uploader" />;
});
jest.mock("../OperatingHoursEditor", () => function MockOperatingHoursEditor() {
  return <div data-testid="hours-editor" />;
});
jest.mock("../SpecialFeaturesEditor", () => function MockSpecialFeaturesEditor() {
  return <div data-testid="features-editor" />;
});
jest.mock("../GoogleBusinessSearch", () => function MockGoogleBusinessSearch() {
  return <div data-testid="google-business-search" />;
});
jest.mock("../DashboardLockedTabView", () => function MockDashboardLockedTabView() {
  return <div data-testid="locked-tab" />;
});

describe("BusinessPageEditor preview action", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (useBusinessAccess as jest.Mock).mockReturnValue({
      hasAccess: true,
      loading: false,
    });
    (businessApi.getBusiness as jest.Mock).mockResolvedValue({
      id: 1,
      name: "Mara Core Kitchen",
      address: {},
      custom_url: "mara-core-kitchen",
      business_page_enabled: true,
      show_reviews: true,
    });
    (businessApi.getBusinessGalleryImages as jest.Mock).mockResolvedValue([]);
    (businessApi.getBusinessOperatingHours as jest.Mock).mockResolvedValue([]);
    (businessApi.getBusinessSpecialFeatures as jest.Mock).mockResolvedValue([]);
  });

  it("renders a preview link to the public business page", async () => {
    render(<BusinessPageEditor businessId={1} />);

    const previewLink = await screen.findByRole("link", {
      name: "Preview page",
    });

    expect(previewLink).toHaveAttribute("href", "/b/mara-core-kitchen");
    expect(previewLink).toHaveAttribute("target", "_blank");
    expect(previewLink).toHaveAttribute("rel", "noopener noreferrer");
  });
});
