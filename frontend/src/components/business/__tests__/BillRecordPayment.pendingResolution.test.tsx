/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

const mockRejectPayment = jest.fn(() =>
  Promise.resolve({ success: true, status: "rejected" as const }),
);
const mockRefresh = jest.fn();

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
}));
jest.mock("@/hooks/useAlternativePayments", () => ({
  useBusinessAlternativePayments: () => ({
    pendingPayments: [
      {
        id: "17",
        billId: "5",
        participantName: "Jane",
        participantAddress: "guest",
        amount: "12500000",
        paymentMethod: 1,
        timestamp: Date.now(),
        status: "pending",
      },
    ],
    loading: false,
    markPayment: jest.fn(),
    rejectPayment: mockRejectPayment,
    refresh: mockRefresh,
  }),
}));

import { BillRecordPayment } from "../BillRecordPayment";

it("lets an authorized operator reject a pending alternative-payment request", async () => {
  const onPaymentRecorded = jest.fn();
  render(
    <BillRecordPayment
      billId={5}
      billNumber="B-5"
      billStatus="open"
      currency="USD"
      remainingAmount={20}
      onPaymentRecorded={onPaymentRecorded}
      tString={(key: string) => key}
    />,
  );

  fireEvent.click(
    screen.getByRole("button", { name: "recordPayment.pending.reject" }),
  );

  await waitFor(() => expect(mockRejectPayment).toHaveBeenCalledWith("17"));
  expect(onPaymentRecorded).toHaveBeenCalledTimes(1);
  expect(mockRefresh).toHaveBeenCalled();
});
