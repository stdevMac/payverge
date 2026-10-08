/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";

import { GuestMenuViews } from "./GuestMenuViews";
import type { Business, MenuCategory } from "../../api/business";

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    currentLanguage: "en",
    t: (key: string) => key,
  }),
}));

jest.mock("./ImageCarousel", () => ({
  ImageCarousel: () => <div data-testid="carousel" />,
}));

jest.mock("../common/CurrencyConverter", () => ({
  CurrencyPrice: ({ amount }: { amount: number }) => <span>{amount}</span>,
}));

const categories = [
  {
    id: "mains",
    name: "Mains",
    items: [
      {
        id: "item-1",
        name: "Harvest Bowl",
        description: "Vegetables and grains",
        price: 18,
        is_available: true,
        image: "/media/menu/harvest.jpg",
      },
    ],
  },
] as unknown as MenuCategory[];

const business = { name: "Testaurant" } as unknown as Business;

const renderView = (viewMode: "grid" | "category-tabs") =>
  render(
    <GuestMenuViews
      categories={categories}
      business={business}
      tableCode="T1"
      currentBill={null}
      viewMode={viewMode}
      activeCategory={0}
      onAddToCart={jest.fn()}
      onItemClick={jest.fn()}
      isOrderingEnabled
    />,
  );

describe("GuestMenuViews item photos", () => {
  it("sizes the grid card photo for the 2/3/4 column layout", () => {
    renderView("grid");
    expect(screen.getByRole("img", { name: "Harvest Bowl" })).toHaveAttribute(
      "sizes",
      "(min-width: 1024px) 25vw, (min-width: 640px) 33vw, 50vw",
    );
  });

  it("sizes the list thumbnail as a fixed 6rem box", () => {
    renderView("category-tabs");
    expect(screen.getByRole("img", { name: "Harvest Bowl" })).toHaveAttribute(
      "sizes",
      "6rem",
    );
  });
});
