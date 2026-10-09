/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
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

describe("PublicMenuDisplay — show_images / show_descriptions design toggles", () => {
  it("hides item media when show_images is false", () => {
    render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        designSettings={{ ...baseProps.designSettings, show_images: false }}
      />,
    );
    // MenuItemMedia renders the no-image placeholder label for items without
    // photos; with images disabled the media block must not render at all.
    expect(screen.queryByText("menu.noImage")).toBeNull();
  });

  it("hides item descriptions when show_descriptions is false", () => {
    render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        designSettings={{ ...baseProps.designSettings, show_descriptions: false }}
      />,
    );
    expect(screen.queryByText("Grilled")).toBeNull();
  });

  it("shows media and descriptions by default", () => {
    render(<PublicMenuDisplay {...(baseProps as any)} />);
    expect(screen.getByText("menu.noImage")).toBeInTheDocument();
    expect(screen.getByText("Grilled")).toBeInTheDocument();
  });
});
