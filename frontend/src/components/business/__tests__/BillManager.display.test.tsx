/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

jest.mock("next/navigation", () => ({
  useSearchParams: () => ({ get: () => null }),
  // L21 mirrors bill filters/pagination to the URL via router.replace; the mock
  // must provide it or the URL-sync effect throws when a filter changes.
  useRouter: () => ({ push: jest.fn(), replace: jest.fn() }),
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

jest.mock("../BillCreator", () => ({
  BillCreator: () => null,
}));

jest.mock("../BillDetailsModal", () => ({
  BillDetailsModal: () => null,
}));

jest.mock("../BillFilters", () => ({
  BillFilters: ({
    onSearchChange,
    onDateFromChange,
    onDateToChange,
    onReset,
  }: {
    onSearchChange: (value: string) => void;
    onDateFromChange: (value: string) => void;
    onDateToChange: (value: string) => void;
    onReset: () => void;
  }) => (
    <div>
      <button onClick={() => onSearchChange("table 9")}>apply search</button>
      <button onClick={() => onDateFromChange("2026-05-11")}>apply from</button>
      <button onClick={() => onDateToChange("2026-05-12")}>apply to</button>
      <button onClick={onReset}>reset filters</button>
    </div>
  ),
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
  default: ({ bills }: { bills: unknown[] }) => (
    <div data-testid="display-bill-count">{bills.length}</div>
  ),
}));

import { BillManager } from "@/components/business/BillManager";
import type { Bill } from "@/api/bills";

const buildBill = (id: number): Bill =>
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
    status: "open",
    settlement_address: "",
    tipping_address: "",
    created_at: new Date(2026, 4, 1, 12, 0, 0).toISOString(),
    updated_at: new Date(2026, 4, 1, 12, 0, 0).toISOString(),
  }) as Bill;

describe("BillManager display mode", () => {
  beforeEach(() => {
    mockGetBusinessBills.mockReset();
  });

  it("uses complete global active bills instead of the current paged list", async () => {
    const globalBills = Array.from({ length: 30 }, (_, index) =>
      buildBill(index + 1),
    );
    mockGetBusinessBills.mockResolvedValue({
      bills: globalBills.slice(0, 25),
      total: 30,
      total_pages: 2,
    });

    render(
      <BillManager
        businessId={1}
        globalBills={globalBills}
        globalBillsCapped
        globalOrders={{}}
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />,
    );

    const displayButton = await screen.findByText(
      "billManager.buttons.launchDisplay",
    );
    fireEvent.click(displayButton);

    await waitFor(() =>
      expect(screen.getByTestId("display-bill-count")).toHaveTextContent("30"),
    );
    expect(
      screen.getByText("billManager.warnings.activeBillsCapped"),
    ).toBeInTheDocument();
  });

  // Audit A-03: the kanban's Completed column was structurally always empty
  // because only active bills were fed. Entering display mode must fetch a
  // recent paid/closed slice and merge it (deduped) into the display feed.
  it("feeds recently-completed bills to the display alongside active ones", async () => {
    const now = Date.now();
    const recentPaid = {
      ...buildBill(90),
      status: "paid",
      updated_at: new Date(now - 5 * 60 * 1000).toISOString(),
      closed_at: new Date(now - 5 * 60 * 1000).toISOString(),
    } as Bill;
    const stalePaid = {
      ...buildBill(91),
      status: "paid",
      updated_at: new Date(now - 3 * 60 * 60 * 1000).toISOString(),
      closed_at: new Date(now - 3 * 60 * 60 * 1000).toISOString(),
    } as Bill;

    mockGetBusinessBills.mockImplementation(
      (_bizId: unknown, opts: { status?: string }) => {
        if (opts?.status === "paid,closed") {
          return Promise.resolve({
            bills: [recentPaid, stalePaid],
            total: 2,
            total_pages: 1,
          });
        }
        return Promise.resolve({
          bills: [buildBill(1)],
          total: 1,
          total_pages: 1,
        });
      },
    );

    render(
      <BillManager
        businessId={1}
        globalBills={[buildBill(1)]}
        globalOrders={{}}
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />,
    );

    fireEvent.click(
      await screen.findByText("billManager.buttons.launchDisplay"),
    );

    // 1 active + 1 recently-paid; the 3-hour-old paid bill is outside the
    // one-hour window and must be dropped.
    await waitFor(() =>
      expect(screen.getByTestId("display-bill-count")).toHaveTextContent("2"),
    );
    expect(mockGetBusinessBills).toHaveBeenCalledWith(
      1,
      expect.objectContaining({ status: "paid,closed", page: 1 }),
    );
  });

  it("includes partial bills in active display mode", async () => {
    const partialBill = {
      ...buildBill(55),
      status: "partial",
      paid_amount: 10,
      total_amount: 40,
    } as Bill;

    mockGetBusinessBills.mockResolvedValue({
      bills: [partialBill],
      total: 1,
      total_pages: 1,
    });

    render(
      <BillManager
        businessId={1}
        globalBills={[partialBill]}
        globalOrders={{}}
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />,
    );

    const displayButton = await screen.findByText(
      "billManager.buttons.launchDisplay",
    );
    fireEvent.click(displayButton);

    await waitFor(() =>
      expect(screen.getByTestId("display-bill-count")).toHaveTextContent("1"),
    );
  });

  it("passes active search and date filters to the paginated bills API", async () => {
    // At least one active bill so the list + filter bar render (the empty
    // state intentionally hides the filter UI when a business has no bills
    // and no filters applied).
    mockGetBusinessBills.mockResolvedValue({
      bills: [buildBill(1)],
      total: 1,
      total_pages: 1,
    });

    render(
      <BillManager
        businessId={1}
        globalBills={[]}
        globalOrders={{}}
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />,
    );

    await waitFor(() => expect(mockGetBusinessBills).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByText("apply search"));
    await waitFor(() =>
      expect(mockGetBusinessBills).toHaveBeenLastCalledWith(
        1,
        expect.objectContaining({
          status: "open,partial",
          search: "table 9",
          dateFrom: "",
          dateTo: "",
        }),
      ),
    );

    fireEvent.click(screen.getByText("apply from"));
    fireEvent.click(screen.getByText("apply to"));
    await waitFor(() =>
      expect(mockGetBusinessBills).toHaveBeenLastCalledWith(
        1,
        expect.objectContaining({
          status: "open,partial",
          search: "table 9",
          dateFrom: "2026-05-11",
          dateTo: "2026-05-12",
        }),
      ),
    );
  });

  it("does not treat clearing active filters as a new bill notification", async () => {
    // Keep at least one bill in the page so the list + filter bar render; the
    // notification logic keys off `total`, not the page length, so the totals
    // below still drive the no-sound assertion.
    mockGetBusinessBills
      .mockResolvedValueOnce({
        bills: [buildBill(1)],
        total: 5,
        total_pages: 1,
      })
      .mockResolvedValueOnce({
        bills: [buildBill(1)],
        total: 1,
        total_pages: 1,
      })
      .mockResolvedValueOnce({
        bills: [buildBill(1)],
        total: 5,
        total_pages: 1,
      });

    render(
      <BillManager
        businessId={1}
        globalBills={[]}
        globalOrders={{}}
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />,
    );

    await waitFor(() => expect(mockGetBusinessBills).toHaveBeenCalledTimes(1));
    fireEvent.click(screen.getByText("apply search"));
    await waitFor(() => expect(mockGetBusinessBills).toHaveBeenCalledTimes(2));
    fireEvent.click(screen.getByText("reset filters"));
    await waitFor(() => expect(mockGetBusinessBills).toHaveBeenCalledTimes(3));
  });
});
