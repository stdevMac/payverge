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

jest.mock("react-hot-toast", () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) =>
    key.endsWith("pendingPayments.requestRef") ? "Request #{id}" : key,
}));
jest.mock("@/hooks/useAlternativePayments", () => ({
  useBusinessAlternativePayments: () => ({
    get pendingPayments() {
      return mockPending.payments;
    },
    paymentBreakdown: null,
    loading: false,
    markPayment: jest.fn(),
    rejectPayment: jest.fn(),
  }),
}));

import AlternativePaymentManager from "../AlternativePaymentManager";

beforeEach(() => {
  jest.useFakeTimers();
  jest.setSystemTime(now);
  mockPending.payments = [];
});

afterEach(() => {
  jest.useRealTimers();
});

it("disables confirm on the standalone manager when the request is older than 24h", () => {
  mockPending.payments = [
    {
      id: "91",
      billId: "387",
      participantName: "Walk-up guest",
      participantAddress: "guest",
      amount: "18040000",
      paymentMethod: PaymentMethod.CARD,
      timestamp: now - ALTERNATIVE_PAYMENT_REQUEST_MAX_AGE_MS - 60 * 60 * 1000,
      status: "pending",
    },
  ];

  render(
    <AlternativePaymentManager billId="387" billTotal={36.08} currency="USD" />,
  );

  expect(screen.getByTestId("pending-payment-expired")).toBeInTheDocument();
  expect(
    screen.getByRole("button", {
      name: /alternativePaymentManager\.buttons\.confirmPayment/,
    }),
  ).toBeDisabled();
  expect(screen.getByText("Request #91")).toBeInTheDocument();
});

it("distinguishes two otherwise identical pending requests by request id", () => {
  mockPending.payments = [
    {
      id: "91",
      billId: "387",
      participantName: "Walk-up guest",
      participantAddress: "guest",
      amount: "18040000",
      paymentMethod: PaymentMethod.CASH,
      timestamp: now - 60 * 1000,
      status: "pending",
    },
    {
      id: "92",
      billId: "387",
      participantName: "Walk-up guest",
      participantAddress: "guest",
      amount: "18040000",
      paymentMethod: PaymentMethod.CASH,
      timestamp: now - 60 * 1000,
      status: "pending",
    },
  ];

  render(
    <AlternativePaymentManager billId="387" billTotal={36.08} currency="USD" />,
  );

  expect(screen.getAllByRole("button", { name: /Request #91/ })).toHaveLength(2);
  expect(screen.getAllByRole("button", { name: /Request #92/ })).toHaveLength(2);
});
