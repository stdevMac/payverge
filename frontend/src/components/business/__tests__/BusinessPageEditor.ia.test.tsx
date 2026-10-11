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
// Stable mock return objects/spies: loadData depends on showError, so a fresh
// jest.fn() per render would change loadData's identity and re-fire the load
// effect (transient spinner mid-assertion). Module-level consts keep it stable.
const mockToast = { showSuccess: jest.fn(), showError: jest.fn() };
jest.mock("@/contexts/ToastContext", () => ({ useToast: () => mockToast }));
const mockTier = { hasAccess: true, loading: false };
jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: () => mockTier }));
// Reviews tab loads enabled plugins — mock to avoid AggregateError/XHR noise.
jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    business: {
      getBusinessPlugins: jest.fn().mockResolvedValue({ plugins: [] }),
    },
  },
}));

// Mock the api surface BusinessPageEditor calls in loadData + handleSave; the
// design save spy is asserted via the imported businessApi below.
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
  // CustomURLInput's availability check — stubbed so the slug field's
  // debounced lookup is deterministic and never hits the network.
  checkCustomURLAvailability: jest.fn().mockResolvedValue({ available: true }),
}));
// Stub DesignCustomization to a marker so we can find the Design sub-tab. It
// exposes a button that fires onDesignSettingsChange so a test can dirty the
// design section (diff-aware save only PUTs sections the operator edited).
// `__esModule`/`default` form so the component's default import resolves the
// stub (a bare `() => () => <div/>` would leave the default import undefined).
jest.mock("@/components/business/DesignCustomization", () => ({
  __esModule: true,
  default: ({
    designSettings,
    onDesignSettingsChange,
  }: {
    designSettings: Record<string, unknown>;
    onDesignSettingsChange: (ds: Record<string, unknown>) => void;
  }) => (
    <div data-testid="design-panel">
      <button
        type="button"
        onClick={() =>
          onDesignSettingsChange({ ...designSettings, primary_color: "#111111" })
        }
      >
        edit design
      </button>
    </div>
  ),
}));

import BusinessPageEditor from "@/components/business/BusinessPageEditor";
import { businessApi } from "@/api/business";

beforeEach(() => {
  resetTestUrl("/business/1/dashboard?tab=business-page");
});

it("edits welcome message + about story and saves them", async () => {
  const { businessApi } = require("@/api/business");
  render(<BusinessPageEditor businessId={1} />);

  const welcome = await screen.findByRole("textbox", { name: /welcome message/i });
  fireEvent.change(welcome, { target: { value: "Hi there" } });

  fireEvent.click(screen.getByRole("button", { name: /save changes/i }));

  await waitFor(() => expect(businessApi.updateBusiness).toHaveBeenCalled());
  const payload = businessApi.updateBusiness.mock.calls.at(-1)[1];
  expect(payload.welcome_message).toContain("Hi there");
});

it("renders a Design sub-tab and includes design settings in the save batch", async () => {
  render(<BusinessPageEditor businessId={1} />);
  // fireEvent.click (not userEvent) for the raw <button> tab strip: userEvent's
  // pointer sequence does not flip activeTab on these plain onClick buttons.
  // Phase 4: the Design + Visuals tabs merged into one "Look & feel" tab.
  // The desktop strip is now a WAI-ARIA tablist (role="tab", not "button").
  const designTab = await screen.findByRole("tab", { name: /look & feel/i });
  fireEvent.click(designTab);
  await waitFor(() => expect(screen.getByTestId("design-panel")).toBeInTheDocument());

  // Diff-aware save only PUTs edited sections, so actually edit a design value
  // (via the stub's trigger) before saving — a bare tab switch must not churn
  // the design section.
  fireEvent.click(screen.getByRole("button", { name: /edit design/i }));
  fireEvent.click(screen.getByRole("button", { name: /save changes/i }));
  await waitFor(() => expect(businessApi.updateBusinessDesignSettings).toHaveBeenCalledWith(1, expect.objectContaining({ primary_color: "#111111" })));
});

it("offers a 'Set your page URL' action that switches to Content when no slug is set", async () => {
  const { businessApi } = require("@/api/business");
  businessApi.getBusiness.mockResolvedValue({ name: "Demo", custom_url: "", business_page_enabled: true });
  render(<BusinessPageEditor businessId={1} />);

  const setUrlBtn = await screen.findByRole("button", { name: /set your page url/i });
  fireEvent.click(setUrlBtn);
  // Lands on Content (essentials), where the slug input renders the payverge.io/b/ prefix.
  const prefix = await screen.findAllByText(/payverge\.io\/b\//i);
  expect(prefix.length).toBeGreaterThan(0);
});

it("shows the live-page link (Open ↗) when a slug is already saved", async () => {
  const { businessApi } = require("@/api/business");
  businessApi.getBusiness.mockResolvedValue({ name: "Demo", custom_url: "demo", business_page_enabled: true });
  render(<BusinessPageEditor businessId={1} />);

  // The live-page zone is driven by the persisted slug (settings.custom_url),
  // wired through CustomURLInput's savedUrl prop.
  const link = await screen.findByRole("link", { name: /open live page/i });
  expect(link).toHaveAttribute("href", "https://payverge.io/b/demo");
});
