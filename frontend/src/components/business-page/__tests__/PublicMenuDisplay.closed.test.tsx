/** @jest-environment jsdom */
/**
 * Tests for the closed-state menu hint and add-to-cart suppression.
 *
 * Task 5 (failing) → Task 6 (implementation) → pass.
 */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import PublicMenuDisplay from "../PublicMenuDisplay";

// ── Mocks ─────────────────────────────────────────────────────────────────────

const t = (k: string) => k;
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ t }),
}));

// ── Base props ────────────────────────────────────────────────────────────────

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
    reason_code: "quote_available",
    delivery_fee: 5,
    minimum_order_amount: 10,
    estimated_total_minutes: 30,
  },
  saved_at: new Date().toISOString(),
};

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("PublicMenuDisplay — closed-state hint + add-to-cart gating", () => {
  it("renders the closed-ordering hint when isOpen is false", () => {
    render(<PublicMenuDisplay {...(baseProps as any)} isOpen={false} />);
    expect(screen.getByTestId("menu-closed-hint")).toBeInTheDocument();
  });

  it("does not render the closed-ordering hint when isOpen is true", () => {
    render(<PublicMenuDisplay {...(baseProps as any)} isOpen={true} />);
    expect(screen.queryByTestId("menu-closed-hint")).toBeNull();
  });

  it("does not render add-to-cart buttons when closed", () => {
    render(<PublicMenuDisplay {...(baseProps as any)} isOpen={false} />);
    // Open the item modal by clicking the item card
    fireEvent.click(screen.getByText("Steak"));
    expect(screen.queryByTestId("add-to-cart-btn")).toBeNull();
    expect(screen.queryByTestId("add-to-cart-btn-no-delivery")).toBeNull();
  });

  it("renders no ordering CTA when open but native delivery is unavailable", () => {
    render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        isOpen
        deliveryAvailable={false}
      />,
    );
    fireEvent.click(screen.getByText("Steak"));
    expect(screen.queryByTestId("add-to-cart-btn")).toBeNull();
    expect(screen.queryByTestId("add-to-cart-btn-no-delivery")).toBeNull();
    expect(screen.queryByTestId("start-delivery-order-btn")).toBeNull();
  });

  it("renders the start-delivery CTA when delivery is on but no address is set", () => {
    const onEditFulfillment = jest.fn();
    render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        isOpen
        deliveryAvailable
        onEditFulfillment={onEditFulfillment}
      />,
    );
    fireEvent.click(screen.getByText("Steak"));
    expect(screen.queryByTestId("add-to-cart-btn")).toBeNull();
    const cta = screen.getByTestId("start-delivery-order-btn");
    fireEvent.click(cta);
    expect(onEditFulfillment).toHaveBeenCalledTimes(1);
  });

  it("renders the real add-to-cart button when a fulfillment context exists", () => {
    render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        isOpen
        deliveryAvailable
        fulfillmentContext={fakeContext}
      />,
    );
    fireEvent.click(screen.getByText("Steak"));
    expect(screen.getByTestId("add-to-cart-btn")).toBeInTheDocument();
  });

  it("allows a live delivery quote while the dining room is closed", () => {
    render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        isOpen={false}
        deliveryAvailable
        fulfillmentContext={fakeContext}
      />,
    );
    expect(screen.queryByTestId("menu-closed-hint")).toBeNull();
    fireEvent.click(screen.getByText("Steak"));
    expect(screen.getByTestId("add-to-cart-btn")).toBeInTheDocument();
  });

  it("hides offers strip and promo chips when dining is closed (no delivery path)", () => {
    render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        isOpen={false}
        offers={[
          {
            id: "o1",
            name: "Lunch 15% Off",
            is_active: true,
            applicable_to: "all",
          } as any,
        ]}
      />,
    );
    expect(screen.queryByTestId("storefront-offers-bundles")).toBeNull();
    // Offer name must not appear as merch strip or item promo chip.
    expect(screen.queryByText("Lunch 15% Off")).toBeNull();
  });

  it("suppresses Unavailable badges when closed (Closed Mode parity with table)", () => {
    render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        isOpen={false}
        categories={[
          {
            name: "Mains",
            description: "",
            items: [
              {
                id: "i1",
                name: "Steak",
                description: "Grilled",
                price: 25,
                images: ["https://example.com/s.jpg"],
                options: [],
                allergens: [],
                dietary_tags: [],
                is_available: false,
                orderability_state: "business_closed",
                item_type: "menu_item" as const,
                menu_item_id: "i1",
              },
            ],
          },
        ]}
      />,
    );
    expect(screen.queryByText("menu.unavailable")).toBeNull();
    expect(screen.getByText("Steak")).toBeInTheDocument();
  });

  it("shows offers merchandising when open", () => {
    render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        isOpen
        offers={[
          {
            id: "o1",
            name: "Lunch 15% Off",
            is_active: true,
            applicable_to: "all",
          } as any,
        ]}
      />,
    );
    expect(screen.getByTestId("storefront-offers-bundles")).toBeInTheDocument();
    // Strip + card chip may both show the offer name.
    expect(screen.getAllByText("Lunch 15% Off").length).toBeGreaterThanOrEqual(1);
  });
});

// Locale parity for call-waiter closed hint (CM6)
describe("serviceCall.closedHint locales", () => {
  const fs = require("fs");
  const path = require("path");
  const messagesDir = path.resolve(
    __dirname,
    "../../../i18n/guest-messages",
  );
  const locales = fs
    .readdirSync(messagesDir)
    .filter((f: string) => f.endsWith(".json") && !f.startsWith("."));

  it("ships closedHint in all 21 guest locales", () => {
    expect(locales.length).toBe(21);
    for (const file of locales) {
      const data = JSON.parse(
        fs.readFileSync(path.join(messagesDir, file), "utf8"),
      );
      const hint = data.serviceCall?.closedHint;
      expect(typeof hint).toBe("string");
      expect(hint.length).toBeGreaterThan(0);
    }
  });
});
