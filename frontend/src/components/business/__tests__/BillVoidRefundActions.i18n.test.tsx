/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

jest.mock("@/api/bills", () => ({
  getBillAudit: jest.fn().mockResolvedValue({ entries: [] }),
  voidBill: jest.fn(),
  refundBillPayment: jest.fn(),
}));

jest.mock("@/api/currency", () => ({
  formatCurrency: (amount: number) => `$${(amount || 0).toFixed(2)}`,
}));

jest.mock("@/components/business/managerPin/ManagerPinProvider", () => ({
  useWithManagerPin: () => ({
    withManagerPin: (fn: (pin?: string) => Promise<unknown>) => fn(undefined),
  }),
}));

// Simulate a Spanish operator: getTranslation maps the void-button key.
const ES: Record<string, string> = {
  "billManager.voidRefund.actions.voidBill": "Anular factura",
  "billManager.voidRefund.actions.refundPayment": "Reembolsar pago",
};
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "es" }),
  getTranslation: (key: string) => ES[key] ?? key,
}));

import { BillVoidRefundActions } from "../BillVoidRefundActions";
import { getBillAudit, refundBillPayment } from "@/api/bills";

function makeBill() {
  return {
    bill: {
      id: 99,
      bill_number: "B-1",
      status: "open",
      paid_amount: 0,
      payments: [],
    },
  } as unknown as Parameters<typeof BillVoidRefundActions>[0]["bill"];
}

it("renders the void action via the translation helper (es), not hardcoded English", async () => {
  render(
    <BillVoidRefundActions
      bill={makeBill()}
      onRefresh={() => {}}
      onCloseParent={() => {}}
      currency="USD"
    />,
  );

  // Translated label present; the old hardcoded English literal absent.
  expect(await screen.findByText("Anular factura")).toBeInTheDocument();
  expect(screen.queryByText("Void bill")).toBeNull();
});

it("renders the void modal copy through the translation helper", async () => {
  render(
    <BillVoidRefundActions
      bill={makeBill()}
      onRefresh={() => {}}
      onCloseParent={() => {}}
      currency="USD"
    />,
  );

  fireEvent.click(await screen.findByText("Anular factura"));

  // Modal warning/copy keys are echoed (not in ES map) — assert the key form
  // shows, proving the literal English string is no longer hardcoded.
  await waitFor(() =>
    expect(
      screen.getByText("billManager.voidRefund.voidModal.warning"),
    ).toBeInTheDocument(),
  );
  // The old hardcoded English warning must be gone.
  expect(
    screen.queryByText(/Voiding cancels the bill and releases the table/),
  ).toBeNull();
});

it("refunds a confirmed alternative payment by alternative_payment_id", async () => {
  (refundBillPayment as jest.Mock).mockResolvedValue({ bill: {} });
  const bill = {
    bill: {
      id: 99,
      bill_number: "B-1",
      status: "partial",
      paid_amount: 12,
      payments: [],
      alternative_payments: [
        {
          id: 7,
          bill_id: 99,
          participant_address: "cashier",
          participant_name: "Cash Guest",
          amount: 12,
          payment_method: "cash",
          status: "confirmed",
          created_at: "2026-06-13T12:00:00Z",
          updated_at: "2026-06-13T12:00:00Z",
        },
      ],
    },
  } as unknown as Parameters<typeof BillVoidRefundActions>[0]["bill"];

  render(
    <BillVoidRefundActions
      bill={bill}
      onRefresh={() => {}}
      onCloseParent={() => {}}
      currency="USD"
    />,
  );

  fireEvent.click(await screen.findByText("Reembolsar pago"));
  expect(await screen.findByText("Cash")).toBeInTheDocument();

  fireEvent.change(screen.getByRole("textbox"), {
    target: { value: "cash returned" },
  });
  const refundButtons = screen.getAllByText("Reembolsar pago");
  fireEvent.click(refundButtons[refundButtons.length - 1]);

  await waitFor(() =>
    expect(refundBillPayment).toHaveBeenCalledWith(
      99,
      { alternative_payment_id: 7, reason: "cash returned" },
      expect.objectContaining({ idempotencyKey: expect.any(String) }),
    ),
  );
});

it("renders audit timestamps in business timezone, not device local time (R17)", async () => {
  // 18:30 UTC → 15:30 in Buenos Aires (UTC-3). The audit log must show the
  // business wall-clock, never the operator device's local time.
  (getBillAudit as jest.Mock).mockResolvedValueOnce({
    entries: [
      {
        id: 1,
        action: "void",
        target_type: "bill",
        reason: "duplicate charge",
        pin_present: true,
        created_at: "2026-06-13T18:30:00Z",
      },
    ],
  });

  render(
    <BillVoidRefundActions
      bill={makeBill()}
      onRefresh={() => {}}
      onCloseParent={() => {}}
      currency="USD"
      businessTimezone="America/Argentina/Buenos_Aires"
    />,
  );

  // Wait for the audit reason to appear (proves the entry rendered).
  await screen.findByText("duplicate charge");

  // Business-time wall clock (15:30) present; the raw UTC hour (18:30) absent.
  expect(screen.getByText(/15:30/)).toBeInTheDocument();
  expect(screen.queryByText(/18:30/)).toBeNull();
});
