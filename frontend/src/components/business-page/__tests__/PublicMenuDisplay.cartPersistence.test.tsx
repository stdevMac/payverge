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

describe("PublicMenuDisplay — cart persistence", () => {
  beforeEach(() => window.sessionStorage.clear());

  it("restores the cart from sessionStorage after an unmount/remount", () => {
    const props = {
      ...(baseProps as any),
      isOpen: true,
      deliveryAvailable: true,
      fulfillmentContext: fakeContext,
    };
    const { unmount } = render(<PublicMenuDisplay {...props} />);
    fireEvent.click(screen.getByText("Steak"));
    fireEvent.click(screen.getByTestId("add-to-cart-btn"));
    expect(screen.getByTestId("open-cart-btn")).toBeInTheDocument();
    unmount();

    render(<PublicMenuDisplay {...props} />);
    expect(screen.getByTestId("open-cart-btn")).toBeInTheDocument();
  });

  it("clears the stored cart when the cart is emptied", () => {
    const props = {
      ...(baseProps as any),
      isOpen: true,
      deliveryAvailable: true,
      fulfillmentContext: fakeContext,
    };
    render(<PublicMenuDisplay {...props} />);
    fireEvent.click(screen.getByText("Steak"));
    fireEvent.click(screen.getByTestId("add-to-cart-btn"));
    fireEvent.click(screen.getByTestId("open-cart-btn"));
    fireEvent.click(screen.getByText("menu.clearCart"));
    expect(window.sessionStorage.getItem("payverge_public_cart_1")).toBeNull();
  });
});
