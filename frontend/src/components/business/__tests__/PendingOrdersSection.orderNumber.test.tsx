/** @jest-environment jsdom */
/** K-8: "Order #" must come from i18n, not a hardcoded English literal. */
import React from "react";
import { render, screen } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
}));
jest.mock("../operational-alerts/OperationalAlertClaimStatus", () => ({
  __esModule: true,
  default: () => null,
}));

import { PendingOrdersSection } from "../PendingOrdersSection";
import type { Order } from "@/api/orders";

const order: Order = {
  id: 1,
  bill_id: 10,
  business_id: 1,
  order_number: "ORD-001",
  status: "pending",
  items: JSON.stringify([
    {
      menu_item_name: "Burger",
      quantity: 1,
      price: 10,
      subtotal: 10,
      options: [],
      special_requests: "",
    },
  ]),
  currency: "USD",
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
} as Order;

it("renders the order heading through tString, with no hardcoded 'Order #'", () => {
  render(
    <PendingOrdersSection
      orders={{ 10: [order] }}
      actionLoading={null}
      onApproveOrder={jest.fn()}
      onRejectOrder={jest.fn()}
      tString={(key: string) => key}
    />,
  );

  expect(screen.queryByText(/Order #/)).not.toBeInTheDocument();
  expect(screen.getByText("orderNumber")).toBeInTheDocument();
});

it("uses an AA-contrast foreground for the light cancel action", () => {
  render(
    <PendingOrdersSection
      orders={{ 10: [order] }}
      actionLoading={null}
      onApproveOrder={jest.fn()}
      onRejectOrder={jest.fn()}
      tString={(key: string) => key}
    />,
  );

  expect(screen.getByRole("button", { name: "cancelOrder" })).toHaveClass(
    "!text-rose-700",
  );
});
