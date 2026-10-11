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
const mockTier = { hasAccess: true, loading: false };
jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: () => mockTier }));
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
    // Unpublished page — the content editor must still be available.
    getBusiness: jest.fn().mockResolvedValue({
      name: "Demo",
      custom_url: "demo",
      business_page_enabled: false,
      design_settings: { primary_color: "#111111" },
      description: "",
      welcome_message: "",
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

describe("BusinessPageEditor — draft editing unlocked (Stream 9 #1)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (businessApi.getBusiness as jest.Mock).mockResolvedValue({
      name: "Demo",
      custom_url: "demo",
      business_page_enabled: false,
      design_settings: { primary_color: "#111111" },
      description: "",
      welcome_message: "",
    });
  });

  it("shows Draft status chip when the page is unpublished", async () => {
    render(<BusinessPageEditor businessId={1} />);
    expect(await screen.findByText(/^Draft$/i)).toBeInTheDocument();
    expect(screen.queryByText(/^Published$/i)).not.toBeInTheDocument();
  });

  it("renders section tab strip even when the page is a draft", async () => {
    render(<BusinessPageEditor businessId={1} />);
    // i18n: essentials→Content, lookAndFeel→Look & feel, operations→Hours & Features
    expect(await screen.findByRole("tab", { name: /content/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /look & feel/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /hours & features/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /contact/i })).toBeInTheDocument();
  });

  it("lets the operator edit content fields while still in draft", async () => {
    render(<BusinessPageEditor businessId={1} />);
    const welcome = await screen.findByRole("textbox", { name: /welcome message/i });
    fireEvent.change(welcome, { target: { value: "Draft hello" } });
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));

    await waitFor(() => expect(businessApi.updateBusiness).toHaveBeenCalled());
    const payload = (businessApi.updateBusiness as jest.Mock).mock.calls.at(-1)[1];
    expect(payload.welcome_message).toContain("Draft hello");
    // Publish remains a separate action — Save must not flip the live flag.
    expect(payload).not.toHaveProperty("business_page_enabled");
  });
});
