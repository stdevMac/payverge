/**
 * L1-26 — icon-only "go to businesses" control must expose a real accessible
 * name via aria-label (not title-as-name), type="button", decorative icon
 * aria-hidden, and a hit target ≥ the 24px AA floor with preferred AAA size.
 *
 * @jest-environment jsdom
 */

import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import DashboardSidebar from "../DashboardSidebar";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: jest.fn() }));

const mockNavigateToVenuesOverview = jest.fn();
jest.mock("@/utils/businessUrl", () => ({
  ...jest.requireActual("@/utils/businessUrl"),
  navigateToVenuesOverview: () => mockNavigateToVenuesOverview(),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: () => {} }),
  getTranslation: (key: string) => key,
}));

jest.mock("next/image", () => ({
  __esModule: true,
  // eslint-disable-next-line @next/next/no-img-element, jsx-a11y/alt-text
  default: (props: any) => <img {...props} />,
}));

jest.mock("@/hooks/useFailedInvoiceCount", () => ({
  useFailedInvoiceCount: jest.fn(() => 0),
}));

jest.mock("@/hooks/useChatUnreadCount", () => ({
  useChatUnreadCount: jest.fn(() => 0),
}));

const mockTierDefaults = {
  access: null,
  loading: false,
  error: null,
  hasAccess: true,
  isSuspended: false,
  lockState: "active" as const,
  aiConfigured: true,
  refetch: jest.fn(),
};

function renderSidebar() {
  return render(
    <DashboardSidebar
      business={{ id: 42, name: "Test Bistro", custom_url: "test-bistro" } as any}
      activeTab="overview"
      setActiveTab={jest.fn()}
      sidebarOpen
      setSidebarOpen={jest.fn()}
      allowedTabs={[]}
      isStaffUser={false}
      staffData={null}
      globalOrders={{}}
      upcomingReservations={[]}
      tutorialOpen={false}
      tutorialTabKey={null}
      tutorialTarget={null}
      onStartTutorial={jest.fn()}
    />,
  );
}

describe("DashboardSidebar — L1-26 go-to-businesses control", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    (useBusinessAccess as jest.Mock).mockReturnValue(mockTierDefaults);
  });

  it("exposes aria-label (not title-only), type=button, hidden icon, ≥40px target", () => {
    renderSidebar();
    const btn = screen.getByRole("button", {
      name: /businessDashboard\.goToBusinesses|goToBusinesses/i,
    });
    expect(btn).toHaveAttribute("type", "button");
    expect(btn.getAttribute("aria-label")).toBeTruthy();
    // title may remain as tooltip but must not be the sole name source
    const svg = btn.querySelector("svg");
    expect(svg).not.toBeNull();
    expect(svg).toHaveAttribute("aria-hidden", "true");
    // Target size: class must include at least h-10 w-10 (40px) — above 24 AA
    // and approaching 44 AAA (neighbours in this row are logo 32 + name).
    expect(btn.className).toMatch(/\bh-10\b|\bmin-h-\[40px\]\b|\bh-11\b/);
    expect(btn.className).toMatch(/\bw-10\b|\bmin-w-\[40px\]\b|\bw-11\b/);
  });

  it("goes to the all-venues overview, not plain /dashboard (no single-venue bounce)", () => {
    renderSidebar();
    fireEvent.click(
      screen.getByRole("button", {
        name: /businessDashboard\.goToBusinesses|goToBusinesses/i,
      }),
    );
    expect(mockNavigateToVenuesOverview).toHaveBeenCalledTimes(1);
  });
});
