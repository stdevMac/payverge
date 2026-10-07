/** @jest-environment jsdom */
/**
 * Kitchen used to filter to approved+ orders, so a kitchen-tab-only operator
 * never saw brand-new (pending) orders until someone approved them from Bills.
 * The board now splits pending orders into a "Needs approval" strip with an
 * in-place approve action that reuses the shared status-change callback.
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    loading: false,
    hasAccess: true,
    isSuspended: false,
  }),
}));

jest.mock("@/api/kitchenOrders", () => ({
  getKitchenOrdersStatus: jest.fn(() =>
    Promise.resolve({ kitchen_enabled: true, orders_enabled: true }),
  ),
  toggleKitchenAndOrders: jest.fn(),
}));

jest.mock("@/api/orders", () => ({
  getOrders: jest.fn(() => Promise.resolve({ orders: [], total: 0 })),
  getAllActiveOrders: jest.fn(() =>
    Promise.resolve({
      items: [],
      metadata: { total: 0, pageSize: 100, pagesFetched: 0 },
      capped: false,
    }),
  ),
  updateOrderStatus: jest.fn(),
  cancelOrder: jest.fn(),
  getOrderStatusText: jest.fn((status: string) => status),
  parseOrderItems: (items: string) => JSON.parse(items || "[]"),
}));

jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: () => ({ retriesExhausted: false, reconnect: jest.fn() }),
}));

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    isWeb3User: true,
    isStaffUser: false,
    staffData: null,
    isLoading: false,
    isInitialized: true,
  }),
}));

import Kitchen from "@/components/business/Kitchen";
import type { Order } from "@/api/orders";

const pendingOrder: Order = {
  id: 7,
  bill_id: 10,
  business_id: 1,
  order_number: "K-7",
  status: "pending",
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
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
} as Order;

function renderKitchen(onOrderStatusChange = jest.fn()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <Kitchen
        businessId={1}
        globalOrders={{ 10: [pendingOrder] }}
        globalOrdersLoaded
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={onOrderStatusChange}
      />
    </QueryClientProvider>,
  );
  return onOrderStatusChange;
}

const APPROVE_BTN =
  "businessDashboard.dashboard.kitchenManager.needsApproval.approve";

it("surfaces a pending order in the Needs-approval strip with a count badge", async () => {
  renderKitchen();
  await waitFor(() =>
    expect(screen.getByTestId("needs-approval-count")).toHaveTextContent("1"),
  );
  expect(screen.getByText(/#K-7/)).toBeInTheDocument();
});

// PV-LIVE-20260720-002: order number must not glue to the items count
// (e.g. "#K-71 items" when count is 1).
it("separates order number from items count in the needs-approval title", async () => {
  renderKitchen();
  await waitFor(() =>
    expect(screen.getByTestId("needs-approval-title-7")).toBeInTheDocument(),
  );
  const title = screen.getByTestId("needs-approval-title-7");
  const text = title.textContent || "";
  // Glue bug was "#K-71 items" when count=1 sat against the order id.
  expect(text).not.toMatch(/#K-71/);
  expect(text).toMatch(/#K-7/);
  expect(text).toContain("·");
});

it("approves a pending order via the shared status-change callback", async () => {
  const onOrderStatusChange = jest.fn().mockResolvedValue(undefined);
  renderKitchen(onOrderStatusChange);

  const approveBtn = await screen.findByRole("button", { name: APPROVE_BTN });
  fireEvent.click(approveBtn);

  await waitFor(() =>
    expect(onOrderStatusChange).toHaveBeenCalledWith(7, "approved", "kitchen"),
  );
  // Optimistically leaves the strip once approved.
  await waitFor(() =>
    expect(screen.queryByTestId("needs-approval-count")).not.toBeInTheDocument(),
  );
});
