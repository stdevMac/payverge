/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { PayrollRunWithCount } from "@/api/accounting";
import { asDollars } from "@/types/money";
import { usePayrollRuns, usePayrollRun, useMarkPayrollRunPaid } from "@/hooks/accounting/useAccountingQueries";
import PayrollTab from "./PayrollTab";

jest.mock("@/hooks/accounting/useAccountingQueries", () => ({
  usePayrollRuns: jest.fn(),
  usePayrollRun: jest.fn(() => ({
    data: undefined,
    isLoading: false,
    isFetching: false,
    isSuccess: false,
  })),
  useMarkPayrollRunPaid: jest.fn(() => ({
    mutate: jest.fn(),
    isPending: false,
  })),
  useVoidPayrollRun: jest.fn(() => ({
    mutate: jest.fn(),
    isPending: false,
  })),
  useDeletePayrollRun: jest.fn(() => ({
    mutate: jest.fn(),
    isPending: false,
  })),
  useCreatePayrollRun: jest.fn(() => ({
    mutate: jest.fn(),
    isPending: false,
  })),
}));

jest.mock("./PayrollRunFormDrawer", () => ({
  __esModule: true,
  default: ({ open }: { open: boolean }) =>
    open ? <div data-testid="payroll-form-drawer" /> : null,
}));

jest.mock("@/api/accounting", () => ({
  accountingApi: {
    payrollExportUrl: jest.fn(
      () =>
        "https://api.test/payroll-runs/export.csv?start=2026-01-01&end=2026-01-31",
    ),
  },
}));

const usePayrollRunsMock = usePayrollRuns as jest.Mock;
const usePayrollRunMock = usePayrollRun as jest.Mock;
const useMarkPaidMock = useMarkPayrollRunPaid as jest.Mock;

const t = (key: string) => key;

function run(
  overrides: Partial<PayrollRunWithCount> & { id: number },
): PayrollRunWithCount {
  return {
    business_id: 42,
    period_start: "2026-01-06",
    period_end: "2026-01-12",
    status: "draft",
    currency: "USD",
    gross_total: asDollars(1000),
    bonus_total: asDollars(0),
    deduction_total: asDollars(0),
    net_total: asDollars(1000),
    line_items: [],
    payee_count: 3,
    ...overrides,
  };
}

function mockPage(
  runs: PayrollRunWithCount[],
  overrides: { total?: number; page?: number; page_size?: number } = {},
) {
  usePayrollRunsMock.mockReturnValue({
    data: {
      runs,
      total: overrides.total ?? runs.length,
      page: overrides.page ?? 1,
      page_size: overrides.page_size ?? 20,
      total_pages: Math.max(
        1,
        Math.ceil(
          (overrides.total ?? runs.length) / (overrides.page_size ?? 20),
        ),
      ),
    },
    isLoading: false,
    isFetching: false,
    isSuccess: true,
  });
}

function renderTab(
  props: Partial<React.ComponentProps<typeof PayrollTab>> = {},
) {
  return render(
    <PayrollTab
      businessId="42"
      start="2026-01-01"
      end="2026-01-31"
      locale="en"
      currency="USD"
      canWrite
      t={t}
      {...props}
    />,
  );
}

describe("PayrollTab", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockPage([
      run({ id: 1, status: "draft", payee_count: 3 }),
      run({
        id: 2,
        status: "paid",
        period_start: "2025-12-30",
        period_end: "2026-01-05",
        payee_count: 2,
        net_total: asDollars(2500),
      }),
      run({
        id: 3,
        status: "void",
        period_start: "2025-12-23",
        period_end: "2025-12-29",
        payee_count: 1,
        net_total: asDollars(800),
      }),
    ]);
  });

  it("renders run rows from usePayrollRuns with period, payee count, and status badges", () => {
    renderTab();

    // formatPeriod("2026-01-06","2026-01-12","en") → "Jan 6, 2026 → Jan 12, 2026"
    expect(screen.getByText(/Jan 6, 2026/)).toBeInTheDocument();
    // One payeeCount cell per row (filter chips also share status labels).
    expect(screen.getAllByText("payroll.payeeCount").length).toBeGreaterThanOrEqual(3);
    // Status badges appear in the table (filter chips also use the same keys).
    expect(screen.getAllByText("payroll.statusDraft").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("payroll.statusPaid").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("payroll.statusVoid").length).toBeGreaterThanOrEqual(1);
    expect(usePayrollRunsMock).toHaveBeenCalled();
  });

  it("applies status badge tones: draft→pending, paid→success, void→neutral", () => {
    renderTab();

    // Badge spans use StatusBadge tone classes; filter chips do not.
    const draftBadge = screen
      .getAllByText("payroll.statusDraft")
      .find((el) => el.className.includes("amber"));
    expect(draftBadge).toBeTruthy();
    const paidBadge = screen
      .getAllByText("payroll.statusPaid")
      .find((el) => el.className.includes("emerald"));
    expect(paidBadge).toBeTruthy();
    const voidBadge = screen
      .getAllByText("payroll.statusVoid")
      .find((el) => /warm|ink/.test(el.className));
    expect(voidBadge).toBeTruthy();
  });

  it("seeds status filter from initialStatusFilter deep-link prop", () => {
    renderTab({ initialStatusFilter: "draft" });

    expect(usePayrollRunsMock).toHaveBeenCalledWith(
      "42",
      expect.objectContaining({ status: "draft" }),
    );
    // Draft chip is selected (SegmentedTabs active).
    const draftChip = screen.getByRole("tab", {
      name: "payroll.statusDraft",
    });
    expect(draftChip).toHaveAttribute("aria-selected", "true");
  });

  it("refires usePayrollRuns with status draft when Draft filter chip is selected", async () => {
    renderTab();

    const draftChip = screen.getByRole("tab", {
      name: "payroll.statusDraft",
    });
    fireEvent.click(draftChip);

    await waitFor(() => {
      const lastCall = usePayrollRunsMock.mock.calls.at(-1);
      expect(lastCall?.[1]).toEqual(
        expect.objectContaining({ status: "draft" }),
      );
    });
  });

  it("refires usePayrollRuns with status paid when Paid filter chip is selected", async () => {
    renderTab();

    fireEvent.click(
      screen.getByRole("tab", { name: "payroll.statusPaid" }),
    );

    await waitFor(() => {
      const lastCall = usePayrollRunsMock.mock.calls.at(-1);
      expect(lastCall?.[1]).toEqual(
        expect.objectContaining({ status: "paid" }),
      );
    });
  });

  it("wires pagination onPageChange into the next usePayrollRuns page", async () => {
    mockPage([run({ id: 1 })], { total: 45, page: 1, page_size: 20 });
    renderTab();

    mockPage([run({ id: 21 })], { total: 45, page: 2, page_size: 20 });

    const nextBtn = screen.getByRole("button", { name: "Next page" });
    fireEvent.click(nextBtn);

    await waitFor(() => {
      const lastCall = usePayrollRunsMock.mock.calls.at(-1);
      expect(lastCall?.[1]).toEqual(expect.objectContaining({ page: 2 }));
    });
  });

  it("opens PayrollRunFormDrawer when New run is clicked", async () => {
    renderTab();

    expect(screen.queryByTestId("payroll-form-drawer")).not.toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", { name: "payroll.newRun" }),
    );

    await waitFor(() => {
      expect(screen.getByTestId("payroll-form-drawer")).toBeInTheDocument();
    });
  });

  it("hides New run when canWrite is false", () => {
    renderTab({ canWrite: false });
    expect(
      screen.queryByRole("button", { name: "payroll.newRun" }),
    ).not.toBeInTheDocument();
  });

  it("opens PayrollRunDetailDrawer (usePayrollRun) when a row is clicked", async () => {
    const fullRun = run({
      id: 1,
      status: "draft",
      payee_count: 3,
      line_items: [
        {
          payee_type: "staff",
          payee_name: "Carlos Server",
          gross_amount: asDollars(1000),
          bonus_amount: asDollars(0),
          deduction_amount: asDollars(0),
          net_amount: asDollars(1000),
        },
      ],
    });
    usePayrollRunMock.mockReturnValue({
      data: fullRun,
      isLoading: false,
      isFetching: false,
      isSuccess: true,
    });

    renderTab();

    const periodCell = screen.getByText(/Jan 6, 2026/);
    const row = periodCell.closest("tr");
    expect(row).toBeTruthy();
    fireEvent.click(row!);

    await waitFor(() => {
      expect(usePayrollRunMock).toHaveBeenCalledWith(
        "42",
        1,
        expect.objectContaining({ enabled: true }),
      );
    });
    // Drawer content surfaces the run title + line item from usePayrollRun.
    expect(screen.getByText("payrollDetail.title")).toBeInTheDocument();
    expect(screen.getByText("Carlos Server")).toBeInTheDocument();
  });

  it("hides New payroll run when canWrite is false", () => {
    renderTab({ canWrite: false });
    expect(
      screen.queryByRole("button", { name: /payroll\.newRun/i }),
    ).not.toBeInTheDocument();
  });

  it("shows Export CSV as a link using payrollExportUrl", () => {
    renderTab();
    const link = screen.getByRole("link", { name: /payroll\.exportCsv/i });
    expect(link).toHaveAttribute(
      "href",
      expect.stringContaining("export.csv"),
    );
  });

  it("shows Mark paid / Delete for draft runs and Void for paid runs", async () => {
    renderTab();

    const draftMenu = screen.getByLabelText("payroll-actions-1");
    fireEvent.click(draftMenu);

    expect(
      await screen.findByRole("menuitem", {
        name: "payrollRuns.markPaid",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("menuitem", { name: "payrollRuns.delete" }),
    ).toBeInTheDocument();

    // Close menu by pressing Escape so the next open is clean.
    fireEvent.keyDown(document.body, { key: "Escape" });

    const paidMenu = screen.getByLabelText("payroll-actions-2");
    fireEvent.click(paidMenu);

    expect(
      await screen.findByRole("menuitem", {
        name: "payrollRuns.voidAction",
      }),
    ).toBeInTheDocument();

    // Void runs have no actions menu.
    expect(screen.queryByLabelText("payroll-actions-3")).not.toBeInTheDocument();
  });

  it("confirms Mark paid then calls useMarkPayrollRunPaid", async () => {
    const mutate = jest.fn();
    useMarkPaidMock.mockReturnValue({ mutate, isPending: false });
    renderTab();

    fireEvent.click(screen.getByLabelText("payroll-actions-1"));
    fireEvent.click(
      await screen.findByRole("menuitem", {
        name: "payrollRuns.markPaid",
      }),
    );

    // Confirmation modal should appear with monolith confirm copy keys.
    expect(
      await screen.findByText("payrollRuns.confirmMarkPaidTitle"),
    ).toBeInTheDocument();

    // Confirm button uses the mark-paid label.
    const dialog = screen.getByRole("dialog");
    const confirmBtn = within(dialog).getByRole("button", {
      name: "payrollRuns.markPaid",
    });
    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(mutate).toHaveBeenCalledWith(1);
    });
  });
});
