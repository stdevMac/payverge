/** @jest-environment jsdom */
import React from "react";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import type { PayrollRun } from "@/api/accounting";
import { asDollars } from "@/types/money";
import {
  usePayrollRun,
  useMarkPayrollRunPaid,
  useVoidPayrollRun,
  useDeletePayrollRun,
} from "@/hooks/accounting/useAccountingQueries";
import PayrollRunDetailDrawer from "./PayrollRunDetailDrawer";

const mockMarkPaid = jest.fn();
const mockVoid = jest.fn();
const mockDelete = jest.fn();

jest.mock("@/hooks/accounting/useAccountingQueries", () => ({
  usePayrollRun: jest.fn(),
  useMarkPayrollRunPaid: jest.fn(() => ({
    mutate: mockMarkPaid,
    isPending: false,
  })),
  useVoidPayrollRun: jest.fn(() => ({
    mutate: mockVoid,
    isPending: false,
  })),
  useDeletePayrollRun: jest.fn(() => ({
    mutate: mockDelete,
    isPending: false,
  })),
}));

jest.mock("../shared/DetailDrawer", () => ({
  __esModule: true,
  default: ({
    open,
    title,
    subtitle,
    footer,
    children,
  }: {
    open: boolean;
    title: string;
    subtitle?: string;
    footer?: React.ReactNode;
    children: React.ReactNode;
  }) =>
    open ? (
      <div data-testid="payroll-detail-drawer">
        <h2>{title}</h2>
        {subtitle ? (
          <p data-testid="payroll-detail-subtitle">{subtitle}</p>
        ) : null}
        <div data-testid="payroll-detail-body">{children}</div>
        {footer ? (
          <div data-testid="payroll-detail-footer">{footer}</div>
        ) : null}
      </div>
    ) : null,
}));

jest.mock("../modals/ConfirmationModal", () => ({
  __esModule: true,
  default: ({
    isOpen,
    title,
    description,
    confirmLabel,
    onConfirm,
  }: {
    isOpen: boolean;
    title: string;
    description: string;
    confirmLabel?: string;
    onConfirm: () => void;
  }) =>
    isOpen ? (
      <div data-testid="payroll-confirm-modal" role="dialog">
        <h3>{title}</h3>
        <p>{description}</p>
        <button type="button" onClick={onConfirm}>
          {confirmLabel ?? "Confirm"}
        </button>
      </div>
    ) : null,
}));

jest.mock("../premium", () => ({
  AnimatedNumberText: ({
    value,
    format,
    className,
  }: {
    value: number;
    format?: (v: number) => string;
    className?: string;
  }) => (
    <span data-testid="animated-net-total" className={className}>
      {format ? format(value) : String(value)}
    </span>
  ),
}));

const usePayrollRunMock = usePayrollRun as jest.Mock;
const useMarkPaidMock = useMarkPayrollRunPaid as jest.Mock;
const useVoidMock = useVoidPayrollRun as jest.Mock;
const useDeleteMock = useDeletePayrollRun as jest.Mock;

const t = (key: string) => key;
const tWith = (key: string, replacements: Record<string, string | number>) => {
  let value = key;
  Object.entries(replacements).forEach(([name, replacement]) => {
    value = value.replace(
      new RegExp(`\\{${name}\\}`, "g"),
      String(replacement),
    );
  });
  if (value === key) {
    return `${key}:${JSON.stringify(replacements)}`;
  }
  return value;
};

function payrollRun(
  overrides: Partial<PayrollRun> & { id: number } = { id: 1 },
): PayrollRun {
  return {
    business_id: 42,
    period_start: "2026-01-06",
    period_end: "2026-01-12",
    status: "draft",
    currency: "USD",
    gross_total: asDollars(1500),
    bonus_total: asDollars(100),
    deduction_total: asDollars(50),
    net_total: asDollars(1550),
    notes: "Week 2 payroll",
    created_at: "2026-01-13T10:00:00Z",
    created_by_staff: { id: 7, name: "Ana Manager" },
    line_items: [
      {
        id: 10,
        payee_type: "staff",
        staff_id: 1,
        payee_name: "Carlos Server",
        gross_amount: asDollars(1000),
        bonus_amount: asDollars(50),
        deduction_amount: asDollars(25),
        net_amount: asDollars(1025),
      },
      {
        id: 11,
        payee_type: "contractor",
        payee_name: "DJ Night",
        gross_amount: asDollars(500),
        bonus_amount: asDollars(50),
        deduction_amount: asDollars(25),
        net_amount: asDollars(525),
      },
    ],
    ...overrides,
  };
}

function mockRunQuery(
  run: PayrollRun | undefined,
  options: { isLoading?: boolean; isError?: boolean } = {},
) {
  usePayrollRunMock.mockReturnValue({
    data: run,
    isLoading: options.isLoading ?? false,
    isError: options.isError ?? false,
    isSuccess: !!run && !options.isError,
    isFetching: options.isLoading ?? false,
  });
}

function renderDrawer(
  props: Partial<React.ComponentProps<typeof PayrollRunDetailDrawer>> = {},
) {
  const defaults: React.ComponentProps<typeof PayrollRunDetailDrawer> = {
    open: true,
    runId: 1,
    businessId: "42",
    locale: "en",
    currency: "USD",
    canWrite: true,
    onClose: jest.fn(),
    t,
    tWith,
  };
  return render(<PayrollRunDetailDrawer {...defaults} {...props} />);
}

describe("PayrollRunDetailDrawer", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockRunQuery(payrollRun());
    useMarkPaidMock.mockReturnValue({
      mutate: mockMarkPaid,
      isPending: false,
    });
    useVoidMock.mockReturnValue({ mutate: mockVoid, isPending: false });
    useDeleteMock.mockReturnValue({ mutate: mockDelete, isPending: false });
  });

  it("fetches usePayrollRun with runId when open", () => {
    renderDrawer({ runId: 9 });

    expect(usePayrollRunMock).toHaveBeenCalledWith(
      "42",
      9,
      expect.objectContaining({ enabled: true }),
    );
  });

  it("disables the query when closed or runId is null", () => {
    renderDrawer({ open: false, runId: 1 });
    expect(usePayrollRunMock).toHaveBeenCalledWith(
      "42",
      1,
      expect.objectContaining({ enabled: false }),
    );

    jest.clearAllMocks();
    mockRunQuery(undefined);
    renderDrawer({ open: true, runId: null });
    expect(usePayrollRunMock).toHaveBeenCalledWith(
      "42",
      0,
      expect.objectContaining({ enabled: false }),
    );
  });

  it("renders period title, status badge, net total hero, and line items", () => {
    renderDrawer();

    expect(screen.getByText("payrollDetail.title")).toBeInTheDocument();
    // formatPeriod("2026-01-06","2026-01-12","en")
    expect(screen.getByText(/Jan 6, 2026/)).toBeInTheDocument();
    expect(screen.getByTestId("animated-net-total")).toHaveTextContent(
      /\$1,550\.00/,
    );

    // Line item payee names
    expect(screen.getByText("Carlos Server")).toBeInTheDocument();
    expect(screen.getByText("DJ Night")).toBeInTheDocument();

    // Column headers
    expect(screen.getByText("payrollDetail.lines.payee")).toBeInTheDocument();
    expect(screen.getByText("payrollDetail.lines.gross")).toBeInTheDocument();
    expect(screen.getByText("payrollDetail.lines.bonus")).toBeInTheDocument();
    expect(
      screen.getByText("payrollDetail.lines.deduction"),
    ).toBeInTheDocument();
    expect(screen.getByText("payrollDetail.lines.net")).toBeInTheDocument();

    // Totals row label
    expect(screen.getByText("payrollDetail.totals")).toBeInTheDocument();
  });

  it("renders notes and audit trail", () => {
    renderDrawer();

    expect(screen.getByText("payrollDetail.notes")).toBeInTheDocument();
    expect(screen.getByText("Week 2 payroll")).toBeInTheDocument();
    expect(
      screen.getByText(/payrollDetail\.audit\.createdBy/),
    ).toBeInTheDocument();
    expect(screen.getByText(/Ana Manager/)).toBeInTheDocument();
  });

  it("shows paid/voided audit when present", () => {
    mockRunQuery(
      payrollRun({
        id: 2,
        status: "void",
        paid_at: "2026-01-14T12:00:00Z",
        paid_by_staff: { id: 8, name: "Owner Kim" },
        voided_at: "2026-01-20T12:00:00Z",
        voided_by_staff: { id: 9, name: "Boss Pat" },
      }),
    );
    renderDrawer({ runId: 2 });

    expect(
      screen.getByText(/payrollDetail\.audit\.paidBy/),
    ).toBeInTheDocument();
    expect(screen.getByText(/Owner Kim/)).toBeInTheDocument();
    expect(
      screen.getByText(/payrollDetail\.audit\.voidedBy/),
    ).toBeInTheDocument();
    expect(screen.getByText(/Boss Pat/)).toBeInTheDocument();
  });

  it("shows Mark paid + Delete for draft when canWrite", () => {
    renderDrawer();

    expect(
      screen.getByRole("button", { name: "payrollDetail.actions.markPaid" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "payrollDetail.actions.delete" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "payrollDetail.actions.void" }),
    ).not.toBeInTheDocument();
  });

  it("shows Void for paid when canWrite", () => {
    mockRunQuery(payrollRun({ id: 3, status: "paid" }));
    renderDrawer({ runId: 3 });

    expect(
      screen.getByRole("button", { name: "payrollDetail.actions.void" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", {
        name: "payrollDetail.actions.markPaid",
      }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "payrollDetail.actions.delete" }),
    ).not.toBeInTheDocument();
  });

  it("hides all footer actions for void status", () => {
    mockRunQuery(payrollRun({ id: 4, status: "void" }));
    renderDrawer({ runId: 4 });

    expect(
      screen.queryByRole("button", {
        name: "payrollDetail.actions.markPaid",
      }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "payrollDetail.actions.delete" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "payrollDetail.actions.void" }),
    ).not.toBeInTheDocument();
  });

  it("hides footer actions when canWrite is false", () => {
    renderDrawer({ canWrite: false });

    expect(
      screen.queryByRole("button", {
        name: "payrollDetail.actions.markPaid",
      }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "payrollDetail.actions.delete" }),
    ).not.toBeInTheDocument();
  });

  it("confirms Mark paid then mutates and closes on success", () => {
    const onClose = jest.fn();
    renderDrawer({ onClose });

    fireEvent.click(
      screen.getByRole("button", { name: "payrollDetail.actions.markPaid" }),
    );
    const dialog = screen.getByTestId("payroll-confirm-modal");
    expect(dialog).toBeInTheDocument();
    expect(
      within(dialog).getByText("payrollDetail.confirmMarkPaidTitle"),
    ).toBeInTheDocument();

    fireEvent.click(
      within(dialog).getByRole("button", {
        name: "payrollDetail.actions.markPaid",
      }),
    );

    expect(mockMarkPaid).toHaveBeenCalledWith(
      1,
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );

    const opts = mockMarkPaid.mock.calls[0][1] as { onSuccess: () => void };
    opts.onSuccess();
    expect(onClose).toHaveBeenCalled();
  });

  it("confirms Void then mutates and closes on success", () => {
    mockRunQuery(payrollRun({ id: 5, status: "paid" }));
    const onClose = jest.fn();
    renderDrawer({ runId: 5, onClose });

    fireEvent.click(
      screen.getByRole("button", { name: "payrollDetail.actions.void" }),
    );
    const dialog = screen.getByTestId("payroll-confirm-modal");
    expect(
      within(dialog).getByText("payrollDetail.confirmVoidTitle"),
    ).toBeInTheDocument();

    fireEvent.click(
      within(dialog).getByRole("button", {
        name: "payrollDetail.actions.void",
      }),
    );

    expect(mockVoid).toHaveBeenCalledWith(
      5,
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
    const opts = mockVoid.mock.calls[0][1] as { onSuccess: () => void };
    opts.onSuccess();
    expect(onClose).toHaveBeenCalled();
  });

  it("confirms Delete then mutates and closes on success", () => {
    const onClose = jest.fn();
    renderDrawer({ onClose });

    fireEvent.click(
      screen.getByRole("button", { name: "payrollDetail.actions.delete" }),
    );
    const dialog = screen.getByTestId("payroll-confirm-modal");
    expect(
      within(dialog).getByText("payrollDetail.confirmDeleteTitle"),
    ).toBeInTheDocument();

    fireEvent.click(
      within(dialog).getByRole("button", {
        name: "payrollDetail.actions.delete",
      }),
    );

    expect(mockDelete).toHaveBeenCalledWith(
      1,
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
    const opts = mockDelete.mock.calls[0][1] as { onSuccess: () => void };
    opts.onSuccess();
    expect(onClose).toHaveBeenCalled();
  });

  it("keeps the drawer open and restores Delete confirmation after failure so it can be retried", () => {
    const onClose = jest.fn();
    renderDrawer({ onClose });

    fireEvent.click(
      screen.getByRole("button", { name: "payrollDetail.actions.delete" }),
    );
    fireEvent.click(
      within(screen.getByTestId("payroll-confirm-modal")).getByRole("button", {
        name: "payrollDetail.actions.delete",
      }),
    );

    const opts = mockDelete.mock.calls[0][1] as {
      onError?: (error: Error) => void;
    };
    expect(opts.onError).toEqual(expect.any(Function));
    act(() => opts.onError?.(new Error("delete failed")));

    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByTestId("payroll-confirm-modal")).toBeInTheDocument();
    expect(
      within(screen.getByTestId("payroll-confirm-modal")).getByRole("button", {
        name: "payrollDetail.actions.delete",
      }),
    ).toBeEnabled();
  });

  it("shows loading state while fetching", () => {
    mockRunQuery(undefined, { isLoading: true });
    renderDrawer();

    expect(screen.getByTestId("payroll-detail-loading")).toBeInTheDocument();
  });
});
