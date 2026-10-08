/** @jest-environment jsdom */
/**
 * #649 sibling path: Bills display mode Registrar pago must receive the
 * venue country so an AR Mercado Pago venue hides Venmo even in English.
 */
import React from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const mockLocale = { current: "en" };

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: mockLocale.current }),
}));
jest.mock("@/hooks/useAlternativePayments", () => ({
  useBusinessAlternativePayments: () => ({
    pendingPayments: [],
    loading: false,
    markPayment: jest.fn(),
    rejectPayment: jest.fn(),
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
jest.mock("@/components/business/BillItemEditor", () => ({
  BillItemEditor: () => null,
}));
jest.mock("@/api/currency", () => ({
  formatCurrency: (value: number) => `$${value}`,
}));

jest.mock("@/api/bills", () => ({
  ...jest.requireActual("@/api/bills"),
  getBill: jest.fn(),
}));

import BillDisplayMode from "@/components/business/BillDisplayMode";
import { getBill } from "@/api/bills";
import type { Bill } from "@/api/bills";

const mockOpenBill = {
  id: 761,
  business_id: 86,
  table_id: 7,
  bill_number: "B-761",
  notes: "",
  items: "[]",
  subtotal: 18.04,
  tax_amount: 0,
  service_fee_amount: 0,
  total_amount: 18.04,
  paid_amount: 0,
  tip_amount: 0,
  currency: "USD",
  status: "open",
  settlement_address: "0x",
  tipping_address: "0x",
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
  physical_item_quantity: 1,
} as Bill;

const METHOD_LABELS: Record<string, string> = {
  "recordPayment.actions.record": "Registrar pago",
  "recordPayment.modal.method": "Método de pago",
  "recordPayment.methods.cash": "Efectivo",
  "recordPayment.methods.card": "Tarjeta",
  "recordPayment.methods.mercadopago": "Mercado Pago",
  "recordPayment.methods.venmo": "Venmo",
  "recordPayment.methods.other": "Otro",
  "recordPayment.methods.crypto": "Cripto",
};

describe("BillDisplayMode tenders (#649)", () => {
  beforeEach(() => {
    mockLocale.current = "en";
    (getBill as jest.Mock).mockResolvedValue({
      bill: mockOpenBill,
      items: [],
      history: [],
    });
  });

  it("hides Venmo and labels card Mercado Pago on an AR venue with English chrome", async () => {
    render(
      <BillDisplayMode
        bills={[mockOpenBill]}
        orders={{}}
        onExit={jest.fn()}
        onCloseBill={jest.fn()}
        onCreateBill={jest.fn()}
        onBillUpdated={jest.fn()}
        businessId={86}
        tString={(key: string) => METHOD_LABELS[key] || key}
        currency="USD"
        country="AR"
      />,
    );

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Registrar pago" }),
      ).toBeInTheDocument(),
    );

    fireEvent.click(screen.getByRole("button", { name: "Registrar pago" }));
    const dialog = await screen.findByRole("dialog");
    const combo = within(dialog).getByRole("button", {
      name: /método de pago/i,
    });
    await userEvent.click(combo);
    const labels = screen
      .getAllByRole("option")
      .map((el) => el.textContent ?? "")
      .join(" ");

    expect(labels).toMatch(/Mercado Pago/i);
    expect(labels).not.toMatch(/Venmo/i);
  });
});
