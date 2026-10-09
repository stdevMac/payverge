/** @jest-environment jsdom */
/**
 * #96 — header counts/totals must not flash intermediate values before the
 * shared orders feed has settled.
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
  getAllBillsForStatus: jest.fn().mockResolvedValue({ bills: [], capped: false }),
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
jest.mock("../shared/DashboardTabShell", () => ({
  __esModule: true,
  default: ({
    header,
    loading,
    children,
  }: {
    header?: { stats?: Array<{ label: string; value: React.ReactNode }> };
    loading?: React.ReactNode;
    children?: React.ReactNode;
  }) => (
    <div>
      {loading ? <div data-testid="bills-skeleton">{loading}</div> : null}
      {header?.stats?.map((s) => (
        <div key={String(s.label)} data-testid={`stat-${s.label}`}>
          {s.value} {s.label}
        </div>
      ))}
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

const buildBill = (id: number, total: number): Bill =>
  ({
    id,
    business_id: 1,
    table_id: id,
    bill_number: `BILL-${id}`,
    notes: "",
    items: "[]",
    subtotal: total,
    tax_amount: 0,
    service_fee_amount: 0,
    total_amount: total,
    paid_amount: 0,
    tip_amount: 0,
    status: "open",
    settlement_address: "",
    tipping_address: "",
    created_at: new Date(2026, 4, 1, 12, 0, 0).toISOString(),
    updated_at: new Date(2026, 4, 1, 12, 0, 0).toISOString(),
  }) as Bill;

const pendingOrder = {
  id: 9,
  bill_id: 1,
  business_id: 1,
  order_number: "ORD-9",
  status: "pending",
  items: "[]",
  currency: "USD",
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
} as Order;

describe("BillManager stats stabilize (#96)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGetBusinessBills.mockResolvedValue({
      bills: [buildBill(1, 100), buildBill(2, 50)],
      total: 2,
      total_pages: 1,
    });
  });

  it("keeps the skeleton up while globalOrdersLoaded is false", async () => {
    render(
      <BillManager
        businessId={1}
        globalBills={[buildBill(1, 100), buildBill(2, 50)]}
        globalBillsLoaded
        globalOrders={{ 1: [pendingOrder] }}
        globalOrdersLoaded={false}
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(mockGetBusinessBills).toHaveBeenCalled();
    });
    expect(screen.getByTestId("bills-skeleton")).toBeInTheDocument();
    expect(
      screen.queryByTestId("stat-billManager.header.results"),
    ).not.toBeInTheDocument();
  });

  it("keeps the skeleton up while globalBillsLoaded is false", async () => {
    render(
      <BillManager
        businessId={1}
        globalBills={[buildBill(1, 100)]}
        globalBillsLoaded={false}
        globalOrders={{ 1: [pendingOrder] }}
        globalOrdersLoaded
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(mockGetBusinessBills).toHaveBeenCalled();
    });
    expect(screen.getByTestId("bills-skeleton")).toBeInTheDocument();
    expect(
      screen.queryByTestId("stat-billManager.header.results"),
    ).not.toBeInTheDocument();
  });

  it("renders stable stats once orders are loaded, using live active totals", async () => {
    render(
      <BillManager
        businessId={1}
        globalBills={[buildBill(1, 100), buildBill(2, 50)]}
        globalBillsLoaded
        globalOrders={{ 1: [pendingOrder] }}
        globalOrdersLoaded
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(
        screen.getByTestId("stat-billManager.header.results"),
      ).toHaveTextContent("2");
    });
    expect(
      screen.getByTestId("stat-billManager.header.visibleValue"),
    ).toHaveTextContent("$150.00");
    expect(
      screen.getByTestId("stat-billManager.header.pendingOrder"),
    ).toHaveTextContent("1");
  });

  it("does not adopt a smaller paged list snapshot for unfiltered active stats", async () => {
    mockGetBusinessBills.mockResolvedValue({
      bills: [buildBill(1, 73.9)],
      total: 2,
      total_pages: 1,
    });

    render(
      <BillManager
        businessId={1}
        globalBills={[
          buildBill(1, 73.9),
          buildBill(2, 269.07),
          buildBill(3, 134.54),
        ]}
        globalBillsLoaded
        globalOrders={{ 1: [pendingOrder], 2: [pendingOrder], 3: [pendingOrder] }}
        globalOrdersLoaded
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(
        screen.getByTestId("stat-billManager.header.results"),
      ).toHaveTextContent("3");
    });
    expect(
      screen.getByTestId("stat-billManager.header.visibleValue"),
    ).toHaveTextContent("$477.51");
    expect(
      screen.getByTestId("stat-billManager.header.pendingOrders"),
    ).toHaveTextContent("3");
  });
});
