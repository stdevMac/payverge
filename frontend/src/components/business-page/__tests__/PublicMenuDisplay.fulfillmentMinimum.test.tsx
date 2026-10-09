/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import PublicMenuDisplay from "../PublicMenuDisplay";
import { computeGuestFulfillmentMinimumDelta } from "@/lib/guestPromotionAvailability";
import { asDollars } from "@/types/money";

const t = (key: string, values?: Record<string, unknown>) => {
  if (key === "accessibility.openItem") return `Open ${values?.name}`;
  if (key === "businessPage.fulfillmentStrip.addMore") {
    return `Add ${values?.amount} more to continue`;
  }
  if (key === "businessPage.fulfillmentStrip.minimumMet") {
    return "Minimum met";
  }
  return key;
};

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ t, currentLanguage: "en" }),
}));

const harvestBowl = {
  id: "harvest-bowl",
  name: "Harvest Bowl",
  description: "Roasted grains",
  price: asDollars(18.5),
  images: [],
  options: [],
  allergens: [],
  dietary_tags: [],
  is_available: true,
};

const icedTea = {
  id: "iced-tea",
  name: "Iced Tea",
  description: "House tea",
  price: asDollars(5),
  images: [],
  options: [],
  allergens: [],
  dietary_tags: [],
  is_available: true,
};

const fulfillmentContext = {
  business_id: 406,
  custom_url: "demo-kitchen",
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
    delivery_fee: asDollars(5.99),
    free_delivery_minimum: asDollars(50),
    minimum_order_amount: asDollars(15),
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
  customUrl: "demo-kitchen",
  businessId: 406,
  businessName: "Core Demo Kitchen",
  categories: [
    { name: "Mains", description: "", items: [harvestBowl] },
    { name: "Drinks", description: "", items: [icedTea] },
  ],
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

const addNamedItem = (name: string) => {
  fireEvent.click(screen.getByRole("button", { name: `Open ${name}` }));
  fireEvent.click(screen.getByTestId("add-to-cart-btn"));
};

describe("PublicMenuDisplay sticky delivery minimum (issue 406)", () => {
  beforeEach(() => window.sessionStorage.clear());

  it("exports the live-cart delta helper used by the sticky strip", () => {
    expect(computeGuestFulfillmentMinimumDelta(0, 15)).toBe(15);
    expect(computeGuestFulfillmentMinimumDelta(18.5, 15)).toBe(0);
    expect(computeGuestFulfillmentMinimumDelta(50, 15)).toBe(0);
  });

  it("updates minimum progress from the live cart, not quote.order_subtotal", async () => {
    render(<PublicMenuDisplay {...(baseProps as any)} />);

    const atZero = screen.getByTestId("fulfillment-minimum-progress");
    expect(atZero.textContent).toMatch(/15/);
    expect(atZero).toHaveTextContent("Add");
    expect(atZero).not.toHaveTextContent("Minimum met");

    addNamedItem("Harvest Bowl");

    await waitFor(() => {
      expect(screen.getByTestId("fulfillment-minimum-progress")).toHaveTextContent(
        "Minimum met",
      );
    });
    expect(screen.getByTestId("fulfillment-minimum-progress").textContent).not.toMatch(
      /15/,
    );

    fireEvent.click(screen.getByTestId("open-cart-btn"));
    expect(screen.getByTestId("checkout-btn")).not.toBeDisabled();
    expect(screen.getAllByText(/\$5\.99/).length).toBeGreaterThan(0);

    fireEvent.click(screen.getByLabelText("menu.close"));
    addNamedItem("Harvest Bowl");
    addNamedItem("Harvest Bowl");

    await waitFor(() => {
      expect(screen.getByTestId("fulfillment-minimum-progress")).toHaveTextContent(
        "Minimum met",
      );
    });
    fireEvent.click(screen.getByTestId("open-cart-btn"));
    expect(screen.queryByText(/\$5\.99/)).not.toBeInTheDocument();
  });
});
