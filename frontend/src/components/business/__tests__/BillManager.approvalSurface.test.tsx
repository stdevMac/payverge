/** @jest-environment jsdom */
/**
 * Related to issue 793: the Cuentas rail badge now counts pending-approval
 * tickets, so an operator who taps it lands here to approve. Landing on the
 * tab with a non-empty approval queue must surface the approval list —
 * scroll it into view — instead of leaving it above the fold while the
 * operator stares at the checks table.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

jest.mock("next/navigation", () => ({
  useSearchParams: () => ({ get: () => null }),
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

jest.mock("@/contexts/StaffPermissionsContext", () => ({
  useStaffPermissionsContext: () => ({
    permissions: [],
    rolePermissions: [],
    customGrants: [],
    isLoading: false,
    isError: false,
    refetch: jest.fn(),
  }),
}));

jest.mock("@/api/currency", () => ({
  formatCurrency: (n: number) => `$${n.toFixed(2)}`,
}));

const mockGetBusinessBills = jest.fn();
jest.mock("@/api/bills", () => ({
  getBusinessBills: (...args: unknown[]) => mockGetBusinessBills(...args),
  getAllBillsForStatus: jest
    .fn()
    .mockResolvedValue({ bills: [], capped: false }),
  getBill: jest.fn(),
  closeBill: jest.fn(),
  isActiveBillStatus: (status: string | undefined) =>
    status === "open" || status === "partial",
}));

jest.mock("@/api/business", () => ({
  getBusiness: jest.fn().mockResolvedValue({
    id: 1,
    default_currency: "USD",
    timezone: "UTC",
  }),
}));

jest.mock("../BillCreator", () => ({ BillCreator: () => null }));
jest.mock("../BillDetailsModal", () => ({ BillDetailsModal: () => null }));
jest.mock("../BillFilters", () => ({
  BillFilters: () => <div data-testid="bill-filters" />,
}));
jest.mock("../BillsTable", () => ({
  BillsTable: () => <div data-testid="painted-bills" />,
}));
jest.mock("../PendingOrdersSection", () => ({
  PendingOrdersSection: () => <div data-testid="pending-orders-section" />,
}));
jest.mock("../KitchenOrdersToggle", () => ({
  KitchenOrdersToggle: () => null,
}));
jest.mock("../BillDisplayMode", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("../shared/DashboardTabShell", () => ({
  __esModule: true,
  default: ({
    loading,
    children,
  }: {
    loading?: React.ReactNode;
    children?: React.ReactNode;
  }) => (
    <div>
      {loading ? <div data-testid="bills-skeleton">{loading}</div> : null}
      {children}
    </div>
  ),
}));
jest.mock("../DashboardLockedTabView", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("../modals/ConfirmationModal", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("../BillsSkeleton", () => ({
  BillsSkeleton: () => <div>skeleton</div>,
}));

import { BillManager } from "@/components/business/BillManager";
import type { Bill } from "@/api/bills";
import type { Order } from "@/api/orders";

const bill = {
  id: 756,
  business_id: 1,
  table_id: 4,
  bill_number: "BILL-756",
  notes: "",
  items: "[]",
  subtotal: 21.53,
  tax_amount: 0,
  service_fee_amount: 0,
  total_amount: 21.53,
  paid_amount: 0,
  tip_amount: 0,
  status: "open",
  settlement_address: "",
  tipping_address: "",
  created_at: new Date(2026, 7, 20, 12, 0, 0).toISOString(),
  updated_at: new Date(2026, 7, 20, 12, 0, 0).toISOString(),
} as Bill;

const mkOrder = (id: number, status: string) =>
  ({
    id,
    bill_id: 756,
    business_id: 1,
    order_number: `ORD-${id}`,
    status,
    items: "[]",
    currency: "USD",
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  }) as Order;

function renderManager(orders: Record<number, Order[]>) {
  return render(
    <BillManager
      businessId={1}
      globalBills={[bill]}
      globalBillsLoaded
      globalOrders={orders}
      globalOrdersLoaded
      kitchenEnabled
      kitchenStatusLoading={false}
      onKitchenStatusChange={jest.fn()}
      onOrderStatusChange={jest.fn()}
    />,
  );
}

describe("BillManager surfaces the approval queue on landing (issue 793)", () => {
  const scrollSpy = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
    mockGetBusinessBills.mockResolvedValue({
      bills: [bill],
      total: 1,
      total_pages: 1,
    });
    window.HTMLElement.prototype.scrollIntoView = scrollSpy;
  });

  afterEach(() => {
    delete (
      window.HTMLElement.prototype as unknown as {
        scrollIntoView?: () => void;
      }
    ).scrollIntoView;
  });

  it("scrolls the pending-orders anchor into view when tickets await approval", async () => {
    renderManager({ 756: [mkOrder(9, "pending"), mkOrder(10, "in_kitchen")] });
    await waitFor(() => expect(scrollSpy).toHaveBeenCalledTimes(1));
  });

  it("only surfaces once — later polls must not yank the scroll again", async () => {
    const { rerender } = renderManager({ 756: [mkOrder(9, "pending")] });
    await waitFor(() => expect(scrollSpy).toHaveBeenCalledTimes(1));
    rerender(
      <BillManager
        businessId={1}
        globalBills={[bill]}
        globalBillsLoaded
        globalOrders={{ 756: [mkOrder(9, "pending"), mkOrder(11, "pending")] }}
        globalOrdersLoaded
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />,
    );
    await waitFor(() =>
      expect(screen.getByTestId("pending-orders-section")).toBeInTheDocument(),
    );
    expect(scrollSpy).toHaveBeenCalledTimes(1);
  });

  it("does not scroll when the approval queue is empty", async () => {
    renderManager({ 756: [mkOrder(10, "in_kitchen")] });
    await waitFor(() =>
      expect(screen.getByTestId("pending-orders-section")).toBeInTheDocument(),
    );
    expect(scrollSpy).not.toHaveBeenCalled();
  });

  it("never scroll-yanks when a pending ticket arrives after a clean landing", async () => {
    // O1: an operator who LANDS with an empty queue keeps their scroll
    // position forever — a later SSE/poll 0→1 transition must not fire the
    // arrival scroll minutes into their work.
    const { rerender } = renderManager({ 756: [mkOrder(10, "in_kitchen")] });
    await waitFor(() =>
      expect(screen.getByTestId("pending-orders-section")).toBeInTheDocument(),
    );
    expect(scrollSpy).not.toHaveBeenCalled();
    rerender(
      <BillManager
        businessId={1}
        globalBills={[bill]}
        globalBillsLoaded
        globalOrders={{
          756: [mkOrder(10, "in_kitchen"), mkOrder(12, "pending")],
        }}
        globalOrdersLoaded
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />,
    );
    await waitFor(() =>
      expect(screen.getByTestId("pending-orders-section")).toBeInTheDocument(),
    );
    expect(scrollSpy).not.toHaveBeenCalled();
  });
});
