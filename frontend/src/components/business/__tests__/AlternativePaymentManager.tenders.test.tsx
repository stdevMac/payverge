/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const mockLocale = { current: "es-AR" };

jest.mock("react-hot-toast", () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/SimpleTranslationProvider");
  return {
    ...actual,
    useSimpleLocale: () => ({ locale: mockLocale.current, setLocale: jest.fn() }),
  };
});
jest.mock("@/hooks/useAlternativePayments", () => ({
  useBusinessAlternativePayments: () => ({
    paymentBreakdown: null,
    pendingPayments: [],
    loading: false,
    markPayment: jest.fn(),
    rejectPayment: jest.fn(),
  }),
}));

import AlternativePaymentManager from "../AlternativePaymentManager";

async function methodOptionLabels() {
  fireEvent.click(
    screen.getByRole("button", { name: /agregar pago|add payment/i }),
  );
  const dialog = await screen.findByRole("dialog");
  const combo = within(dialog).getByRole("button", {
    name: /método de pago|payment method/i,
  });
  await userEvent.click(combo);
  return screen.getAllByRole("option").map((el) => el.textContent ?? "");
}

describe("AlternativePaymentManager tenders (#649)", () => {
  beforeEach(() => {
    mockLocale.current = "es-AR";
  });

  it("hides Venmo and labels card Mercado Pago for an es-AR AR venue", async () => {
    render(
      <AlternativePaymentManager
        billId="761"
        billTotal={18.04}
        currency="USD"
        country="AR"
      />,
    );

    const labels = (await methodOptionLabels()).join(" ");
    expect(labels).toMatch(/Mercado Pago/i);
    expect(labels).toMatch(/Efectivo/i);
    expect(labels).not.toMatch(/Venmo/i);
    expect(labels).not.toMatch(/Cripto/i);
  });

  it("hides Venmo on an AR venue even when operator chrome is English", async () => {
    mockLocale.current = "en";
    render(
      <AlternativePaymentManager
        billId="761"
        billTotal={18.04}
        currency="USD"
        country="AR"
      />,
    );

    const labels = (await methodOptionLabels()).join(" ");
    expect(labels).toMatch(/Mercado Pago/i);
    expect(labels).not.toMatch(/Venmo/i);
  });
});
