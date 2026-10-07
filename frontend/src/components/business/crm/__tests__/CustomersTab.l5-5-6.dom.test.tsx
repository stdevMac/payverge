/**
 * D1 / L5-5 + L5-6: CRM CustomersTab modal seams that source greps alone cannot
 * prove.
 *
 * L5-5: add-customer form must reset on open/close so values never resurrect
 * across the NextUI exit fade / next open.
 * Revert-proof: openAddModal without resetNewCustomerForm → reopen keeps "Stale".
 *
 * L5-6: details modal must open (with list-row snapshot) BEFORE awaiting
 * getCustomerDetails. Awaiting first races post-save loadCustomers skeleton
 * unmounts.
 * Revert-proof: move setDetailsModal(true) after await → with a hanging fetch
 * the details modal never appears.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import CustomersTab from "../CustomersTab";

const I18N_STUB: Record<string, string> = {
  "businessDashboard.crm.summary.totalCustomers": "Total customers",
  "businessDashboard.crm.summary.activeThisMonth": "Active this month",
  "businessDashboard.crm.summary.avgLifetimeSpend": "Avg lifetime spend",
  "businessDashboard.crm.summary.topTierCustomers": "Top-tier customers",
  "businessDashboard.crm.tiers.all": "All",
  "businessDashboard.crm.searchPlaceholder": "Search customers",
  "businessDashboard.crm.addCustomer": "Add Customer",
  "businessDashboard.crm.customerNamePlaceholder": "Full name",
  "businessDashboard.crm.phonePlaceholder": "+1 555 123 4567",
  "businessDashboard.crm.name": "Name",
  "businessDashboard.crm.email": "Email",
  "businessDashboard.crm.phone": "Phone",
  "businessDashboard.crm.validation.nameEmailRequired":
    "Name and email are required to save.",
  "businessDashboard.crm.toasts.loadError": "Couldn't load customers.",
  "businessDashboard.crm.cancel": "Cancel",
  "businessDashboard.crm.save": "Save",
};

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => I18N_STUB[key] ?? key,
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
      tiers: [],
    }),
  ),
}));

jest.mock("@/components/common/CurrencyConverter", () => ({
  CurrencyPrice: ({ amount }: { amount: number }) => <span>${amount}</span>,
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

const GOLD = {
  id: 1,
  name: "AI Guest 16",
  email: "g16@x.test",
  loyalty_points: 628,
  loyalty_tier: "Gold",
  total_spent: 1740,
  visits: 8,
};

jest.mock("@/api/crm", () => ({
  getCustomers: () => Promise.resolve([]),
  businessCRMAPI: {
    getCustomers: jest.fn(),
    getCustomerDetails: jest.fn(),
    updateCustomerNotes: jest.fn(),
    updateCustomerTags: jest.fn(() => Promise.resolve({})),
    addCustomer: jest.fn(() => Promise.resolve({})),
    exportCustomers: jest.fn(() => Promise.resolve(new Blob())),
    getCRMStatus: jest.fn(() => Promise.resolve({ enabled: true })),
    toggleCRM: jest.fn(() => Promise.resolve({ enabled: true })),
  },
}));

import { businessCRMAPI } from "@/api/crm";
const getCustomersMock = businessCRMAPI.getCustomers as jest.Mock;
const getCustomerDetailsMock = businessCRMAPI.getCustomerDetails as jest.Mock;

beforeEach(() => {
  jest.clearAllMocks();
  getCustomersMock.mockResolvedValue({
    customers: [GOLD],
    total: 1,
    total_pages: 1,
    summary: {
      total_customers: 1,
      active_this_month: 1,
      avg_lifetime_spend: 100,
      top_tier_count: 1,
    },
  });
  getCustomerDetailsMock.mockResolvedValue({
    customer: { ...GOLD, customer: { name: GOLD.name } },
  });
});

describe("CustomersTab L5-5 add form reset (DOM)", () => {
  it("clears add-customer fields when the modal is closed and reopened", async () => {
    render(<CustomersTab businessId={1} />);
    await screen.findByText(/AI Guest 16/);

    fireEvent.click(screen.getByRole("button", { name: /Add Customer/i }));
    const nameInput = await screen.findByTestId("crm-add-name");
    fireEvent.change(nameInput, { target: { value: "Stale Name" } });
    fireEvent.change(screen.getByTestId("crm-add-email"), {
      target: { value: "stale@x.test" },
    });
    expect(nameInput).toHaveValue("Stale Name");

    // closeAddModal → resetNewCustomerForm (L5-5).
    fireEvent.click(screen.getByText("Cancel"));

    await waitFor(() => {
      expect(screen.queryByTestId("crm-add-name")).not.toBeInTheDocument();
    });

    // Open again — openAddModal also resets before show.
    fireEvent.click(screen.getByRole("button", { name: /Add Customer/i }));
    const reopened = await screen.findByTestId("crm-add-name");
    expect(reopened).toHaveValue("");
    expect(screen.getByTestId("crm-add-email")).toHaveValue("");
  });
});

describe("CustomersTab L5-6 details open before fetch (DOM)", () => {
  it("opens the details modal while getCustomerDetails is still in flight", async () => {
    let resolveDetails: (v: unknown) => void = () => {};
    getCustomerDetailsMock.mockReturnValue(
      new Promise((resolve) => {
        resolveDetails = resolve;
      }),
    );

    render(<CustomersTab businessId={1} />);
    await screen.findByText(/AI Guest 16/);

    fireEvent.click(screen.getByRole("button", { name: "AI Guest 16" }));

    // Modal must appear before the fetch resolves (open-first).
    await waitFor(() => {
      expect(screen.getByTestId("crm-details-loading")).toBeInTheDocument();
    });
    expect(getCustomerDetailsMock).toHaveBeenCalled();
    // List-row snapshot name still visible in the open modal.
    expect(screen.getAllByText(/AI Guest 16/).length).toBeGreaterThan(0);

    resolveDetails({
      customer: { ...GOLD, customer: { name: GOLD.name } },
    });
    await waitFor(() => {
      expect(
        screen.queryByTestId("crm-details-loading"),
      ).not.toBeInTheDocument();
    });
  });
});
