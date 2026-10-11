/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { resetTestUrl } from "@/test/nextNavigationMock";

// #225: Business Page sub-tabs are URL-backed; need a stateful navigation mock.
jest.mock("next/navigation", () =>
  require("@/test/nextNavigationMock").createStatefulNavigationMock(),
);

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return { useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }), getTranslation: actual.getTranslation };
});
const mockToast = { showSuccess: jest.fn(), showError: jest.fn() };
jest.mock("@/contexts/ToastContext", () => ({ useToast: () => mockToast }));
const mockTier = { hasAccess: true, loading: false };
jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: () => mockTier }));
jest.mock("@/components/business/DesignCustomization", () => ({ __esModule: true, default: () => <div data-testid="design-panel" /> }));
// Stub the section editors so we can assert they render regardless of show_*.
jest.mock("@/components/business/OperatingHoursEditor", () => ({ __esModule: true, default: () => <div data-testid="hours-editor" /> }));
jest.mock("@/components/business/SpecialFeaturesEditor", () => ({ __esModule: true, default: () => <div data-testid="features-editor" /> }));
// Reviews tab loads enabled plugins — mock to avoid AggregateError/XHR noise.
jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    business: {
      getBusinessPlugins: jest.fn().mockResolvedValue({ plugins: [] }),
    },
  },
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    // show_operating_hours / show_special_features default false in state; the
    // editors must STILL render. (getBusiness does not set them true.)
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

describe("BusinessPageEditor — always render section editors (IA-7 #2)", () => {
  beforeEach(() => {
    resetTestUrl("/business/1/dashboard?tab=business-page");
  });

  it("renders the Operations editors even when show_* flags are false", async () => {
    render(<BusinessPageEditor businessId={1} />);
    // Navigate to the Operations tab (show_operating_hours/show_special_features
    // are false by default — the editors must STILL be present).
    const opsTab = (await screen.findAllByRole("tab", { name: /operations|hours/i }))[0];
    fireEvent.click(opsTab);
    await waitFor(() => expect(screen.getByTestId("hours-editor")).toBeInTheDocument());
    expect(screen.getByTestId("features-editor")).toBeInTheDocument();
  });

  it("the Show switch still flips the public-visibility flag (does not gate the editor)", async () => {
    render(<BusinessPageEditor businessId={1} />);
    const opsTab = (await screen.findAllByRole("tab", { name: /operations|hours/i }))[0];
    fireEvent.click(opsTab);
    await waitFor(() => expect(screen.getByTestId("hours-editor")).toBeInTheDocument());
    // The first switch in the Operations panel is the hours "show on page" toggle.
    const switches = screen.getAllByRole("switch");
    expect(switches.length).toBeGreaterThan(0);
    fireEvent.click(switches[0]);
    // Editor still present after toggling visibility off->on (no longer gated).
    expect(screen.getByTestId("hours-editor")).toBeInTheDocument();
  });
});
