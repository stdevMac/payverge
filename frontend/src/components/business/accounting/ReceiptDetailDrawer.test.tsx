/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { FiscalDeliveryTask, ReceiptRow } from "@/api/fiscal";
import { listReceiptDelivery } from "@/api/fiscal";
import {
  useCreditNote,
  useResendReceipt,
  useRetryDelivery,
} from "@/hooks/accounting/useAccountingQueries";
import ReceiptDetailDrawer from "./ReceiptDetailDrawer";

const mockRetry = jest.fn();
const mockResend = jest.fn();
const mockCredit = jest.fn();

jest.mock("@/hooks/accounting/useAccountingQueries", () => ({
  useRetryDelivery: jest.fn(() => ({
    mutate: mockRetry,
    isPending: false,
  })),
  useResendReceipt: jest.fn(() => ({
    mutate: mockResend,
    isPending: false,
  })),
  useCreditNote: jest.fn(() => ({
    mutate: mockCredit,
    isPending: false,
  })),
}));

jest.mock("@/api/fiscal", () => ({
  listReceiptDelivery: jest.fn(() => Promise.resolve([])),
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
      <div data-testid="detail-drawer-shell">
        <h2>{title}</h2>
        {subtitle ? <p data-testid="detail-subtitle">{subtitle}</p> : null}
        <div data-testid="detail-body">{children}</div>
        {footer ? <div data-testid="detail-footer">{footer}</div> : null}
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
      <div data-testid="credit-confirm-modal" role="dialog">
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
    <span data-testid="animated-total" className={className}>
      {format ? format(value) : String(value)}
    </span>
  ),
}));

const listReceiptDeliveryMock = listReceiptDelivery as jest.Mock;
const useRetryMock = useRetryDelivery as jest.Mock;
const useResendMock = useResendReceipt as jest.Mock;
const useCreditMock = useCreditNote as jest.Mock;

const t = (key: string, params?: Record<string, string | number>) => {
  if (!params) return key;
  return Object.entries(params).reduce(
    (acc, [name, value]) =>
      acc.replace(new RegExp(`\\{${name}\\}`, "g"), String(value)),
    key,
  );
};

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

function receipt(
  overrides: Partial<ReceiptRow> & { id: number } = { id: 9 },
): ReceiptRow {
  const id = overrides.id;
  return {
    business_id: 42,
    settings_id: 1,
    bill_id: 501,
    payment_id: null,
    alternative_payment_id: null,
    country: "AR",
    provider: "arca",
    action: "issue",
    receipt_type: "invoice_b",
    receipt_number: "0001-00000009",
    provider_receipt_id: null,
    auth_code: "12345678901234",
    auth_expires_at: null,
    qr_payload: null,
    qr_image_path: null,
    pdf_path: null,
    customer_doc_type: "CUIT",
    customer_doc_number: "20123456789",
    total_amount_cents: 12500,
    tip_amount_cents: 1500,
    currency: "ARS",
    status: "authorized",
    error_code: null,
    error_message: null,
    issued_at: "2026-01-15T18:00:00Z",
    created_at: "2026-01-15T18:00:00Z",
    updated_at: "2026-01-15T18:00:00Z",
    delivery: [
      { task_id: 1, channel: "artifact", status: "succeeded" },
      { task_id: 2, channel: "email", status: "dead" },
    ],
    needs_attention: true,
    ...overrides,
    id,
  };
}

function deliveryTasks(): FiscalDeliveryTask[] {
  return [
    {
      id: 1,
      receipt_id: 9,
      channel: "artifact",
      status: "succeeded",
      attempts: 1,
      max_attempts: 5,
      masked_recipient: undefined,
    },
    {
      id: 2,
      receipt_id: 9,
      channel: "email",
      status: "dead",
      attempts: 5,
      max_attempts: 5,
      last_error_category: "smtp_bounce",
      masked_recipient: "g***@example.com",
    },
    {
      id: 3,
      receipt_id: 9,
      channel: "print",
      status: "pending",
      attempts: 0,
      max_attempts: 5,
    },
  ];
}

function renderDrawer(
  props: Partial<React.ComponentProps<typeof ReceiptDetailDrawer>> = {},
) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  const defaults: React.ComponentProps<typeof ReceiptDetailDrawer> = {
    open: true,
    receipt: receipt(),
    businessId: "42",
    locale: "en",
    currency: "ARS",
    canWrite: true,
    canManageSensitive: true,
    businessTimezone: "America/Argentina/Buenos_Aires",
    onClose: jest.fn(),
    t,
    tWith,
  };
  return render(
    <QueryClientProvider client={client}>
      <ReceiptDetailDrawer {...defaults} {...props} />
    </QueryClientProvider>,
  );
}

describe("ReceiptDetailDrawer", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    listReceiptDeliveryMock.mockResolvedValue(deliveryTasks());
    useRetryMock.mockReturnValue({ mutate: mockRetry, isPending: false });
    useResendMock.mockReturnValue({ mutate: mockResend, isPending: false });
    useCreditMock.mockReturnValue({ mutate: mockCredit, isPending: false });
  });

  it("renders receipt fields: number, auth code, issued at, masked customer, amount+tip", async () => {
    renderDrawer();

    expect(screen.getByText("receiptDetail.title")).toBeInTheDocument();
    expect(screen.getByTestId("detail-subtitle")).toHaveTextContent(
      "0001-00000009",
    );
    expect(screen.getByText("#501")).toBeInTheDocument();
    expect(screen.getByText("12345678901234")).toBeInTheDocument();
    // Masked customer keeps last 4 digits
    expect(screen.getByText(/CUIT/)).toBeInTheDocument();
    expect(screen.getByText(/\*+6789/)).toBeInTheDocument();
    expect(screen.getByTestId("animated-total")).toBeInTheDocument();
    expect(screen.getByText(/receiptDetail\.tip/i)).toBeInTheDocument();
    expect(screen.getByText("receiptDetail.authCode")).toBeInTheDocument();
    expect(screen.getByText("receiptDetail.issuedAt")).toBeInTheDocument();

    await waitFor(() => {
      expect(listReceiptDeliveryMock).toHaveBeenCalledWith(42, 9);
    });
  });

  it("shows error message when receipt failed", () => {
    renderDrawer({
      receipt: receipt({
        id: 9,
        status: "failed_permanent",
        error_message: "AFIP rejected CAE request",
      }),
    });

    const alert = screen.getByTestId("receipt-detail-error");
    expect(alert).toHaveAttribute("role", "alert");
    expect(alert).toHaveTextContent("AFIP rejected CAE request");
    expect(alert).toHaveTextContent("receiptDetail.error");
  });

  it("fetches listReceiptDelivery only when the drawer is open", async () => {
    const { rerender } = renderDrawer({ open: false, receipt: receipt() });
    expect(listReceiptDeliveryMock).not.toHaveBeenCalled();

    const client = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    rerender(
      <QueryClientProvider client={client}>
        <ReceiptDetailDrawer
          open
          receipt={receipt()}
          businessId="42"
          locale="en"
          currency="ARS"
          canWrite
          canManageSensitive
          onClose={jest.fn()}
          t={t}
          tWith={tWith}
        />
      </QueryClientProvider>,
    );

    await waitFor(() => {
      expect(listReceiptDeliveryMock).toHaveBeenCalledTimes(1);
      expect(listReceiptDeliveryMock).toHaveBeenCalledWith(42, 9);
    });
  });

  it("renders per-channel delivery rows with Retry on dead tasks via useRetryDelivery", async () => {
    renderDrawer();

    await waitFor(() => {
      expect(screen.getByTestId("delivery-task-2")).toBeInTheDocument();
    });

    expect(screen.getByTestId("delivery-task-1")).toBeInTheDocument();
    expect(screen.getByTestId("delivery-task-3")).toBeInTheDocument();
    expect(screen.getByText("g***@example.com")).toBeInTheDocument();

    // Only dead channel gets a Retry button
    expect(screen.getByTestId("delivery-retry-2")).toBeInTheDocument();
    expect(screen.queryByTestId("delivery-retry-1")).not.toBeInTheDocument();
    expect(screen.queryByTestId("delivery-retry-3")).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId("delivery-retry-2"));
    expect(mockRetry).toHaveBeenCalledWith(2, expect.any(Object));
  });

  it("shows Re-send and Credit note in footer for authorized receipts", () => {
    renderDrawer();

    expect(screen.getByTestId("receipt-detail-resend")).toBeInTheDocument();
    expect(screen.getByTestId("receipt-detail-credit")).toBeInTheDocument();
  });

  it("hides Credit note when canManageSensitive is false", () => {
    renderDrawer({ canManageSensitive: false });

    expect(screen.getByTestId("receipt-detail-resend")).toBeInTheDocument();
    expect(screen.queryByTestId("receipt-detail-credit")).not.toBeInTheDocument();
  });

  it("hides write actions when canWrite is false", () => {
    renderDrawer({ canWrite: false, canManageSensitive: false });

    expect(screen.queryByTestId("receipt-detail-resend")).not.toBeInTheDocument();
    expect(screen.queryByTestId("receipt-detail-credit")).not.toBeInTheDocument();
  });

  it("calls useResendReceipt on Re-send", () => {
    renderDrawer();
    fireEvent.click(screen.getByTestId("receipt-detail-resend"));
    expect(mockResend).toHaveBeenCalledWith(9);
  });

  it("confirms credit note with required reason then calls useCreditNote", async () => {
    const onClose = jest.fn();
    mockCredit.mockImplementation(
      (
        _vars: { receiptId: number; reason: string },
        opts?: { onSuccess?: () => void },
      ) => {
        opts?.onSuccess?.();
      },
    );
    renderDrawer({ onClose });

    fireEvent.click(screen.getByTestId("receipt-detail-credit"));

    expect(
      await screen.findByText("receiptDetail.confirmCreditTitle"),
    ).toBeInTheDocument();

    fireEvent.change(screen.getByTestId("credit-reason"), {
      target: { value: "customer refund" },
    });
    fireEvent.click(screen.getByTestId("credit-confirm"));

    expect(mockCredit).toHaveBeenCalledWith(
      expect.objectContaining({ receiptId: 9, reason: "customer refund" }),
      expect.any(Object),
    );
    expect(onClose).toHaveBeenCalled();
  });

  it("hides credit note for already-credited / credit_note action receipts", () => {
    renderDrawer({
      receipt: receipt({ id: 9, action: "credit_note", status: "authorized" }),
    });

    expect(screen.queryByTestId("receipt-detail-credit")).not.toBeInTheDocument();
  });
});
