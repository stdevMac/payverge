/** @jest-environment jsdom */
/**
 * D1 / L5-11: CRM segment drilldown must write `?focus=` into the URL (not only
 * React state). Clicking a segment card on the Segments tab must land on
 * Customers with focus=lapsed (etc.) via the real openSegment → useOptionalUrlState path.
 */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

const mockReplace = jest.fn();
let mockSearch = new URLSearchParams("tab=crm&sub=segments");

jest.mock("next/navigation", () => ({
  useSearchParams: () => mockSearch,
  usePathname: () => "/business/1/dashboard",
  useRouter: () => ({ replace: mockReplace, push: jest.fn() }),
}));

const I18N_STUB: Record<string, string> = {
  "businessDashboard.crm.tabs.customers": "Customers",
  "businessDashboard.crm.tabs.segments": "Segments",
  "businessDashboard.crm.tabs.loyalty": "Loyalty",
  "businessDashboard.crm.segments.lapsed.title": "Lapsed",
  "businessDashboard.crm.segments.lapsed.description": "Haven't visited",
  "businessDashboard.crm.segments.vip.title": "VIP",
  "businessDashboard.crm.segments.vip.description": "Top spenders",
  "businessDashboard.crm.segments.new.title": "New",
  "businessDashboard.crm.segments.new.description": "First visits",
  "businessDashboard.crm.segments.atRisk.title": "At risk",
  "businessDashboard.crm.segments.atRisk.description": "Drifting",
  "businessDashboard.crm.segments.viewCustomers": "View customers",
  "businessDashboard.crm.segments.count": "{count} customers",
};

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => I18N_STUB[key] ?? key,
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    access: null,
    loading: false,
    error: null,
    hasAccess: true,
    isSuspended: false,
    lockState: "active",
    aiConfigured: false,
    refetch: jest.fn(),
  }),
}));

jest.mock("@/api/crm", () => ({
  businessCRMAPI: {
    getCustomers: jest.fn(() =>
      Promise.resolve({ customers: [], total_pages: 1 }),
    ),
    getCustomerDetails: jest.fn(() => Promise.resolve({ customer: null })),
    updateCustomerNotes: jest.fn(() => Promise.resolve({})),
    updateCustomerTags: jest.fn(() => Promise.resolve({})),
    exportCustomers: jest.fn(() => Promise.resolve(new Blob())),
    getCRMStatus: jest.fn(() => Promise.resolve({ enabled: true })),
    setCRMStatus: jest.fn(() => Promise.resolve({ enabled: true })),
  },
  getSegments: jest.fn(() =>
    Promise.resolve({ lapsed: 3, vip: 1, new: 2, atRisk: 0, at_risk: 0 }),
  ),
}));

jest.mock("@/api/business", () => ({
  getBusiness: jest.fn(() => Promise.resolve({ default_currency: "USD" })),
}));

jest.mock("@/api/loyalty", () => ({
  getLoyalty: jest.fn(() =>
    Promise.resolve({
      program: { id: 1, business_id: 1, enabled: true, points_per_dollar: 1 },
      tiers: [],
    }),
  ),
  putLoyalty: jest.fn(() => Promise.resolve({})),
  previewLoyalty: jest.fn(() =>
    Promise.resolve({ total_customers: 0, tier_distribution: {} }),
  ),
}));

jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({
    showSuccess: jest.fn(),
    showError: jest.fn(),
    showInfo: jest.fn(),
    showWarning: jest.fn(),
  }),
}));

import CRMManager from "@/components/business/CRMManager";
import { getSegments } from "@/api/crm";

describe("CRMManager L5-11 focus write-back DOM (D1)", () => {
  beforeEach(() => {
    mockReplace.mockClear();
    mockSearch = new URLSearchParams("tab=crm&sub=segments");
    (getSegments as jest.Mock).mockResolvedValue({
      lapsed: 3,
      vip: 1,
      new: 2,
      atRisk: 0,
      at_risk: 0,
    });
  });

  it("writes focus=lapsed when a segment card is opened", async () => {
    render(
      <CRMManager
        businessId={1}
        subTab="segments"
        onSubTabChange={jest.fn()}
      />,
    );

    // Wait for segments to load (non-empty counts).
    const lapsed = await screen.findByText("Lapsed");
    fireEvent.click(lapsed.closest("button") || lapsed);

    await waitFor(() => {
      expect(mockReplace).toHaveBeenCalled();
    });
    const hrefs = mockReplace.mock.calls.map((c) => String(c[0]));
    expect(hrefs.some((h) => h.includes("focus=lapsed"))).toBe(true);
  });

  it("keeps sub=customers and focus=at-risk on the FINAL href (#376)", async () => {
    // Dashboard handleCrmSubTab reads window.location and only writes tab+sub.
    // A second replace from that path must not win and drop focus.
    window.history.replaceState(
      null,
      "",
      "/business/1/dashboard?tab=crm&sub=segments",
    );
    (getSegments as jest.Mock).mockResolvedValue({
      lapsed: 3,
      vip: 1,
      new: 2,
      atRisk: 2,
      at_risk: 2,
    });

    const handleCrmSubTab = (sub: "customers" | "segments" | "loyalty") => {
      const url = new URL(window.location.href);
      url.searchParams.set("tab", "crm");
      url.searchParams.set("sub", sub);
      mockReplace(url.pathname + url.search, { scroll: false });
    };

    render(
      <CRMManager
        businessId={1}
        subTab="segments"
        onSubTabChange={handleCrmSubTab}
      />,
    );

    const atRisk = await screen.findByRole("button", { name: /At risk — 2/i });
    fireEvent.click(atRisk);

    await waitFor(() => {
      expect(mockReplace).toHaveBeenCalled();
    });
    expect(mockReplace).toHaveBeenCalledTimes(1);
    const finalHref = String(mockReplace.mock.calls.at(-1)?.[0] ?? "");
    expect(finalHref).toContain("sub=customers");
    expect(finalHref).toContain("focus=at-risk");
  });
});
