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
jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: () => ({ hasAccess: true, loading: false }) }));
jest.mock("@/components/business/DesignCustomization", () => ({ __esModule: true, default: () => <div data-testid="design-panel" /> }));
// Reviews tab loads enabled plugins — mock to avoid AggregateError/XHR noise.
jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    business: {
      getBusinessPlugins: jest.fn().mockResolvedValue({ plugins: [] }),
    },
  },
}));

// Stub GoogleBusinessSearch: surface currentGoogleInfo + a button that fires the
// new onUpdate slice (simulating a successful link).
jest.mock("@/components/business/GoogleBusinessSearch", () => ({
  __esModule: true,
  default: ({ currentGoogleInfo, onUpdate }: any) => (
    <div>
      <span data-testid="cur-pid">{currentGoogleInfo?.google_place_id || "none"}</span>
      <button data-testid="g-link" onClick={() => onUpdate?.({
        google_place_id: "PID123", google_business_name: "Linked Co",
        google_review_link: "rl", google_business_url: "bu", google_reviews_enabled: true,
      })}>link</button>
    </div>
  ),
}));

const mockGetBusiness = jest.fn().mockResolvedValue({ name: "Demo", custom_url: "demo", business_page_enabled: true, design_settings: { primary_color: "#111111" } });
jest.mock("@/api/business", () => ({
  businessApi: {
    getBusiness: (...a: any[]) => mockGetBusiness(...a),
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

beforeEach(() => {
  resetTestUrl("/business/1/dashboard?tab=business-page");
});

it("merges the Google slice in place without a full reload", async () => {
  render(<BusinessPageEditor businessId={1} />);
  // Navigate to the Reviews tab where GoogleBusinessSearch lives.
  // The desktop strip is now a WAI-ARIA tablist (role="tab", not "button").
  const reviewsTab = await screen.findByRole("tab", { name: /reviews/i });
  fireEvent.click(reviewsTab);

  expect(await screen.findByTestId("cur-pid")).toHaveTextContent("none");
  await waitFor(() => expect(mockGetBusiness).toHaveBeenCalledTimes(1));

  fireEvent.click(screen.getByTestId("g-link"));

  // Merged in place: currentGoogleInfo updates, and getBusiness was NOT called
  // again (no full loadData reload that would wipe unsaved edits).
  await waitFor(() => expect(screen.getByTestId("cur-pid")).toHaveTextContent("PID123"));
  expect(mockGetBusiness).toHaveBeenCalledTimes(1);
});
