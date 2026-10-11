/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import PersistentGuestNav from "../PersistentGuestNav";
import { convertAmount } from "@/api/currency";

jest.mock("react-hot-toast", () => ({ __esModule: true, default: jest.fn() }));

jest.mock("next/navigation", () => ({
  usePathname: () => "/t/TBL1/menu",
}));

jest.mock("@/contexts/CustomerAuthContext", () => ({
  useCustomerAuth: () => ({ customerId: null }),
}));

jest.mock("../../../i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params?.amount ? `${key}:${params.amount}` : key,
    currentLanguage: "en",
  }),
}));

jest.mock("../../guest/SimpleLanguageSelector", () => ({
  SimpleLanguageSelector: () => <div data-testid="lang-selector" />,
}));

jest.mock("@/api/currency", () => {
  const actual = jest.requireActual("@/api/currency");
  return {
    ...actual,
    convertAmount: jest.fn(async (amount: number) => ({
      converted_amount: amount * 2,
    })),
  };
});

describe("PersistentGuestNav convert-then-format (#499)", () => {
  it("uses the converted remainder in the bill tab aria-label and visible badge", async () => {
    render(
      <PersistentGuestNav
        tableCode="TBL1"
        isOrderingEnabled
        defaultCurrency="ARS"
        displayCurrency="USD"
        currentBill={
          {
            bill: { total_amount: 10, paid_amount: 0 },
            items: [{ quantity: 1 }],
          } as any
        }
      />,
    );

    await waitFor(() => {
      expect(
        screen.getByRole("link", { name: /\$20\.00/ }),
      ).toBeInTheDocument();
    });
    const billTab = screen.getByRole("link", { name: /\$20\.00/ });
    expect(billTab).toHaveTextContent("$20.00");
    expect(billTab.getAttribute("aria-label")).toMatch(/\$20\.00/);
    expect(billTab.getAttribute("aria-label")).not.toMatch(/\$10\.00/);
    expect(billTab).not.toHaveTextContent("$10.00");
    expect(screen.queryByRole("link", { name: /\$10\.00/ })).toBeNull();
    expect(convertAmount).toHaveBeenCalled();
  });
});
