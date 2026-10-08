/** @jest-environment jsdom */
import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";

import PublicMenuDisplay from "../PublicMenuDisplay";

const t = (key: string, values?: Record<string, unknown>) => {
  if (key === "accessibility.openItem") return `Open ${values?.name}`;
  return key;
};

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
          id: "steak",
          name: "Steak Plate",
          description: "Grilled",
          price: 25,
          images: ["/steak-1.jpg", "/steak-2.jpg"],
          options: [],
          allergens: [],
          dietary_tags: [],
          is_available: true,
        },
      ],
    },
  ],
  offers: [],
  bundles: [],
  loading: false,
  isOpen: true,
  designSettings: {
    primary_color: "#1a6b6a",
    secondary_color: "#2a8b8a",
    corner_radius: "medium",
    shadow_intensity: "subtle",
    font_family: "Inter",
  },
  businessCurrencies: { default_currency: "USD", display_currency: "USD" },
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
    reason_code: "quote_available" as const,
    order_subtotal: 25,
    delivery_fee: 5,
    free_delivery_minimum: 0,
    minimum_order_amount: 10,
    estimated_total_minutes: 30,
  },
  saved_at: new Date().toISOString(),
};

describe("PublicMenuDisplay accessibility", () => {
  beforeEach(() => window.sessionStorage.clear());

  it("uses a dedicated details control instead of nesting carousel buttons in a button-like card", () => {
    render(<PublicMenuDisplay {...(baseProps as any)} />);

    const details = screen.getByRole("button", { name: "Open Steak Plate" });
    const card = details.closest("article");
    expect(card).not.toBeNull();
    expect(card).not.toHaveAttribute("role", "button");
    expect(card).not.toHaveAttribute("tabindex");

    const carouselControls = within(card as HTMLElement).getAllByRole("button", {
      name: /Image \d of 2/,
    });
    expect(carouselControls).toHaveLength(2);
    carouselControls.forEach((control) => {
      expect(details).not.toContainElement(control);
    });
  });

  it("exposes item details as a modal dialog and restores trigger focus on Escape", async () => {
    render(<PublicMenuDisplay {...(baseProps as any)} />);

    const details = screen.getByRole("button", { name: "Open Steak Plate" });
    act(() => details.focus());
    fireEvent.click(details);

    const dialog = screen.getByRole("dialog", { name: "Steak Plate" });
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(
      screen.queryByRole("button", { name: "Open Steak Plate" }),
    ).not.toBeInTheDocument();

    fireEvent.keyDown(dialog, { key: "Escape", code: "Escape" });
    await waitFor(() => expect(dialog).not.toBeInTheDocument());
    expect(details).toHaveFocus();
  });

  it("gives the header dismiss control a distinct name from the footer close action", () => {
    render(<PublicMenuDisplay {...(baseProps as any)} />);

    fireEvent.click(screen.getByRole("button", { name: "Open Steak Plate" }));
    const dialog = screen.getByRole("dialog", { name: "Steak Plate" });

    expect(
      within(dialog).getByRole("button", {
        name: "menu.dismissItemDetails",
      }),
    ).toBeInTheDocument();
    expect(
      within(dialog).getAllByRole("button", { name: "menu.close" }),
    ).toHaveLength(1);
  });

  it("exposes the cart panel as a modal dialog and restores opener focus", async () => {
    render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        deliveryAvailable
        fulfillmentContext={fulfillmentContext}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Open Steak Plate" }));
    fireEvent.click(screen.getByTestId("add-to-cart-btn"));
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: "Steak Plate" }),
      ).not.toBeInTheDocument(),
    );

    const opener = await screen.findByTestId("open-cart-btn");
    act(() => opener.focus());
    fireEvent.click(opener);

    const dialog = screen.getByRole("dialog", { name: "menu.cart (1)" });
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(dialog).toHaveClass("!m-0");

    fireEvent.keyDown(dialog, { key: "Escape", code: "Escape" });
    await waitFor(() => expect(dialog).not.toBeInTheDocument());
    expect(opener).toHaveFocus();
  });
});
