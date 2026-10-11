/** @jest-environment jsdom */
/**
 * #852 — the operator bills list must label money with the venue's currency.
 *
 * The list used to resolve its label from a separate getBusiness() call and
 * read only `default_currency`, so an ARS venue whose display currency is ARS
 * rendered peso totals as USD. The bill rows now carry their own resolved
 * currency; the list must prefer it, and fall back to the business payload's
 * display_currency before default_currency.
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
  formatCurrency: (n: number, currency: string) =>
    `${currency} ${n.toFixed(2)}`,
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

const mockGetBusiness = jest.fn();
jest.mock("@/api/business", () => ({
  getBusiness: (...args: unknown[]) => mockGetBusiness(...args),
}));

jest.mock("../BillCreator", () => ({ BillCreator: () => null }));
jest.mock("../BillDetailsModal", () => ({ BillDetailsModal: () => null }));
jest.mock("../BillFilters", () => ({ BillFilters: () => null }));
jest.mock("../BillsTable", () => ({
  BillsTable: ({ currency }: { currency?: string }) => (
    <div data-testid="bills-table-currency">{currency}</div>
  ),
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
jest.mock("../shared/DashboardTabShell", () => ({
  __esModule: true,
  default: ({
    header,
    children,
  }: {
    header?: { stats?: Array<{ label: string; value: React.ReactNode }> };
    children?: React.ReactNode;
  }) => (
    <div>
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

const buildBill = (id: number, total: number, currency?: string): Bill =>
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
    ...(currency ? { currency } : {}),
    status: "open",
    settlement_address: "",
    tipping_address: "",
    created_at: new Date(2026, 4, 1, 12, 0, 0).toISOString(),
    updated_at: new Date(2026, 4, 1, 12, 0, 0).toISOString(),
  }) as Bill;

const renderManager = (bills: Bill[]) =>
  render(
    <BillManager
      businessId={1}
      globalBills={bills}
      globalBillsLoaded
      globalOrders={{}}
      globalOrdersLoaded
      kitchenEnabled
      kitchenStatusLoading={false}
      onKitchenStatusChange={jest.fn()}
      onOrderStatusChange={jest.fn()}
    />,
  );

describe("BillManager venue currency (#852)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGetBusiness.mockResolvedValue({
      id: 1,
      default_currency: "USD",
      timezone: "UTC",
    });
  });

  it("labels money with the bill's own currency, not the business default", async () => {
    const bills = [buildBill(1, 100, "ARS"), buildBill(2, 50, "ARS")];
    mockGetBusinessBills.mockResolvedValue({
      bills,
      total: 2,
      total_pages: 1,
    });

    renderManager(bills);

    await waitFor(() => {
      expect(screen.getByTestId("bills-table-currency")).toHaveTextContent(
        "ARS",
      );
    });
    expect(
      screen.getByTestId("stat-billManager.header.visibleValue"),
    ).toHaveTextContent("ARS 150.00");
  });

  it("falls back to the business display_currency when the bills carry none", async () => {
    const bills = [buildBill(3, 100)];
    mockGetBusinessBills.mockResolvedValue({
      bills,
      total: 1,
      total_pages: 1,
    });
    mockGetBusiness.mockResolvedValue({
      id: 1,
      display_currency: "ARS",
      default_currency: "USD",
      timezone: "UTC",
    });

    renderManager(bills);

    await waitFor(() => {
      expect(screen.getByTestId("bills-table-currency")).toHaveTextContent(
        "ARS",
      );
    });
    expect(
      screen.getByTestId("stat-billManager.header.visibleValue"),
    ).toHaveTextContent("ARS 100.00");
  });
});
