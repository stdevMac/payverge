/** @jest-environment jsdom */
import React from "react";
import fs from "fs";
import path from "path";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import PublicMenuDisplay from "../PublicMenuDisplay";
import type { Bundle, MenuCategory, Offer } from "@/api/business";
import { asDollars } from "@/types/money";

const t = (key: string, values?: Record<string, unknown>) => {
  if (key === "accessibility.openItem") return `Open ${values?.name}`;
  if (key === "menu.search.noResultsTitle") return "No items found";
  if (key === "menu.search.noResultsDescription") {
    return "Try adjusting your search or browse all categories.";
  }
  if (key === "menu.filters.clear") return "Clear";
  return key;
};

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ t, currentLanguage: "en" }),
}));

const steak: MenuCategory["items"][number] = {
  id: "demo-steak",
  name: "Steak Plate",
  description: "Charred ribeye, herb butter",
  price: asDollars(42),
  images: [],
  options: [],
  allergens: [],
  dietary_tags: [],
  is_available: true,
};

const cocktail: MenuCategory["items"][number] = {
  id: "demo-cocktail",
  name: "Demo Spritz",
  description: "Aperitivo highball",
  price: asDollars(12),
  images: [],
  options: [],
  allergens: [],
  dietary_tags: [],
  is_available: true,
};

const dessert: MenuCategory["items"][number] = {
  id: "demo-dessert",
  name: "Chocolate Tart",
  description: "Dark chocolate tart",
  price: asDollars(11),
  images: [],
  options: [],
  allergens: [],
  dietary_tags: [],
  is_available: true,
};

const harvestBowl: MenuCategory["items"][number] = {
  id: "harvest-bowl",
  name: "Harvest Bowl",
  description: "Roasted grains and greens",
  price: asDollars(18.5),
  images: [],
  options: [],
  allergens: [],
  dietary_tags: [],
  is_available: true,
};

const categories: MenuCategory[] = [
  { name: "Mains", description: "", items: [steak, harvestBowl] },
  { name: "Drinks", description: "", items: [cocktail] },
  { name: "Desserts", description: "", items: [dessert] },
];

const steakOffer = {
  id: 78,
  name: "$5 Off the Steak Plate",
  description: "Dinner discount on the steak",
  is_active: true,
  applicable_to: "item",
  target_id: "demo-steak",
  discount_type: "fixed",
  discount_value: 5,
} as Offer;

const lunchOffer = {
  id: 2,
  name: "Weekday Lunch 15% Off",
  description: "All-menu lunch special",
  is_active: true,
  applicable_to: "all",
  discount_type: "percentage",
  discount_value: 15,
} as Offer;

const dateNight = {
  id: 9,
  name: "Date Night for Two",
  description: "A steak, two spritzes, and a tart to share.",
  price: asDollars(60),
  is_active: true,
  items: [
    { menu_item_id: "demo-steak", name: "Steak Plate", quantity: 1 },
    { menu_item_id: "demo-cocktail", name: "Demo Spritz", quantity: 2 },
    { menu_item_id: "demo-dessert", name: "Chocolate Tart", quantity: 1 },
  ],
} as Bundle;

const openOrderability = {
  "demo-steak": { state: "available" as const, orderable: true },
  "demo-cocktail": { state: "available" as const, orderable: true },
  "demo-dessert": { state: "available" as const, orderable: true },
  "harvest-bowl": { state: "available" as const, orderable: true },
};

const baseProps = {
  customUrl: "demo",
  businessId: 345,
  businessName: "Demo Lounge",
  categories,
  offers: [steakOffer, lunchOffer],
  bundles: [dateNight],
  itemOrderability: openOrderability,
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

const typeSearch = async (value: string) => {
  fireEvent.change(screen.getByPlaceholderText("menu.search.placeholder"), {
    target: { value },
  });
  await waitFor(() => {
    expect(
      (screen.getByPlaceholderText("menu.search.placeholder") as HTMLInputElement)
        .value,
    ).toBe(value);
  });
};

describe("PublicMenuDisplay storefront merchandising", () => {
  beforeEach(() => window.sessionStorage.clear());

  it("wires the shared sellable-target helpers used by /t (issue 345)", () => {
    const source = fs.readFileSync(
      path.resolve(__dirname, "../PublicMenuDisplay.tsx"),
      "utf8",
    );
    expect(source).toContain("filterGuestSellableOffers");
    expect(source).toContain("filterGuestSellableBundles");
    expect(source).toContain("resolveGuestBundleChildName");
    expect(source).toContain("shouldHideGuestPromoMerchForSearch");
    expect(source).toContain("computeGuestFulfillmentMinimumDelta");
    expect(source).not.toMatch(
      /showPromoMerch && \(offers\.length > 0 \|\| bundles\.length > 0\)/,
    );
  });

  it("hides $5 Off the Steak Plate and Date Night when steak is 86'd", () => {
    render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        itemOrderability={{
          ...openOrderability,
          "demo-steak": { state: "inventory_out", orderable: false },
        }}
      />,
    );

    expect(screen.queryByText("$5 Off the Steak Plate")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Open Date Night for Two" }),
    ).not.toBeInTheDocument();
    expect(screen.getAllByText("Weekday Lunch 15% Off").length).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: "Open Steak Plate" })).toBeInTheDocument();
    expect(screen.getByText("menu.unavailable")).toBeInTheDocument();
  });

  it("resolves bundle children from the localized catalog, not the English snapshot (issue 398)", () => {
    const spanishCategories: MenuCategory[] = [
      {
        name: "Principales",
        description: "",
        items: [
          { ...steak, name: "Plato de bistec" },
          { ...harvestBowl, name: "Bowl de cosecha" },
        ],
      },
      {
        name: "Bebidas",
        description: "",
        items: [{ ...cocktail, name: "Spritz demo" }],
      },
      {
        name: "Postres",
        description: "",
        items: [{ ...dessert, name: "Tarta de chocolate" }],
      },
    ];
    const spanishBundle = {
      ...dateNight,
      name: "Cita romántica para dos",
      description:
        "Un plato de carne, dos cócteles spritz y una tarta de chocolate para compartir.",
    };

    render(
      <PublicMenuDisplay
        {...(baseProps as any)}
        businessId={398}
        categories={spanishCategories}
        offers={[]}
        bundles={[spanishBundle]}
      />,
    );

    const merch = screen.getByTestId("storefront-offers-bundles");
    expect(merch).toHaveTextContent("Cita romántica para dos");
    expect(merch).toHaveTextContent("1× Plato de bistec");
    expect(merch).toHaveTextContent("2× Spritz demo");
    expect(merch).toHaveTextContent("1× Tarta de chocolate");
    expect(merch).not.toHaveTextContent("Steak Plate");
    expect(merch).not.toHaveTextContent("Demo Spritz");
    expect(merch).not.toHaveTextContent("Chocolate Tart");
    expect(screen.getByRole("button", { name: "Open Plato de bistec" })).toBeInTheDocument();
  });

  it("hides leftover merch beside No items found and offers a clear action (issue 417)", async () => {
    render(<PublicMenuDisplay {...(baseProps as any)} businessId={417} />);

    expect(screen.getByTestId("storefront-offers-bundles")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Open Date Night for Two" }),
    ).toBeInTheDocument();

    await typeSearch("zzzz-no-match");
    await waitFor(() => {
      expect(screen.getByText("No items found")).toBeInTheDocument();
    });
    expect(screen.queryByTestId("storefront-offers-bundles")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Open Date Night for Two" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("$5 Off the Steak Plate")).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId("menu-search-clear"));
    await waitFor(() => {
      expect(screen.getByTestId("storefront-offers-bundles")).toBeInTheDocument();
    });
    expect(
      screen.getByRole("button", { name: "Open Date Night for Two" }),
    ).toBeInTheDocument();
  });

  it("filters item name, description, bundle, child, and offer queries", async () => {
    render(<PublicMenuDisplay {...(baseProps as any)} businessId={4171} />);

    await typeSearch("Harvest Bowl");
    await waitFor(() => {
      expect(
        screen.queryByRole("button", { name: "Open Steak Plate" }),
      ).not.toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Open Harvest Bowl" })).toBeInTheDocument();
    expect(screen.queryByTestId("storefront-offers-bundles")).not.toBeInTheDocument();

    await typeSearch("Roasted grains");
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Open Harvest Bowl" })).toBeInTheDocument();
    });

    await typeSearch("Date Night");
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Open Date Night for Two" }),
      ).toBeInTheDocument();
    });

    await typeSearch("Chocolate Tart");
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Open Date Night for Two" }),
      ).toBeInTheDocument();
    });

    await typeSearch("Weekday Lunch");
    await waitFor(() => {
      expect(screen.getAllByText("Weekday Lunch 15% Off").length).toBeGreaterThan(0);
    });
    expect(screen.queryByText("$5 Off the Steak Plate")).not.toBeInTheDocument();
  });
});
