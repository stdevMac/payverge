/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import AccountingDashboard from "@/components/business/AccountingDashboard";
import { accountingApi } from "@/api/accounting";
import { getBusinessStaff } from "@/api/staff";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));
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
    receiptsExportUrl: jest.fn(() => "https://api.test/receipts.csv"),
  },
  listReceiptsPage: jest.fn().mockResolvedValue({
    receipts: [],
    total: 0,
    page: 1,
    page_size: 20,
    total_pages: 0,
  }),
  receiptsExportUrl: jest.fn(() => "https://api.test/receipts.csv"),
}));
jest.mock("@/api/staff", () => ({ getBusinessStaff: jest.fn() }));
jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: jest.fn() }));
jest.mock("@/components/business/fiscal/FiscalDashboard", () => ({
  __esModule: true,
  default: () => <div data-testid="invoice-dashboard">Invoices</div>,
}));


function renderDashboard(ui: React.ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
  );
}

describe("AccountingDashboard — URL sync", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (useBusinessAccess as jest.Mock).mockReturnValue({
      hasAccess: true,
      isSuspended: false,
      loading: false,
      access: null,
      error: null,
      lockState: "active",
      aiConfigured: false,
      refetch: jest.fn(),
    });
    (accountingApi.getSummary as jest.Mock).mockResolvedValue({
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
    });
    (accountingApi.listEntries as jest.Mock).mockResolvedValue({
      entries: [],
      total: 0,
      page: 1,
      page_size: 5,
      total_pages: 1,
    });
    (getBusinessStaff as jest.Mock).mockResolvedValue({
      staff: [],
      pending_invitations: [],
    });
  });

  it("fires onTabChange with the inner tab key when a tab is selected", async () => {
    const onTabChange = jest.fn();
    renderDashboard(<AccountingDashboard
        businessId="42"
        initialTab="overview"
        onTabChange={onTabChange}
      />,
    );
    fireEvent.click(await screen.findByRole("tab", { name: /invoices/i }));
    await waitFor(() => {
      expect(onTabChange).toHaveBeenCalledWith("invoices");
    });
  });

  it("renders the inner tab passed via initialTab without firing onTabChange", async () => {
    const onTabChange = jest.fn();
    renderDashboard(<AccountingDashboard
        businessId="42"
        initialTab="invoices"
        onTabChange={onTabChange}
      />,
    );
    expect(await screen.findByTestId("invoice-dashboard")).toBeTruthy();
    expect(onTabChange).not.toHaveBeenCalled();
  });
});
