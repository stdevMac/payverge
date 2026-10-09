/** @jest-environment jsdom */
/**
 * Related to issue 371: header "2 Results · Active Bills 2" must paint both
 * active checks. A paged list of one row is a lie.
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
  BillFilters: ({ collapseIdleDates }: { collapseIdleDates?: boolean }) => (
    <div
      data-testid="bill-filters"
      data-collapse-idle-dates={collapseIdleDates ? "true" : "false"}
    />
  ),
}));
jest.mock("../BillsTable", () => ({
  BillsTable: ({
    bills,
  }: {
    bills: Array<{ id: number; bill_number: string }>;
  }) => (
    <ul data-testid="painted-bills">
      {bills.map((bill) => (
        <li key={bill.id} data-testid={`painted-bill-${bill.id}`}>
          {bill.bill_number}
        </li>
      ))}
    </ul>
  ),
}));
jest.mock("../PendingOrdersSection", () => ({
  PendingOrdersSection: ({ compact }: { compact?: boolean }) => (
    <div
      data-testid="pending-orders-section"
      data-compact={compact ? "true" : "false"}
    />
  ),
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
      {header?.stats?.map((stat) => (
        <div key={String(stat.label)} data-testid={`stat-${stat.label}`}>
          {stat.value} {stat.label}
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

const buildBill = (
  id: number,
  total: number,
  status: "open" | "partial" = "open",
): Bill =>
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
    paid_amount: status === "partial" ? total / 2 : 0,
    tip_amount: 0,
    status,
    settlement_address: "",
    tipping_address: "",
    created_at: new Date(2026, 7, id, 12, 0, 0).toISOString(),
    updated_at: new Date(2026, 7, id, 12, 0, 0).toISOString(),
  }) as Bill;

const pendingOrder = {
  id: 9,
  bill_id: 756,
  business_id: 1,
  order_number: "ORD-9",
  status: "pending",
  items: "[]",
  currency: "USD",
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
} as Order;

describe("BillManager first-screen honesty (issue 371)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGetBusinessBills.mockResolvedValue({
      bills: [buildBill(756, 21.53)],
      total: 2,
      total_pages: 1,
    });
  });

  it("paints every unfiltered active bill the header counts, not the paged slice", async () => {
    render(
      <BillManager
        businessId={1}
        globalBills={[buildBill(756, 21.53), buildBill(387, 36.08, "partial")]}
        globalBillsLoaded
        globalOrders={{ 756: [pendingOrder] }}
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

    expect(screen.getByTestId("painted-bills").children).toHaveLength(2);
    expect(screen.getByTestId("painted-bill-756")).toHaveTextContent(
      "BILL-756",
    );
    expect(screen.getByTestId("painted-bill-387")).toHaveTextContent(
      "BILL-387",
    );
    expect(screen.getByTestId("pending-orders-section")).toHaveAttribute(
      "data-compact",
      "true",
    );
    expect(screen.getByTestId("bill-filters")).toHaveAttribute(
      "data-collapse-idle-dates",
      "true",
    );
  });

  it("does not clip the bills panel behind overflow-hidden", () => {
    const src = require("fs").readFileSync(
      require("path").join(__dirname, "../BillManager.tsx"),
      "utf8",
    ) as string;
    expect(src).toMatch(/listedActiveBills/);
    expect(src).toMatch(/PremiumPanel className="overflow-visible"/);
    expect(src).not.toMatch(/PremiumPanel className="overflow-hidden"/);
    expect(src).toMatch(/<PendingOrdersSection[\s\S]*compact/);
  });
});
