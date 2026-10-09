/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import BusinessPageEditor from "../BusinessPageEditor";
import { businessApi } from "@/api/business";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { resetTestUrl } from "@/test/nextNavigationMock";

// #225: Business Page sub-tabs are URL-backed; need a stateful navigation mock.
jest.mock("next/navigation", () =>
  require("@/test/nextNavigationMock").createStatefulNavigationMock(),
);

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

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => {
    const translations: Record<string, string> = {
      "businessSettings.businessPage.button.preview": "Preview page",
      "businessSettings.businessPage.button.previewDraftDisabled":
        "Publish to open the public page",
      "businessSettings.businessPage.button.previewLiveDesign":
        "Open design live preview",
      "businessSettings.businessPage.button.previewDraftTooltip":
        "This page is still a draft.",
      "businessSettings.businessPage.button.setUrl": "Set your page URL",
      "businessSettings.businessPage.button.save": "Save Changes",
      "businessSettings.businessPage.title": "Business Page Settings",
      "businessSettings.businessPage.enableDescription": "Create a public page",
      "businessSettings.businessPage.status.draft": "Draft",
    };
    return translations[key] || key;
  },
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
// Default-export stubs go through the explicit `{ __esModule, default }` shape
// rather than returning a bare component factory: the bare form is an anonymous
// component definition, which `react/display-name` rejects.
jest.mock("../BannerImageUploader", () => ({
  __esModule: true,
  default: () => <div data-testid="banner-uploader" />,
}));
jest.mock("../CustomURLInput", () => ({
  __esModule: true,
  default: () => <div data-testid="custom-url-input" />,
}));
jest.mock("../GalleryImageUploader", () => ({
  __esModule: true,
  default: () => <div data-testid="gallery-uploader" />,
}));
jest.mock("../OperatingHoursEditor", () => ({
  __esModule: true,
  default: () => <div data-testid="hours-editor" />,
}));
jest.mock("../SpecialFeaturesEditor", () => ({
  __esModule: true,
  default: () => <div data-testid="features-editor" />,
}));
jest.mock("../GoogleBusinessSearch", () => ({
  __esModule: true,
  default: () => <div data-testid="google-business-search" />,
}));
jest.mock("../DashboardLockedTabView", () => ({
  __esModule: true,
  default: () => <div data-testid="locked-tab" />,
}));
jest.mock("../DesignCustomization", () => ({
  __esModule: true,
  default: () => <div data-testid="design-customization" />,
}));

describe("BusinessPageEditor draft-with-slug residual (L6-27)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    resetTestUrl("/business/1/dashboard?tab=business-page");
    (useBusinessAccess as jest.Mock).mockReturnValue({
      hasAccess: true,
      loading: false,
    });
    (businessApi.getBusiness as jest.Mock).mockResolvedValue({
      id: 1,
      name: "Draft Kitchen",
      address: {},
      custom_url: "draft-kitchen",
      business_page_enabled: false, // draft
      show_reviews: true,
    });
    (businessApi.getBusinessGalleryImages as jest.Mock).mockResolvedValue([]);
    (businessApi.getBusinessOperatingHours as jest.Mock).mockResolvedValue([]);
    (businessApi.getBusinessSpecialFeatures as jest.Mock).mockResolvedValue([]);
  });

  it("shows disabled public preview + design live-preview link when draft has a slug", async () => {
    render(<BusinessPageEditor businessId={1} />);

    const disabled = await screen.findByTestId("draft-preview-disabled");
    expect(disabled).toBeDisabled();
    expect(screen.queryByRole("link", { name: "Preview page" })).not.toBeInTheDocument();

    const designLink = screen.getByTestId("draft-design-preview-link");
    expect(designLink).toHaveTextContent("Open design live preview");
    fireEvent.click(designLink);
    // Switching to design tab should surface the design customization surface
    await waitFor(() => {
      expect(screen.getByTestId("design-customization")).toBeInTheDocument();
    });
  });
});
