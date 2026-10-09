/** @jest-environment jsdom */
/**
 * K-14/K-5: a recently-cancelled card must render its reason + meta and must
 * SURVIVE a poll tick that replaces the orders prop with a fresh
 * equal-content object (poll data now includes cancelled — the panel no
 * longer self-destructs within 60s).
 */
import React from "react";
import { render, screen } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
}));
jest.mock(
  "../operational-alerts/OperationalAlertClaimStatus",
  () => ({ __esModule: true, default: () => null }),
);

import { PendingOrdersSection } from "../PendingOrdersSection";
import type { Order } from "@/api/orders";

const cancelled = (): Order =>
  ({
    id: 1,
    bill_id: 10,
    business_id: 1,
    order_number: "ORD-001",
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
    created_at: new Date(Date.now() - 35 * 60_000).toISOString(),
    updated_at: new Date(Date.now() - 30 * 60_000).toISOString(),
    cancelled_at: new Date(Date.now() - 30 * 60_000).toISOString(),
    cancelled_by: "guest",
    cancel_reason: "ordered by mistake",
  }) as Order;

const props = {
  actionLoading: null,
  onApproveOrder: jest.fn(),
  onRejectOrder: jest.fn(),
  tString: (key: string) => key,
};

it("renders the cancelled card with reason + actor, and keeps it across a poll-tick prop swap", () => {
  const { rerender } = render(
    <PendingOrdersSection {...props} orders={{ 10: [cancelled()] }} />,
  );

  expect(screen.getByText("cancelledOrdersTitle")).toBeInTheDocument();
  expect(screen.getByText("ordered by mistake")).toBeInTheDocument();
  expect(screen.getByText("cancelledByLabel")).toBeInTheDocument();
  expect(screen.getByText("cancelledOrderMeta")).toBeInTheDocument();

  rerender(
    <PendingOrdersSection {...props} orders={{ 10: [cancelled()] }} />,
  );

  expect(screen.getByText("cancelledOrdersTitle")).toBeInTheDocument();
  expect(screen.getByText("ordered by mistake")).toBeInTheDocument();
});
