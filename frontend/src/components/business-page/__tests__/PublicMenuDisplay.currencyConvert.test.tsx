/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import PublicMenuDisplay from "../PublicMenuDisplay";
import { convertAmount } from "@/api/currency";

jest.mock("@/api/currency", () => {
  const actual = jest.requireActual("@/api/currency");
  return {
    ...actual,
    convertAmount: jest.fn(async (amount: number) => ({
      converted_amount: amount * 2,
    })),
  };
});

const t = (k: string) => k;
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ t, currentLanguage: "en" }),
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
  businessCurrencies: { default_currency: "ARS", display_currency: "USD" },
};

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
    delivery_fee: 5,
    free_delivery_minimum: 100,
    minimum_order_amount: 10,
    estimated_total_minutes: 30,
  },
  saved_at: new Date().toISOString(),
};

describe("PublicMenuDisplay convert-then-format (#499)", () => {
  it("converts add-to-cart, view-cart, line, subtotal, fee, and review-order amounts", async () => {
    render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        isOpen
        deliveryAvailable
        fulfillmentContext={fulfillmentContext}
      />,
    );

    fireEvent.click(screen.getByText("Steak"));

    await waitFor(() => {
      expect(screen.getByTestId("add-to-cart-btn")).toHaveTextContent("$50.00");
    });
    expect(screen.getByTestId("add-to-cart-btn")).not.toHaveTextContent(
      "$25.00",
    );

    fireEvent.click(screen.getByTestId("add-to-cart-btn"));

    await waitFor(() => {
      expect(screen.getByTestId("open-cart-btn")).toHaveTextContent("$50.00");
    });
    expect(screen.getByTestId("open-cart-btn")).not.toHaveTextContent("$25.00");

    fireEvent.click(screen.getByTestId("open-cart-btn"));

    await waitFor(() => {
      expect(convertAmount).toHaveBeenCalled();
      expect(screen.getByTestId("cart-panel")).toHaveTextContent("$50.00");
      expect(screen.getByTestId("cart-panel")).toHaveTextContent("$10.00");
      expect(screen.getByTestId("checkout-btn")).toHaveTextContent("$60.00");
    });
    expect(screen.getByTestId("checkout-btn")).not.toHaveTextContent("$30.00");
    expect(screen.getByTestId("cart-panel")).not.toHaveTextContent("$5.00");
    expect(screen.getByTestId("cart-panel")).not.toHaveTextContent("$25.00");
  });
});
