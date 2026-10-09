/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";

import { MobileMenuItem } from "./MobileMenuItem";
import type { MenuItem } from "../../api/business";

jest.mock("./ImageCarousel", () => ({
  ImageCarousel: () => <div data-testid="carousel" />,
}));

jest.mock("../common/CurrencyConverter", () => ({
  CurrencyPrice: ({ amount }: { amount: number }) => <span>{amount}</span>,
}));

jest.mock("@nextui-org/react", () => ({
  Button: ({ children, onPress, onClick, "aria-label": ariaLabel }: any) => (
    <button type="button" onClick={onClick ?? onPress} aria-label={ariaLabel}>
      {children}
    </button>
  ),
  Chip: ({ children }: any) => <span>{children}</span>,
  Badge: ({ children }: any) => <span>{children}</span>,
}));

const item: MenuItem = {
  id: "m1",
  name: "Margherita Pizza",
  description: "Classic",
  price: 12,
  is_available: true,
} as MenuItem;

const t = (key: string) => key;

describe("MobileMenuItem named actions (#424)", () => {
  it("includes the item name on the add control", () => {
    const tNamed = (key: string, params?: Record<string, string | number>) => {
      if (key === "accessibility.addItemToCart") {
        return `Add ${params?.name} to cart`;
      }
      return key;
    };
    render(
      <MobileMenuItem
        item={item}
        isAnimating={false}
        isAdded={false}
        quantity={0}
        defaultCurrency="USD"
        displayCurrency="USD"
        locale="en"
        onItemClick={jest.fn()}
        onAddToCart={jest.fn()}
        isOrderingEnabled
        t={tNamed}
      />,
    );
    expect(
      screen.getByRole("button", { name: "Add Margherita Pizza to cart" }),
    ).toBeInTheDocument();
  });
});

describe("MobileMenuItem keyboard access", () => {
  it("labels an orderable inventory warning as low stock, not available", () => {
    render(
      <MobileMenuItem
        item={{ ...item, orderability_state: "inventory_warning" }}
        isAnimating={false}
        isAdded={false}
        quantity={0}
        defaultCurrency="USD"
        displayCurrency="USD"
      locale="en"
        onItemClick={jest.fn()}
        t={t}
      />,
    );

    expect(screen.getByText("menu.lowStock")).toBeInTheDocument();
    expect(screen.queryByText("menu.filters.available")).not.toBeInTheDocument();
  });

  it("exposes a dedicated details button without nesting interactive controls", () => {
    render(
      <MobileMenuItem
        item={item}
        isAnimating={false}
        isAdded={false}
        quantity={0}
        defaultCurrency="USD"
        displayCurrency="USD"
      locale="en"
        onItemClick={jest.fn()}
        t={t}
      />,
    );
    const details = screen.getByRole("button", {
      name: "accessibility.openItem",
    });
    expect(details.closest('[role="button"]')).toBeNull();
  });

  it("opens item details from the dedicated keyboard button", () => {
    const onItemClick = jest.fn();
    render(
      <MobileMenuItem
        item={item}
        isAnimating={false}
        isAdded={false}
        quantity={0}
        defaultCurrency="USD"
        displayCurrency="USD"
      locale="en"
        onItemClick={onItemClick}
        t={t}
      />,
    );
    const details = screen.getByRole("button", {
      name: "accessibility.openItem",
    });

    fireEvent.click(details);
    expect(onItemClick).toHaveBeenCalledTimes(1);
  });

  it("adds an item without also opening details", () => {
    const onItemClick = jest.fn();
    const onAddToCart = jest.fn();
    render(
      <MobileMenuItem
        item={item}
        isAnimating={false}
        isAdded={false}
        quantity={0}
        defaultCurrency="USD"
        displayCurrency="USD"
      locale="en"
        onItemClick={onItemClick}
        onAddToCart={onAddToCart}
        isOrderingEnabled
        t={t}
      />,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "accessibility.addItemToCart" }),
    );
    expect(onAddToCart).toHaveBeenCalledTimes(1);
    expect(onItemClick).not.toHaveBeenCalled();
  });
});

describe("MobileMenuItem Closed Mode chrome", () => {
  it("suppresses Unavailable badge when orderability is business_closed", () => {
    render(
      <MobileMenuItem
        item={{
          ...item,
          is_available: false,
          orderability_state: "business_closed",
        }}
        isAnimating={false}
        isAdded={false}
        quantity={0}
        defaultCurrency="USD"
        displayCurrency="USD"
      locale="en"
        onItemClick={jest.fn()}
        t={t}
      />,
    );

    expect(screen.queryByText("menu.unavailable")).not.toBeInTheDocument();
    expect(screen.queryByText("menu.outOfStock")).not.toBeInTheDocument();
  });

  it("suppresses Unavailable when venueClosed prop is set even without state", () => {
    render(
      <MobileMenuItem
        item={{ ...item, is_available: false }}
        venueClosed
        isAnimating={false}
        isAdded={false}
        quantity={0}
        defaultCurrency="USD"
        displayCurrency="USD"
      locale="en"
        onItemClick={jest.fn()}
        t={t}
      />,
    );

    expect(screen.queryByText("menu.unavailable")).not.toBeInTheDocument();
  });

  it("hides promotion offer chips when closed", () => {
    render(
      <MobileMenuItem
        item={{
          ...item,
          is_available: false,
          orderability_state: "business_closed",
          promotion_offers: [
            { id: "o1", name: "Lunch Deal" } as any,
            { id: "o2", name: "Happy Hour" } as any,
          ],
        }}
        isAnimating={false}
        isAdded={false}
        quantity={0}
        defaultCurrency="USD"
        displayCurrency="USD"
      locale="en"
        onItemClick={jest.fn()}
        t={t}
      />,
    );

    expect(screen.queryByText("Lunch Deal")).not.toBeInTheDocument();
    expect(screen.queryByText("Happy Hour")).not.toBeInTheDocument();
  });

  it("86s a live catalog row that only stamps inventory_status", () => {
    render(
      <MobileMenuItem
        item={{
          ...item,
          is_available: true,
          inventory_status: "out_of_stock",
        }}
        isAnimating={false}
        isAdded={false}
        quantity={0}
        defaultCurrency="USD"
        displayCurrency="USD"
      locale="en"
        onItemClick={jest.fn()}
        onAddToCart={jest.fn()}
        isOrderingEnabled
        t={t}
      />,
    );

    expect(screen.getByText("menu.outOfStock")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "accessibility.addItemToCart" }),
    ).not.toBeInTheDocument();
  });

  it("still shows inventory_out Unavailable when venue is open", () => {
    render(
      <MobileMenuItem
        item={{
          ...item,
          is_available: false,
          orderability_state: "inventory_out",
        }}
        isAnimating={false}
        isAdded={false}
        quantity={0}
        defaultCurrency="USD"
        displayCurrency="USD"
      locale="en"
        onItemClick={jest.fn()}
        t={t}
      />,
    );

    expect(screen.getByText("menu.outOfStock")).toBeInTheDocument();
    expect(screen.queryByText("menu.unavailable")).not.toBeInTheDocument();
  });

  it("still shows generic Unavailable for non-closed unavailability when open", () => {
    render(
      <MobileMenuItem
        item={{ ...item, is_available: false }}
        isAnimating={false}
        isAdded={false}
        quantity={0}
        defaultCurrency="USD"
        displayCurrency="USD"
      locale="en"
        onItemClick={jest.fn()}
        t={t}
      />,
    );

    expect(screen.getByText("menu.unavailable")).toBeInTheDocument();
  });

  it("shows promotion chips when open", () => {
    render(
      <MobileMenuItem
        item={{
          ...item,
          promotion_offers: [{ id: "o1", name: "Lunch Deal" } as any],
        }}
        isAnimating={false}
        isAdded={false}
        quantity={0}
        defaultCurrency="USD"
        displayCurrency="USD"
      locale="en"
        onItemClick={jest.fn()}
        t={t}
      />,
    );

    expect(screen.getByText("Lunch Deal")).toBeInTheDocument();
  });
});
