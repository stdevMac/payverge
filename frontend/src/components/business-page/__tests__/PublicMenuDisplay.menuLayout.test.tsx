/** @jest-environment jsdom */
import React from "react";
import { render } from "@testing-library/react";
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

describe("PublicMenuDisplay — menu_layout (grid | list)", () => {
  it("renders the multi-column grid container by default (grid)", () => {
    const { container } = render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        designSettings={{ ...baseProps.designSettings, menu_layout: "grid" }}
      />,
    );
    expect(container.innerHTML).toMatch(/grid-cols-1 md:grid-cols-2 lg:grid-cols-3/);
  });

  it("renders a single-column stack with side-by-side cards when menu_layout is list", () => {
    const { container } = render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        designSettings={{ ...baseProps.designSettings, menu_layout: "list" }}
      />,
    );
    // List container: flex-col stack, NOT the 3-col grid.
    expect(container.innerHTML).toMatch(/flex flex-col gap-4/);
    expect(container.innerHTML).not.toMatch(/grid-cols-1 md:grid-cols-2 lg:grid-cols-3/);
    // List card goes side-by-side on md.
    expect(container.innerHTML).toMatch(/md:flex md:items-stretch/);
  });
});
