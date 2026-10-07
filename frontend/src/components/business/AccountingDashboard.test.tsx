/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import AccountingDashboard from "@/components/business/AccountingDashboard";
import { accountingApi } from "@/api/accounting";
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
    listPayrollRunsPage: jest
      .fn()
      .mockResolvedValue({
        runs: [],
        total: 0,
        page: 1,
        page_size: 20,
        total_pages: 0,
      }),
    createEntry: jest.fn(),
    voidEntry: jest.fn(),
    createPayrollRun: jest.fn(),
    markPayrollRunPaid: jest.fn(),
    getFoodCost: jest.fn().mockResolvedValue(null),
    payrollExportUrl: jest.fn(() => "https://api.test/payroll/export.csv"),
    entriesExportUrl: jest.fn(
      () =>
        "https://api.test/entries/export.csv?start=2026-01-01&end=2026-01-31",
    ),
    listAccountingCategories: jest.fn().mockResolvedValue({
      defaults: [],
      custom: [],
    }),
    listRecurringTemplates: jest.fn().mockResolvedValue([]),
    getPeriodLock: jest.fn().mockResolvedValue({ locked_through: null }),
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
    getUnpaidBills: jest.fn().mockResolvedValue({
      bills: [],
      total: 0,
      page: 1,
      page_size: 25,
      total_pages: 0,
    }),
  },
}));

function renderDashboard(ui: React.ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
  );
}

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
    receiptsExportUrl: jest.fn(
      () =>
        "https://api.test/fiscal/receipts/export.csv?start=2026-01-01&end=2026-01-31",
    ),
  },
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
  receiptsExportUrl: jest.fn(
    () =>
      "https://api.test/fiscal/receipts/export.csv?start=2026-01-01&end=2026-01-31",
  ),
}));

jest.mock("@/api/staff", () => ({
  getBusinessStaff: jest.fn(),
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: jest.fn(),
}));

jest.mock("@/components/business/fiscal/FiscalDashboard", () => ({
  __esModule: true,
  default: ({ businessId }: { businessId: number }) => (
    <div data-testid="invoice-dashboard">
      Invoices for business {businessId}
    </div>
  ),
}));

describe("AccountingDashboard", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (useBusinessAccess as jest.Mock).mockReturnValue({
      access: null,
      loading: false,
      error: null,
      hasAccess: true,
      isSuspended: false,
      lockState: "active",
      aiConfigured: false,
      refetch: jest.fn(),
    });
    (accountingApi.getSummary as jest.Mock).mockResolvedValue({
      currency: "USD",
      auto_income_total: 85,
      manual_income_total: 30,
      expense_total: 40,
      payroll_total: 20,
      billed_total: 100,
      collected_total: 85,
      collection_gap: 15,
      income_breakdown: [],
      expense_breakdown: [],
      payroll_summary: { paid_runs: 1, total_net: 20 },
      skipped_manual_entries: 0,
      warnings: [],
    });
    const entriesFixture = [
      {
        id: 1,
        entry_type: "expense",
        category: "inventory",
        amount: 40,
        currency: "USD",
        occurred_at: "2026-01-15T12:00:00Z",
        description: "Inventory restock",
      },
    ];
    (accountingApi.listEntries as jest.Mock).mockResolvedValue({
      entries: entriesFixture,
      total: 1,
      page: 1,
      page_size: 5,
      total_pages: 1,
    });
    (accountingApi.listPayrollRunsPage as jest.Mock).mockResolvedValue({
      runs: [
        {
          id: 7,
          status: "draft",
          period_start: "2026-01-01T00:00:00Z",
          period_end: "2026-01-15T00:00:00Z",
          net_total: 155,
          line_items: [],
          payee_count: 0,
        },
      ],
      total: 1,
      page: 1,
      page_size: 20,
      total_pages: 1,
    });
    (getBusinessStaff as jest.Mock).mockResolvedValue({
      staff: [
        {
          id: 9,
          name: "Manager",
          email: "manager@example.com",
          role: "manager",
          business_id: 42,
          created_at: "",
          updated_at: "",
        },
      ],
      pending_invitations: [],
    });
  });

  it("renders summary cards and existing records", async () => {
    renderDashboard(<AccountingDashboard businessId="42" />);

    // Overview P&L labels (Task 25) — no 5-row preview tables.
    expect(await screen.findByText("Net profit / loss")).toBeTruthy();
    expect(screen.getByText("Bill income")).toBeTruthy();
    expect(screen.getByText("Recent activity")).toBeTruthy();
    expect(await screen.findByText(/Inventory restock/)).toBeTruthy();
  });

  it("issues a single accounting/summary fetch per range (no parent+Overview double fetch)", async () => {
    renderDashboard(<AccountingDashboard businessId="42" />);

    expect(await screen.findByText("Net profit / loss")).toBeTruthy();

    await waitFor(() => {
      expect(accountingApi.getSummary).toHaveBeenCalled();
    });
    // useSummary is the sole owner; AccountingTab no longer raw-getSummary.
    expect(accountingApi.getSummary).toHaveBeenCalledTimes(1);
  });

  it("entries tab loads server-paginated rows and shows New entry", async () => {
    renderDashboard(<AccountingDashboard businessId="42" />);

    fireEvent.click(await screen.findByRole("tab", { name: "Entries" }));

    expect(await screen.findByText("Inventory restock")).toBeTruthy();
    expect(screen.getByRole("button", { name: /new entry/i })).toBeTruthy();
    expect(screen.getByRole("link", { name: /export csv/i })).toBeTruthy();
    await waitFor(() => {
      expect(accountingApi.listEntries).toHaveBeenCalled();
    });
  });

  it("renders invalid currency codes without crashing", async () => {
    (accountingApi.getSummary as jest.Mock).mockResolvedValue({
      currency: "bad-currency",
      auto_income_total: 85,
      manual_income_total: 30,
      expense_total: 40,
      payroll_total: 20,
      billed_total: 100,
      collected_total: 85,
      collection_gap: 15,
      income_breakdown: [],
      expense_breakdown: [],
      payroll_summary: { paid_runs: 1, total_net: 20 },
      skipped_manual_entries: 0,
      warnings: [],
    });

    renderDashboard(<AccountingDashboard businessId="42" />);

    // Currency note + reconciliation use static formatMoney (not AnimatedNumber).
    expect(await screen.findByText("All amounts in bad-currency")).toBeTruthy();
    expect(screen.getByText("BAD-CURRENCY 100.00")).toBeTruthy();
  });

  it("renders accounting warnings from the summary response", async () => {
    (accountingApi.getSummary as jest.Mock).mockResolvedValue({
      currency: "ARS",
      auto_income_total: 85,
      manual_income_total: 10_000,
      expense_total: 40,
      payroll_total: 20,
      billed_total: 100,
      collected_total: 85,
      collection_gap: 15,
      income_breakdown: [],
      expense_breakdown: [],
      payroll_summary: { paid_runs: 1, total_net: 20 },
      skipped_manual_entries: 1,
      warnings: [
        {
          code: "fx_rate_missing",
          params: {
            count: "1",
            scope: "manual_income",
            currency: "ARS",
            details: "#2 BTC",
          },
        },
      ],
    });

    renderDashboard(<AccountingDashboard businessId="42" />);

    expect(await screen.findByText("Accounting warnings")).toBeTruthy();
    // Warnings are now structured (code + params) and rendered via i18n; the
    // scope label resolves to the plural "manual income entries" phrase and the
    // skipped-entry detail ("#2 BTC") is preserved in the details suffix.
    expect(
      screen.getByText(
        "Skipped 1 manual income entries that could not be converted to ARS: #2 BTC",
      ),
    ).toBeTruthy();
  });

  it("adds invoices as an internal accounting tab", async () => {
    renderDashboard(<AccountingDashboard businessId="42" />);

    fireEvent.click(await screen.findByRole("tab", { name: "Invoices" }));

    expect(screen.getByTestId("invoice-dashboard")).toHaveTextContent(
      "Invoices for business 42",
    );
  });

  it("can open directly on the invoices tab for legacy fiscal deep links", async () => {
    renderDashboard(
      <AccountingDashboard businessId="42" initialTab="invoices" />,
    );

    expect(await screen.findByTestId("invoice-dashboard")).toBeInTheDocument();
  });

  it("keeps accounting available when staff loading fails", async () => {
    (getBusinessStaff as jest.Mock).mockRejectedValue(
      new Error("staff unavailable"),
    );

    renderDashboard(<AccountingDashboard businessId="42" />);

    expect(await screen.findByText("Net profit / loss")).toBeTruthy();
    expect(await screen.findByText(/Inventory restock/)).toBeTruthy();
  });

  it("opens the create drawer from New entry on the entries tab", async () => {
    renderDashboard(<AccountingDashboard businessId="42" />);

    fireEvent.click(await screen.findByRole("tab", { name: "Entries" }));
    fireEvent.click(await screen.findByRole("button", { name: /new entry/i }));

    // EntryFormDrawer surfaces in DetailDrawer with create fields.
    expect(await screen.findByTestId("detail-drawer")).toBeInTheDocument();
    expect(screen.getByLabelText(/^Amount$/i)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /save entry/i }),
    ).toBeInTheDocument();
    expect(accountingApi.createEntry).not.toHaveBeenCalled();
  });
});
