/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";

import { GuestMenuViews } from "./GuestMenuViews";
import type { Business, MenuCategory } from "../../api/business";

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    currentLanguage: "en",
    t: (key: string, params?: Record<string, string | number>) => {
      const table: Record<string, string> = {
        "menu.add": "Add",
        "menu.addToCart": "Add to Cart",
        "menu.itemAdded": "Added {quantity}!",
        "menu.itemAddedNamed": "Added {quantity} {name}!",
        "menu.quantityQ": "Quantity:",
        "menu.close": "Close",
        "menu.totalLabel": "Total",
        "accessibility.openItem": "Open {name}",
        "accessibility.addItemToCart": "Add {name} to cart",
        "accessibility.addItemToOrder": "Add {name} to order",
        "accessibility.increaseItemQuantity": "Increase {name} quantity",
        "accessibility.decreaseItemQuantity": "Decrease {name} quantity",
      };
      const raw = table[key] ?? key;
      if (!params) return raw;
      return Object.entries(params).reduce(
        (acc, [name, value]) => acc.replaceAll(`{${name}}`, String(value)),
        raw,
      );
    },
  }),
}));

jest.mock("./ImageCarousel", () => ({
  ImageCarousel: () => <div data-testid="carousel" />,
}));

jest.mock("../common/CurrencyConverter", () => ({
  CurrencyPrice: ({ amount }: { amount: number }) => <span>{amount}</span>,
}));

jest.mock("next/image", () => ({
  __esModule: true,
  // eslint-disable-next-line @next/next/no-img-element
  default: ({ alt = "", ...props }: any) => <img alt={alt} {...props} />,
}));

const categories = [
  {
    id: "mains",
    name: "Mains",
    items: [
      {
        id: "item-1",
        name: "Market Tacos",
        description: "Street tacos",
        price: 21,
        is_available: true,
      },
      {
        id: "item-2",
        name: "Iced Tea",
        description: "House tea",
        price: 4,
        is_available: true,
      },
    ],
  },
] as unknown as MenuCategory[];

const business = { name: "Testaurant" } as unknown as Business;

const renderView = (
  viewMode: "detailed" | "compact" | "grid" | "category-tabs",
  onAddToCart = jest.fn(),
) =>
  render(
    <GuestMenuViews
      categories={categories}
      business={business}
      tableCode="T1"
      currentBill={null}
      viewMode={viewMode}
      activeCategory={0}
      onAddToCart={onAddToCart}
      onItemClick={jest.fn()}
      isOrderingEnabled
    />,
  );

describe("GuestMenuViews named add controls (#424)", () => {
  it.each(["detailed", "compact", "grid", "category-tabs"] as const)(
    "includes the item name on %s add controls",
    (viewMode) => {
      renderView(viewMode);
      expect(
        screen.getAllByRole("button", { name: /Add Market Tacos/i }).length,
      ).toBeGreaterThan(0);
    },
  );
});

describe("GuestMenuViews add confirmation (#426)", () => {
  it("reports the committed dialog quantity, not a hardcoded 1", () => {
    const onAddToCart = jest.fn();
    renderView("detailed", onAddToCart);

    fireEvent.click(screen.getAllByRole("button", { name: "Open Market Tacos" })[0]);
    const increase = screen.getByRole("button", {
      name: "Increase Market Tacos quantity",
    });
    fireEvent.click(increase);
    fireEvent.click(increase);
    const addButtons = screen.getAllByRole("button", {
      name: "Add Market Tacos to cart",
    });
    fireEvent.click(addButtons[addButtons.length - 1]);

    expect(onAddToCart).toHaveBeenCalledWith(
      "Market Tacos",
      21,
      3,
      "",
      [],
      expect.objectContaining({ itemType: "menu_item" }),
    );
    expect(screen.getByTestId("guest-menu-item-status")).toHaveTextContent(
      "Added 3 Market Tacos!",
    );
    expect(screen.getAllByText("Added 3!").length).toBeGreaterThan(0);
    expect(screen.queryByText("Added 1!")).not.toBeInTheDocument();
  });

  it("does not show Added success when addToCart returns false", () => {
    const onAddToCart = jest.fn(() => false);
    renderView("detailed", onAddToCart);
    fireEvent.click(screen.getAllByRole("button", { name: /Add Iced Tea/i })[0]);
    expect(onAddToCart).toHaveBeenCalled();
    expect(screen.queryByText("Added 1!")).not.toBeInTheDocument();
    expect(screen.getByTestId("guest-menu-item-status")).toHaveTextContent("");
  });

  it("still reports one unit for a card quick-add", () => {
    const onAddToCart = jest.fn();
    renderView("detailed", onAddToCart);
    fireEvent.click(screen.getAllByRole("button", { name: /Add Market Tacos/i })[0]);
    expect(onAddToCart).toHaveBeenCalledWith(
      "Market Tacos",
      21,
      1,
      "",
      [],
      expect.any(Object),
    );
    expect(screen.getByTestId("guest-menu-item-status")).toHaveTextContent(
      "Added 1 Market Tacos!",
    );
  });
});
