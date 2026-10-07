/** @jest-environment jsdom */
/**
 * Dinner-service QA: pending cards must not squeeze (#90/#121), must cap growth
 * (#124), and must hide promo discount lines from the food list (#107).
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
}));
jest.mock("../operational-alerts/OperationalAlertClaimStatus", () => ({
  __esModule: true,
  default: () => null,
}));

import { PendingOrdersSection, foodOrderItems } from "../PendingOrdersSection";
import type { Order } from "@/api/orders";

const tString = (key: string) =>
  ({
    orderNumber: "Order #{orderNumber}",
    billNumber: "Bill",
    pending: "Pending",
    approve: "Approve",
    cancelOrder: "Cancel Order",
    pendingOrdersTitle: "Pending Orders",
    pendingOrdersCount: "{count} tickets awaiting approval",
    pendingOrdersCountOne: "1 ticket awaiting approval",
    pendingOrdersShowAll: "Show {count} more",
    pendingOrdersShowFewer: "Show fewer",
    additionalItems: "+{count} more items",
    notesLabel: "Notes",
  })[key] ?? key;

function makeOrder(
  id: number,
  overrides: Partial<Order> & { items?: string } = {},
): Order {
  return {
    id,
    bill_id: 10,
    business_id: 1,
    order_number: `ORD-B75-VERY-LONG-ORDER-NUMBER-${id}`,
    status: "pending",
    items: JSON.stringify([
      {
        menu_item_name: "Burger",
        quantity: 1,
        price: 10,
        subtotal: 10,
        item_type: "menu_item",
        options: [],
        special_requests: "",
      },
    ]),
    currency: "USD",
    created_at: new Date(Date.now() - id * 1000).toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  } as Order;
}

describe("PendingOrdersSection layout / discounts", () => {
  it("foodOrderItems drops discount lines so promos are not orderable food", () => {
    const items = foodOrderItems(
      JSON.stringify([
        {
          menu_item_name: "Burger",
          quantity: 1,
          price: 10,
          subtotal: 10,
          item_type: "menu_item",
        },
        {
          menu_item_name: "Weekday Lunch 15% Off",
          quantity: 1,
          price: -1.5,
          subtotal: -1.5,
          item_type: "discount",
        },
      ]),
    );
    expect(items.map((i) => i.menu_item_name)).toEqual(["Burger"]);
  });

  it("does not render discount lines among pending food items", () => {
    const order = makeOrder(1, {
      items: JSON.stringify([
        {
          menu_item_name: "Burger",
          quantity: 1,
          price: 10,
          subtotal: 10,
          item_type: "menu_item",
          options: [],
          special_requests: "",
        },
        {
          menu_item_name: "Weekday Lunch 15% Off",
          quantity: 1,
          price: -1.5,
          subtotal: -1.5,
          item_type: "discount",
          options: [],
          special_requests: "",
        },
      ]),
    });

    render(
      <PendingOrdersSection
        orders={{ 10: [order] }}
        actionLoading={null}
        onApproveOrder={jest.fn()}
        onRejectOrder={jest.fn()}
        tString={tString}
      />,
    );

    expect(screen.getByText(/Burger/)).toBeInTheDocument();
    expect(screen.queryByText(/Weekday Lunch 15% Off/)).not.toBeInTheDocument();
  });

  it("truncates long order numbers and keeps the full value in the title", () => {
    render(
      <PendingOrdersSection
        orders={{ 10: [makeOrder(1)] }}
        actionLoading={null}
        onApproveOrder={jest.fn()}
        onRejectOrder={jest.fn()}
        tString={tString}
      />,
    );

    const heading = screen.getByTestId("pending-order-number");
    expect(heading).toHaveClass("truncate");
    expect(heading).toHaveAttribute(
      "title",
      "Order #ORD-B75-VERY-LONG-ORDER-NUMBER-1",
    );
  });

  it("uses an auto-fit grid so a single card is not squeezed to ~200px", () => {
    render(
      <PendingOrdersSection
        orders={{ 10: [makeOrder(1)] }}
        actionLoading={null}
        onApproveOrder={jest.fn()}
        onRejectOrder={jest.fn()}
        tString={tString}
      />,
    );

    const grid = screen.getByTestId("pending-orders-grid");
    expect(grid.className).toMatch(/auto-fit/);
    expect(grid.className).not.toMatch(/lg:grid-cols-3/);
  });

  it("caps the pending panel and expands on demand", async () => {
    const user = userEvent.setup();
    const orders = Array.from({ length: 8 }, (_, i) => makeOrder(i + 1));

    render(
      <PendingOrdersSection
        orders={{ 10: orders }}
        actionLoading={null}
        onApproveOrder={jest.fn()}
        onRejectOrder={jest.fn()}
        tString={tString}
      />,
    );

    expect(screen.getAllByTestId("pending-order-number")).toHaveLength(6);
    const toggle = screen.getByTestId("pending-orders-toggle");
    expect(toggle).toHaveTextContent("Show 2 more");

    await user.click(toggle);
    expect(screen.getAllByTestId("pending-order-number")).toHaveLength(8);
    expect(toggle).toHaveTextContent("Show fewer");
    expect(screen.getByTestId("pending-orders-grid").className).toMatch(
      /max-h-\[36rem\]/,
    );
  });

  it("compact mode keeps approve/cancel and marks the strip for the first screen", () => {
    render(
      <PendingOrdersSection
        compact
        orders={{ 10: [makeOrder(1)] }}
        actionLoading={null}
        onApproveOrder={jest.fn()}
        onRejectOrder={jest.fn()}
        tString={tString}
      />,
    );

    const section = screen.getByTestId("pending-orders-section");
    expect(section).toHaveAttribute("data-compact", "true");
    expect(section.className).toMatch(/mb-4/);
    expect(screen.getByRole("button", { name: "Approve" })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Cancel Order" }),
    ).toBeInTheDocument();
    expect(screen.getByTestId("pending-order-number")).toBeInTheDocument();
  });
});
