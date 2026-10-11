/** @jest-environment jsdom */
/**
 * Related to issue 182 — CRM list chrome:
 * Add Customer lives in the page header next to Export, not between search
 * and tier chips. Spend thresholds and the points rule stay visible.
 */
import React from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";

const I18N_STUB: Record<string, string> = {
  "businessDashboard.crm.title": "CRM",
  "businessDashboard.crm.description": "Customers and loyalty",
  "businessDashboard.crm.export": "Export Customers",
  "businessDashboard.crm.addCustomer": "Add Customer",
  "businessDashboard.crm.tabs.customers": "Customers",
  "businessDashboard.crm.tabs.segments": "Segments",
  "businessDashboard.crm.tabs.loyalty": "Loyalty",
  "businessDashboard.crm.shell.activeMode": "Active",
  "businessDashboard.crm.shell.pausedMode": "Paused",
  "businessDashboard.crm.searchPlaceholder": "Search customers",
  "businessDashboard.crm.filterByTierAria": "Filter by loyalty tier",
  "businessDashboard.crm.customerList": "Customers",
  "businessDashboard.crm.tiers.all": "All",
  "businessDashboard.crm.tiers.legendPrefix": "Tiers by lifetime spend:",
  "businessDashboard.crm.tiers.thresholdHint": "{tier} · spent ≥ {amount}",
  "businessDashboard.crm.tiers.fallbackRule":
    "Tiers follow lifetime spend from the Loyalty program.",
  "businessDashboard.crm.points.rule":
    "Points: {rate} per {amount} spent. Balance can be adjusted and does not set the guest's tier.",
  "businessDashboard.crm.points.fallbackRule":
    "Points come from the Loyalty earn rate and optional comps — they do not set the guest's tier.",
  "businessDashboard.crm.points.independentHint":
    "Point balance is independent of lifetime spend and does not set the guest's tier.",
  "businessDashboard.crm.summary.totalCustomers": "Total customers",
  "businessDashboard.crm.summary.activeThisMonth": "Active this month",
  "businessDashboard.crm.summary.avgLifetimeSpend": "Avg lifetime spend",
  "businessDashboard.crm.summary.topTierCustomers": "Top-tier customers",
  "businessDashboard.crm.name": "Name",
  "businessDashboard.crm.email": "Email",
  "businessDashboard.crm.loyaltyPoints": "Loyalty Points",
  "businessDashboard.crm.totalSpent": "Total Spent",
  "businessDashboard.crm.visits": "Visits",
  "businessDashboard.crm.lastVisit": "Last Visit",
  "businessDashboard.crm.actions": "Actions",
  "businessDashboard.crm.view": "View",
  "businessDashboard.crm.remove.action": "Remove",
  "businessDashboard.crm.tableAria": "Customers",
  "businessDashboard.crm.customerNamePlaceholder": "Full name",
};

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (
    key: string,
    _locale?: string,
    params?: Record<string, string | number>,
  ) => {
    let value = I18N_STUB[key] ?? key;
    if (params) {
      for (const [name, next] of Object.entries(params)) {
        value = value.replaceAll(`{${name}}`, String(next));
      }
    }
    return value;
  },
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
      Promise.resolve({
        customers: [
          {
            id: 1,
            customer: { name: "Ada Guest", email: "ada@x.test" },
            loyalty_points: 210,
            loyalty_tier: "Gold",
            total_spent: 855,
            visit_count: 8,
            last_visit_at: "2026-08-01T00:00:00Z",
          },
        ],
        total: 1,
        total_pages: 1,
        summary: {
          total_customers: 1,
          active_this_month: 1,
          avg_lifetime_spend: 855,
          top_tier_count: 1,
        },
      }),
    ),
    getCustomerDetails: jest.fn(() =>
      Promise.resolve({
        customer: {
          id: 1,
          customer: { name: "Ada Guest", email: "ada@x.test" },
          loyalty_points: 210,
          loyalty_tier: "Gold",
          total_spent: 855,
          visit_count: 8,
        },
      }),
    ),
    addCustomer: jest.fn(() => Promise.resolve({ created: true })),
    exportCustomers: jest.fn(() => Promise.resolve(new Blob())),
    getCRMStatus: jest.fn(() => Promise.resolve({ enabled: true })),
    toggleCRM: jest.fn(() => Promise.resolve({ enabled: true })),
  },
}));

jest.mock("@/api/business", () => ({
  getBusiness: jest.fn(() => Promise.resolve({ default_currency: "USD" })),
}));

jest.mock("@/api/loyalty", () => ({
  getLoyalty: jest.fn(() =>
    Promise.resolve({
      program: {
        id: 1,
        business_id: 1,
        enabled: true,
        points_per_dollar: 1,
        redemption_points_per_dollar: 100,
      },
      tiers: [
        { name: "Bronze", min_lifetime_spent: 0, sort_order: 0 },
        { name: "Silver", min_lifetime_spent: 250, sort_order: 1 },
        { name: "Gold", min_lifetime_spent: 1000, sort_order: 2 },
      ],
    }),
  ),
  putLoyalty: jest.fn(() => Promise.resolve({})),
  previewLoyalty: jest.fn(() =>
    Promise.resolve({ total_customers: 0, tier_distribution: {} }),
  ),
}));

jest.mock("@/components/common/CurrencyConverter", () => ({
  CurrencyPrice: ({ amount }: { amount: number }) => <span>${amount}</span>,
}));

jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({
    showSuccess: jest.fn(),
    showError: jest.fn(),
    showInfo: jest.fn(),
    showWarning: jest.fn(),
  }),
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

import CRMManager from "@/components/business/CRMManager";

describe("CRMManager list chrome", () => {
  it("puts Add Customer in the page header next to Export, not between search and chips", async () => {
    render(<CRMManager businessId={1} />);

    const add = await screen.findByTestId("crm-add-customer");
    const exportBtn = await screen.findByTestId("crm-export-customers");
    expect(add).toHaveTextContent("Add Customer");
    expect(exportBtn).toHaveTextContent(/Export Customers/);

    const header = screen.getByRole("banner");
    expect(within(header).getByTestId("crm-add-customer")).toBeInTheDocument();
    expect(within(header).getByTestId("crm-export-customers")).toBeInTheDocument();

    const filters = await screen.findByTestId("crm-customer-filters");
    expect(
      within(filters).queryByRole("button", { name: /Add Customer/i }),
    ).not.toBeInTheDocument();
    expect(within(filters).getByPlaceholderText("Search customers")).toBeInTheDocument();
    expect(within(filters).getByRole("button", { name: "Gold" })).toBeInTheDocument();

    const search = within(filters).getByPlaceholderText("Search customers");
    expect(add.compareDocumentPosition(search) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("surfaces loyalty spend thresholds and the points earn rule on the list", async () => {
    render(<CRMManager businessId={1} />);

    const legend = await screen.findByTestId("crm-tier-legend");
    expect(legend).toHaveTextContent("Tiers by lifetime spend:");
    expect(legend).toHaveTextContent(/Gold/);
    expect(legend).toHaveTextContent(/1000|1,000/);

    const pointsRule = await screen.findByTestId("crm-points-rule");
    expect(pointsRule).toHaveTextContent(/Points:/);
    expect(pointsRule).toHaveTextContent(/does not set the guest's tier/);
  });

  it("opens the add-customer modal from the page header", async () => {
    render(<CRMManager businessId={1} />);
    fireEvent.click(await screen.findByTestId("crm-add-customer"));
    await waitFor(() => {
      expect(screen.getByTestId("crm-add-name")).toBeInTheDocument();
    });
  });
});
