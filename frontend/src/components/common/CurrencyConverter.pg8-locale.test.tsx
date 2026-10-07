/**
 * PG-8 / PG-20 — CurrencyConverter must honor locale (not only CurrencyPrice).
 * Guest bill hero (page.tsx) and line items (GuestBill) must share one locale
 * so totals never mix en-US ($67.27) with es (67,27 US$) on one screen.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import CurrencyConverter, { CurrencyPrice } from "./CurrencyConverter";

jest.mock("../../api/currency", () => ({
  convertAmount: jest.fn(async (amount: number) => ({
    converted_amount: amount,
  })),
  formatCurrency: jest.fn(
    (amount: number, code: string, _symbol?: string, locale?: string) =>
      `FMT(${amount}|${code}|${locale ?? "default"})`,
  ),
}));

import { formatCurrency } from "../../api/currency";

describe("CurrencyConverter PG-8 locale prop", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("threads locale into formatCurrency for the primary amount", async () => {
    render(
      <CurrencyConverter
        amount={67.27}
        fromCurrency="USD"
        displayCurrency="USD"
        showUSDCConversion={false}
        locale="es"
      />,
    );

    await waitFor(() => {
      expect(formatCurrency).toHaveBeenCalledWith(
        67.27,
        "USD",
        undefined,
        "es",
      );
    });
    expect(screen.getByText(/FMT\(67\.27\|USD\|es\)/)).toBeInTheDocument();
  });

  it("CurrencyPrice threads the same required locale", async () => {
    render(
      <CurrencyPrice
        amount={67.27}
        fromCurrency="USD"
        displayCurrency="USD"
        locale="es"
      />,
    );

    await waitFor(() => {
      expect(formatCurrency).toHaveBeenCalledWith(
        67.27,
        "USD",
        undefined,
        "es",
      );
    });
    expect(screen.getByText(/FMT\(67\.27\|USD\|es\)/)).toBeInTheDocument();
  });
});
