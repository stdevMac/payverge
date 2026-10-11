/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

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
  getBusinessByCustomUrl: jest.fn((_: string, language?: string) => {
    if (language === "es") {
      return Promise.resolve({
        description: "Un restaurante modelo de Payverge con datos operativos reales.",
        welcome_message: "Bienvenidos.",
        about_story: "Un entorno de demostración realista.",
      });
    }
    return Promise.resolve({
      description: "A Payverge showcase restaurant with realistic operational demo data.",
      welcome_message: "Welcome.",
      about_story: "A realistic demo.",
    });
  }),
}));
jest.mock("@/api/business", () => ({
  businessApi: {
    getBusiness: jest.fn().mockResolvedValue({
      name: "Demo Lounge",
      custom_url: "payverge-ai-pro-demo-lounge",
      business_page_enabled: true,
      description: "A Payverge showcase restaurant with realistic operational demo data.",
      welcome_message: "Welcome.",
      about_story: "A realistic demo.",
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
import { getBusinessByCustomUrl } from "@/api/publicBusiness";

describe("BusinessPageEditor — operator locale copy (#694)", () => {
  it("overlays Spanish public copy for an es-AR operator instead of the English seed", async () => {
    render(<BusinessPageEditor businessId={1} />);
    await waitFor(() => {
      expect(getBusinessByCustomUrl).toHaveBeenCalledWith(
        "payverge-ai-pro-demo-lounge",
        "es-AR",
      );
    });
    await waitFor(() => {
      expect(getBusinessByCustomUrl).toHaveBeenCalledWith(
        "payverge-ai-pro-demo-lounge",
        "es",
      );
    });
    expect(
      await screen.findByDisplayValue(
        /Un restaurante modelo de Payverge/i,
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByDisplayValue(/A Payverge showcase restaurant/i),
    ).not.toBeInTheDocument();
  });
});
