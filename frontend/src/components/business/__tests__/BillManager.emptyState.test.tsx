/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

const mockReplace = jest.fn();
jest.mock("next/navigation", () => ({
  useSearchParams: () => ({ get: () => null }),
  useRouter: () => ({ replace: mockReplace }),
  usePathname: () => "/business/1/dashboard",
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    hasAccess: true,
    isSuspended: false,
    loading: false,
  }),
}));

const mockGetBusinessBills = jest.fn();

jest.mock("@/api/bills", () => ({
  getBusinessBills: (...args: unknown[]) => mockGetBusinessBills(...args),
  getAllBillsForStatus: jest.fn(),
  getBill: jest.fn(),
  closeBill: jest.fn(),
  isActiveBillStatus: (status: string | undefined) =>
    status === "open" || status === "partial",
}));

jest.mock("@/api/business", () => ({
  getBusiness: jest.fn().mockResolvedValue({
    default_currency: "USD",
    timezone: "UTC",
  }),
}));

jest.mock("../BillCreator", () => ({
  BillCreator: () => null,
}));

jest.mock("../BillDetailsModal", () => ({
  BillDetailsModal: () => null,
}));

jest.mock("../BillFilters", () => ({
  BillFilters: () => null,
}));

jest.mock("../BillsTable", () => ({
  BillsTable: () => null,
}));

jest.mock("../PendingOrdersSection", () => ({
  PendingOrdersSection: () => null,
}));

jest.mock("../KitchenOrdersToggle", () => ({
  KitchenOrdersToggle: () => null,
}));

jest.mock("../BillDisplayMode", () => ({
  __esModule: true,
  default: () => null,
}));

import { BillManager } from "@/components/business/BillManager";

describe("BillManager empty state", () => {
  beforeEach(() => {
    mockGetBusinessBills.mockReset();
    mockReplace.mockClear();
    mockGetBusinessBills.mockResolvedValue({
      bills: [],
      total: 0,
      total_pages: 1,
    });
  });

  it("does not replace bill navigation with the legacy top-level empty state", async () => {
    render(
      <BillManager
        businessId={1}
        globalOrders={{}}
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />,
    );

    await screen.findByRole("tab", { name: "billManager.tabs.active" });
    expect(
      screen.queryByText("billManager.empty.subtitle"),
    ).not.toBeInTheDocument();
  });

  it("keeps Active and History reachable when the active-bill query is empty", async () => {
    render(
      <BillManager
        businessId={1}
        globalOrders={{}}
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />,
    );

    const activeTab = await screen.findByRole("tab", {
      name: "billManager.tabs.active",
    });
    const historyTab = screen.getByRole("tab", {
      name: "billManager.tabs.history",
    });
    expect(activeTab).toHaveAttribute("aria-selected", "true");

    fireEvent.click(historyTab);

    await waitFor(() =>
      expect(mockGetBusinessBills).toHaveBeenLastCalledWith(
        1,
        // "All" history now includes voided bills so real money reversals
        // stay reachable (previously omitted).
        expect.objectContaining({ status: "paid,closed,voided" }),
      ),
    );
    expect(historyTab).toHaveAttribute("aria-selected", "true");
  });
});
