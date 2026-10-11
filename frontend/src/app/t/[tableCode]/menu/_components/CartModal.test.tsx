/** @jest-environment jsdom */
import React from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import CartModal from "./CartModal";
import type { CartItem } from "../_types";

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

const cart: CartItem[] = [
  {
    itemType: "menu_item",
    name: "Burger",
    price: 10,
    quantity: 2,
    menuItemId: "i1",
  },
];

const zeroSettlement = (netSubtotal: number) => ({
  netSubtotal,
  tax: 0,
  serviceFee: 0,
  tip: 0,
  finalTotal: netSubtotal,
});

describe("CartModal", () => {
  it("renders the authoritative settlement breakdown and final total", () => {
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
          baseSubtotal: 20,
          discountTotal: 4,
          autoDiscountTotal: 4,
          promoDiscount: 0,
          discountedSubtotal: 16,
          netSubtotal: 16,
          tax: 1.6,
          serviceFee: 0.8,
          tip: 0,
          finalTotal: 18.4,
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

    expect(screen.getByText("common.tax")).toBeInTheDocument();
    expect(screen.getByText("common.serviceFee")).toBeInTheDocument();
    expect(screen.getByText("common.total")).toBeInTheDocument();
    expect(screen.getByText("18.4")).toBeInTheDocument();
  });

  it("renders cart items", () => {
    render(
      <CartModal
        isOpen={true}
        onClose={jest.fn()}
        cart={cart}
        orderLoading={false}
        quotePending={false}
        quoteError={null}
        quoteValid={true}
        quoteBlocked={false}
        onUpdateQuantity={jest.fn()}
        onSubmitOrder={jest.fn()}
        onClearCart={jest.fn()}
        t={(key: string) => key}
        tableCode="T1"
        promotionPreview={{
          baseSubtotal: 20,
          discountTotal: 0,
          autoDiscountTotal: 0,
          promoDiscount: 0,
          discountedSubtotal: 20,
          ...zeroSettlement(20),
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
    expect(screen.getByText("Burger")).toBeInTheDocument();
  });

  it("applied-promo message shows the entered code, not the offer name (PROMO-001)", () => {
    render(
      <CartModal
        isOpen={true}
        onClose={jest.fn()}
        cart={cart}
        orderLoading={false}
        quotePending={false}
        quoteError={null}
        quoteValid={true}
        quoteBlocked={false}
        onUpdateQuantity={jest.fn()}
        onSubmitOrder={jest.fn()}
        onClearCart={jest.fn()}
        // Echo interpolated params so we can assert the code that was used.
        t={(key: string, params?: Record<string, string | number>) =>
          params ? `${key}|${JSON.stringify(params)}` : key
        }
        tableCode="T1"
        promotionPreview={{
          baseSubtotal: 20,
          discountTotal: 4,
          autoDiscountTotal: 0,
          promoDiscount: 4,
          discountedSubtotal: 16,
          ...zeroSettlement(16),
          applied: [],
        }}
        appliedPromo={{
          id: 1,
          name: "Summer Sale",
          code: "SUMMER20",
          discount_type: "percentage",
          discount_value: 20,
          applicable_to: "all",
          target_id: null,
        }}
        onPromoApplied={jest.fn()}
        onPromoRemoved={jest.fn()}
        businessCurrencies={{
          default_currency: "USD",
          display_currency: "USD",
        }}
      />,
    );
    // The interpolated code must be the guest-entered code, not the name.
    expect(screen.getByText(/"code":"SUMMER20"/)).toBeInTheDocument();
    expect(screen.queryByText(/"code":"Summer Sale"/)).not.toBeInTheDocument();
  });

  it("renders the promo discount as its own summary line reflecting the total (PROMO-002)", () => {
    render(
      <CartModal
        isOpen={true}
        onClose={jest.fn()}
        cart={cart}
        orderLoading={false}
        quotePending={false}
        quoteError={null}
        quoteValid={true}
        quoteBlocked={false}
        onUpdateQuantity={jest.fn()}
        onSubmitOrder={jest.fn()}
        onClearCart={jest.fn()}
        t={(key: string) => key}
        tableCode="T1"
        promotionPreview={{
          baseSubtotal: 20,
          discountTotal: 4,
          autoDiscountTotal: 0,
          promoDiscount: 4,
          discountedSubtotal: 16,
          ...zeroSettlement(16),
          applied: [],
        }}
        appliedPromo={{
          id: 1,
          name: "Summer Sale",
          code: "SUMMER20",
          discount_type: "percentage",
          discount_value: 20,
          applicable_to: "all",
          target_id: null,
        }}
        onPromoApplied={jest.fn()}
        onPromoRemoved={jest.fn()}
        businessCurrencies={{
          default_currency: "USD",
          display_currency: "USD",
        }}
      />,
    );
    // The promo discount surfaces as a dedicated summary line (the mocked
    // CurrencyPrice renders the raw amount)...
    expect(screen.getByText("menu.promoCode")).toBeInTheDocument();
    expect(screen.getByText("4")).toBeInTheDocument();
    // ...and the reduced total (16 of a 20 cart) is what the guest sees, proving
    // the promo is folded into the total (the pre-fix bug left the total at 20).
    expect(screen.getByText("16")).toBeInTheDocument();
  });

  it("applied-promo message falls back to the offer name when no code is present (PROMO-001)", () => {
    render(
      <CartModal
        isOpen={true}
        onClose={jest.fn()}
        cart={cart}
        orderLoading={false}
        quotePending={false}
        quoteError={null}
        quoteValid={true}
        quoteBlocked={false}
        onUpdateQuantity={jest.fn()}
        onSubmitOrder={jest.fn()}
        onClearCart={jest.fn()}
        t={(key: string, params?: Record<string, string | number>) =>
          params ? `${key}|${JSON.stringify(params)}` : key
        }
        tableCode="T1"
        promotionPreview={{
          baseSubtotal: 20,
          discountTotal: 4,
          autoDiscountTotal: 0,
          promoDiscount: 4,
          discountedSubtotal: 16,
          ...zeroSettlement(16),
          applied: [],
        }}
        appliedPromo={{
          id: 2,
          name: "Auto Deal",
          discount_type: "percentage",
          discount_value: 20,
          applicable_to: "all",
          target_id: null,
        }}
        onPromoApplied={jest.fn()}
        onPromoRemoved={jest.fn()}
        businessCurrencies={{
          default_currency: "USD",
          display_currency: "USD",
        }}
      />,
    );
    expect(screen.getByText(/"code":"Auto Deal"/)).toBeInTheDocument();
  });

  it("shows place order button", () => {
    render(
      <CartModal
        isOpen={true}
        onClose={jest.fn()}
        cart={cart}
        orderLoading={false}
        quotePending={false}
        quoteError={null}
        quoteValid={true}
        quoteBlocked={false}
        onUpdateQuantity={jest.fn()}
        onSubmitOrder={jest.fn()}
        onClearCart={jest.fn()}
        t={(key: string) => key}
        tableCode="T1"
        promotionPreview={{
          baseSubtotal: 20,
          discountTotal: 0,
          autoDiscountTotal: 0,
          promoDiscount: 0,
          discountedSubtotal: 20,
          ...zeroSettlement(20),
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
    expect(screen.getByText("menu.placeOrder")).toBeInTheDocument();
  });

  it("blocks checkout until the current server quote is valid and surfaces quote errors", () => {
    const onSubmitOrder = jest.fn();
    const props: React.ComponentProps<typeof CartModal> = {
      isOpen: true,
      onClose: jest.fn(),
      cart,
      orderLoading: false,
      quotePending: true,
      quoteError: null,
      quoteValid: false,
      quoteBlocked: false,
      onUpdateQuantity: jest.fn(),
      onSubmitOrder,
      onClearCart: jest.fn(),
      t: (key: string) => key,
      tableCode: "T1",
      promotionPreview: {
        baseSubtotal: 20,
        discountTotal: 0,
        autoDiscountTotal: 0,
        promoDiscount: 0,
        discountedSubtotal: 20,
        ...zeroSettlement(20),
        applied: [],
      },
      appliedPromo: null,
      onPromoApplied: jest.fn(),
      onPromoRemoved: jest.fn(),
      businessCurrencies: {
        default_currency: "USD",
        display_currency: "USD",
      },
    };
    const { rerender } = render(<CartModal {...props} />);

    let placeOrder = screen.getByRole("button", { name: /menu\.placeOrder/ });
    expect(placeOrder).toBeDisabled();
    fireEvent.click(placeOrder);
    expect(onSubmitOrder).not.toHaveBeenCalled();

    rerender(
      <CartModal
        {...props}
        quotePending={false}
        quoteError={new Error("malformed quote")}
      />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent(
      "errors.menuUnavailableDescription",
    );
    expect(
      screen.getByRole("button", { name: "menu.placeOrder" }),
    ).toBeDisabled();

    rerender(
      <CartModal
        {...props}
        quotePending={false}
        quoteError={null}
        quoteValid
      />,
    );
    placeOrder = screen.getByRole("button", { name: "menu.placeOrder" });
    expect(placeOrder).toBeEnabled();
    fireEvent.click(placeOrder);
    expect(onSubmitOrder).toHaveBeenCalledTimes(1);
  });

  it("visibly explains and blocks an authoritative orderability rejection", () => {
    const onSubmitOrder = jest.fn();
    render(
      <CartModal
        isOpen
        onClose={jest.fn()}
        cart={cart}
        orderLoading={false}
        quotePending={false}
        quoteError={null}
        quoteValid={false}
        quoteBlocked
        onUpdateQuantity={jest.fn()}
        onSubmitOrder={onSubmitOrder}
        onClearCart={jest.fn()}
        t={(key: string) => key}
        tableCode="T1"
        promotionPreview={{
          baseSubtotal: 20,
          discountTotal: 0,
          autoDiscountTotal: 0,
          promoDiscount: 0,
          discountedSubtotal: 20,
          ...zeroSettlement(20),
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

    expect(screen.getByRole("alert")).toHaveTextContent(
      "menu.orderErrorItemUnavailableGeneric",
    );
    const placeOrder = screen.getByRole("button", {
      name: "menu.placeOrder",
    });
    expect(placeOrder).toBeDisabled();
    fireEvent.click(placeOrder);
    expect(onSubmitOrder).not.toHaveBeenCalled();
  });

  it("does not render 0 tax/service while the quote is pending (#509)", () => {
    render(
      <CartModal
        isOpen
        onClose={jest.fn()}
        cart={cart}
        orderLoading={false}
        quotePending
        quoteError={null}
        quoteValid={false}
        quoteBlocked={false}
        onUpdateQuantity={jest.fn()}
        onSubmitOrder={jest.fn()}
        onClearCart={jest.fn()}
        t={(key: string) => key}
        tableCode="T1"
        promotionPreview={{
          baseSubtotal: 20,
          discountTotal: 0,
          autoDiscountTotal: 0,
          promoDiscount: 0,
          discountedSubtotal: 20,
          ...zeroSettlement(20),
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

    expect(screen.getByText("common.tax")).toBeInTheDocument();
    expect(screen.getByText("common.serviceFee")).toBeInTheDocument();
    expect(screen.queryByText("0")).not.toBeInTheDocument();
    expect(
      screen.getAllByTestId("quote-pending-amount").length,
    ).toBeGreaterThanOrEqual(2);
    expect(
      screen.getByRole("button", { name: /menu\.placeOrder/ }),
    ).toBeDisabled();
  });
});

const interpolate = (key: string, params?: Record<string, string | number>) => {
  const table: Record<string, string> = {
    "menu.placeOrder": "Place Order",
    "menu.clearCart": "Clear Cart",
    "menu.clearCartConfirm": "Tap again to clear",
    "menu.clearCartConfirmAria": "Tap again to clear cart",
    "menu.yourOrder": "Your Order",
    "menu.remove": "Remove",
    "menu.item": "item",
    "menu.items": "items",
    "menu.cartQuantityIs": "{name} quantity is now {quantity}",
    "menu.cartItemRemoved": "Removed {name}",
    "menu.cartEmptyAnnouncement": "Cart is empty",
    "menu.cartCleared": "Cart cleared",
    "accessibility.decreaseItemQuantity": "Decrease {name} quantity",
    "accessibility.increaseItemQuantity": "Increase {name} quantity",
    "accessibility.removeNamedItem": "Remove {name}",
  };
  const raw = table[key] ?? key;
  if (!params) return raw;
  return Object.entries(params).reduce(
    (acc, [name, value]) => acc.replaceAll(`{${name}}`, String(value)),
    raw,
  );
};

const twoItemCart: CartItem[] = [
  {
    itemType: "menu_item",
    name: "Market Tacos",
    price: 21,
    quantity: 3,
    menuItemId: "tacos",
  },
  {
    itemType: "menu_item",
    name: "Iced Tea",
    price: 4,
    quantity: 1,
    menuItemId: "tea",
  },
];

function CartHarness({
  initial = twoItemCart,
  onSubmitOrder = jest.fn(),
  onClearCart = jest.fn(),
}: {
  initial?: CartItem[];
  onSubmitOrder?: () => void;
  onClearCart?: () => void;
}) {
  const [items, setItems] = React.useState(initial);
  return (
    <CartModal
      isOpen
      onClose={jest.fn()}
      cart={items}
      orderLoading={false}
      quotePending={false}
      quoteError={null}
      quoteValid
      quoteBlocked={false}
      onUpdateQuantity={(index, quantity) => {
        setItems((prev) =>
          quantity <= 0
            ? prev.filter((_, i) => i !== index)
            : prev.map((item, i) =>
                i === index ? { ...item, quantity } : item,
              ),
        );
      }}
      onSubmitOrder={onSubmitOrder}
      onClearCart={onClearCart}
      t={interpolate}
      tableCode="T1"
      promotionPreview={{
        baseSubtotal: 67,
        discountTotal: 0,
        autoDiscountTotal: 0,
        promoDiscount: 0,
        discountedSubtotal: 67,
        ...zeroSettlement(67),
        applied: [],
      }}
      appliedPromo={null}
      onPromoApplied={jest.fn()}
      onPromoRemoved={jest.fn()}
      businessCurrencies={{
        default_currency: "USD",
        display_currency: "USD",
      }}
    />
  );
}

describe("CartModal clear-cart safety (#419)", () => {
  it("keeps Clear Cart the same compact size when armed and never submits", () => {
    const onSubmitOrder = jest.fn();
    const onClearCart = jest.fn();
    render(
      <CartHarness onSubmitOrder={onSubmitOrder} onClearCart={onClearCart} />,
    );
    const clear = screen.getByTestId("cart-clear");
    const place = screen.getByTestId("cart-place-order");
    expect(clear.className).toMatch(/w-12/);
    expect(clear.className).toMatch(/min-w-12/);
    expect(place).toBeEnabled();

    fireEvent.click(clear);
    expect(onClearCart).not.toHaveBeenCalled();
    expect(onSubmitOrder).not.toHaveBeenCalled();
    expect(clear).toHaveAttribute("aria-pressed", "true");
    expect(clear.className).toMatch(/w-12/);
    expect(clear.className).not.toMatch(/px-4/);
    expect(place).toBeDisabled();

    fireEvent.click(clear);
    expect(onClearCart).toHaveBeenCalledTimes(1);
    expect(onSubmitOrder).not.toHaveBeenCalled();
  });
});

const nyFiscalT = (key: string, params?: Record<string, string | number>) => {
  const copy: Record<string, string> = {
    "menu.fiscalCustomer.toggle": "Need a fiscal invoice with your details?",
    "menu.fiscalCustomer.hint":
      "Optional. Add your tax ID so we can issue a fiscal invoice (factura) in your name.",
    "menu.fiscalCustomer.docTypeLabel": "Document type",
    "menu.fiscalCustomer.docTypeNone": "Consumidor Final",
    "menu.fiscalCustomer.docTypeDni": "DNI",
    "menu.fiscalCustomer.docTypeCuit": "CUIT",
    "menu.fiscalCustomer.docTypeCuil": "CUIL",
    "menu.fiscalCustomer.taxConditionLabel": "Tax condition",
    "menu.fiscalCustomer.taxConsumidorFinal": "Consumidor Final",
    "menu.fiscalCustomer.taxResponsableInscripto": "Responsable Inscripto",
    "menu.fiscalCustomer.cuitHint":
      "A Factura A requires a CUIT/CUIL. Without one, we'll issue a Factura B.",
    "menu.placeOrder": "Place Order",
    "common.total": "Total",
  };
  return interpolate(key, params) === key ? (copy[key] ?? key) : interpolate(key, params);
};

const nyCartProps = {
  isOpen: true,
  onClose: jest.fn(),
  cart,
  orderLoading: false,
  quotePending: false,
  quoteError: null,
  quoteValid: true,
  quoteBlocked: false,
  onUpdateQuantity: jest.fn(),
  onSubmitOrder: jest.fn(),
  onClearCart: jest.fn(),
  t: nyFiscalT,
  tableCode: "EFDJQQ9J5B",
  promotionPreview: {
    baseSubtotal: 20,
    discountTotal: 0,
    autoDiscountTotal: 0,
    promoDiscount: 0,
    discountedSubtotal: 20,
    ...zeroSettlement(20),
    applied: [],
  },
  appliedPromo: null,
  onPromoApplied: jest.fn(),
  onPromoRemoved: jest.fn(),
  businessCurrencies: {
    default_currency: "USD",
    display_currency: "USD",
  },
} satisfies React.ComponentProps<typeof CartModal>;

function expandFiscalIfPresent() {
  const toggle = screen.queryByRole("button", {
    name: /Need a fiscal invoice with your details\?/,
  });
  if (!toggle) return;
  fireEvent.click(toggle);
  const tax = screen.queryByLabelText("Tax condition");
  if (tax) {
    fireEvent.change(tax, { target: { value: "responsable_inscripto" } });
  }
}

describe("CartModal fiscal identity country gate (#548)", () => {
  it("does not render Consumidor Final / CUIT / Factura A for a NY/USD venue", () => {
    render(<CartModal {...nyCartProps} country="US" />);

    expandFiscalIfPresent();

    expect(screen.queryAllByText("Consumidor Final")).toHaveLength(0);
    expect(screen.queryAllByText("CUIT")).toHaveLength(0);
    expect(screen.queryAllByText(/Factura A/)).toHaveLength(0);
    expect(
      screen.queryByText("Need a fiscal invoice with your details?"),
    ).not.toBeInTheDocument();
  });

  it("fails closed when the venue country is unknown", () => {
    render(<CartModal {...nyCartProps} />);

    expandFiscalIfPresent();

    expect(screen.queryAllByText("Consumidor Final")).toHaveLength(0);
    expect(screen.queryAllByText("CUIT")).toHaveLength(0);
    expect(screen.queryAllByText(/Factura A/)).toHaveLength(0);
  });

  it("still renders the AFIP identity form for an AR venue", () => {
    render(<CartModal {...nyCartProps} country="AR" />);

    fireEvent.click(
      screen.getByRole("button", {
        name: /Need a fiscal invoice with your details\?/,
      }),
    );
    fireEvent.change(screen.getByLabelText("Tax condition"), {
      target: { value: "responsable_inscripto" },
    });

    expect(screen.getAllByText("Consumidor Final").length).toBeGreaterThan(0);
    expect(screen.getByText("CUIT")).toBeInTheDocument();
    expect(screen.getByText(/Factura A/)).toBeInTheDocument();
  });
});

describe("CartModal named actions and live status (#424 #425)", () => {
  it("names quantity and remove controls after the row item", () => {
    render(<CartHarness />);
    expect(
      screen.getByRole("button", { name: "Increase Market Tacos quantity" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Decrease Iced Tea quantity" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Remove Iced Tea" }),
    ).toBeInTheDocument();
  });

  it("announces quantity changes and moves focus after removing a row", () => {
    render(<CartHarness />);
    fireEvent.click(
      screen.getByRole("button", { name: "Increase Market Tacos quantity" }),
    );
    expect(screen.getByTestId("guest-cart-live")).toHaveTextContent(
      "Market Tacos quantity is now 4",
    );

    const removeTea = screen.getByRole("button", { name: "Remove Iced Tea" });
    act(() => {
      removeTea.focus();
    });
    fireEvent.click(removeTea);
    expect(screen.getByTestId("guest-cart-live")).toHaveTextContent(
      "Removed Iced Tea",
    );
    expect(
      screen.queryByRole("button", { name: "Remove Iced Tea" }),
    ).toBeNull();
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Remove Market Tacos" }),
    );
  });

  it("announces an empty cart and focuses the heading after the last row", () => {
    render(
      <CartHarness
        initial={[
          {
            itemType: "menu_item",
            name: "Iced Tea",
            price: 4,
            quantity: 1,
            menuItemId: "tea",
          },
        ]}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Remove Iced Tea" }));
    expect(screen.getByTestId("guest-cart-live")).toHaveTextContent(
      "Cart is empty",
    );
    expect(document.activeElement).toBe(
      screen.getByRole("heading", { name: "Your Order" }),
    );
  });
});
