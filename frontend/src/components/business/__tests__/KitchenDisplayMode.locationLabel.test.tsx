/** @jest-environment jsdom */
/**
 * R3-OK KDS labels: the location chip must render via getBillLocationLabel
 * (table name / counter / delivery aware) instead of the old raw
 * "T-{table_id}" literal that leaked the DB id and rendered nothing for
 * counter/delivery bills.
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

const baseOrder = (id: number, bill: Record<string, unknown> | undefined): Order =>
  ({
    id,
    bill_id: 10,
    business_id: 1,
    order_number: `K-${id}`,
    status: "approved",
    items: JSON.stringify([
      { id: `i${id}`, menu_item_name: "Soup", quantity: 1, price: 10, subtotal: 10 },
    ]),
    currency: "USD",
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    bill,
  }) as unknown as Order;

const renderKds = (data: Order[]) =>
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

it("labels a named table bill with its friendly name (no raw DB id)", () => {
  renderKds([
    baseOrder(1, { id: 5, table_id: 36, table_name: "Window 1" }),
  ]);
  // formatEntityName prefixes the table label word when the name doesn't
  // already start with it: "<label> Window 1". The friendly name is present
  // and the raw DB-id literal ("T-36") is gone.
  expect(screen.getByText(/Window 1/)).toBeInTheDocument();
  expect(screen.queryByText("T-36")).not.toBeInTheDocument();
});

it("labels a counter bill as counter, not Table 0", () => {
  renderKds([
    baseOrder(2, { id: 6, table_id: 0, counter_id: 3 }),
  ]);
  expect(screen.getByText("kitchenDisplay.location.counter")).toBeInTheDocument();
  expect(screen.queryByText(/T-0|Table 0/)).not.toBeInTheDocument();
});

it("labels a delivery bill (no table, no counter) as delivery", () => {
  renderKds([baseOrder(3, { id: 7, table_id: 0 })]);
  expect(screen.getByText("kitchenDisplay.location.delivery")).toBeInTheDocument();
});

it("renders no location chip when the order has no bill preloaded", () => {
  renderKds([baseOrder(4, undefined)]);
  expect(
    screen.queryByText(/kitchenDisplay\.location\./),
  ).not.toBeInTheDocument();
});
