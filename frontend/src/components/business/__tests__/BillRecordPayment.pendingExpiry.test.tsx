/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { PaymentMethod } from "@/types/alternativePayments";
import { ALTERNATIVE_PAYMENT_REQUEST_MAX_AGE_MS } from "@/lib/alternativePaymentExpiry";

const now = Date.parse("2026-08-14T15:00:00.000Z");
const mockPending = {
  payments: [] as Array<{
    id: string;
    billId: string;
    participantName: string;
    participantAddress: string;
    amount: string;
    paymentMethod: PaymentMethod;
    timestamp: number;
    status: "pending";
  }>,
};
const mockMarkPayment = jest.fn();

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
}));
jest.mock("@/hooks/useAlternativePayments", () => ({
  useBusinessAlternativePayments: () => ({
    get pendingPayments() {
      return mockPending.payments;
    },
    loading: false,
    markPayment: mockMarkPayment,
    rejectPayment: jest.fn(),
    refresh: jest.fn(),
  }),
}));

import { BillRecordPayment } from "../BillRecordPayment";

function renderPayment() {
  return render(
    <BillRecordPayment
      billId={387}
      billNumber="B-387"
      billStatus="open"
      currency="USD"
      remainingAmount={20}
      onPaymentRecorded={jest.fn()}
      tString={(key: string) => key}
    />,
  );
}

beforeEach(() => {
  jest.useFakeTimers();
  jest.setSystemTime(now);
  mockMarkPayment.mockReset();
  mockPending.payments = [];
});

afterEach(() => {
  jest.useRealTimers();
});

it("shows requested age and disables confirm when the request is older than 24h", () => {
  mockPending.payments = [
    {
      id: "91",
      billId: "387",
      participantName: "Card guest",
      participantAddress: "guest",
      amount: "20000000",
      paymentMethod: PaymentMethod.CARD,
      timestamp: now - ALTERNATIVE_PAYMENT_REQUEST_MAX_AGE_MS - 60 * 60 * 1000,
      status: "pending",
    },
  ];

  renderPayment();

  expect(screen.getByTestId("pending-payment-requested-at")).toBeInTheDocument();
  expect(screen.getByTestId("pending-payment-age")).toHaveTextContent("25h");
  expect(screen.getByTestId("pending-payment-expired")).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "recordPayment.pending.confirm" }),
  ).toBeDisabled();
});

it("keeps confirm enabled for a fresh pending request", () => {
  mockPending.payments = [
    {
      id: "92",
      billId: "387",
      participantName: "Card guest",
      participantAddress: "guest",
      amount: "20000000",
      paymentMethod: PaymentMethod.CARD,
      timestamp: now - 30 * 60 * 1000,
      status: "pending",
    },
  ];

  renderPayment();

  expect(screen.getByTestId("pending-payment-age")).toHaveTextContent("30m");
  expect(screen.queryByTestId("pending-payment-expired")).not.toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "recordPayment.pending.confirm" }),
  ).toBeEnabled();
});
