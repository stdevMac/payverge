/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

/**
 * Audit L9-1 (severity 2, tenant-isolation UX): opening a bill that belongs to
 * a different business must NOT render BillDetailsModal inside the current
 * business shell. Backend RequireBillBusinessAccess is correct; this is a
 * frontend-only deep-link / multi-business shell leak.
 */

const mockRouterReplace = jest.fn();
const mockSearchParamsGet = jest.fn((_key: string): string | null => null);

jest.mock("next/navigation", () => ({
  useSearchParams: () => ({
    get: (key: string) => mockSearchParamsGet(key),
  }),
  useRouter: () => ({ push: jest.fn(), replace: mockRouterReplace }),
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

jest.mock("react-hot-toast", () => {
  const error = jest.fn();
  const success = jest.fn();
  const toastFn = Object.assign(jest.fn(), { error, success });
  return { __esModule: true, default: toastFn, toast: toastFn };
});

const mockGetBusinessBills = jest.fn();
const mockGetBill = jest.fn();

jest.mock("@/api/bills", () => ({
  getBusinessBills: (...args: unknown[]) => mockGetBusinessBills(...args),
  getAllBillsForStatus: jest.fn(),
  getBill: (...args: unknown[]) => mockGetBill(...args),
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

jest.mock("../BillCreator", () => ({ BillCreator: () => null }));
jest.mock("../BillFilters", () => ({ BillFilters: () => null }));
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

// Capture modal open state via props so we can assert it never opens for a
// foreign bill without relying on real BillDetailsModal (heavy, dynamic).
const mockBillDetailsModal = jest.fn(
  ({ isOpen }: { isOpen: boolean }) =>
    isOpen ? <div data-testid="bill-details-modal">open</div> : null,
);
jest.mock("../BillDetailsModal", () => ({
  BillDetailsModal: (props: { isOpen: boolean }) => mockBillDetailsModal(props),
}));

jest.mock("../BillsTable", () => ({
  BillsTable: ({
    bills,
    onViewBill,
  }: {
    bills: Array<{ id: number }>;
    onViewBill: (id: number) => void;
  }) => (
    <button type="button" onClick={() => onViewBill(bills[0]?.id ?? 7)}>
      view first bill
    </button>
  ),
}));

import toast from "react-hot-toast";
import { BillManager } from "@/components/business/BillManager";
import type { Bill, BillWithItemsResponse } from "@/api/bills";

const CURRENT_BUSINESS_ID = 1;
const FOREIGN_BUSINESS_ID = 99;

const buildBill = (id: number, businessId: number): Bill =>
  ({
    id,
    business_id: businessId,
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
    status: "open",
    settlement_address: "",
    tipping_address: "",
    created_at: new Date(2026, 4, 1, 12, 0, 0).toISOString(),
    updated_at: new Date(2026, 4, 1, 12, 0, 0).toISOString(),
  }) as Bill;

const buildBillDetails = (
  id: number,
  businessId: number,
): BillWithItemsResponse => ({
  bill: buildBill(id, businessId),
  items: [],
});

const renderManager = () =>
  render(
    <BillManager
      businessId={CURRENT_BUSINESS_ID}
      globalBills={[]}
      globalOrders={{}}
      kitchenEnabled
      kitchenStatusLoading={false}
      onKitchenStatusChange={jest.fn()}
      onOrderStatusChange={jest.fn()}
    />,
  );

describe("BillManager tenant isolation (L9-1)", () => {
  beforeEach(() => {
    mockGetBusinessBills.mockReset();
    mockGetBill.mockReset();
    mockRouterReplace.mockReset();
    mockSearchParamsGet.mockReset();
    mockSearchParamsGet.mockReturnValue(null);
    mockBillDetailsModal.mockClear();
    (toast.error as jest.Mock).mockClear();

    // jsdom location for router.replace URL construction in the guard.
    window.history.replaceState(
      {},
      "",
      "/business/1/dashboard?tab=bills&billId=7&billPage=2",
    );

    mockGetBusinessBills.mockResolvedValue({
      // List can show a row under this business shell; getBill is the source
      // of truth for nested bill.business_id (BillWithItemsResponse).
      bills: [buildBill(7, CURRENT_BUSINESS_ID)],
      total: 1,
      total_pages: 1,
    });
  });

  it("does not open the details modal for a foreign-business bill, toasts, and strips billId", async () => {
    mockGetBill.mockResolvedValue(
      buildBillDetails(7, FOREIGN_BUSINESS_ID),
    );

    renderManager();

    const viewButton = await screen.findByText("view first bill");
    fireEvent.click(viewButton);

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        "billManager.errors.billNotInThisBusiness",
      ),
    );

    expect(screen.queryByTestId("bill-details-modal")).not.toBeInTheDocument();

    // Never opened with isOpen=true.
    const openCalls = mockBillDetailsModal.mock.calls.filter(
      ([props]) => props?.isOpen === true,
    );
    expect(openCalls).toHaveLength(0);

    await waitFor(() => {
      expect(mockRouterReplace).toHaveBeenCalled();
      const urls = mockRouterReplace.mock.calls.map(
        (c) => String(c[0]) as string,
      );
      expect(urls.some((u) => !u.includes("billId="))).toBe(true);
      // Other params preserved.
      const stripped = urls.find((u) => !u.includes("billId="));
      expect(stripped).toMatch(/tab=bills/);
    });
  });

  it("opens the details modal when the bill belongs to the current business", async () => {
    mockGetBill.mockResolvedValue(
      buildBillDetails(7, CURRENT_BUSINESS_ID),
    );

    renderManager();

    const viewButton = await screen.findByText("view first bill");
    fireEvent.click(viewButton);

    await waitFor(() =>
      expect(screen.getByTestId("bill-details-modal")).toBeInTheDocument(),
    );
    expect(toast.error).not.toHaveBeenCalledWith(
      "billManager.errors.billNotInThisBusiness",
    );
  });
});
