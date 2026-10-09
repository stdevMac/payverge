/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import type { CategoryTotal } from "@/api/accounting";
import CategoryBreakdown from "./CategoryBreakdown";

const formatMoney = (value: number, currency: string) =>
  `${currency} ${value.toFixed(2)}`;

function items(
  rows: Array<{ category: string; total: number }>,
): CategoryTotal[] {
  return rows.map((r) => ({
    category: r.category,
    total: r.total as CategoryTotal["total"],
  }));
}

describe("CategoryBreakdown", () => {
  it("renders one bar per category sorted desc with percent width and formatMoney labels", () => {
    render(
      <CategoryBreakdown
        title="Income"
        items={items([
          { category: "catering", total: 50 },
          { category: "event", total: 150 },
          { category: "service", total: 100 },
        ])}
        currency="USD"
        formatMoney={formatMoney}
        formatCategory={(c) => c.toUpperCase()}
        emptyLabel="No categories"
      />,
    );

    expect(screen.getByText("Income")).toBeInTheDocument();
    const rows = screen.getAllByTestId("category-breakdown-row");
    expect(rows).toHaveLength(3);

    // Sorted desc: event (150) → service (100) → catering (50)
    expect(rows[0]).toHaveTextContent("EVENT");
    expect(rows[0]).toHaveTextContent("USD 150.00");
    expect(rows[1]).toHaveTextContent("SERVICE");
    expect(rows[2]).toHaveTextContent("CATERING");

    const barEvent = rows[0].querySelector(
      "[data-testid='category-breakdown-bar']",
    ) as HTMLElement;
    const barService = rows[1].querySelector(
      "[data-testid='category-breakdown-bar']",
    ) as HTMLElement;
    const barCatering = rows[2].querySelector(
      "[data-testid='category-breakdown-bar']",
    ) as HTMLElement;

    expect(barEvent.style.width).toBe("100%");
    expect(barService.style.width).toBe("67%");
    expect(barCatering.style.width).toBe("33%");
  });

  it("shows empty label when there are no categories", () => {
    render(
      <CategoryBreakdown
        title="Expenses"
        items={[]}
        currency="USD"
        formatMoney={formatMoney}
        formatCategory={(c) => c}
        emptyLabel="No expense categories"
      />,
    );
    expect(screen.getByText("No expense categories")).toBeInTheDocument();
    expect(screen.queryByTestId("category-breakdown-row")).toBeNull();
  });

  it("renders view-all control when onViewAll is provided", () => {
    const onViewAll = jest.fn();
    render(
      <CategoryBreakdown
        title="Income"
        items={items([{ category: "catering", total: 10 }])}
        currency="USD"
        formatMoney={formatMoney}
        formatCategory={(c) => c}
        emptyLabel="empty"
        viewAllLabel="View all"
        onViewAll={onViewAll}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "View all" }));
    expect(onViewAll).toHaveBeenCalledTimes(1);
  });
});
