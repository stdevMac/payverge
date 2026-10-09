/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const mockLocale = { current: "es-AR" };

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

import { BillRecordPayment } from "../BillRecordPayment";

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

function renderRecordPayment(country: string | null = "US") {
  return render(
    <BillRecordPayment
      billId={761}
      billNumber="B-761"
      billStatus="open"
      currency="USD"
      remainingAmount={18.04}
      onPaymentRecorded={jest.fn()}
      tString={(key: string) => METHOD_LABELS[key] || key}
      businessId={86}
      country={country}
    />,
  );
}

describe("BillRecordPayment tenders (#649)", () => {
  beforeEach(() => {
    mockLocale.current = "es-AR";
  });

  async function methodOptionLabels() {
    fireEvent.click(screen.getByRole("button", { name: "Registrar pago" }));
    const dialog = await screen.findByRole("dialog");
    const combo = within(dialog).getByRole("button", {
      name: /método de pago/i,
    });
    await userEvent.click(combo);
    return screen.getAllByRole("option").map((el) => el.textContent?.trim());
  }

  it("offers Mercado Pago and hides Venmo on an es-AR US demo venue", async () => {
    renderRecordPayment("US");
    const labels = await methodOptionLabels();
    expect(labels).toEqual(expect.arrayContaining(["Efectivo", "Mercado Pago", "Otro"]));
    expect(labels).not.toEqual(expect.arrayContaining(["Venmo"]));
    expect(labels?.join(" ")).not.toMatch(/Venmo/i);
    expect(labels?.join(" ")).not.toMatch(/Cripto/i);
  });

  it("hides Venmo and labels card Mercado Pago on an es-AR AR venue", async () => {
    renderRecordPayment("AR");
    const labels = await methodOptionLabels();
    expect(labels).toEqual(
      expect.arrayContaining(["Efectivo", "Mercado Pago", "Otro"]),
    );
    expect(labels.join(" ")).not.toMatch(/Venmo/i);
    expect(labels.join(" ")).not.toMatch(/Cripto/i);
  });

  it("hides Venmo on an AR venue even when operator chrome is English", async () => {
    mockLocale.current = "en";
    renderRecordPayment("AR");
    const labels = await methodOptionLabels();
    expect(labels.join(" ")).toMatch(/Mercado Pago/i);
    expect(labels.join(" ")).not.toMatch(/Venmo/i);
  });

  it("keeps Venmo for a US English operator", async () => {
    mockLocale.current = "en";
    renderRecordPayment("US");
    const labels = await methodOptionLabels();
    expect(labels.join(" ")).toMatch(/Venmo/i);
    expect(labels.join(" ")).not.toMatch(/Mercado Pago/i);
  });
});
