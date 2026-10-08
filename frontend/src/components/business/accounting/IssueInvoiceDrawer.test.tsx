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
import type { IssuableBill } from "@/api/fiscal";
import { resolveReceiptType } from "@/api/fiscal";
import {
  useIssuableBills,
  useIssueInvoice,
} from "@/hooks/accounting/useAccountingQueries";
import IssueInvoiceDrawer from "./IssueInvoiceDrawer";

const mockIssueMutate = jest.fn();
let mockIssuableData: IssuableBill[] = [];
let lastIssuableQ: string | undefined;

jest.mock("@/api/fiscal", () => {
  const actual = jest.requireActual("@/api/fiscal");
  return {
    ...actual,
    resolveReceiptType: jest.fn().mockResolvedValue({
      receipt_type: "factura_b",
      letter: "B",
    }),
  };
});

const resolveReceiptTypeMock = resolveReceiptType as jest.MockedFunction<
  typeof resolveReceiptType
>;

jest.mock("@/hooks/accounting/useAccountingQueries", () => ({
  useIssuableBills: jest.fn((_biz: unknown, q: string = "") => {
    lastIssuableQ = q;
    return {
      data: mockIssuableData,
      isLoading: false,
      isFetching: false,
      isSuccess: true,
    };
  }),
  useIssueInvoice: jest.fn(() => ({
    mutate: mockIssueMutate,
    isPending: false,
  })),
}));

const detailDrawerProps: Array<Record<string, unknown>> = [];

jest.mock("../shared/DetailDrawer", () => ({
  __esModule: true,
  default: (props: {
    open: boolean;
    title: string;
    children: React.ReactNode;
    [key: string]: unknown;
  }) => {
    detailDrawerProps.push(props);
    const { open, title, children } = props;
    return open ? (
      <div data-testid="issue-invoice-drawer">
        <h2>{title}</h2>
        <div data-testid="issue-invoice-body">{children}</div>
      </div>
    ) : null;
  },
}));

const useIssuableBillsMock = useIssuableBills as jest.Mock;
const useIssueInvoiceMock = useIssueInvoice as jest.Mock;

const t = (key: string, params?: Record<string, string | number>) => {
  const templates: Record<string, string> = {
    "issueInvoice.letterWillIssue": "Will issue FACTURA {letter}",
    "issueInvoice.typeWillIssue": "Will issue {type}",
    "invoices.receiptType.invoice": "Invoice",
    "invoices.receiptType.receipt": "Receipt",
    "invoices.receiptType.factura_b": "Factura B",
  };
  const template = templates[key] ?? key;
  if (!params) return template;
  return Object.entries(params).reduce(
    (acc, [name, value]) =>
      acc.replace(new RegExp(`\\{${name}\\}`, "g"), String(value)),
    template,
  );
};

function bill(
  overrides: Partial<IssuableBill> & { bill_id: number },
): IssuableBill {
  return {
    bill_number: `B-${overrides.bill_id}`,
    table_label: `Table ${overrides.bill_id}`,
    closed_at: "2026-01-15T18:00:00Z",
    total_amount: 42.5 as IssuableBill["total_amount"],
    currency: "ARS",
    ...overrides,
  };
}

function renderDrawer(
  props: Partial<React.ComponentProps<typeof IssueInvoiceDrawer>> = {},
) {
  const defaults: React.ComponentProps<typeof IssueInvoiceDrawer> = {
    open: true,
    onClose: jest.fn(),
    businessId: "42",
    locale: "en",
    currency: "ARS",
    businessTimezone: "America/Argentina/Buenos_Aires",
    t,
  };
  return render(<IssueInvoiceDrawer {...defaults} {...props} />);
}

describe("IssueInvoiceDrawer", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.useFakeTimers();
    detailDrawerProps.length = 0;
    lastIssuableQ = undefined;
    resolveReceiptTypeMock.mockResolvedValue({
      receipt_type: "factura_b",
      letter: "B",
      country: "AR",
    });
    mockIssuableData = [
      bill({ bill_id: 101, bill_number: "B-101", table_label: "Patio 1" }),
      bill({ bill_id: 102, bill_number: "B-102", table_label: "Bar 2" }),
    ];
    useIssuableBillsMock.mockImplementation((_biz: unknown, q: string = "") => {
      lastIssuableQ = q;
      return {
        data: mockIssuableData,
        isLoading: false,
        isFetching: false,
        isSuccess: true,
      };
    });
    useIssueInvoiceMock.mockReturnValue({
      mutate: mockIssueMutate,
      isPending: false,
    });
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  // R2-3b: the drawer sets `dirty` but used to pass no `discardConfirm`, so
  // DetailDrawer fell back to hardcoded English copy for every locale.
  it("passes localized discard copy to DetailDrawer", () => {
    renderDrawer();
    const props = detailDrawerProps[detailDrawerProps.length - 1];
    expect(props.discardConfirm).toEqual({
      title: "discardConfirm.issueInvoice.title",
      description: "discardConfirm.issueInvoice.description",
      confirmLabel: "discardConfirm.confirm",
      cancelLabel: "discardConfirm.cancel",
    });
  });

  it("opens with useIssuableBills(\"\") results (recent paid un-invoiced bills)", () => {
    renderDrawer();

    expect(screen.getByTestId("issue-invoice-drawer")).toBeInTheDocument();
    expect(screen.getByText("issueInvoice.title")).toBeInTheDocument();
    expect(useIssuableBillsMock).toHaveBeenCalledWith("42", "", {
      enabled: true,
    });
    expect(screen.getByText("B-101")).toBeInTheDocument();
    expect(screen.getByText("Patio 1")).toBeInTheDocument();
    expect(screen.getByText("B-102")).toBeInTheDocument();
    expect(screen.getByText("Bar 2")).toBeInTheDocument();
  });

  it("debounces search and filters via useIssuableBills(q)", async () => {
    renderDrawer();

    const search = screen.getByPlaceholderText(
      "issueInvoice.searchPlaceholder",
    );
    fireEvent.change(search, { target: { value: "Patio" } });

    // Immediate keystroke should not yet query with "Patio".
    expect(lastIssuableQ).toBe("");

    act(() => {
      jest.advanceTimersByTime(350);
    });

    await waitFor(() => {
      expect(useIssuableBillsMock).toHaveBeenCalledWith("42", "Patio", {
        enabled: true,
      });
    });
  });

  it("issues a bill via the mutation after receiver confirm, then success banner and drops the row", async () => {
    mockIssueMutate.mockImplementation(
      (
        _args: { billId: number },
        opts?: { onSuccess?: () => void },
      ) => {
        opts?.onSuccess?.();
      },
    );

    renderDrawer();

    const row = screen.getByTestId("issuable-bill-101");
    const issueBtn = within(row).getByRole("button", {
      name: /issueInvoice\.issue/i,
    });
    fireEvent.click(issueBtn);

    // Step 2: receiver form
    expect(screen.getByTestId("issue-receiver-form")).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("issue-confirm"));

    expect(mockIssueMutate).toHaveBeenCalledWith(
      expect.objectContaining({ billId: 101 }),
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );

    await waitFor(() => {
      expect(screen.getByRole("status")).toHaveTextContent(
        "issueInvoice.success",
      );
    });
    expect(screen.queryByTestId("issuable-bill-101")).not.toBeInTheDocument();
    expect(screen.getByTestId("issuable-bill-102")).toBeInTheDocument();
  });

  // R2-5: `dirty` counts the receiver fields, but the success handler only
  // cleared selectedBill / manualMode / manualBillId — so closing the drawer
  // right after a successful issuance raised a false "unsaved edits" confirm.
  it("is no longer dirty after a successful issuance", async () => {
    mockIssueMutate.mockImplementation(
      (
        _args: { billId: number },
        opts?: { onSuccess?: () => void },
      ) => {
        opts?.onSuccess?.();
      },
    );

    renderDrawer();

    const row = screen.getByTestId("issuable-bill-101");
    fireEvent.click(
      within(row).getByRole("button", { name: /issueInvoice\.issue/i }),
    );

    const nameInput = screen
      .getAllByLabelText("issueInvoice.receiver.nameLabel")
      .find((n) => n instanceof HTMLInputElement) as HTMLInputElement;
    fireEvent.change(nameInput, { target: { value: "Acme SRL" } });

    expect(detailDrawerProps[detailDrawerProps.length - 1].dirty).toBe(true);

    fireEvent.click(screen.getByTestId("issue-confirm"));

    await waitFor(() => {
      expect(screen.getByRole("status")).toHaveTextContent(
        "issueInvoice.success",
      );
    });

    expect(detailDrawerProps[detailDrawerProps.length - 1].dirty).toBe(false);
  });

  it("shows error alert when issue mutation fails", async () => {
    mockIssueMutate.mockImplementation(
      (
        _args: { billId: number },
        opts?: { onError?: (err: Error) => void },
      ) => {
        opts?.onError?.(new Error("backend said no"));
      },
    );

    renderDrawer();

    const row = screen.getByTestId("issuable-bill-101");
    fireEvent.click(
      within(row).getByRole("button", { name: /issueInvoice\.issue/i }),
    );
    fireEvent.click(screen.getByTestId("issue-confirm"));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent("backend said no");
    });
    // Receiver form stays open on failure so the operator can correct and retry.
    expect(screen.getByTestId("issue-receiver-form")).toBeInTheDocument();
  });

  // #907: the picker marks already-invoiced bills and swaps Issue for "View
  // invoice", but the issue-by-ID fallback below it bypasses the picker
  // entirely. The backend used to swallow that duplicate on its per-bill
  // idempotency key and answer 202 {"ok":true}, so the drawer showed "Invoice
  // issue queued" for a bill that already had a factura. It now answers 409 +
  // fiscal_receipt_already_issued, and the drawer must name that specifically.
  it("names the already-invoiced conflict when issuing by ID hits a live receipt", async () => {
    mockIssueMutate.mockImplementation(
      (
        _args: { billId: number },
        opts?: { onError?: (err: unknown) => void },
      ) => {
        const err = new Error(
          "Request failed with status code 409",
        ) as Error & {
          status?: number;
          response?: { status: number; data: unknown };
        };
        err.status = 409;
        err.response = {
          status: 409,
          data: {
            error: "This bill already has an invoice.",
            code: "fiscal_receipt_already_issued",
          },
        };
        opts?.onError?.(err);
      },
    );

    renderDrawer();

    fireEvent.click(
      screen.getByRole("button", { name: /issueInvoice\.issueById/i }),
    );
    fireEvent.change(screen.getByLabelText("issueInvoice.billIdLabel"), {
      target: { value: "1704" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: /issueInvoice\.continue/i }),
    );
    fireEvent.click(screen.getByTestId("issue-confirm"));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent(
        "issueInvoice.alreadyIssuedError",
      );
    });
    // Not the generic failure copy, and never the success banner.
    expect(screen.getByRole("alert")).not.toHaveTextContent(
      "issueInvoice.error",
    );
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("shows empty copy when no issuable bills", () => {
    mockIssuableData = [];
    renderDrawer();

    expect(screen.getByText("issueInvoice.empty")).toBeInTheDocument();
    expect(screen.getByText("issueInvoice.emptyHint")).toBeInTheDocument();
  });

  it("shows a load error instead of the empty waiting state when the picker query fails", () => {
    mockIssuableData = [];
    useIssuableBillsMock.mockImplementation(() => ({
      data: undefined,
      isLoading: false,
      isFetching: false,
      isSuccess: false,
      isError: true,
      refetch: jest.fn(),
    }));
    renderDrawer();

    expect(screen.getByTestId("issuable-bills-error")).toHaveTextContent(
      "issueInvoice.loadError",
    );
    expect(screen.queryByText("issueInvoice.empty")).not.toBeInTheDocument();
  });

  it("offers View invoice for already-invoiced paid bills", () => {
    const onViewExisting = jest.fn();
    const onClose = jest.fn();
    mockIssuableData = [
      bill({
        bill_id: 201,
        bill_number: "B-201",
        table_label: "Table 6",
        existing_receipt_id: 909,
      }),
    ];
    renderDrawer({ onViewExisting, onClose });

    expect(screen.getByText("issueInvoice.alreadyIssued")).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("view-existing-invoice-201"));
    expect(onViewExisting).toHaveBeenCalledWith({
      receiptId: 909,
      bill: expect.objectContaining({ bill_id: 201 }),
    });
    expect(onClose).toHaveBeenCalled();
  });

  it("previews Invoice/Receipt for US venues (never FACTURA A/B/C)", async () => {
    resolveReceiptTypeMock.mockResolvedValue({
      receipt_type: "receipt",
      letter: "",
      country: "US",
    });

    renderDrawer();

    const row = screen.getByTestId("issuable-bill-101");
    fireEvent.click(
      within(row).getByRole("button", { name: /issueInvoice\.issue/i }),
    );

    await waitFor(() => {
      expect(screen.getByTestId("letter-preview")).toHaveTextContent(
        "Will issue Receipt",
      );
    });
    expect(screen.getByTestId("letter-preview")).not.toHaveTextContent(
      /FACTURA/i,
    );
  });

  it("previews FACTURA letter for Argentina venues", async () => {
    resolveReceiptTypeMock.mockResolvedValue({
      receipt_type: "factura_b",
      letter: "B",
      country: "AR",
    });

    renderDrawer();

    const row = screen.getByTestId("issuable-bill-101");
    fireEvent.click(
      within(row).getByRole("button", { name: /issueInvoice\.issue/i }),
    );

    await waitFor(() => {
      expect(screen.getByTestId("letter-preview")).toHaveTextContent(
        "Will issue FACTURA B",
      );
    });
  });

  it("reveals collapsed issue-by-ID fallback and issues by numeric id", async () => {
    mockIssueMutate.mockImplementation(
      (
        _args: { billId: number },
        opts?: { onSuccess?: () => void },
      ) => {
        opts?.onSuccess?.();
      },
    );

    renderDrawer();

    expect(
      screen.queryByLabelText("issueInvoice.billIdLabel"),
    ).not.toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", { name: /issueInvoice\.issueById/i }),
    );

    const idInput = screen.getByLabelText("issueInvoice.billIdLabel");
    fireEvent.change(idInput, { target: { value: "777" } });
    fireEvent.click(
      screen.getByRole("button", { name: /issueInvoice\.continue/i }),
    );

    // Receiver step then confirm.
    expect(screen.getByTestId("issue-receiver-form")).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("issue-confirm"));

    expect(mockIssueMutate).toHaveBeenCalledWith(
      expect.objectContaining({ billId: 777 }),
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );

    await waitFor(() => {
      expect(screen.getByRole("status")).toHaveTextContent(
        "issueInvoice.success",
      );
    });
  });
});
