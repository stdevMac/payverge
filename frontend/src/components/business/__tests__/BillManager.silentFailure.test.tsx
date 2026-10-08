/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

// Regression for audit findings A-02 / A-01 / D-03: a failed bills-list load and
// a failed view-bill click must surface a toast instead of clearing the spinner
// silently (leaving the operator on a stale/empty list, or unable to reach the
// void/refund/audit UI, with no indication anything went wrong).
//
// Related to issue 647 (money honesty): the failed-load branch must also refuse
// to paint header stats, because "0 Resultados · 0,00 US$" on a failed fetch is
// indistinguishable from a genuinely empty floor. These tests deliberately keep
// the REAL DashboardTabShell and the REAL operator copy so the assertions read
// the same strings an operator would — an earlier version of this file queried a
// data-testid the shell never renders, under a key-passthrough i18n mock, so
// every "no zero total" assertion passed whether or not the guard existed.

const mockSearchParam = jest.fn((_key: string) => null as string | null);

jest.mock("next/navigation", () => ({
  useSearchParams: () => ({
    get: (key: string) => mockSearchParam(key),
  }),
  // L9-3: opening a bill writes ?billId= back through router.replace, so the
  // stub needs replace as well as push or handleViewBill throws before its
  // error handling runs.
  useRouter: () => ({ push: jest.fn(), replace: jest.fn() }),
  usePathname: () => "/business/1/dashboard",
}));

// Real translations, pinned to Spanish (the locale the failure was reported in).
// Only the locale hook is stubbed — getTranslation stays the shipped one.
jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/SimpleTranslationProvider");
  return {
    ...actual,
    useSimpleLocale: () => ({ locale: "es", setLocale: jest.fn() }),
  };
});

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    hasAccess: true,
    isSuspended: false,
    loading: false,
  }),
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: jest.fn(), success: jest.fn() },
}));

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

jest.mock("../BillCreator", () => ({ BillCreator: () => null }));
jest.mock("../BillDetailsModal", () => ({ BillDetailsModal: () => null }));
jest.mock("../BillFilters", () => ({ BillFilters: () => null }));
jest.mock("../PendingOrdersSection", () => ({
  PendingOrdersSection: () => null,
}));
jest.mock("../KitchenOrdersToggle", () => ({
  KitchenOrdersToggle: () => <div>kitchen upsell</div>,
}));
jest.mock("../BillDisplayMode", () => ({
  __esModule: true,
  default: () => null,
}));

// Expose the view trigger so we can exercise handleViewBill (the real BillsTable
// renders a per-row "View" button wired to onViewBill).
jest.mock("../BillsTable", () => ({
  BillsTable: ({
    bills,
    onViewBill,
  }: {
    bills: Array<{ id: number }>;
    onViewBill: (id: number) => void;
  }) => (
    <button onClick={() => onViewBill(bills[0]?.id ?? 1)}>
      view first bill
    </button>
  ),
}));

import toast from "react-hot-toast";
import { getTranslation } from "@/i18n/SimpleTranslationProvider";
import { BillManager } from "@/components/business/BillManager";
import type { Bill } from "@/api/bills";

// Shipped operator copy, not a stub: "Resultados", "Total listado", etc.
const es = (key: string): string => {
  const value = getTranslation(`billManager.${key}`, "es");
  return Array.isArray(value) ? (value[0] ?? key) : (value as string);
};

// Any rendered money amount: "0,00 US$", "$0.00", "1.473,07 US$".
const MONEY = /\d+[.,]\d{2}|US\s*\$|\$\s*\d/;

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

const renderManager = (
  extraProps: Partial<React.ComponentProps<typeof BillManager>> = {},
) =>
  render(
    <BillManager
      businessId={1}
      globalBills={[]}
      globalOrders={{}}
      kitchenEnabled
      kitchenStatusLoading={false}
      onKitchenStatusChange={jest.fn()}
      onOrderStatusChange={jest.fn()}
      {...extraProps}
    />,
  );

describe("BillManager silent-failure handling", () => {
  beforeEach(() => {
    mockGetBusinessBills.mockReset();
    mockGetBill.mockReset();
    mockSearchParam.mockReset();
    mockSearchParam.mockReturnValue(null);
    (toast.error as jest.Mock).mockClear();
  });

  it("toasts when the bills-list load fails (A-02)", async () => {
    mockGetBusinessBills.mockRejectedValue(new Error("network"));

    const { container } = renderManager();

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(es("messages.loadBillsError")),
    );
    expect(
      await screen.findByText(es("emptyState.listLoadFailed")),
    ).toBeInTheDocument();
    expect(
      screen.getByText(es("emptyState.listLoadFailedHint")),
    ).toBeInTheDocument();
    expect(screen.getByTestId("bills-list-load-failed")).toBeInTheDocument();

    // The header stats must be withheld entirely — no results count, no total.
    const rendered = container.textContent ?? "";
    expect(rendered).not.toContain(es("header.results"));
    expect(rendered).not.toContain(es("header.visibleValue"));
    expect(rendered).not.toMatch(MONEY);
    // Same outage used to keep the kitchen activation card mounted, so the
    // failed load read as "empty till + enable kitchen".
    expect(screen.queryByText("kitchen upsell")).not.toBeInTheDocument();
  });

  it("does not paint fake $0 totals when the parent bills feed is already empty (#647)", async () => {
    mockGetBusinessBills.mockRejectedValue(new Error("network"));

    const { container } = renderManager({
      globalBillsLoaded: true,
      globalOrdersLoaded: true,
    });

    expect(
      await screen.findByText(es("emptyState.listLoadFailed")),
    ).toBeInTheDocument();

    // With both feeds empty AND the fetch failed, every input to the header
    // stats is zero — this is exactly the state that used to render
    // "0 Resultados · 0,00 US$" and read as a genuinely empty floor.
    const rendered = container.textContent ?? "";
    expect(rendered).not.toContain(es("header.results"));
    expect(rendered).not.toContain(es("header.visibleValue"));
    expect(rendered).not.toContain(es("header.pendingOrders"));
    expect(rendered).not.toMatch(MONEY);
    expect(screen.queryByText("kitchen upsell")).not.toBeInTheDocument();
  });

  it("does not paint fake $0 totals when a 200 payload omits the bills array (#647)", async () => {
    // The live miss collapsed a failed / empty-shaped list response into the
    // same success header as a genuine empty till: "0 Resultados · 0,00 US$".
    // `data.bills || []` is what made that collapse possible.
    mockGetBusinessBills.mockResolvedValue({ total: 0 });

    const { container } = renderManager({
      globalBillsLoaded: true,
      globalOrdersLoaded: true,
      kitchenEnabled: false,
    });

    expect(
      await screen.findByTestId("bills-list-load-failed"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(es("emptyState.listLoadFailed")),
    ).toBeInTheDocument();

    const rendered = container.textContent ?? "";
    expect(rendered).not.toContain(es("header.results"));
    expect(rendered).not.toContain(es("header.visibleValue"));
    expect(rendered).not.toMatch(/0\s+Resultados/);
    expect(rendered).not.toMatch(MONEY);
    expect(screen.queryByText("kitchen upsell")).not.toBeInTheDocument();
  });

  it("does not paint fake $0 when the live bills feed failed and the list is empty (#647)", async () => {
    // Live miss: axios toasts "Couldn't reach the server" from getAllActiveBills
    // while the paged list can still 200 `{ bills: [] }`. That used to render
    // the same success header as a genuine empty till.
    mockGetBusinessBills.mockResolvedValue({
      bills: [],
      total: 0,
      total_pages: 1,
    });
    const onRefreshLiveBills = jest.fn();

    const { container } = renderManager({
      globalBillsLoaded: true,
      globalOrdersLoaded: true,
      globalBillsFailed: true,
      onRefreshLiveBills,
      kitchenEnabled: false,
    });

    expect(
      await screen.findByTestId("bills-list-load-failed"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(es("emptyState.listLoadFailed")),
    ).toBeInTheDocument();

    const rendered = container.textContent ?? "";
    expect(rendered).not.toContain(es("header.results"));
    expect(rendered).not.toContain(es("header.visibleValue"));
    expect(rendered).not.toMatch(/0\s+Resultados/);
    expect(rendered).not.toMatch(MONEY);
    expect(screen.queryByText("kitchen upsell")).not.toBeInTheDocument();

    fireEvent.click(screen.getByText(es("emptyState.listLoadRetry")));
    expect(onRefreshLiveBills).toHaveBeenCalled();
  });

  it("does not swap a later search-empty into retry chrome while the live feed is still failed (#647)", async () => {
    mockSearchParam.mockImplementation((key: string) =>
      key === "billSearch" ? "zzzz-no-match" : null,
    );
    mockGetBusinessBills.mockResolvedValue({
      bills: [],
      total: 0,
      total_pages: 1,
    });

    const { container } = renderManager({
      globalBillsLoaded: true,
      globalOrdersLoaded: true,
      globalBillsFailed: true,
    });

    await waitFor(() => {
      expect(container.textContent ?? "").toContain(es("header.results"));
    });
    expect(container.textContent ?? "").toMatch(/0\s*Resultados/);
    expect(
      screen.queryByTestId("bills-list-load-failed"),
    ).not.toBeInTheDocument();
    expect(mockGetBusinessBills).toHaveBeenCalledWith(
      1,
      expect.objectContaining({ search: "zzzz-no-match" }),
    );
  });

  it("still paints 0 results for a genuine empty list", async () => {
    mockGetBusinessBills.mockResolvedValue({
      bills: [],
      total: 0,
      total_pages: 1,
    });

    const { container } = renderManager({
      globalBillsLoaded: true,
      globalOrdersLoaded: true,
      globalBillsFailed: false,
    });

    await waitFor(() => {
      expect(container.textContent ?? "").toContain(es("header.results"));
    });
    expect(container.textContent ?? "").toMatch(/0\s*Resultados/);
    expect(
      screen.queryByTestId("bills-list-load-failed"),
    ).not.toBeInTheDocument();
  });

  it("keeps last-known totals when a later list fetch fails (#647)", async () => {
    const openBill = buildBill(3);
    mockGetBusinessBills
      .mockResolvedValueOnce({
        bills: [openBill],
        total: 1,
        total_pages: 1,
      })
      .mockRejectedValue(new Error("network"));

    const { rerender, container } = render(
      <BillManager
        businessId={1}
        globalBills={[openBill]}
        globalBillsLoaded
        globalOrdersLoaded
        globalOrders={{}}
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(container.textContent ?? "").toMatch(MONEY);
    });

    rerender(
      <BillManager
        businessId={1}
        globalBills={[{ ...openBill, notes: "sse-patch" }]}
        globalBillsLoaded
        globalOrdersLoaded
        globalOrders={{}}
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />,
    );

    expect(
      await screen.findByTestId("bills-list-stale-banner", {}, { timeout: 2000 }),
    ).toBeInTheDocument();
    expect(container.textContent ?? "").toMatch(MONEY);
    expect(container.textContent ?? "").not.toMatch(/0\s+Resultados/);
    expect(
      screen.queryByTestId("bills-list-load-failed"),
    ).not.toBeInTheDocument();
  });

  it("toasts when opening a bill fails instead of silently doing nothing (A-01)", async () => {
    mockGetBusinessBills.mockResolvedValue({
      bills: [buildBill(7)],
      total: 1,
      total_pages: 1,
    });
    mockGetBill.mockRejectedValue(new Error("500"));

    renderManager();

    const viewButton = await screen.findByText("view first bill");
    fireEvent.click(viewButton);

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(es("messages.viewBillError")),
    );
  });
});
