/** @jest-environment jsdom */
import React from "react";
import { render, waitFor } from "@testing-library/react";

// Audit E-01: the standard Bills LIST must reflect live state. When the SSE-fed
// globalBills prop changes (a guest payment / bill event landed), BillManager
// must re-fetch its active list instead of showing a paid bill as still open
// until the operator changes a filter/tab.

jest.mock("next/navigation", () => ({
  useSearchParams: () => ({ get: () => null }),
  useRouter: () => ({ push: jest.fn() }),
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

jest.mock("../BillCreator", () => ({ BillCreator: () => null }));
jest.mock("../BillDetailsModal", () => ({ BillDetailsModal: () => null }));
jest.mock("../BillFilters", () => ({ BillFilters: () => null }));
jest.mock("../BillsTable", () => ({ BillsTable: () => null }));
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
import type { Bill } from "@/api/bills";

const buildBill = (id: number, status: Bill["status"] = "open"): Bill =>
  ({
    id,
    business_id: 1,
    table_id: id,
    bill_number: `BILL-${id}`,
    notes: "",
    items: "[]",
    subtotal: 10,
    tax_amount: 0,
    service_fee_amount: 0,
    total_amount: 10,
    paid_amount: 0,
    tip_amount: 0,
    status,
    settlement_address: "",
    tipping_address: "",
    created_at: new Date(2026, 4, 1, 12, 0, 0).toISOString(),
    updated_at: new Date(2026, 4, 1, 12, 0, 0).toISOString(),
  }) as Bill;

const props = {
  businessId: 1,
  globalOrders: {},
  kitchenEnabled: true,
  kitchenStatusLoading: false,
  onKitchenStatusChange: jest.fn(),
  onOrderStatusChange: jest.fn(),
};

describe("BillManager live refresh on SSE bill events (E-01)", () => {
  beforeEach(() => {
    mockGetBusinessBills.mockReset();
    mockGetBusinessBills.mockResolvedValue({
      bills: [buildBill(1)],
      total: 1,
      total_pages: 1,
    });
  });

  it("re-fetches the active list when globalBills changes, but not on the initial value", async () => {
    const { rerender } = render(
      <BillManager {...props} globalBills={[buildBill(1)]} />,
    );

    // Initial mount load only — the live-refresh effect must skip the first
    // (initial) globalBills value, so exactly one fetch so far.
    await waitFor(() => expect(mockGetBusinessBills).toHaveBeenCalledTimes(1));

    // A guest payment lands: the dashboard patches globalBills (new reference).
    rerender(
      <BillManager
        {...props}
        globalBills={[buildBill(1, "paid"), buildBill(2)]}
      />,
    );

    // The active list re-fetches (after the debounce) to pick up the new state.
    await waitFor(() => expect(mockGetBusinessBills).toHaveBeenCalledTimes(2), {
      timeout: 2000,
    });
  });
});
