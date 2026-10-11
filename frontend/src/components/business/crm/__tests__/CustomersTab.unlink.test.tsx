/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import CustomersTab from "../CustomersTab";
import { businessCRMAPI } from "@/api/crm";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/api/crm", () => ({
  businessCRMAPI: {
    getCustomers: jest.fn(),
    getCustomerDetails: jest.fn(),
    unlinkCustomer: jest.fn(),
    addCustomer: jest.fn(),
    updateCustomerNotes: jest.fn(),
    updateCustomerTags: jest.fn(),
    exportCustomers: jest.fn(),
  },
}));

jest.mock("@/api/business", () => ({
  getBusiness: jest.fn().mockResolvedValue({ default_currency: "USD" }),
}));

jest.mock("@/api/loyalty", () => ({
  getLoyalty: jest.fn().mockResolvedValue({
    program: {
      id: 1,
      business_id: 42,
      enabled: true,
      points_per_dollar: 1,
      redemption_points_per_dollar: 100,
    },
    tiers: [],
  }),
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

const mockGetCustomers = businessCRMAPI.getCustomers as jest.Mock;
const mockUnlink = businessCRMAPI.unlinkCustomer as jest.Mock;

describe("CustomersTab unlink (L5-9)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGetCustomers.mockResolvedValue({
      customers: [
        {
          id: 7,
          customer: { name: "Ada Guest", email: "ada@example.com" },
          loyalty_points: 10,
          total_spent: 50,
          visit_count: 2,
          last_visit_at: "2026-08-01T00:00:00Z",
        },
      ],
      total: 1,
      page: 1,
      page_size: 20,
      summary: {
        total_customers: 1,
        active_this_month: 1,
        avg_lifetime_spend: 50,
        top_tier_customers: 0,
      },
    });
    mockUnlink.mockResolvedValue({ unlinked: true });
  });

  it("does not call unlink until ConfirmationModal is confirmed", async () => {
    render(<CustomersTab businessId={42} />);
    expect(await screen.findByText("Ada Guest")).toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", {
        name: /businessDashboard\.crm\.remove\.action Ada Guest/i,
      }),
    );

    expect(
      await screen.findByText("businessDashboard.crm.remove.confirmTitle"),
    ).toBeInTheDocument();
    expect(mockUnlink).not.toHaveBeenCalled();

    fireEvent.click(
      screen.getByText("businessDashboard.crm.remove.confirmAction"),
    );
    await waitFor(() => expect(mockUnlink).toHaveBeenCalledWith(42, 7));
  });
});
