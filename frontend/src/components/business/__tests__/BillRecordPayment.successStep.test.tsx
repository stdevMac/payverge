/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
}));
const mockMarkPayment = jest.fn(() => Promise.resolve({ success: true }));
jest.mock("@/hooks/useAlternativePayments", () => ({
  useBusinessAlternativePayments: () => ({
    pendingPayments: [],
    loading: false,
    markPayment: mockMarkPayment,
    refresh: jest.fn(),
  }),
}));
jest.mock("@/api/cashRegister", () => ({
  __esModule: true,
  cashRegisterApi: {
    getCurrent: jest.fn().mockResolvedValue({
      session: { status: "open" },
      unassigned_cash_total: 0,
      unassigned_cash_count: 0,
    }),
  },
}));
jest.mock("@/components/business/payments/MercadoPagoPointCharge", () => ({
  MercadoPagoPointCharge: () => null,
}));
jest.mock("@/components/business/payments/MercadoPagoQRCharge", () => ({
  MercadoPagoQRCharge: () => null,
}));

import { BillRecordPayment } from "../BillRecordPayment";

it("offers print + close when the payment settles the balance", async () => {
  const onPrintReceipt = jest.fn();
  const onRequestCloseBill = jest.fn();
  render(
    <BillRecordPayment
      billId={5}
      billNumber="B-5"
      billStatus="open"
      currency="USD"
      remainingAmount={20}
      onPaymentRecorded={jest.fn()}
      tString={(k: string) => k}
      businessId={7}
      canPrint
      onPrintReceipt={onPrintReceipt}
      onRequestCloseBill={onRequestCloseBill}
    />,
  );
  fireEvent.click(
    screen.getByRole("button", { name: "recordPayment.actions.record" }),
  );
  // amount defaults to the full remaining (20.00) — submit as-is once the
  // cash session lookup confirms an open drawer.
  const submit = screen.getByRole("button", {
    name: "recordPayment.modal.submit",
  });
  await waitFor(() => expect(submit).toBeEnabled());
  fireEvent.click(submit);
  const settled = await screen.findByTestId("record-payment-settled");
  expect(settled).toBeInTheDocument();
  fireEvent.click(
    screen.getByRole("button", { name: "recordPayment.settled.print" }),
  );
  expect(onPrintReceipt).toHaveBeenCalled();
  fireEvent.click(
    screen.getByRole("button", { name: "recordPayment.settled.closeBill" }),
  );
  expect(onRequestCloseBill).toHaveBeenCalled();
});
