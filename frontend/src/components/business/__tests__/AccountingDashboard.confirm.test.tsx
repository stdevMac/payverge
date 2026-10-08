/** @jest-environment jsdom */
/**
 * Round 5 audit — Task 5 (F-C2)
 *
 * AccountingDashboard had two irreversible actions wired to a single click:
 *   - "Void entry" on the Entries tab
 *   - "Mark payroll paid" on the Payroll tab
 *
 * Both now go through a ConfirmationModal. These smoke tests lock in the gate:
 *   1. Clicking Void opens the modal and does NOT call the API; confirming
 *      from the modal fires voidEntry exactly once.
 *   2. Clicking Mark Paid opens the modal and does NOT call the API;
 *      dismissing the modal leaves markPayrollRunPaid untouched.
 */

import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// ---------- Mocks ----------

// Translation provider — return the key so assertions can find labels.
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

// useBusinessAccess — grant access immediately so the dashboard renders.
jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    hasAccess: true,
    isSuspended: false,
    loading: false,
    access: null,
    error: null,
    lockState: "active",
    aiConfigured: false,
    refetch: jest.fn(),
  }),
}));

// Staff API — no staff required for the confirm-gate tests.
jest.mock("@/api/staff", () => ({
  getBusinessStaff: jest.fn().mockResolvedValue({
    staff: [],
    pending_invitations: [],
  }),
}));

// Accounting API — the mutation spies live outside so the tests can assert
// against them. Names are prefixed with `mock` so Jest's hoisting rule allows
// them to be referenced inside the mock factory.
const mockVoidEntry = jest.fn().mockResolvedValue(undefined);
const mockMarkPayrollRunPaid = jest.fn().mockResolvedValue({ id: 42 });

jest.mock("@/api/wasteVariance", () => ({
  wasteVarianceApi: { getWasteVariance: jest.fn().mockResolvedValue(null) },
}));

jest.mock("@/api/laborCost", () => ({
  laborCostApi: { getLaborCost: jest.fn().mockResolvedValue(null) },
}));

jest.mock("@/api/fiscal", () => ({
  fiscalApi: {
    listReceipts: jest.fn().mockResolvedValue([]),
    listReceiptsPage: jest.fn().mockResolvedValue({
      receipts: [],
      total: 0,
      page: 1,
      page_size: 20,
      total_pages: 0,
    }),
    getSettings: jest.fn().mockResolvedValue(null),
    updateSettings: jest.fn(),
    issueReceipt: jest.fn(),
    retryReceipt: jest.fn(),
  },
  listReceipts: jest.fn().mockResolvedValue([]),
  getSettings: jest.fn().mockResolvedValue(null),
  updateSettings: jest.fn(),
  issueReceipt: jest.fn(),
  retryReceipt: jest.fn(),
}));

jest.mock("@/api/accounting", () => {
  // Fixtures live inside the factory so the hoisted `jest.mock` call can
  // safely evaluate them before the module's top-level declarations run.
  const summary = {
    start_date: "2026-04-01",
    end_date: "2026-04-15",
    currency: "USD",
    auto_income_total: 0,
    manual_income_total: 0,
    expense_total: 0,
    payroll_total: 0,
    billed_total: 0,
    collected_total: 0,
    collection_gap: 0,
    income_breakdown: [],
    expense_breakdown: [],
    payroll_summary: {
      paid_runs: 0,
      total_gross: 0,
      total_bonus: 0,
      total_deduction: 0,
      total_net: 0,
    },
    warnings: [],
  };
  const entriesPage = {
    entries: [
      {
        id: 7,
        business_id: 1,
        entry_type: "expense",
        category: "inventory",
        amount: 123.45,
        currency: "USD",
        occurred_at: "2026-04-10T00:00:00Z",
        description: "Flour delivery",
        voided_at: null,
      },
    ],
    total: 1,
    page: 1,
    page_size: 50,
    total_pages: 1,
  };
  const payrollRuns = [
    {
      id: 42,
      period_start: "2026-04-01T00:00:00Z",
      period_end: "2026-04-15T00:00:00Z",
      status: "draft",
      gross_total: 500,
      bonus_total: 0,
      deduction_total: 0,
      net_total: 500,
      line_items: [],
    },
  ];
  return {
    accountingApi: {
      getSummary: jest.fn().mockResolvedValue(summary),
      listEntries: jest.fn().mockResolvedValue(entriesPage),
      getTimeseries: jest.fn().mockResolvedValue({
        start: "2026-04-01",
        end: "2026-04-15",
        bucket: "day",
        currency: "USD",
        series: [],
      }),
      listPayrollRunsPage: jest.fn().mockResolvedValue({
        runs: payrollRuns,
        total: payrollRuns.length,
        page: 1,
        page_size: 20,
        total_pages: 1,
      }),
      voidEntry: (...args: unknown[]) => mockVoidEntry(...args),
      markPayrollRunPaid: (...args: unknown[]) =>
        mockMarkPayrollRunPaid(...args),
      createEntry: jest.fn(),
      createPayrollRun: jest.fn(),
      listAccountingCategories: jest
        .fn()
        .mockResolvedValue({ defaults: [], custom: [] }),
      getFoodCost: jest.fn().mockResolvedValue(null),
      getProfitLoss: jest.fn().mockResolvedValue({
        current: {
          revenue: 0,
          other_income: 0,
          other_income_by_category: [],
          cogs: 0,
          labor: 0,
          opex: 0,
          opex_by_category: [],
          net: 0,
          currency: "USD",
        },
      }),
      profitLossExportUrl: jest.fn(() => "https://api.test/pnl.csv"),
      payrollExportUrl: jest.fn(
        () => "https://api.test/payroll/export.csv",
      ),
      entriesExportUrl: jest.fn(
        () =>
          "https://api.test/entries/export.csv?start=2026-04-01&end=2026-04-15",
      ),
    },
  };
});

// Import after mocks are in place.
import AccountingDashboard from "@/components/business/AccountingDashboard";

function renderDashboard(ui: React.ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
  );
}

// Switch tabs via the tab button text (translation keys are returned as-is).
async function switchTab(label: string) {
  const tab = await screen.findByRole("tab", { name: label });
  await act(async () => {
    fireEvent.click(tab);
  });
}

describe("AccountingDashboard — irreversible-action confirmations", () => {
  beforeEach(() => {
    mockVoidEntry.mockClear();
    mockMarkPayrollRunPaid.mockClear();
  });

  it("does not call voidEntry until the confirmation modal is confirmed", async () => {
    renderDashboard(<AccountingDashboard businessId="biz-1" />);

    // Wait for Overview P&L to mount (Task 25 KPI labels).
    await waitFor(() => {
      expect(
        screen.getByText(
          "businessDashboard.accountingDashboard.overview.kpi.billIncome",
        ),
      ).toBeInTheDocument();
    });

    // Move to the Entries tab so row actions render.
    await switchTab("businessDashboard.accountingDashboard.tabs.entries");

    // Open the row actions menu, then choose Void.
    await screen.findByText("Flour delivery");
    const actionsTrigger = await screen.findByRole("button", {
      name: "entry-actions-7",
    });
    await act(async () => {
      fireEvent.click(actionsTrigger);
    });
    const voidItem = await screen.findByRole("menuitem", {
      name: "businessDashboard.accountingDashboard.entries.menu.void",
    });
    await act(async () => {
      fireEvent.click(voidItem);
    });

    // Modal is now visible — but no API call yet.
    expect(
      await screen.findByRole("dialog"),
    ).toBeInTheDocument();
    expect(mockVoidEntry).not.toHaveBeenCalled();

    // Confirm — ConfirmationModal renders the confirm button with the
    // confirmLabel we passed (`entries.void`). Scope the query to the dialog
    // so we don't pick up other Void controls.
    const dialog = screen.getByRole("dialog");
    const confirmBtn = await within(dialog).findByRole("button", {
      name: "businessDashboard.accountingDashboard.entries.void",
    });
    await act(async () => {
      fireEvent.click(confirmBtn);
    });

    await waitFor(() => {
      expect(mockVoidEntry).toHaveBeenCalledTimes(1);
    });
    expect(mockVoidEntry).toHaveBeenCalledWith("biz-1", 7);
  });

  it("does not call markPayrollRunPaid when the confirmation modal is cancelled", async () => {
    renderDashboard(<AccountingDashboard businessId="biz-1" />);

    await waitFor(() => {
      expect(
        screen.getByText(
          "businessDashboard.accountingDashboard.overview.kpi.billIncome",
        ),
      ).toBeInTheDocument();
    });

    await switchTab("businessDashboard.accountingDashboard.tabs.payroll");

    // Mark-paid lives in the row actions menu after the Payroll redesign.
    const actionsTrigger = await screen.findByRole("button", {
      name: "payroll-actions-42",
    });
    await act(async () => {
      fireEvent.click(actionsTrigger);
    });
    const markBtn = await screen.findByRole("menuitem", {
      name: "businessDashboard.accountingDashboard.payrollRuns.markPaid",
    });
    await act(async () => {
      fireEvent.click(markBtn);
    });

    // Modal open, no API call yet.
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toBeInTheDocument();
    expect(mockMarkPayrollRunPaid).not.toHaveBeenCalled();

    // Click Cancel inside the modal (ConfirmationModal labels it
    // "Cancel" by default since we do not pass a cancelLabel).
    const cancelBtn = within(dialog).getByRole("button", { name: /cancel/i });
    await act(async () => {
      fireEvent.click(cancelBtn);
    });

    // Modal closed, API still untouched.
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    expect(mockMarkPayrollRunPaid).not.toHaveBeenCalled();
  });
});
