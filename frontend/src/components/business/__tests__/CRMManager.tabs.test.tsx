/** @jest-environment jsdom */
/**
 * Verifies CRMManager renders Customers / Segments / Loyalty sub-tabs and
 * switches between them. Lift-and-shift refactor — the Customers tab keeps
 * existing functionality, Segments and Loyalty are placeholders for now.
 */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";

// Minimal i18n dictionary for the keys this test touches. Keep it small —
// every test that mocks the translation provider gains a maintenance burden,
// so we only stub the labels the CRMManager renders here.
const I18N_STUB: Record<string, string> = {
  "businessDashboard.crm.tabs.customers": "Customers",
  "businessDashboard.crm.tabs.segments": "Segments",
  "businessDashboard.crm.tabs.loyalty": "Loyalty",
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
}));

jest.mock("@/api/business", () => ({
  getBusiness: jest.fn(() => Promise.resolve({ default_currency: "USD" })),
}));

// LoyaltyTab mounts when the Loyalty sub-tab is clicked. Mock its data
// loaders + toast context so the click doesn't blow up the test.
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

describe("CRMManager tabs", () => {
  it("shows Customers / Segments / Loyalty sub-tabs and switches between them", async () => {
    render(<CRMManager businessId={1} />);
    expect(
      await screen.findByRole("tab", { name: "Customers" }),
    ).toHaveAttribute("aria-selected", "true");

    fireEvent.click(screen.getByRole("tab", { name: "Loyalty" }));
    expect(screen.getByRole("tab", { name: "Loyalty" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("renders the page header title and live status once status resolves", async () => {
    render(<CRMManager businessId={1} />);
    // While the status fetch is in flight the shell renders only the loading
    // skeleton (no header), so both header assertions must wait for resolve.
    expect(
      await screen.findByText("businessDashboard.crm.title"),
    ).toBeInTheDocument();
    expect(
      await screen.findByText("businessDashboard.crm.shell.activeMode"),
    ).toBeInTheDocument();
  });
});
