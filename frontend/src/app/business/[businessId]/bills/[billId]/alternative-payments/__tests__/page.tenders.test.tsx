/** @jest-environment jsdom */
/**
 * #649 — Registrar pago sibling: the standalone alternative-payments page
 * must hide Venmo for an AR Mercado Pago venue even when chrome is English,
 * and must not advertise Venmo in the method sidebar.
 */
import React from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const mockLocale = { current: "en" };

jest.mock("next/navigation", () => ({
  useParams: () => ({
    businessId: "demo-admin-8-ai-pro",
    billId: "761",
  }),
}));

jest.mock("react-hot-toast", () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/SimpleTranslationProvider");
  return {
    ...actual,
    useSimpleLocale: () => ({
      locale: mockLocale.current,
      setLocale: jest.fn(),
    }),
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

jest.mock("@/utils/resolveBusinessId", () => ({
  resolveNumericBusinessId: jest.fn().mockResolvedValue(86),
}));

jest.mock("@/api/business", () => ({
  getBusiness: jest.fn().mockResolvedValue({
    default_currency: "USD",
    address: { country: "AR" },
  }),
}));

import AlternativePaymentsPage from "../page";

describe("alternative-payments AR venue tenders (#649)", () => {
  beforeEach(() => {
    mockLocale.current = "en";
  });

  async function openMethodOptions() {
    fireEvent.click(
      await screen.findByRole("button", { name: /add payment|agregar pago/i }),
    );
    const dialog = await screen.findByRole("dialog");
    const combo = within(dialog).getByRole("button", {
      name: /payment method|método de pago/i,
    });
    await userEvent.click(combo);
    return screen.getAllByRole("option").map((el) => el.textContent ?? "");
  }

  it("hides Venmo and labels card Mercado Pago for an English operator at an AR venue", async () => {
    render(<AlternativePaymentsPage />);

    await waitFor(() => {
      expect(screen.queryByText(/Venmo/i)).not.toBeInTheDocument();
    });

    const labels = (await openMethodOptions()).join(" ");
    expect(labels).toMatch(/Mercado Pago/i);
    expect(labels).not.toMatch(/Venmo/i);
    expect(labels).not.toMatch(/Crypto/i);
  });

  it("does not list Venmo in the method sidebar for es-AR + AR", async () => {
    mockLocale.current = "es-AR";
    render(<AlternativePaymentsPage />);

    await waitFor(() => {
      expect(screen.getByText(/Mercado Pago/i)).toBeInTheDocument();
    });
    expect(screen.queryByText(/Venmo/i)).not.toBeInTheDocument();
  });
});
