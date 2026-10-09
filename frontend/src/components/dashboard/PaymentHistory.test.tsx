/** @jest-environment jsdom */
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import PaymentHistory from "./PaymentHistory";
import { getPaymentHistoryPage } from "@/api/payments";
import { getBill } from "@/api/bills";

jest.mock("@/api/payments", () => ({
  __esModule: true,
  getPaymentHistoryPage: jest.fn(),
  exportPayments: jest.fn(),
}));
jest.mock("@/api/business", () => ({
  getBusiness: jest.fn().mockResolvedValue({ default_currency: "EUR" }),
}));
jest.mock("@/api/bills", () => ({
  __esModule: true,
  getBill: jest.fn(),
}));
// Reachability only: assert the row control opens BillDetailsModal. The modal
// body (void/refund) is covered elsewhere and must not be rebuilt here.
jest.mock("@/components/business/BillDetailsModal", () => ({
  BillDetailsModal: ({
    isOpen,
    bill,
  }: {
    isOpen: boolean;
    bill: { bill?: { id?: number } } | null;
  }) =>
    isOpen && bill?.bill?.id != null ? (
      <div data-testid="bill-details-modal" data-bill-id={String(bill.bill.id)}>
        BillDetailsModal
      </div>
    ) : null,
}));

// The component now consumes the server-paginated envelope. This shim lets the
// existing row-based fixtures below stay unchanged: pass an array, get an
// {items,total,page,page_size} page back.
const mockGetPaymentHistory = {
  mockResolvedValue: (rows: unknown[]) =>
    (getPaymentHistoryPage as jest.Mock).mockResolvedValue({
      items: rows,
      total: (rows as unknown[]).length,
      page: 1,
      page_size: 20,
    }),
  mockImplementation: (fn: (...a: unknown[]) => unknown) =>
    (getPaymentHistoryPage as jest.Mock).mockImplementation(fn as never),
};
const mockGetPaymentHistoryPage = getPaymentHistoryPage as jest.Mock;

const row = (over: Partial<Record<string, unknown>> = {}) => ({
  id: 1,
  bill_id: 1,
  bill_number: "1001",
  table_name: "T1",
  payer_address: "0x1234567890abcdef",
  amount: 100,
  tip_amount: 10,
  currency: "EUR",
  tx_hash: "0xabcdef1234567890",
  status: "confirmed",
  created_at: "2026-07-01T12:00:00Z",
  updated_at: "2026-07-01T12:00:00Z",
  ...over,
});

describe("PaymentHistory", () => {
  afterEach(() => jest.clearAllMocks());

  it("formats amounts in the business currency, not hardcoded USD", async () => {
    mockGetPaymentHistory.mockResolvedValue([row()]);
    render(<PaymentHistory businessId="42" />);
    // EUR formatting → euro sign somewhere, and no US dollar sign.
    await waitFor(() =>
      expect(screen.getAllByText(/€/).length).toBeGreaterThan(0),
    );
    expect(screen.queryByText(/\$100/)).not.toBeInTheDocument();
  });

  it("sums only confirmed payments into the Total Revenue / Tips tiles", async () => {
    mockGetPaymentHistory.mockResolvedValue([
      row({ id: 1, amount: 100, tip_amount: 10, status: "confirmed" }),
      row({ id: 2, amount: 999, tip_amount: 99, status: "failed" }),
      row({ id: 3, amount: 500, tip_amount: 50, status: "refunded" }),
      row({ id: 4, amount: 200, tip_amount: 20, status: "pending" }),
    ]);
    render(<PaymentHistory businessId="42" />);
    await waitFor(() => expect(mockGetPaymentHistoryPage).toHaveBeenCalled());
    // Only the confirmed row (100 / 10) should be in the money tiles. The
    // failed/refunded/pending amounts must not inflate them.
    await waitFor(() =>
      expect(
        screen.getAllByText((t) => /€\s?100([.,]00)?/.test(t)).length,
      ).toBeGreaterThan(0),
    );
    // 1099 (all amounts) must never appear as a revenue total.
    expect(
      screen.queryByText((t) => /1[.,]?099/.test(t)),
    ).not.toBeInTheDocument();
  });

  it("renders a per-row currency when present", async () => {
    mockGetPaymentHistory.mockResolvedValue([
      row({ id: 1, currency: "JPY", amount: 1200, tip_amount: 0 }),
    ]);
    render(<PaymentHistory businessId="42" />);
    await waitFor(() =>
      expect(screen.getAllByText(/¥|JPY/).length).toBeGreaterThan(0),
    );
  });

  it("shows a human payer label with the wallet hex demoted to a tooltip", async () => {
    // Canonical EVM address: 0x + exactly 40 hex chars.
    const addr = "0x000000000000000000000000000000000000dE01";
    mockGetPaymentHistory.mockResolvedValue([
      row({ id: 1, payer_address: addr, tx_hash: "0xdeadbeef" }),
    ]);
    const { container } = render(<PaymentHistory businessId="42" />);
    // Primary line is a human method label, not the raw hex. Scope to the table
    // body so the method-filter "Crypto" option doesn't count.
    await waitFor(() =>
      expect(
        within(container.querySelector("tbody")!).getByText(/USDC|Cripto|Crypto/),
      ).toBeInTheDocument(),
    );
    // Secondary line carries the full address in a title tooltip.
    expect(screen.getByTitle(addr)).toBeInTheDocument();
  });

  it("labels non-wallet payers as guest and omits the hex line", async () => {
    mockGetPaymentHistory.mockResolvedValue([
      row({ id: 1, payer_address: "0xguest", tx_hash: "" }),
    ]);
    render(<PaymentHistory businessId="42" />);
    await waitFor(() =>
      expect(screen.getByText(/Guest|Invitado/)).toBeInTheDocument(),
    );
    // No wallet-hex secondary line for a sentinel payer.
    expect(screen.queryByTitle("0xguest")).not.toBeInTheDocument();
  });

  // Regression (audit L5 follow-up): staff-entered manual payments carry a
  // synthetic `manual_*` tx hash from the backend — a cash/card payment marked
  // paid by staff must render as Manual, never as crypto.
  it("labels staff manual payments as Manual, not crypto", async () => {
    mockGetPaymentHistory.mockResolvedValue([
      row({ id: 1, payer_address: "staff_manual", tx_hash: "manual_1_abc" }),
    ]);
    const { container } = render(<PaymentHistory businessId="42" />);
    // Scope to the payment table — the method-filter dropdown legitimately lists
    // "Manual"/"Crypto"/"Online" as options, which is not what these regressions
    // guard against (they guard the row's payer label).
    const table = () => within(container.querySelector("tbody")!);
    await waitFor(() =>
      expect(table().getByText(/^Manual$/)).toBeInTheDocument(),
    );
    expect(table().queryByText(/Crypto|Cripto|USDC/)).not.toBeInTheDocument();
    expect(screen.queryByTitle("staff_manual")).not.toBeInTheDocument();
  });

  // Regression (audit L5 round 2): plugin settlements (Stripe/PayPal/
  // MercadoPago) write payer "plugin" with a synthetic `plugin_*` tx hash —
  // they must render as Online, never as crypto.
  it("labels plugin payments as Online, not crypto", async () => {
    mockGetPaymentHistory.mockResolvedValue([
      row({ id: 1, payer_address: "plugin", tx_hash: "plugin_pay_123" }),
    ]);
    const { container } = render(<PaymentHistory businessId="42" />);
    const table = () => within(container.querySelector("tbody")!);
    await waitFor(() =>
      expect(table().getByText(/Online|En línea/)).toBeInTheDocument(),
    );
    expect(table().queryByText(/Crypto|Cripto|USDC/)).not.toBeInTheDocument();
    expect(screen.queryByTitle("plugin")).not.toBeInTheDocument();
  });

  it("handles a payment with no payer_address without crashing", async () => {
    mockGetPaymentHistory.mockResolvedValue([
      row({ id: 1, payer_address: "", tx_hash: "" }),
    ]);
    render(<PaymentHistory businessId="42" />);
    await waitFor(() =>
      expect(screen.getByText(/Guest|Invitado/)).toBeInTheDocument(),
    );
  });

  // Task 7 / finding 62: method filter options are generated from the server's
  // available_methods — never the hardcoded crypto/manual/stripe list. Options
  // whose backing value matches zero rows must not appear.
  it("builds method filter options from available_methods only", async () => {
    (getPaymentHistoryPage as jest.Mock).mockResolvedValue({
      items: [row({ id: 1, method: "crypto" })],
      total: 1,
      page: 1,
      page_size: 20,
      available_methods: ["crypto", "card"],
    });
    render(<PaymentHistory businessId="42" />);
    await waitFor(() => expect(getPaymentHistoryPage).toHaveBeenCalled());

    // Open the method select (aria-label from methodOptions.label).
    const methodSelect = await screen.findByLabelText(/Method|Método/i);
    expect(methodSelect).toBeInTheDocument();

    // The request must not send a non-canonical method key.
    const lastCall = (getPaymentHistoryPage as jest.Mock).mock.calls.at(-1);
    const query = lastCall?.[1] ?? {};
    expect(query.method === undefined || query.method === "all" || query.method === "crypto" || query.method === "card").toBe(
      true,
    );
    expect(query.method).not.toBe("stripe");
    expect(query.method).not.toBe("manual");
  });

  // Task 8 / findings 5+43: every money row drills into BillDetailsModal so
  // refund/void (already wired there) are reachable from Payment History.
  it("exposes a View control on every payment row that opens BillDetailsModal", async () => {
    mockGetPaymentHistory.mockResolvedValue([
      row({ id: 11, bill_id: 501, bill_number: "B2-0d60c280-49f" }),
      row({ id: 12, bill_id: 502, bill_number: "DEMO-DEL-9", table_name: "Delivery" }),
    ]);
    (getBill as jest.Mock).mockImplementation(async (billId: number) => ({
      bill: {
        id: billId,
        bill_number: `B-${billId}`,
        status: "paid",
        business_id: 42,
        paid_amount: 10,
        payments: [],
        alternative_payments: [],
      },
      items: [],
    }));

    render(<PaymentHistory businessId="42" />);
    await waitFor(() => expect(mockGetPaymentHistoryPage).toHaveBeenCalled());

    const viewButtons = await screen.findAllByRole("button", {
      name: /view bill|ver cuenta|view details|ver detalles/i,
    });
    expect(viewButtons.length).toBe(2);

    await userEvent.click(viewButtons[0]);
    await waitFor(() => {
      expect(getBill).toHaveBeenCalledWith(501, undefined, expect.any(Number));
    });
    expect(await screen.findByTestId("bill-details-modal")).toHaveAttribute(
      "data-bill-id",
      "501",
    );
  });

  it("never renders a DEMO- prefix on payment history bill numbers", async () => {
    mockGetPaymentHistory.mockResolvedValue([
      row({ id: 1, bill_id: 9, bill_number: "DEMO-1001" }),
    ]);
    render(<PaymentHistory businessId="42" />);
    await waitFor(() =>
      expect(screen.getByText(/#?1001/)).toBeInTheDocument(),
    );
    expect(screen.queryByText(/DEMO-/i)).not.toBeInTheDocument();
  });
});
