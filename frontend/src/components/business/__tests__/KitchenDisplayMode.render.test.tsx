/** @jest-environment jsdom */
/**
 * K-14: first dedicated KDS render coverage — column grouping, counts,
 * FIFO ordering inside a column (locks K-7), and the empty-column state.
 * jsdom has no matchMedia, so the component renders the stacked mobile list;
 * grouping/order semantics are identical to the desktop columns.
 */
import React from "react";
import { render, screen } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  getTranslation: (key: string) => key,
}));

jest.mock("@/api/orders", () => ({
  updateOrderStatus: jest.fn(),
  parseOrderItems: (items: string) => JSON.parse(items || "[]"),
}));

jest.mock(
  "@/components/business/operational-alerts/EnableAlertSoundButton",
  () => ({ __esModule: true, default: () => null }),
);
jest.mock(
  "@/components/business/operational-alerts/OperationalAlertClaimStatus",
  () => ({ __esModule: true, default: () => null }),
);

import KitchenDisplayMode from "@/components/business/KitchenDisplayMode";
import type { Order } from "@/api/orders";

const makeOrder = (
  id: number,
  status: Order["status"],
  createdAgoMs: number,
): Order =>
  ({
    id,
    bill_id: 10,
    business_id: 1,
    order_number: `K-${id}`,
    status,
    items: JSON.stringify([
      {
        id: `i${id}`,
        menu_item_name: "Soup",
        quantity: 2,
        price: 12,
        options: [{ id: "o1", name: "Extra bread", price_change: 1.5, is_required: false }],
        special_requests: "no salt",
        subtotal: 24,
      },
    ]),
    currency: "USD",
    created_at: new Date(Date.now() - createdAgoMs).toISOString(),
    updated_at: new Date().toISOString(),
  }) as Order;

const orders: Order[] = [
  makeOrder(1, "approved", 5 * 60_000),
  makeOrder(2, "approved", 20 * 60_000),
  makeOrder(3, "in_kitchen", 10 * 60_000),
  makeOrder(4, "ready", 2 * 60_000),
];

const renderKds = (data: Order[] = orders) =>
  render(
    <KitchenDisplayMode
      orders={data}
      onExit={jest.fn()}
      onRefresh={jest.fn()}
      businessId={1}
      locale={"en" as never}
      onOrderStatusChange={jest.fn()}
    />,
  );

it("renders every active ticket with its action button, modifiers and requests", () => {
  renderKds();

  expect(screen.getByText("#K-1")).toBeInTheDocument();
  expect(screen.getByText("#K-2")).toBeInTheDocument();
  expect(screen.getByText("#K-3")).toBeInTheDocument();
  expect(screen.getByText("#K-4")).toBeInTheDocument();

  expect(screen.getAllByText("kitchenDisplay.actions.start")).toHaveLength(2);
  expect(screen.getAllByText("kitchenDisplay.actions.ready")).toHaveLength(1);
  expect(screen.getAllByText("kitchenDisplay.actions.serve")).toHaveLength(1);

  expect(screen.getAllByText("+ Extra bread").length).toBeGreaterThan(0);
  expect(screen.getAllByText(/no salt/).length).toBeGreaterThan(0);
});

it("orders tickets FIFO within a column (oldest first) — K-7 lock", () => {
  renderKds();

  const older = screen.getByText("#K-2");
  const newer = screen.getByText("#K-1");
  expect(older.compareDocumentPosition(newer) & 4).toBeTruthy();
});

it("shows the per-column empty state when a column has no tickets", () => {
  renderKds([makeOrder(4, "ready", 2 * 60_000)]);
  expect(
    screen.getAllByText("kitchenDisplay.noOrders").length,
  ).toBeGreaterThanOrEqual(2);
});
