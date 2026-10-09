/**
 * Issue #561 — USDC chip must share the diner's decimal convention with the
 * primary total (de: "34,99 $", not a toFixed "USDC 34.99" next to it).
 *
 * Uses the real formatCurrency so this fails until crypto codes go through
 * Intl.NumberFormat instead of Number.prototype.toFixed.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

jest.mock("../../api/currency", () => {
  const actual = jest.requireActual("../../api/currency") as typeof import("../../api/currency");
  return {
    ...actual,
    convertAmount: jest.fn(async (amount: number) => ({
      converted_amount: amount,
    })),
  };
});

import CurrencyConverter from "./CurrencyConverter";
import { formatCurrency } from "../../api/currency";

const COMMA_LOCALES = ["de", "de-DE", "fr", "es", "it"] as const;

describe("CurrencyConverter USDC chip locale decimals (#561)", () => {
  it.each(COMMA_LOCALES)(
    "formats the USDC chip with the same comma decimal as the %s total",
    async (locale) => {
      const amount = 34.99;
      const expectedPrimary = formatCurrency(amount, "USD", undefined, locale);

      render(
        <CurrencyConverter
          amount={amount}
          fromCurrency="USD"
          displayCurrency="USD"
          showUSDCConversion
          locale={locale}
        />,
      );

      await waitFor(() => {
        expect(screen.getByText(/USDC/)).toBeInTheDocument();
      });

      const chip = screen.getByText(/USDC/);
      expect(chip.textContent).toContain("34,99");
      expect(chip.textContent).not.toContain("34.99");
      // Same decimal convention as the locale-formatted total (NBSP-tolerant).
      expect(expectedPrimary.replace(/\s/g, " ")).toContain("34,99");
      expect(expectedPrimary).not.toContain("34.99");
    },
  );
});
