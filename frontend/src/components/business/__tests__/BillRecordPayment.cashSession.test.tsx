/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const mockMarkPayment = jest.fn(() => Promise.resolve({ success: true }));
const mockGetCurrent = jest.fn();
const mockOpenSession = jest.fn();

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
}));
jest.mock("@/hooks/useAlternativePayments", () => ({
  useBusinessAlternativePayments: () => ({
    pendingPayments: [],
    loading: false,
    markPayment: mockMarkPayment,
    rejectPayment: jest.fn(),
    refresh: jest.fn(),
  }),
}));
jest.mock("@/api/cashRegister", () => ({
  __esModule: true,
  cashRegisterApi: {
    getCurrent: (...args: unknown[]) => mockGetCurrent(...args),
    openSession: (...args: unknown[]) => mockOpenSession(...args),
  },
}));
jest.mock("@/components/business/payments/MercadoPagoPointCharge", () => ({
  MercadoPagoPointCharge: () => null,
}));
jest.mock("@/components/business/payments/MercadoPagoQRCharge", () => ({
  MercadoPagoQRCharge: () => null,
}));

import { PaymentMethod } from "@/types/alternativePayments";
import { BillRecordPayment } from "../BillRecordPayment";

function renderRecordPayment() {
  return render(
    <BillRecordPayment
      billId={5}
      billNumber="B-5"
      billStatus="open"
      currency="USD"
      remainingAmount={20}
      onPaymentRecorded={jest.fn()}
      tString={(key: string) => key}
      businessId={42}
    />,
  );
}

function openModal() {
  fireEvent.click(
    screen.getByRole("button", { name: "recordPayment.actions.record" }),
  );
}

function submitButton() {
  return screen.getByRole("button", { name: "recordPayment.modal.submit" });
}

beforeEach(() => {
  mockMarkPayment.mockClear();
  mockGetCurrent.mockReset();
  mockOpenSession.mockReset();
});

it("disables Cash Mark as received while the cash session is still unknown", async () => {
  mockGetCurrent.mockReturnValue(new Promise(() => {}));

  renderRecordPayment();
  openModal();

  expect(mockGetCurrent).toHaveBeenCalledWith("42");
  expect(submitButton()).toBeDisabled();
  expect(
    screen.getByTestId("cash-session-checking"),
  ).toHaveTextContent("recordPayment.warnings.cashSessionChecking");
  expect(screen.queryByTestId("cash-session-warning")).not.toBeInTheDocument();

  fireEvent.click(submitButton());
  expect(mockMarkPayment).not.toHaveBeenCalled();
});

it("keeps Cash submit disabled and shows the required warning when no session is open", async () => {
  mockGetCurrent.mockResolvedValue({
    session: null,
    unassigned_cash_total: 0,
    unassigned_cash_count: 0,
  });

  renderRecordPayment();
  openModal();

  await waitFor(() => {
    expect(screen.getByTestId("cash-session-warning")).toHaveTextContent(
      "recordPayment.warnings.cashSessionRequired",
    );
  });
  expect(submitButton()).toBeDisabled();
  expect(screen.queryByText(/Unassigned cash/i)).not.toBeInTheDocument();
});

it("opens a cash drawer from the warning and then allows Cash submit", async () => {
  mockGetCurrent.mockResolvedValue({
    session: null,
    unassigned_cash_total: 0,
    unassigned_cash_count: 0,
    suggested_opening_float: 200,
  });
  mockOpenSession.mockResolvedValue({
    id: 9,
    status: "open",
    opening_float: 200,
  });

  renderRecordPayment();
  openModal();

  await waitFor(() => {
    expect(screen.getByTestId("cash-session-open-drawer")).toBeInTheDocument();
  });
  fireEvent.click(screen.getByTestId("cash-session-open-drawer"));

  await waitFor(() => {
    expect(mockOpenSession).toHaveBeenCalledWith("42", {
      opening_float: 200,
      opening_note: "recordPayment.actions.openDrawer",
    });
    expect(submitButton()).toBeEnabled();
  });
});

it("shows an explicit error and keeps Cash submit disabled when session lookup fails", async () => {
  mockGetCurrent.mockRejectedValue(new Error("network down"));

  renderRecordPayment();
  openModal();

  await waitFor(() => {
    expect(screen.getByTestId("cash-session-error")).toHaveTextContent(
      "recordPayment.errors.cashSessionLookupFailed",
    );
  });
  expect(submitButton()).toBeDisabled();
  fireEvent.click(submitButton());
  expect(mockMarkPayment).not.toHaveBeenCalled();
});

it("lets Card record while the cash session is still unknown", async () => {
  mockGetCurrent.mockReturnValue(new Promise(() => {}));

  renderRecordPayment();
  openModal();
  expect(submitButton()).toBeDisabled();

  await userEvent.click(
    screen.getByRole("button", { name: /recordPayment\.methods\.cash/ }),
  );
  await userEvent.click(
    await screen.findByRole("option", { name: "recordPayment.methods.card" }),
  );

  await waitFor(() => {
    expect(submitButton()).toBeEnabled();
  });
  fireEvent.click(submitButton());
  await waitFor(() => {
    expect(mockMarkPayment).toHaveBeenCalledWith(
      undefined,
      "20.00",
      PaymentMethod.CARD,
      undefined,
      expect.objectContaining({ participantName: "B-5" }),
    );
  });
});
