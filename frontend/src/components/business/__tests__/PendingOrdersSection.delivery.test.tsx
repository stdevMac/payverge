/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
}));

// OperationalAlertClaimStatus uses its own context — stub it out.
jest.mock(
  "../operational-alerts/OperationalAlertClaimStatus",
  () => ({
    __esModule: true,
    default: () => null,
  }),
);

// NextUI Button renders fine in jsdom; no need to mock.

import { PendingOrdersSection } from "../PendingOrdersSection";
import type { Order } from "@/api/orders";

// Stable future timestamp for awaiting-payment tests.
const FUTURE = new Date(Date.now() + 5 * 60 * 1000).toISOString();
const PAST = new Date(Date.now() - 60 * 1000).toISOString();

const buildOrder = (overrides: Partial<Order> = {}): Order => ({
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
  ...overrides,
});

const tString = (key: string) => key;

const defaultProps = {
  actionLoading: null,
  onApproveOrder: jest.fn(),
  onRejectOrder: jest.fn(),
  tString,
};

describe("PendingOrdersSection — delivery cards", () => {
  it("renders delivery badge, name, address when order.delivery is present", () => {
    const order = buildOrder({
      delivery: {
        delivery_id: 42,
        delivery_number: "DLV-007",
        delivery_status: "pending",
        customer_name: "Alice Smith",
        customer_phone: "+1-555-0100",
        street: "123 Main St",
        city: "Springfield",
        payment_expires_at: null,
      },
    });

    render(
      <PendingOrdersSection
        {...defaultProps}
        orders={{ 10: [order] }}
      />,
    );

    // Delivery badge label (key from tString passthrough)
    expect(screen.getByText("pendingOrders.deliveryBadge")).toBeInTheDocument();
    // Delivery number
    expect(screen.getByText("DLV-007")).toBeInTheDocument();
    // Customer name and phone (combined in one element)
    expect(screen.getByText("Alice Smith · +1-555-0100")).toBeInTheDocument();
    // Address
    expect(screen.getByText("123 Main St, Springfield")).toBeInTheDocument();
  });

  it("hides Approve but keeps Cancel + countdown when confirmed + future expiry", () => {
    const order = buildOrder({
      delivery: {
        delivery_id: 42,
        delivery_number: "DLV-008",
        delivery_status: "confirmed",
        customer_name: "Bob Jones",
        customer_phone: "+1-555-0200",
        street: "456 Elm St",
        city: "Shelbyville",
        payment_expires_at: FUTURE,
      },
    });

    render(
      <PendingOrdersSection
        {...defaultProps}
        orders={{ 10: [order] }}
      />,
    );

    // Awaiting payment label visible
    expect(
      screen.getByText("pendingOrders.awaitingPayment"),
    ).toBeInTheDocument();

    // Approve must NOT be present (guest still has a payment window)
    expect(screen.queryByText("approve")).not.toBeInTheDocument();

    // Cancel must remain so the card is never a dead end vs dine-in siblings
    expect(screen.getByText("cancelOrder")).toBeInTheDocument();

    // A countdown element should be present (mm:ss pattern or expiredLabel)
    // We accept either format since the timer may tick in CI.
    const countdownEl =
      screen.queryByText(/^\d{2}:\d{2}$/) ||
      screen.queryByText("pendingOrders.expired");
    expect(countdownEl).not.toBeNull();
  });

  it("shows Cancel only (no Approve) when confirmed but payment_expires_at is in the past", () => {
    const order = buildOrder({
      delivery: {
        delivery_id: 42,
        delivery_number: "DLV-009",
        delivery_status: "confirmed",
        customer_name: "Carol Lee",
        customer_phone: "+1-555-0300",
        street: "789 Oak Ave",
        city: "Capital City",
        payment_expires_at: PAST,
      },
    });

    render(
      <PendingOrdersSection
        {...defaultProps}
        orders={{ 10: [order] }}
      />,
    );

    // Accept is invalid once payment expired (delivery is confirmed, not pending)
    expect(screen.queryByText("approve")).not.toBeInTheDocument();
    expect(screen.getByText("pendingOrders.expired")).toBeInTheDocument();
    expect(screen.getByText("cancelOrder")).toBeInTheDocument();
  });

  it("plain dine-in order: no delivery block, Approve/Reject intact", () => {
    const order = buildOrder(); // no delivery field

    render(
      <PendingOrdersSection
        {...defaultProps}
        orders={{ 10: [order] }}
      />,
    );

    // No delivery badge
    expect(
      screen.queryByText("pendingOrders.deliveryBadge"),
    ).not.toBeInTheDocument();

    // Standard buttons present
    expect(screen.getByText("approve")).toBeInTheDocument();
    expect(screen.getByText("cancelOrder")).toBeInTheDocument();
  });
});
