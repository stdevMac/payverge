/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";

import PublicMenuDisplay from "../PublicMenuDisplay";
import type { MenuCategory } from "@/api/business";
import { asDollars } from "@/types/money";

const t = (key: string, values?: Record<string, unknown>) =>
  key === "accessibility.openItem" ? `Open ${values?.name}` : key;
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ t, currentLanguage: "en" }),
}));

const categories: MenuCategory[] = [
  {
    name: "Mains",
    description: "",
    items: [
      {
        id: "steak",
        name: "Steak Plate",
        description: "Grilled",
        price: asDollars(25),
        images: [],
        options: [],
        allergens: [],
        dietary_tags: [],
        is_available: true,
      },
    ],
  },
];

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
    eligible: true,
    reason_code: "quote_available" as const,
    message: "",
    order_subtotal: asDollars(0),
    delivery_fee: asDollars(5),
    free_delivery_minimum: asDollars(0),
    minimum_order_amount: asDollars(10),
    meets_minimum: false,
    estimated_prep_time: 15,
    estimated_delivery_minutes: 15,
    estimated_total_minutes: 30,
    fulfillment_mode: "delivery",
    partner_fallback_available: false,
    external_partner_links: [],
  },
  saved_at: new Date().toISOString(),
};

const baseProps = {
  customUrl: "test",
  businessId: 1,
  businessName: "Test",
  categories,
  offers: [],
  bundles: [],
  loading: false,
  isOpen: true,
  deliveryAvailable: true,
  fulfillmentContext,
  designSettings: {
    primary_color: "#1a6b6a",
    secondary_color: "#2a8b8a",
    corner_radius: "medium",
    shadow_intensity: "subtle",
    font_family: "Inter",
  },
  businessCurrencies: { default_currency: "USD", display_currency: "USD" },
};

describe("PublicMenuDisplay authoritative orderability", () => {
  beforeEach(() => window.sessionStorage.clear());

  it("blocks a regular item when the backend projection overrides its manual available flag", () => {
    render(
      <PublicMenuDisplay
        {...baseProps}
        itemOrderability={{
          steak: { state: "inventory_out", orderable: false },
        }}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Open Steak Plate" }));

    expect(screen.getByText("menu.itemUnavailable")).toBeInTheDocument();
    expect(screen.queryByTestId("add-to-cart-btn")).not.toBeInTheDocument();
  });

  it("allows a regular item when the backend projection overrides its manual unavailable flag", () => {
    render(
      <PublicMenuDisplay
        {...baseProps}
        categories={[
          {
            ...categories[0],
            items: [{ ...categories[0].items[0], is_available: false }],
          },
        ]}
        itemOrderability={{
          steak: { state: "available", orderable: true },
        }}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Open Steak Plate" }));

    expect(screen.queryByText("menu.itemUnavailable")).not.toBeInTheDocument();
    expect(screen.getByTestId("add-to-cart-btn")).toBeInTheDocument();
  });

  it("labels an orderable inventory warning as low stock on the card and details dialog", () => {
    render(
      <PublicMenuDisplay
        {...baseProps}
        itemOrderability={{
          steak: { state: "inventory_warning", orderable: true },
        }}
      />,
    );

    expect(screen.getByText("menu.lowStock")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Open Steak Plate" }));

    expect(
      within(screen.getByRole("dialog", { name: "Steak Plate" })).getByText(
        "menu.lowStock",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText("menu.filters.available")).not.toBeInTheDocument();
  });

  it("hides a bundle when any child item is not orderable", () => {
    render(
      <PublicMenuDisplay
        {...baseProps}
        bundles={[
          {
            id: 9,
            name: "Steak Dinner",
            description: "Dinner bundle",
            price: asDollars(30),
            items: [{ menu_item_id: "steak", quantity: 1 }],
            is_active: true,
          },
        ]}
        itemOrderability={{
          steak: { state: "inventory_out", orderable: false },
        }}
      />,
    );

    expect(
      screen.queryByRole("button", { name: "Open Steak Dinner" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("Steak Dinner")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open Steak Plate" })).toBeInTheDocument();
    expect(screen.queryByTestId("add-to-cart-btn")).not.toBeInTheDocument();
  });

  it("removes a restored cart line that the current projection rejects", async () => {
    window.sessionStorage.setItem(
      "payverge_public_cart_1",
      JSON.stringify([
        {
          key: "steak",
          menu_item_name: "Steak Plate",
          menu_item_id: "steak",
          quantity: 1,
          unit_price: 25,
          item_type: "menu_item",
          options: [],
        },
      ]),
    );

    render(
      <PublicMenuDisplay
        {...baseProps}
        itemOrderability={{
          steak: { state: "inventory_out", orderable: false },
        }}
      />,
    );

    await waitFor(() =>
      expect(screen.queryByTestId("open-cart-btn")).not.toBeInTheDocument(),
    );
    await waitFor(() =>
      expect(window.sessionStorage.getItem("payverge_public_cart_1")).toBeNull(),
    );
  });

  it("preserves a restored cart when the menu fetch did not produce an authoritative snapshot", async () => {
    const persistedCart = JSON.stringify([
      {
        key: "legacy-special",
        menu_item_name: "Legacy Special",
        menu_item_id: "legacy-special",
        quantity: 1,
        unit_price: 25,
        item_type: "menu_item",
        options: [],
      },
    ]);
    window.sessionStorage.setItem("payverge_public_cart_1", persistedCart);

    render(
      <PublicMenuDisplay
        {...baseProps}
        itemOrderability={{}}
        menuSnapshotAuthoritative={false}
      />,
    );

    await waitFor(() => expect(screen.getByTestId("open-cart-btn")).toBeInTheDocument());
    expect(window.sessionStorage.getItem("payverge_public_cart_1")).toBe(persistedCart);
  });
});
