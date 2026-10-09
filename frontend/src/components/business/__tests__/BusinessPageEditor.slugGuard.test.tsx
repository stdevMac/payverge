/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return { useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }), getTranslation: actual.getTranslation };
});
const mockToast = { showSuccess: jest.fn(), showError: jest.fn() };
jest.mock("@/contexts/ToastContext", () => ({ useToast: () => mockToast }));
jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    business: {
      getBusinessPlugins: jest.fn().mockResolvedValue({ plugins: [] }),
    },
  },
}));
jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: () => ({ hasAccess: true, loading: false }) }));
jest.mock("@/components/business/DesignCustomization", () => ({ __esModule: true, default: () => <div data-testid="design-panel" /> }));

// Stub CustomURLInput so it can fire the new onAvailabilityChange callback
// deterministically (the real one debounces a network check). Also call
// onChange so the editor marks profile dirty — SaveBar is disabled when clean.
jest.mock("@/components/business/CustomURLInput", () => ({
  __esModule: true,
  default: ({ onAvailabilityChange, onChange }: any) => (
    <button
      data-testid="slug-taken"
      onClick={() => {
        onChange?.("taken-slug");
        onAvailabilityChange?.({ checked: true, available: false });
      }}
    >
      mark taken
    </button>
  ),
}));

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
import { businessApi } from "@/api/business";

it("blocks save when the slug is known-taken and surfaces an error", async () => {
  render(<BusinessPageEditor businessId={1} />);
  fireEvent.click(await screen.findByTestId("slug-taken"));

  fireEvent.click(screen.getByRole("button", { name: /save changes/i }));

  await waitFor(() => expect(mockToast.showError).toHaveBeenCalled());
  expect(businessApi.updateBusiness).not.toHaveBeenCalled();
});
