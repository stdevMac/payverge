/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return { useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }), getTranslation: actual.getTranslation };
});
// The real useToast returns stable callbacks. A fresh jest.fn() per render
// changes loadData's identity every render and reloads the page in a loop.
jest.mock("@/contexts/ToastContext", () => {
  const toast = { showSuccess: jest.fn(), showError: jest.fn() };
  return { useToast: () => toast };
});
jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    business: {
      getBusinessPlugins: jest.fn().mockResolvedValue({ plugins: [] }),
    },
  },
}));
jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: () => ({ hasAccess: true, loading: false }) }));
jest.mock("@/components/business/DesignCustomization", () => ({ __esModule: true, default: () => <div data-testid="design-panel" /> }));

// Stub the toggle so onToggle fires directly (the real one is gated behind a
// confirm modal). Both header + card variants render this stub; we click one.
jest.mock("@/components/business/BusinessPageToggle", () => ({
  BusinessPageToggle: ({ onToggle }: { onToggle: (v: boolean) => void }) => (
    <button data-testid="toggle-publish" onClick={() => onToggle(false)}>toggle</button>
  ),
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getBusiness: jest.fn().mockResolvedValue({ name: "Demo", custom_url: "demo", business_page_enabled: true, welcome_message: "WM", design_settings: { primary_color: "#111111" } }),
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

it("publish toggle sends ONLY business_page_enabled, never the in-progress form", async () => {
  render(<BusinessPageEditor businessId={1} />);
  const toggles = await screen.findAllByTestId("toggle-publish");
  fireEvent.click(toggles[0]);

  await waitFor(() => expect(businessApi.updateBusiness).toHaveBeenCalled());
  const payload = (businessApi.updateBusiness as jest.Mock).mock.calls.at(-1)[1];

  expect(payload).toEqual({ business_page_enabled: false });
  expect(payload).not.toHaveProperty("welcome_message");
  expect(payload).not.toHaveProperty("banner_images");
});
