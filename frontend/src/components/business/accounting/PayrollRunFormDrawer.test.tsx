/** @jest-environment jsdom */
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
import type { StaffMember } from "@/api/staff";
import { getBusinessStaff } from "@/api/staff";
import {
  useCreatePayrollRun,
  useMarkPayrollRunPaid,
} from "@/hooks/accounting/useAccountingQueries";
import { asDollars } from "@/types/money";
import { queryKeys } from "@/api/queryKeys";
import PayrollRunFormDrawer, {
  defaultPayrollPeriod,
} from "./PayrollRunFormDrawer";

const mockCreateMutate = jest.fn();
const mockMarkPaidMutate = jest.fn();

jest.mock("@/api/staff", () => ({
  getBusinessStaff: jest.fn(),
}));

jest.mock("@/hooks/accounting/useAccountingQueries", () => ({
  useCreatePayrollRun: jest.fn(() => ({
    mutate: mockCreateMutate,
    isPending: false,
  })),
  useMarkPayrollRunPaid: jest.fn(() => ({
    mutate: mockMarkPaidMutate,
    isPending: false,
  })),
}));

const detailDrawerProps: Array<Record<string, unknown>> = [];

jest.mock("../shared/DetailDrawer", () => ({
  __esModule: true,
  default: (props: {
    open: boolean;
    title: string;
    footer?: React.ReactNode;
    children: React.ReactNode;
    [key: string]: unknown;
  }) => {
    detailDrawerProps.push(props);
    const { open, title, footer, children } = props;
    return open ? (
      <div data-testid="payroll-form-drawer">
        <h2>{title}</h2>
        <div data-testid="payroll-form-body">{children}</div>
        {footer ? (
          <div data-testid="payroll-form-footer">{footer}</div>
        ) : null}
      </div>
    ) : null;
  },
}));

jest.mock("@/components/ui/fields/StaffSelect", () => ({
  StaffSelect: ({
    label,
    value,
    onChange,
    staff,
  }: {
    label: string;
    value: number | null;
    onChange: (id: number | null) => void;
    staff: StaffMember[];
  }) => (
    <select
      aria-label={label}
      value={value != null ? String(value) : ""}
      onChange={(e) =>
        onChange(e.target.value ? Number(e.target.value) : null)
      }
    >
      <option value="">—</option>
      {staff.map((s) => (
        <option key={s.id} value={s.id}>
          {s.name}
        </option>
      ))}
    </select>
  ),
}));

const getBusinessStaffMock = getBusinessStaff as jest.Mock;
const useCreateMock = useCreatePayrollRun as jest.Mock;
const useMarkPaidMock = useMarkPayrollRunPaid as jest.Mock;

const t = (key: string) => key;

const STAFF: StaffMember[] = [
  {
    id: 7,
    name: "Dana Ruiz",
    email: "dana@x.com",
    role: "server",
    business_id: 42,
    is_active: true,
    created_at: "",
    updated_at: "",
  },
  {
    id: 8,
    name: "Sam Lee",
    email: "sam@x.com",
    role: "host",
    business_id: 42,
    is_active: true,
    created_at: "",
    updated_at: "",
  },
];

function getFieldInput(label: string): HTMLInputElement | HTMLTextAreaElement {
  const nodes = screen.getAllByLabelText(label);
  const control = nodes.find(
    (n) =>
      n instanceof HTMLInputElement || n instanceof HTMLTextAreaElement,
  );
  if (!control) {
    throw new Error(`input/textarea for ${label} not found`);
  }
  return control;
}

function renderDrawer(
  props: Partial<React.ComponentProps<typeof PayrollRunFormDrawer>> = {},
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const defaults: React.ComponentProps<typeof PayrollRunFormDrawer> = {
    open: true,
    onClose: jest.fn(),
    businessId: "42",
    currency: "USD",
    locale: "en",
    lastPeriodEnd: null,
    t,
  };
  const view = render(
    <QueryClientProvider client={client}>
      <PayrollRunFormDrawer {...defaults} {...props} />
    </QueryClientProvider>,
  );
  return { ...view, client };
}

async function goToStep2() {
  fireEvent.click(
    within(screen.getByTestId("payroll-form-footer")).getByRole("button", {
      name: "payrollForm.next",
    }),
  );
  await screen.findByText("payrollForm.prefillHint");
}

async function fillFirstLineGross(amount: string) {
  const grossInputs = screen.getAllByLabelText("payrollForm.gross");
  fireEvent.change(grossInputs[0], { target: { value: amount } });
}

describe("defaultPayrollPeriod", () => {
  it("defaults to current week (Mon–Sun) when no last run exists", () => {
    // Wednesday 2026-03-11
    const now = new Date(2026, 2, 11);
    expect(defaultPayrollPeriod(null, now)).toEqual({
      start: "2026-03-09",
      end: "2026-03-15",
    });
  });

  it("defaults to day after last run end through +6 days when a last run exists", () => {
    expect(defaultPayrollPeriod("2026-01-12")).toEqual({
      start: "2026-01-13",
      end: "2026-01-19",
    });
  });
});

describe("PayrollRunFormDrawer", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    detailDrawerProps.length = 0;
    getBusinessStaffMock.mockResolvedValue({
      staff: STAFF,
      pending_invitations: [],
    });
    useCreateMock.mockReturnValue({
      mutate: mockCreateMutate,
      isPending: false,
    });
    useMarkPaidMock.mockReturnValue({
      mutate: mockMarkPaidMutate,
      isPending: false,
    });
  });

  // R2-3b: the drawer sets `dirty` but used to pass no `discardConfirm`, so
  // DetailDrawer fell back to hardcoded English copy for every locale.
  it("passes localized discard copy to DetailDrawer", () => {
    renderDrawer({ lastPeriodEnd: "2026-01-12" });
    const props = detailDrawerProps[detailDrawerProps.length - 1];
    expect(props.discardConfirm).toEqual({
      title: "discardConfirm.payroll.title",
      description: "discardConfirm.payroll.description",
      confirmLabel: "discardConfirm.confirm",
      cancelLabel: "discardConfirm.cancel",
    });
  });

  it("renders title and period step with defaults from lastPeriodEnd", () => {
    renderDrawer({ lastPeriodEnd: "2026-01-12" });

    expect(screen.getByText("payrollForm.title")).toBeInTheDocument();
    expect(getFieldInput("payrollForm.periodStart")).toHaveValue("2026-01-13");
    expect(getFieldInput("payrollForm.periodEnd")).toHaveValue("2026-01-19");
    expect(screen.getByText(/payrollForm\.stepPeriod/)).toBeInTheDocument();
  });

  it("blocks Next when period end is before start", () => {
    renderDrawer({ lastPeriodEnd: "2026-01-12" });

    fireEvent.change(getFieldInput("payrollForm.periodStart"), {
      target: { value: "2026-02-10" },
    });
    fireEvent.change(getFieldInput("payrollForm.periodEnd"), {
      target: { value: "2026-02-01" },
    });

    fireEvent.click(
      within(screen.getByTestId("payroll-form-footer")).getByRole("button", {
        name: "payrollForm.next",
      }),
    );

    expect(screen.getByText("payrollForm.errors.periodInvalid")).toBeInTheDocument();
    expect(screen.queryByText("payrollForm.prefillHint")).not.toBeInTheDocument();
  });

  it("opens step 2 with one prefilled line per active staff member", async () => {
    renderDrawer();

    await goToStep2();

    await waitFor(() => {
      expect(getBusinessStaffMock).toHaveBeenCalledWith("42");
    });

    await waitFor(() => {
      expect(screen.getByDisplayValue("Dana Ruiz")).toBeInTheDocument();
      expect(screen.getByDisplayValue("Sam Lee")).toBeInTheDocument();
    });

    // Two gross inputs (one per staff line)
    expect(screen.getAllByLabelText("payrollForm.gross")).toHaveLength(2);
  });

  it("refreshes untouched defaults when a same-size roster changes", async () => {
    const { client } = renderDrawer();
    await goToStep2();
    await screen.findByDisplayValue("Dana Ruiz");

    act(() => {
      client.setQueryData(queryKeys.staff.list("42"), {
        staff: [
          { ...STAFF[0], name: "Dana Updated" },
          { ...STAFF[1], name: "Sam Updated" },
        ],
        pending_invitations: [],
      });
    });

    expect(await screen.findByDisplayValue("Dana Updated")).toBeInTheDocument();
    expect(screen.getByDisplayValue("Sam Updated")).toBeInTheDocument();
    expect(screen.queryByDisplayValue("Dana Ruiz")).toBeNull();
  });

  it("removes a line and adds a contractor line", async () => {
    renderDrawer();
    await goToStep2();

    await waitFor(() => {
      expect(screen.getAllByLabelText("payrollForm.gross")).toHaveLength(2);
    });

    fireEvent.click(screen.getAllByRole("button", { name: "payrollForm.removeLine" })[0]);

    await waitFor(() => {
      expect(screen.getAllByLabelText("payrollForm.gross")).toHaveLength(1);
    });

    fireEvent.click(
      screen.getByRole("button", { name: "payrollForm.addContractorLine" }),
    );

    await waitFor(() => {
      expect(screen.getByLabelText("payrollForm.contractorName")).toBeInTheDocument();
      expect(screen.getAllByLabelText("payrollForm.gross")).toHaveLength(2);
    });
  });

  it("renders live per-line net = gross + bonus − deduction", async () => {
    renderDrawer();
    await goToStep2();

    await waitFor(() => {
      expect(screen.getAllByLabelText("payrollForm.gross")).toHaveLength(2);
    });

    const gross = screen.getAllByLabelText("payrollForm.gross")[0];
    const bonus = screen.getAllByLabelText("payrollForm.bonus")[0];
    const deduction = screen.getAllByLabelText("payrollForm.deduction")[0];

    fireEvent.change(gross, { target: { value: "100" } });
    fireEvent.change(bonus, { target: { value: "20" } });
    fireEvent.change(deduction, { target: { value: "15" } });

    // Net cell shows 105 (not currency-formatted necessarily — raw live number is fine)
    expect(screen.getByTestId("payroll-line-net-0")).toHaveTextContent("105");
  });

  it("Create draft posts only lines with gross > 0", async () => {
    renderDrawer({ lastPeriodEnd: "2026-01-12" });
    await goToStep2();

    await waitFor(() => {
      expect(screen.getAllByLabelText("payrollForm.gross")).toHaveLength(2);
    });

    // Only first staff line has gross
    await fillFirstLineGross("500");

    fireEvent.click(
      within(screen.getByTestId("payroll-form-footer")).getByRole("button", {
        name: "payrollForm.next",
      }),
    );

    await screen.findByText("payrollForm.totals");

    fireEvent.click(
      within(screen.getByTestId("payroll-form-footer")).getByRole("button", {
        name: "payrollForm.createDraft",
      }),
    );

    await waitFor(() => {
      expect(mockCreateMutate).toHaveBeenCalledWith(
        expect.objectContaining({
          period_start: "2026-01-13",
          period_end: "2026-01-19",
          line_items: [
            expect.objectContaining({
              payee_type: "staff",
              staff_id: 7,
              gross_amount: asDollars(500),
            }),
          ],
        }),
        expect.any(Object),
      );
    });
    expect(mockMarkPaidMutate).not.toHaveBeenCalled();
  });

  it("Create & mark paid chains useMarkPayrollRunPaid after create", async () => {
    renderDrawer({ lastPeriodEnd: "2026-01-12" });
    await goToStep2();

    await waitFor(() => {
      expect(screen.getAllByLabelText("payrollForm.gross")).toHaveLength(2);
    });
    await fillFirstLineGross("250");

    fireEvent.click(
      within(screen.getByTestId("payroll-form-footer")).getByRole("button", {
        name: "payrollForm.next",
      }),
    );
    await screen.findByText("payrollForm.totals");

    mockCreateMutate.mockImplementation((_payload, opts) => {
      opts?.onSuccess?.({
        id: 99,
        period_start: "2026-01-13",
        period_end: "2026-01-19",
        status: "draft",
        gross_total: asDollars(250),
        bonus_total: asDollars(0),
        deduction_total: asDollars(0),
        net_total: asDollars(250),
        line_items: [],
      });
    });

    fireEvent.click(
      within(screen.getByTestId("payroll-form-footer")).getByRole("button", {
        name: "payrollForm.createAndMarkPaid",
      }),
    );

    await waitFor(() => {
      expect(mockCreateMutate).toHaveBeenCalled();
      expect(mockMarkPaidMutate).toHaveBeenCalledWith(99, expect.any(Object));
    });
  });

  it("blocks Next from lines when no line has gross > 0", async () => {
    renderDrawer();
    await goToStep2();

    await waitFor(() => {
      expect(screen.getAllByLabelText("payrollForm.gross")).toHaveLength(2);
    });

    fireEvent.click(
      within(screen.getByTestId("payroll-form-footer")).getByRole("button", {
        name: "payrollForm.next",
      }),
    );

    expect(screen.getByText("payrollForm.errors.linesRequired")).toBeInTheDocument();
    expect(screen.queryByText("payrollForm.totals")).not.toBeInTheDocument();
  });

  it("surfaces mutation errors inline with role=alert", async () => {
    renderDrawer({ lastPeriodEnd: "2026-01-12" });
    await goToStep2();
    await waitFor(() => {
      expect(screen.getAllByLabelText("payrollForm.gross")).toHaveLength(2);
    });
    await fillFirstLineGross("100");

    fireEvent.click(
      within(screen.getByTestId("payroll-form-footer")).getByRole("button", {
        name: "payrollForm.next",
      }),
    );
    await screen.findByText("payrollForm.totals");

    mockCreateMutate.mockImplementation((_payload, opts) => {
      opts?.onError?.(new Error("server says no"));
    });

    fireEvent.click(
      within(screen.getByTestId("payroll-form-footer")).getByRole("button", {
        name: "payrollForm.createDraft",
      }),
    );

    await waitFor(() => {
      const alert = screen.getByRole("alert");
      expect(alert).toHaveTextContent("server says no");
    });
  });

  it("does not render when closed", () => {
    renderDrawer({ open: false });
    expect(screen.queryByTestId("payroll-form-drawer")).not.toBeInTheDocument();
  });

  // L6-17: review step must show localized dates, not raw ISO YYYY-MM-DD.
  it("formats period dates on the review step via formatDay", async () => {
    renderDrawer({ lastPeriodEnd: "2026-01-12" });
    await goToStep2();
    await waitFor(() => {
      expect(screen.getAllByLabelText("payrollForm.gross")).toHaveLength(2);
    });
    await fillFirstLineGross("100");

    fireEvent.click(
      within(screen.getByTestId("payroll-form-footer")).getByRole("button", {
        name: "payrollForm.next",
      }),
    );

    await screen.findByText("payrollForm.totals");
    const review = screen.getByTestId("payroll-form-step-review");
    expect(review.textContent).toMatch(/Jan 13, 2026/);
    expect(review.textContent).toMatch(/Jan 19, 2026/);
    expect(review.textContent).not.toMatch(/2026-01-13/);
    expect(review.textContent).not.toMatch(/2026-01-19/);
  });

});
