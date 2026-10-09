/** @jest-environment jsdom */
/**
 * K-5: the "recently cancelled" panel must show cancellations from the last
 * 2 hours only (the poll now keeps them flowing instead of wiping them ≤60s
 * after the SSE patch), capped so an unusual day can't flood the board.
 */
import React from "react";
import { render, screen } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
}));

jest.mock(
  "../operational-alerts/OperationalAlertClaimStatus",
  () => ({
    __esModule: true,
    default: () => null,
  }),
);

import { PendingOrdersSection } from "../PendingOrdersSection";
import type { Order } from "@/api/orders";

const cancelledOrder = (
  id: number,
  cancelledAgoMs: number,
  overrides: Partial<Order> = {},
): Order =>
  ({
    id,
    bill_id: 10,
    business_id: 1,
    order_number: `ORD-${id}`,
    status: "cancelled",
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
    created_at: new Date(Date.now() - cancelledAgoMs - 60_000).toISOString(),
    updated_at: new Date(Date.now() - cancelledAgoMs).toISOString(),
    cancelled_at: new Date(Date.now() - cancelledAgoMs).toISOString(),
    cancel_reason: "guest changed mind",
    ...overrides,
  }) as Order;

const tString = (key: string) => {
  if (key === "orderNumber") return "Order #{orderNumber}";
  return key;
};
const defaultProps = {
  actionLoading: null,
  onApproveOrder: jest.fn(),
  onRejectOrder: jest.fn(),
  tString,
};

describe("PendingOrdersSection — recently-cancelled window (K-5)", () => {
  it("shows a 1h-old cancellation, hides a 3h-old one", () => {
    render(
      <PendingOrdersSection
        {...defaultProps}
        orders={{
          10: [
            cancelledOrder(1, 60 * 60 * 1000),
            cancelledOrder(2, 3 * 60 * 60 * 1000),
          ],
        }}
      />,
    );

    expect(screen.getByText(/ORD-1/)).toBeInTheDocument();
    expect(screen.queryByText(/ORD-2/)).not.toBeInTheDocument();
  });

  it("caps the panel at 20 cards", () => {
    const many: Order[] = Array.from({ length: 25 }, (_, i) =>
      cancelledOrder(i + 1, (i + 1) * 60_000),
    );
    render(
      <PendingOrdersSection {...defaultProps} orders={{ 10: many }} />,
    );

    expect(screen.getAllByText("cancelled")).toHaveLength(20);
  });

  it("hides the panel entirely when every cancellation is stale", () => {
    render(
      <PendingOrdersSection
        {...defaultProps}
        orders={{ 10: [cancelledOrder(1, 5 * 60 * 60 * 1000)] }}
      />,
    );
    expect(
      screen.queryByText("cancelledOrdersTitle"),
    ).not.toBeInTheDocument();
  });
});
