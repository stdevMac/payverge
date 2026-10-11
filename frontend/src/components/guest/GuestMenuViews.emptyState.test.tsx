/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";

import { GuestMenuViews } from "./GuestMenuViews";
import type { Business, MenuCategory } from "../../api/business";

// Identity-ish guest t(): return a readable string per key so we can assert copy.
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    currentLanguage: "en",
    t: (key: string) => {
      const table: Record<string, string> = {
        "menu.search.noResultsTitle": "No items found",
        "menu.search.noResultsDescription":
          "Try adjusting your search or browse all categories.",
        "menu.menuComingSoon": "Menu Coming Soon",
        "menu.menuComingSoonDescription":
          "The menu for this restaurant is being prepared.",
        "menu.filters.clear": "Clear",
      };
      return table[key] ?? key;
    },
  }),
}));

// CurrencyPrice pulls in currency hooks we don't need for the empty-state branch.
jest.mock("../common/CurrencyConverter", () => ({
  CurrencyPrice: ({ amount }: { amount: number }) => <span>{amount}</span>,
}));

const business = { name: "Testaurant" } as unknown as Business;

const baseProps = {
  business,
  tableCode: "T1",
  currentBill: null,
  viewMode: "detailed" as const,
  activeCategory: 0,
  onAddToCart: jest.fn(),
  onItemClick: jest.fn(),
};

describe("GuestMenuViews empty states", () => {
  it("renders the 'menu coming soon' empty-state when there are no items and no active filters", () => {
    render(
      <GuestMenuViews
        {...baseProps}
        categories={[] as MenuCategory[]}
        hasActiveFilters={false}
      />,
    );
    expect(screen.getByText("Menu Coming Soon")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Clear" }),
    ).not.toBeInTheDocument();
  });

  it("renders the 'no results' empty-state with a Clear action when filters are active and yield nothing", () => {
    const onClearFilters = jest.fn();
    render(
      <GuestMenuViews
        {...baseProps}
        categories={
          [{ name: "Mains", description: "", items: [] }] as MenuCategory[]
        }
        hasActiveFilters
        onClearFilters={onClearFilters}
      />,
    );
    expect(screen.getByText("No items found")).toBeInTheDocument();
    const clearBtn = screen.getByRole("button", { name: "Clear" });
    fireEvent.click(clearBtn);
    expect(onClearFilters).toHaveBeenCalledTimes(1);
  });
});
