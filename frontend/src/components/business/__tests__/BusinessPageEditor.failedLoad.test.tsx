/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "es-AR", setLocale: jest.fn() }),
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
    getBusiness: jest.fn(),
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

import { businessApi } from "@/api/business";
import BusinessPageEditor from "@/components/business/BusinessPageEditor";

const mockGetBusiness = businessApi.getBusiness as jest.MockedFunction<
  typeof businessApi.getBusiness
>;

describe("BusinessPageEditor — failed load must not look unpublished (#691)", () => {
  beforeEach(() => {
    mockGetBusiness.mockReset();
    mockGetBusiness.mockRejectedValue(new Error("network"));
  });

  it("does not show Draft / Activate while the live config failed to load", async () => {
    render(<BusinessPageEditor businessId={1} />);
    await waitFor(() => {
      expect(screen.getByText(/Estado no disponible|Status unavailable/i)).toBeInTheDocument();
    });
    expect(screen.queryByText(/^Borrador$|^Draft$/i)).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Activar Página Comercial|Activate Business Page/i }),
    ).not.toBeInTheDocument();
  });

  it("retry recovers to Published instead of inviting Activate on a live page", async () => {
    mockGetBusiness
      .mockRejectedValueOnce(new Error("network"))
      .mockResolvedValueOnce({
        name: "Demo Lounge",
        custom_url: "payverge-ai-pro-demo-lounge",
        business_page_enabled: true,
        description: "Live page",
        design_settings: { primary_color: "#111111" },
      } as Awaited<ReturnType<typeof businessApi.getBusiness>>);

    render(<BusinessPageEditor businessId={1} />);
    await waitFor(() => {
      expect(screen.getByText(/Estado no disponible|Status unavailable/i)).toBeInTheDocument();
    });
    fireEvent.click(
      screen.getByRole("button", { name: /Reintentar|Retry/i }),
    );
    expect(
      await screen.findByText(/^Publicada$|^Published$/i),
    ).toBeInTheDocument();
    expect(screen.queryByText(/^Borrador$|^Draft$/i)).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Activar Página Comercial|Activate Business Page/i }),
    ).not.toBeInTheDocument();
  });
});
