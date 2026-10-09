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

function setupDefaultMocks() {
  (useBusinessAccess as jest.Mock).mockReturnValue(TIER_ACTIVE);
  (accountingApi.getSummary as jest.Mock).mockResolvedValue(EMPTY_SUMMARY);
  (accountingApi.listEntries as jest.Mock).mockResolvedValue(EMPTY_ENTRIES);
    (accountingApi.listPayrollRunsPage as jest.Mock).mockResolvedValue({
      runs: [],
      total: 0,
      page: 1,
      page_size: 20,
      total_pages: 0,
    });
  (getBusinessStaff as jest.Mock).mockResolvedValue({
    staff: [],
    pending_invitations: [],
  });
}


function renderDashboard(ui: React.ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
  );
}

// Pre-redesign inline payroll form tests — create/line UX now lives in
// PayrollRunFormDrawer (+ PayrollRunFormDrawer.test.tsx). Kept as skipped
// historical coverage until Task 28 deletes the legacy suite.
describe.skip("AccountingDashboard — payroll line removal", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    setupDefaultMocks();
  });

  it("removes an added payroll line via the per-line remove control", async () => {
    renderDashboard(<AccountingDashboard businessId="42" initialTab="payroll" />);

    // Open the payroll run form.
    fireEvent.click(await screen.findByRole("button", { name: /new payroll run/i }));

    // One line by default. Add a second line.
    fireEvent.click(
      await screen.findByRole("button", { name: /add payroll line/i }),
    );

    await waitFor(() => {
      expect(
        screen.getAllByRole("button", { name: /remove payroll line/i }),
      ).toHaveLength(2);
    });

    // Remove a line — count drops back to one.
    const removeButtons = screen.getAllByRole("button", {
      name: /remove payroll line/i,
    });
    fireEvent.click(removeButtons[0]);

    await waitFor(() => {
      expect(
        screen.getAllByRole("button", { name: /remove payroll line/i }),
      ).toHaveLength(1);
    });

    // The last remaining remove button is disabled (never remove the final line).
    expect(
      screen.getByRole("button", { name: /remove payroll line/i }),
    ).toBeDisabled();
  });
});

describe.skip("AccountingDashboard — payroll net-negative guard", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    setupDefaultMocks();
  });

  it("rejects a payroll line where deduction exceeds gross+bonus before hitting the API", async () => {
    renderDashboard(<AccountingDashboard businessId="42" initialTab="payroll" />);

    // Open the payroll run form.
    fireEvent.click(await screen.findByRole("button", { name: /new payroll run/i }));

    // Switch first line to contractor so we avoid the staff-select requirement.
    // The payee type <select> is the first combobox in the form.
    const allSelects = await screen.findAllByRole("combobox");
    const payeeSelect = allSelects[0];
    fireEvent.change(payeeSelect, { target: { value: "contractor" } });

    // Fill contractor name (now visible since we switched to contractor).
    // Index 0 = payrollNotes (form-level), index 1 = payee_name (contractor).
    const contractorNameInputs = screen.getAllByRole("textbox");
    fireEvent.change(contractorNameInputs[1], { target: { value: "Alice" } });

    // Set gross=10, deduction=50 → net = -40 (net-negative).
    // Number inputs appear in order: gross, bonus, deduction.
    const numberInputs = screen.getAllByRole("spinbutton");
    fireEvent.change(numberInputs[0], { target: { value: "10" } }); // gross
    fireEvent.change(numberInputs[2], { target: { value: "50" } }); // deduction (index 2)

    // Click Create Payroll Run.
    fireEvent.click(screen.getByRole("button", { name: /create payroll run/i }));

    // The API must NOT have been called (client-side guard fires first).
    await waitFor(() => {
      expect(accountingApi.createPayrollRun).not.toHaveBeenCalled();
    });

    // An error message about net-negative must be visible.
    await waitFor(() => {
      expect(
        screen.getByText(/net.*negative|deduction.*exceed|net pay.*zero/i),
      ).toBeInTheDocument();
    });
  });
});

describe.skip("AccountingDashboard — payroll create optimistic append", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    setupDefaultMocks();
  });

  it("shows the newly created run immediately even when its period is outside the current date window", async () => {
    // Simulate a payroll run that falls far in the past — outside the default
    // 30-day window.  The component must still display it optimistically after
    // create rather than waiting for the filtered re-fetch to surface it.
    const newRun = {
      id: 99,
      business_id: 42,
      period_start: "2025-01-01T00:00:00Z",
      period_end: "2025-01-15T00:00:00Z",
      status: "draft" as const,
      paid_at: null,
      notes: "",
      gross_total: 500,
      bonus_total: 0,
      deduction_total: 0,
      net_total: 500,
      line_items: [],
    };

    // createPayrollRun resolves with the new run; listPayrollRunsPage keeps
    // returning [] (the re-fetch won't find the run because it's outside the window).
    (accountingApi.createPayrollRun as jest.Mock).mockResolvedValue(newRun);
    (accountingApi.listPayrollRunsPage as jest.Mock).mockResolvedValue({
      runs: [],
      total: 0,
      page: 1,
      page_size: 20,
      total_pages: 0,
    });

    renderDashboard(<AccountingDashboard businessId="42" initialTab="payroll" />);

    // Open the payroll run form.
    fireEvent.click(await screen.findByRole("button", { name: /new payroll run/i }));

    // Switch to contractor to skip the staff-select requirement.
    const allSelects = await screen.findAllByRole("combobox");
    const payeeSelect = allSelects[0];
    fireEvent.change(payeeSelect, { target: { value: "contractor" } });

    // Fill contractor name.
    // Index 0 = payrollNotes (form-level), index 1 = payee_name (contractor).
    const contractorNameInputs = screen.getAllByRole("textbox");
    fireEvent.change(contractorNameInputs[1], { target: { value: "Bob" } });

    // Fill in a valid gross amount (500).
    // Number inputs appear in order: gross, bonus, deduction.
    const numberInputs = screen.getAllByRole("spinbutton");
    fireEvent.change(numberInputs[0], { target: { value: "500" } }); // gross

    // Click Create Payroll Run.
    fireEvent.click(screen.getByRole("button", { name: /create payroll run/i }));

    // Wait for create to be called.
    await waitFor(() => {
      expect(accountingApi.createPayrollRun).toHaveBeenCalled();
    });

    // The new run must appear in the DOM even though the re-fetch returns [].
    // Either "Draft" badge or the net total "$500.00" must be visible.
    await waitFor(() => {
      const hasDraft = screen.queryAllByText(/draft/i).length > 0;
      const hasAmount = screen.queryAllByText(/500/).length > 0;
      expect(hasDraft || hasAmount).toBe(true);
    });

    // A success confirmation (e.g. "Payroll run created") must also be visible.
    await waitFor(() => {
      expect(screen.getAllByText(/payroll run created/i).length).toBeGreaterThan(0);
    });
  });
});
