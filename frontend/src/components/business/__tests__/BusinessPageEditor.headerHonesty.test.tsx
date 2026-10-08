/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

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
jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => mockTier,
}));
jest.mock("@/components/business/DesignCustomization", () => ({
  __esModule: true,
  default: () => <div data-testid="design-panel" />,
}));
// Keep real toggle so we can assert Disable placement relative to Preview.
jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    business: { getBusinessPlugins: jest.fn().mockResolvedValue({ plugins: [] }) },
  },
}));
jest.mock("@/api/business", () => ({
  businessApi: {
    getBusiness: jest.fn().mockResolvedValue({
      name: "Demo",
      custom_url: "demo-cafe",
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
import { businessApi } from "@/api/business";

describe("BusinessPageEditor — honest header and destructive placement", () => {
  it("shows published header copy when the page is already published", async () => {
    render(<BusinessPageEditor businessId={1} />);
    await waitFor(() => {
      expect(screen.getByText(/Published/i)).toBeInTheDocument();
    });
    // Must not claim "Create a public page…" on an already-published page.
    expect(
      screen.queryByText(/Create a public page for your business/i),
    ).not.toBeInTheDocument();
    // Published subtitle names the live slug / managing the live page.
    expect(
      screen.getByText(/Your public page is live at \/b\/demo-cafe/i),
    ).toBeInTheDocument();
  });

  it("does not claim the page is live while it has no saved slug", async () => {
    (businessApi.getBusiness as jest.Mock).mockResolvedValueOnce({
      name: "Demo",
      custom_url: "",
      business_page_enabled: true,
      design_settings: { primary_color: "#111111" },
    });
    render(<BusinessPageEditor businessId={1} />);
    await waitFor(() => {
      expect(screen.getByText(/Needs a URL/i)).toBeInTheDocument();
    });
    expect(screen.queryByText(/Your public page is live/i)).not.toBeInTheDocument();
    expect(
      screen.getByText(/guests cannot reach it until you save a Custom URL Slug/i),
    ).toBeInTheDocument();
  });

  it("keeps Disable Page out of the header and in the Essentials danger zone (#214)", async () => {
    render(<BusinessPageEditor businessId={1} />);
    const disable = await screen.findByRole("button", { name: /Disable Page/i });
    const preview = await screen.findByRole("link", { name: /Preview page/i });
    const header = preview.closest("header");
    const danger = screen.getByTestId("business-page-danger-zone");

    expect(header).toBeTruthy();
    expect(header).not.toContainElement(disable);
    expect(header).toContainElement(preview);
    expect(danger).toContainElement(disable);
  });
});
