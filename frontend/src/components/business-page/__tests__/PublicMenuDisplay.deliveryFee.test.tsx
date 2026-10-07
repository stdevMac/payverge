/** @jest-environment jsdom */
// DLV-GUEST-3: the cart footer must apply the free-delivery threshold against
// the LIVE cart subtotal (like GuestDeliveryCheckout's effectiveBaseFee)
// instead of echoing the stale address-time quote fee, and the cart CTA must
// read as a review/checkout action since it only opens the checkout modal.
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import PublicMenuDisplay from "../PublicMenuDisplay";

const t = (k: string) => k;
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ t }),
}));

const baseProps = {
  customUrl: "test",
  businessId: 1,
  businessName: "Test",
  categories: [
    {
      name: "Mains",
      description: "",
      items: [
        {
          id: "i1",
          name: "Steak",
          description: "Grilled",
          price: 25,
          images: [],
          options: [],
          allergens: [],
          dietary_tags: [],
          is_available: true,
          item_type: "menu_item" as const,
          menu_item_id: "i1",
        },
      ],
    },
  ],
  offers: [],
  bundles: [],
  loading: false,
  designSettings: {
    primary_color: "#1a6b6a",
    secondary_color: "#2a8b8a",
    corner_radius: "medium",
    shadow_intensity: "subtle",
    font_family: "Inter",
  },
  businessCurrencies: { default_currency: "USD", display_currency: "USD" },
};

// Quote captured at address time (order_subtotal = 0): base fee 5.99,
// free above 50, minimum 10.
const fulfillmentContext = {
  business_id: 1,
  custom_url: "test",
  mode: "delivery" as const,
  delivery_address: {
    street: "1 Main St",
    city: "Springfield",
    country: "US",
    formatted_address: "1 Main St, Springfield, US",
  },
  contactless_delivery: false,
  leave_at_door: false,
  quote: {
    delivery_fee: 5.99,
    free_delivery_minimum: 50,
    minimum_order_amount: 10,
    order_subtotal: 0,
    estimated_total_minutes: 30,
  },
  saved_at: new Date().toISOString(),
};

const addSteakToCart = () => {
  fireEvent.click(screen.getByText("Steak"));
  fireEvent.click(screen.getByTestId("add-to-cart-btn"));
};

describe("PublicMenuDisplay — cart footer delivery fee (DLV-GUEST-3)", () => {
  it("keeps the quoted base fee while the cart is below the free-delivery threshold", () => {
    render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        isOpen
        deliveryAvailable
        fulfillmentContext={fulfillmentContext}
      />,
    );
    addSteakToCart(); // subtotal 25 < 50
    fireEvent.click(screen.getByTestId("open-cart-btn"));

    // Fee row shows the base fee; CTA total = 25 + 5.99.
    expect(screen.getAllByText(/\$5\.99/).length).toBeGreaterThan(0);
    expect(screen.getByTestId("checkout-btn")).toHaveTextContent("$30.99");
  });

  it("zeroes the fee and the CTA total once the live cart subtotal meets free_delivery_minimum", () => {
    render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        isOpen
        deliveryAvailable
        fulfillmentContext={fulfillmentContext}
      />,
    );
    addSteakToCart();
    addSteakToCart(); // cart subtotal is now >= free_delivery_minimum (50)
    fireEvent.click(screen.getByTestId("open-cart-btn"));

    // Fee is 0 — the stale quoted 5.99 must not appear anywhere in the footer,
    // and the CTA total equals the bare subtotal (what checkout will show).
    expect(screen.queryByText(/\$5\.99/)).not.toBeInTheDocument();
    const cta = screen.getByTestId("checkout-btn").textContent ?? "";
    const totalMatch = /\$([\d,.]+)/.exec(cta);
    expect(totalMatch).not.toBeNull();
    const ctaTotal = parseFloat((totalMatch as RegExpExecArray)[1].replace(/,/g, ""));
    // Subtotal is a multiple of the 25.00 item price with NO 5.99 fee added.
    expect(ctaTotal % 25).toBe(0);
    expect(ctaTotal).toBeGreaterThanOrEqual(50);
  });

  it("labels the cart CTA as a review action, not place-order (it only opens checkout)", () => {
    render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        isOpen
        deliveryAvailable
        fulfillmentContext={fulfillmentContext}
      />,
    );
    addSteakToCart();
    fireEvent.click(screen.getByTestId("open-cart-btn"));

    expect(screen.getByTestId("checkout-btn")).toHaveTextContent("menu.reviewOrder");
    expect(screen.getByTestId("checkout-btn")).not.toHaveTextContent("menu.placeOrder");
  });

  it("includes tax and service fee in the cart CTA total", () => {
    render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        isOpen
        deliveryAvailable
        taxRate={8}
        serviceFeeRate={5}
        fulfillmentContext={fulfillmentContext}
      />,
    );
    addSteakToCart();
    fireEvent.click(screen.getByTestId("open-cart-btn"));
    const cta = screen.getByTestId("checkout-btn").textContent ?? "";
    const totalMatch = /\$([\d,.]+)/.exec(cta);
    expect(totalMatch).not.toBeNull();
    const ctaTotal = parseFloat((totalMatch as RegExpExecArray)[1].replace(/,/g, ""));
    expect(ctaTotal).toBeGreaterThan(30.99);
    expect(screen.getByText("businessPage.checkout.tax")).toBeTruthy();
    expect(screen.getByText("businessPage.checkout.serviceFee")).toBeTruthy();
  });
});
