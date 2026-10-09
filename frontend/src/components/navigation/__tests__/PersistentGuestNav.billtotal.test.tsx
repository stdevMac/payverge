/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import PersistentGuestNav from "../PersistentGuestNav";

jest.mock("react-hot-toast", () => ({ __esModule: true, default: jest.fn() }));

jest.mock("next/navigation", () => ({
  usePathname: () => "/t/TBL1/menu",
}));

jest.mock("@/contexts/CustomerAuthContext", () => ({
  useCustomerAuth: () => ({ customerId: null }),
}));

jest.mock("../../../i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock("../../guest/SimpleLanguageSelector", () => ({
  SimpleLanguageSelector: () => <div data-testid="lang-selector" />,
}));

// Render the raw amount so rounding is observable.
jest.mock("../../common/CurrencyConverter", () => ({
  CurrencyPrice: ({ amount }: { amount: number }) => (
    <span data-testid="nav-bill-total">{String(amount)}</span>
  ),
}));

jest.mock("@/utils/guestCurrencyFormatter", () => ({
  formatGuestCurrency: (amount: number) => `$${amount}`,
  formatConvertedGuestCurrency: (amount: number) => `$${amount}`,
  normalizeGuestLocale: () => "en",
}));

describe("PersistentGuestNav bill total", () => {
  it("passes the exact dollar amount to CurrencyPrice (no rounding)", () => {
    render(
      <PersistentGuestNav
        tableCode="TBL1"
        isOrderingEnabled
        currentBill={
          { bill: { total_amount: 12.5 }, items: [] } as unknown as Parameters<
            typeof PersistentGuestNav
          >[0]["currentBill"]
        }
      />,
    );
    expect(screen.getByTestId("nav-bill-total").textContent).toBe("12.5");
  });
});
