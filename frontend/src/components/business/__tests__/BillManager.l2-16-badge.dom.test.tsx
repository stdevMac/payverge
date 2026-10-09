/**
 * D1 / L2-16: Facturas history/active tab badges must show the exact bill total
 * (e.g. 234), never the default rail cap "9+", while the header "results" stat
 * already shows the full count.
 *
 * The AnimatedBadge unit test with cap={null} alone is not enough — L2-16 is
 * the BillManager wiring of badgeCap: null into SegmentedTabs. Revert-proof:
 * removing badgeCap (or setting a finite 9) makes this DOM assertion fail.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

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

const HIGH_TOTAL = 234;

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

describe("BillManager L2-16 tab badge exact totals (DOM)", () => {
  beforeEach(() => {
    mockGetBusinessBills.mockReset();
    // Page of bills + high total so a default cap of 9 would render "9+".
    mockGetBusinessBills.mockResolvedValue({
      bills: Array.from({ length: 10 }, (_, i) => buildBill(i + 1)),
      total: HIGH_TOTAL,
      total_pages: 24,
    });
  });

  it("renders the exact filtered total on the active tab badge, never 9+", async () => {
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

    // AnimatedBadge is aria-hidden on tab badges — assert visible digits on the tab.
    // Header results + tab badge both paint 234 (getByText alone is ambiguous).
    await waitFor(() => {
      const activeTab = screen.getByRole("tab", {
        name: /billManager\.tabs\.active/i,
      });
      expect(activeTab.textContent).toMatch(new RegExp(String(HIGH_TOTAL)));
      expect(activeTab.textContent).not.toMatch(/9\+/);
    });

    // No capped badge anywhere on the page for this total.
    expect(screen.queryByText("9+")).toBeNull();
  });

  it("keeps the exact total on the history tab badge after switch", async () => {
    mockGetBusinessBills.mockImplementation(
      (_biz: unknown, opts: { status?: string }) => {
        // History uses paid/closed/voided filter; still return high total.
        const isHistory =
          typeof opts?.status === "string" &&
          (opts.status.includes("paid") || opts.status.includes("closed"));
        return Promise.resolve({
          bills: [buildBill(isHistory ? 900 : 1)],
          total: HIGH_TOTAL,
          total_pages: 24,
        });
      },
    );

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

    await waitFor(() => {
      expect(
        screen.getByRole("tab", { name: /billManager\.tabs\.active/i })
          .textContent,
      ).toMatch(/234/);
    });

    fireEvent.click(
      screen.getByRole("tab", { name: /billManager\.tabs\.history/i }),
    );

    await waitFor(() => {
      const historyTab = screen.getByRole("tab", {
        name: /billManager\.tabs\.history/i,
      });
      expect(historyTab.textContent).toMatch(/234/);
      expect(historyTab.textContent).not.toMatch(/9\+/);
    });
    expect(screen.queryByText("9+")).toBeNull();
  });
});
