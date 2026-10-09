/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import CartModal from "./CartModal";
import type { CartItem } from "../_types";
import { validatePromoCode } from "../../../../../api/promo";

jest.mock("../../../../../i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ currentLanguage: "en" }),
}));

jest.mock("@/components/common/CurrencyConverter", () => {
  const CurrencyPrice = ({ amount }: { amount: number }) => (
    <span>{amount}</span>
  );
  return {
    __esModule: true,
    default: ({ amount }: { amount: number }) => <span>{amount}</span>,
    CurrencyPrice,
  };
});

jest.mock("../../../../../api/promo", () => ({
  validatePromoCode: jest.fn(),
}));

const cart: CartItem[] = [
  {
    itemType: "menu_item",
    name: "Burger",
    price: 10,
    quantity: 1,
    menuItemId: "i1",
  },
];

describe("CartModal promo error announcement", () => {
  beforeEach(() => {
    (validatePromoCode as jest.Mock).mockReset();
  });

  it("announces an invalid promo code and points the input at the alert", async () => {
    (validatePromoCode as jest.Mock).mockRejectedValue(new Error("invalid"));

    render(
      <CartModal
        isOpen
        onClose={jest.fn()}
        cart={cart}
        orderLoading={false}
        quotePending={false}
        quoteError={null}
        quoteValid
        quoteBlocked={false}
        onUpdateQuantity={jest.fn()}
        onSubmitOrder={jest.fn()}
        onClearCart={jest.fn()}
        t={(key: string) => key}
        tableCode="T1"
        promotionPreview={{
          baseSubtotal: 10,
          discountTotal: 0,
          autoDiscountTotal: 0,
          promoDiscount: 0,
          discountedSubtotal: 10,
          netSubtotal: 10,
          tax: 0,
          serviceFee: 0,
          tip: 0,
          finalTotal: 10,
          applied: [],
        }}
        appliedPromo={null}
        onPromoApplied={jest.fn()}
        onPromoRemoved={jest.fn()}
        businessCurrencies={{
          default_currency: "USD",
          display_currency: "USD",
        }}
      />,
    );

    fireEvent.click(screen.getByText("menu.promoCode"));
    fireEvent.change(
      screen.getByRole("textbox", { name: "menu.promoCodePlaceholder" }),
      { target: { value: "NOPE" } },
    );
    fireEvent.click(screen.getByRole("button", { name: "menu.applyPromo" }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("menu.promoInvalid");
    expect(alert).toHaveAttribute("id", "cart-promo-error");
    expect(alert.className).toContain("text-rose-700");

    const input = screen.getByRole("textbox", {
      name: "menu.promoCodePlaceholder",
    });
    expect(input).toHaveAttribute("aria-describedby", "cart-promo-error");
    expect(input).toHaveAttribute("aria-invalid", "true");
  });
});
