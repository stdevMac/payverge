/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
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

describe("BusinessPageEditor — accessible tablist (A11Y-1)", () => {
  beforeEach(() => {
    resetTestUrl("/business/1/dashboard?tab=business-page");
  });

  it("exposes a tablist with role=tab buttons and aria-selected on the active tab", async () => {
    render(<BusinessPageEditor businessId={1} />);
    const tablists = await screen.findAllByRole("tablist");
    expect(tablists.length).toBeGreaterThan(0);
    // Essentials is the default active tab.
    const tabs = screen.getAllByRole("tab");
    expect(tabs.length).toBeGreaterThan(0);
    const active = tabs.filter((t) => t.getAttribute("aria-selected") === "true");
    expect(active).toHaveLength(1);
    // Roving tabindex: the active tab is the only tabbable one. The strip is
    // now the shared SegmentedTabs, which owns the tablist/tab semantics and
    // emits no per-tab aria-controls (there is no separate tabpanel id to
    // point at — the section content is a single keyed region).
    expect(active[0]).toHaveAttribute("tabindex", "0");
    tabs
      .filter((t) => t.getAttribute("aria-selected") !== "true")
      .forEach((t) => expect(t).toHaveAttribute("tabindex", "-1"));
  });

  it("ArrowRight moves selection to the next tab; Home/End jump to first/last", async () => {
    render(<BusinessPageEditor businessId={1} />);
    await screen.findAllByRole("tablist");
    const tabs = screen.getAllByRole("tab");
    const first = tabs[0];
    first.focus();
    fireEvent.keyDown(first, { key: "ArrowRight" });
    // After ArrowRight the 2nd tab (Look & feel) becomes selected.
    const afterRight = screen.getAllByRole("tab");
    expect(afterRight[1]).toHaveAttribute("aria-selected", "true");

    fireEvent.keyDown(afterRight[1], { key: "End" });
    const afterEnd = screen.getAllByRole("tab");
    expect(afterEnd[afterEnd.length - 1]).toHaveAttribute("aria-selected", "true");

    fireEvent.keyDown(afterEnd[afterEnd.length - 1], { key: "Home" });
    const afterHome = screen.getAllByRole("tab");
    expect(afterHome[0]).toHaveAttribute("aria-selected", "true");
  });

  it("serves every breakpoint from a single scrollable tablist (no mobile Select)", async () => {
    render(<BusinessPageEditor businessId={1} />);
    // The custom desktop-tablist + mobile-<Select> split was replaced by the
    // shared SegmentedTabs strip, which scrolls horizontally on narrow screens.
    // One tablist exposes every section as a role=tab at all breakpoints.
    const tablists = await screen.findAllByRole("tablist");
    expect(tablists).toHaveLength(1);
    const tabs = screen.getAllByRole("tab");
    expect(tabs.length).toBe(5);
    // Labels come from the real i18n bundle (businessSettings.businessPage.tabs.*).
    ["Content", "Look & feel", "Hours & Features", "Reviews", "Contact"].forEach(
      (name) => {
        expect(
          screen.getByRole("tab", { name: new RegExp(name, "i") }),
        ).toBeInTheDocument();
      },
    );
  });
});
