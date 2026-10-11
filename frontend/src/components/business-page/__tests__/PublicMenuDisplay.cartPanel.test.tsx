/** @jest-environment jsdom */
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

const fakeContext = {
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
    delivery_fee: 5,
    minimum_order_amount: 10,
    estimated_total_minutes: 30,
  },
  saved_at: new Date().toISOString(),
};

describe("PublicMenuDisplay — cart panel without fulfillment context", () => {
  it("shows a set-address CTA that reopens the delivery flow", () => {
    const onEditFulfillment = jest.fn();
    const { rerender } = render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        isOpen
        deliveryAvailable
        fulfillmentContext={fakeContext}
        onEditFulfillment={onEditFulfillment}
      />,
    );
    // Add an item while the context exists
    fireEvent.click(screen.getByText("Steak"));
    fireEvent.click(screen.getByTestId("add-to-cart-btn"));
    // Context gets cleared (e.g. via the fulfillment strip) but cart persists
    rerender(
      <PublicMenuDisplay
        {...(baseProps as any)}
        isOpen
        deliveryAvailable
        fulfillmentContext={null}
        onEditFulfillment={onEditFulfillment}
      />,
    );
    fireEvent.click(screen.getByTestId("open-cart-btn"));
    expect(screen.queryByTestId("checkout-btn")).toBeNull();
    fireEvent.click(screen.getByTestId("cart-set-address-btn"));
    expect(onEditFulfillment).toHaveBeenCalledTimes(1);
  });
});
