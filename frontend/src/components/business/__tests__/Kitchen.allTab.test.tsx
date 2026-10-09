/** @jest-environment jsdom */
/**
 * K-6: "All orders" tab content must not depend on which data source fed the
 * board. The dashboard poll now includes `delivered` (on active bills), so
 * the embedded path must surface delivered tickets in the All tab just like
 * the standalone API fallback always did.
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

const deliveredOrder: Order = {
  id: 9,
  bill_id: 10,
  business_id: 1,
  order_number: "K-9",
  status: "delivered",
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

const cancelledOrder: Order = {
  ...deliveredOrder,
  id: 10,
  order_number: "K-10",
  status: "cancelled",
} as Order;

const renderKitchen = (orders: Order[]) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <Kitchen
        businessId={1}
        globalOrders={{ 10: orders }}
        globalOrdersLoaded
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />
    </QueryClientProvider>,
  );
};

it("shows embedded delivered orders in the All Orders tab", async () => {
  renderKitchen([deliveredOrder]);

  const allTab = await screen.findByRole("tab", {
    name: "businessDashboard.dashboard.kitchenManager.tabs.allOrders",
  });
  fireEvent.click(allTab);

  await waitFor(() => expect(screen.getByText("#K-9")).toBeInTheDocument());
});

it.each([
  ["delivered", deliveredOrder, "#K-9"],
  ["cancelled", cancelledOrder, "#K-10"],
] as const)(
  "renders %s orders without kitchen controls",
  async (_status, order, title) => {
    renderKitchen([order]);
    fireEvent.click(
      await screen.findByRole("tab", {
        name: "businessDashboard.dashboard.kitchenManager.tabs.allOrders",
      }),
    );
    await waitFor(() => expect(screen.getByText(title)).toBeInTheDocument());

    expect(
      screen.queryByText(
        "businessDashboard.dashboard.kitchenManager.order.itemsToPrepare",
      ),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByText(title));

    expect(
      screen.queryByText(
        "businessDashboard.dashboard.kitchenManager.modal.itemsToPrepare",
      ),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", {
        name: /businessDashboard\.dashboard\.kitchenManager\.eightySix\.action/,
      }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText(
        "businessDashboard.dashboard.kitchenManager.cancelOrder",
      ),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText(
        "businessDashboard.dashboard.kitchenManager.modal.markReady",
      ),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText(
        "businessDashboard.dashboard.kitchenManager.modal.markDelivered",
      ),
    ).not.toBeInTheDocument();
  },
);
