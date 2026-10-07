/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import AccountingDashboard from "@/components/business/AccountingDashboard";
import { accountingApi } from "@/api/accounting";
import { fiscalApi } from "@/api/fiscal";
import { getBusinessStaff } from "@/api/staff";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

jest.mock("@/api/accounting", () => ({
  accountingApi: {
    getSummary: jest.fn(),
    listEntries: jest.fn(),
    getTimeseries: jest.fn().mockResolvedValue({
      start: "2026-01-01",
      end: "2026-01-31",
      bucket: "day",
      currency: "USD",
      series: [],
    }),
    listPayrollRunsPage: jest.fn().mockResolvedValue({ runs: [], total: 0, page: 1, page_size: 20, total_pages: 0 }),
    createEntry: jest.fn(),
    voidEntry: jest.fn(),
    createPayrollRun: jest.fn(),
    markPayrollRunPaid: jest.fn(),
    deletePayrollRun: jest.fn(),
    voidPayrollRun: jest.fn(),
    getFoodCost: jest.fn().mockResolvedValue(null),
    getProfitLoss: jest.fn().mockResolvedValue({ current: { revenue: 0, other_income: 0, other_income_by_category: [], cogs: 0, labor: 0, opex: 0, opex_by_category: [], net: 0, currency: "USD" } }),
    profitLossExportUrl: jest.fn(() => "https://api.test/pnl.csv"),
    payrollExportUrl: jest.fn(() => "https://api.test/payroll/export.csv"),
    entriesExportUrl: jest.fn(() => "https://api.test/entries/export.csv"),
  },
}));
jest.mock("@/api/wasteVariance", () => ({
  wasteVarianceApi: { getWasteVariance: jest.fn().mockResolvedValue(null) },
}));
jest.mock("@/api/laborCost", () => ({
  laborCostApi: { getLaborCost: jest.fn().mockResolvedValue(null) },
}));
jest.mock("@/api/fiscal", () => ({
  getSettings: jest.fn().mockResolvedValue(null),
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
  },
}));
jest.mock("@/api/staff", () => ({ getBusinessStaff: jest.fn() }));
jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: jest.fn() }));
jest.mock("@/components/business/fiscal/FiscalDashboard", () => ({
  __esModule: true,
  default: () => <div data-testid="invoice-dashboard" />,
}));

const TIER_ACTIVE = {
  access: null,
  loading: false,
  error: null,
  hasAccess: true,
  isSuspended: false,
  lockState: "active",
  aiConfigured: false,
  refetch: jest.fn(),
};

const EMPTY_SUMMARY = {
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
  payroll_summary: { paid_runs: 0, total_net: 0 },
  skipped_manual_entries: 0,
  warnings: [],
};

const EMPTY_ENTRIES = {
  entries: [],
  total: 0,
  page: 1,
  page_size: 50,
  total_pages: 0,
};

function draftRun(overrides = {}) {
  return {
    id: 7,
    business_id: 42,
    period_start: "2026-03-01T00:00:00Z",
    period_end: "2026-03-15T00:00:00Z",
    status: "draft" as const,
    paid_at: null,
    currency: "USD",
    notes: "",
    gross_total: 500,
    bonus_total: 0,
    deduction_total: 0,
    net_total: 500,
    line_items: [],
    ...overrides,
  };
}

function setupMocks(runs: unknown[]) {
  (useBusinessAccess as jest.Mock).mockReturnValue(TIER_ACTIVE);
  (accountingApi.getSummary as jest.Mock).mockResolvedValue(EMPTY_SUMMARY);
  (accountingApi.listEntries as jest.Mock).mockResolvedValue(EMPTY_ENTRIES);
  (accountingApi.listPayrollRunsPage as jest.Mock).mockResolvedValue({
    runs,
    total: runs.length,
    page: 1,
    page_size: 20,
    total_pages: 1,
  });
  (accountingApi.deletePayrollRun as jest.Mock).mockResolvedValue(undefined);
  (accountingApi.voidPayrollRun as jest.Mock).mockResolvedValue({ ...draftRun(), status: "void" });
  (getBusinessStaff as jest.Mock).mockResolvedValue({ staff: [], pending_invitations: [] });
}


function renderDashboard(ui: React.ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
  );
}

describe("AccountingDashboard — initial fiscal load contract", () => {
  beforeEach(() => jest.clearAllMocks());

  it("requests a paginated fiscal activity page without an accounting load error", async () => {
    setupMocks([]);
    const consoleErrorSpy = jest
      .spyOn(console, "error")
      .mockImplementation(() => undefined);

    try {
      renderDashboard(<AccountingDashboard businessId="42" initialTab="payroll" />);

      await waitFor(() => {
        expect(fiscalApi.listReceiptsPage).toHaveBeenCalledWith(
          42,
          expect.objectContaining({ page: 1, page_size: 10 }),
        );
      });

      expect(consoleErrorSpy).not.toHaveBeenCalledWith(
        "Failed to load accounting data:",
        expect.anything(),
      );
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });
});

describe("AccountingDashboard — payroll delete/void (owner)", () => {
  beforeEach(() => jest.clearAllMocks());

  it("lets an owner delete a draft run via confirm", async () => {
    setupMocks([draftRun()]);
    renderDashboard(<AccountingDashboard businessId="42" initialTab="payroll" />);

    fireEvent.click(
      await screen.findByRole("button", { name: "payroll-actions-7" }),
    );
    fireEvent.click(
      await screen.findByRole("menuitem", { name: /^delete$/i }),
    );

    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: /delete/i }));

    await waitFor(() => {
      expect(accountingApi.deletePayrollRun).toHaveBeenCalledWith("42", 7);
    });
  });

  it("lets an owner void a paid run via confirm", async () => {
    setupMocks([draftRun({ id: 9, status: "paid", paid_at: "2026-03-20T00:00:00Z" })]);
    renderDashboard(<AccountingDashboard businessId="42" initialTab="payroll" />);

    fireEvent.click(
      await screen.findByRole("button", { name: "payroll-actions-9" }),
    );
    fireEvent.click(
      await screen.findByRole("menuitem", { name: /^void$/i }),
    );

    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: /void/i }));

    await waitFor(() => {
      expect(accountingApi.voidPayrollRun).toHaveBeenCalledWith("42", 9);
    });
  });

  it("renders a Void badge for a voided run", async () => {
    setupMocks([draftRun({ id: 11, status: "void", paid_at: "2026-03-20T00:00:00Z" })]);
    renderDashboard(<AccountingDashboard businessId="42" initialTab="payroll" />);

    // Exact "Void" matches the status badge only (the Recent Activity item now
    // legitimately reads "Payroll run voided", which the old /void/i regex also
    // caught, making it ambiguous).
    expect(await screen.findByText("Void")).toBeInTheDocument();
  });
});

describe("AccountingDashboard — payroll read-only (manager)", () => {
  beforeEach(() => jest.clearAllMocks());

  it("hides all payroll write controls when canManagePayroll is false", async () => {
    setupMocks([draftRun()]);
    renderDashboard(<AccountingDashboard
        businessId="42"
        initialTab="payroll"
        canManagePayroll={false}
      />,
    );

    // The draft run renders, but no write actions are offered.
    await screen.findByText(/draft/i);
    expect(screen.queryByRole("button", { name: /new payroll run/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /mark paid/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /^delete$/i })).toBeNull();
  });
});
