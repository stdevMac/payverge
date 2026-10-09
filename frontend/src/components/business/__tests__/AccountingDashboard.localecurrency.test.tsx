/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
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
  },
}));

jest.mock("@/api/staff", () => ({ getBusinessStaff: jest.fn() }));
jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: jest.fn() }));

// Force the operator locale to Spanish; keep the real getTranslation so labels
// resolve. The component's locale must drive money formatting.
jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "es", setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});

jest.mock("@/components/business/fiscal/FiscalDashboard", () => ({
  __esModule: true,
  default: () => <div data-testid="invoice-dashboard" />,
}));


function renderDashboard(ui: React.ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
  );
}

describe("AccountingDashboard — locale-aware currency formatting", () => {
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
    (accountingApi.listEntries as jest.Mock).mockResolvedValue({
      entries: [],
      total: 0,
      page: 1,
      page_size: 5,
      total_pages: 0,
    });
    (getBusinessStaff as jest.Mock).mockResolvedValue({
      staff: [],
      pending_invitations: [],
    });
  });

  it("uses the Spanish decimal comma, not the en-US dot, for money", async () => {
    renderDashboard(<AccountingDashboard businessId="42" />);

    // es formats with a decimal comma (e.g. "100,00" billed). Prefer static
    // reconciliation figures — net P&L uses AnimatedNumberText which may lag.
    expect(await screen.findByText(/100,00/)).toBeTruthy();
    expect(screen.queryByText("$55.00")).toBeNull();
    expect(screen.queryByText("$100.00")).toBeNull();
  });
});
