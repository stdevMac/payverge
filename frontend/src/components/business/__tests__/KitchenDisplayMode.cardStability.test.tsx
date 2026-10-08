/** @jest-environment jsdom */
/**
 * K-2 regression: the 1s KDS clock must NOT destroy/recreate order cards.
 * We capture a card's DOM node, advance the clock past a tick, and assert
 * the same node is still attached (a remount produces a new node).
 */
import React from "react";
import { render, screen, act } from "@testing-library/react";

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

const order: Order = {
  id: 1,
  bill_id: 10,
  business_id: 1,
  order_number: "A-1",
  status: "approved",
  items: JSON.stringify([
    {
      id: "i1",
      menu_item_name: "Soup",
      quantity: 1,
      price: 12,
      options: [],
      special_requests: "",
      subtotal: 12,
    },
  ]),
  currency: "USD",
  created_at: new Date(Date.now() - 5 * 60_000).toISOString(),
  updated_at: new Date().toISOString(),
} as Order;

beforeEach(() => {
  jest.useFakeTimers();
});
afterEach(() => {
  jest.useRealTimers();
});

it("keeps the same card DOM node across a 1s clock tick", () => {
  render(
    <KitchenDisplayMode
      orders={[order]}
      onExit={jest.fn()}
      onRefresh={jest.fn()}
      businessId={1}
      locale={"en" as never}
      onOrderStatusChange={jest.fn()}
    />,
  );

  const heading = screen.getByText("#A-1");

  act(() => {
    jest.advanceTimersByTime(1100);
  });

  expect(document.body.contains(heading)).toBe(true);
  expect(screen.getByText("#A-1")).toBe(heading);
});
