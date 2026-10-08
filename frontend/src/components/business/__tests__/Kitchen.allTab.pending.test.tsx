/** @jest-environment jsdom */
/**
 * All Orders must surface pending tickets (and never show a lying empty state
 * while Needs approval / Overview still count those orders).
 */
import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) =>
    /needsApproval\.items(_one|_other)?$/.test(key) ? "{count} items" : key,
}));

jest.mock("@/contexts/StaffPermissionsContext", () => ({
  useStaffPermissionsContext: () => ({
    permissions: [],
    rolePermissions: [],
    customGrants: [],
    isLoading: false,
    isError: false,
    refetch: jest.fn(),
  }),
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

jest.mock("@/api/business", () => ({
  getBusiness: jest.fn(() =>
    Promise.resolve({ default_currency: "USD", timezone: "UTC" }),
  ),
  getMenu: jest.fn(() => Promise.resolve({ parsed_categories: [] })),
}));

import Kitchen from "@/components/business/Kitchen";
import type { Order } from "@/api/orders";

const pendingOrder: Order = {
  id: 7,
  bill_id: 10,
  business_id: 1,
  order_number: "ORD-B75-pending",
  status: "pending",
  items: JSON.stringify([
    {
      id: "bundle",
      menu_item_name: "Date Night",
      quantity: 4,
      price: 80,
      options: [],
      special_requests: "",
      subtotal: 320,
      item_type: "bundle",
    },
    {
      id: "steak",
      menu_item_name: "Steak",
      quantity: 4,
      price: 0,
      options: [],
      special_requests: "",
      subtotal: 0,
      item_type: "bundle_item",
    },
    {
      id: "wine",
      menu_item_name: "Wine",
      quantity: 4,
      price: 0,
      options: [],
      special_requests: "",
      subtotal: 0,
      item_type: "bundle_item",
    },
    {
      id: "salad",
      menu_item_name: "Salad",
      quantity: 4,
      price: 0,
      options: [],
      special_requests: "",
      subtotal: 0,
      item_type: "bundle_item",
    },
    {
      id: "dessert",
      menu_item_name: "Dessert",
      quantity: 4,
      price: 0,
      options: [],
      special_requests: "",
      subtotal: 0,
      item_type: "bundle_item",
    },
  ]),
  currency: "USD",
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
} as Order;

function renderKitchen() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <Kitchen
        businessId={1}
        globalOrders={{ 10: [pendingOrder] }}
        globalOrdersLoaded
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />
    </QueryClientProvider>,
  );
}

it("lists pending tickets in All Orders instead of a lying empty state", async () => {
  renderKitchen();

  fireEvent.click(
    await screen.findByRole("tab", {
      name: "businessDashboard.dashboard.kitchenManager.tabs.allOrders",
    }),
  );

  // Banner + All Orders card both show the ticket — empty-state lie is gone.
  await waitFor(() =>
    expect(screen.getAllByText("#ORD-B75-pending").length).toBeGreaterThanOrEqual(2),
  );
  expect(screen.queryByTestId("kitchen-board-empty")).not.toBeInTheDocument();
  expect(
    screen.getByRole("button", {
      name: "businessDashboard.dashboard.kitchenManager.order.viewOrderAria",
    }),
  ).toBeInTheDocument();
});

it("keeps Needs approval outside the stage tabpanel", async () => {
  renderKitchen();

  const banner = await screen.findByTestId("needs-approval-banner");
  const readyTab = screen.getByRole("tab", {
    name: "businessDashboard.dashboard.kitchenManager.tabs.ready",
  });
  fireEvent.click(readyTab);

  // Banner stays mounted above the rail while Ready shows its own empty state.
  expect(screen.getByTestId("needs-approval-banner")).toBe(banner);
  expect(banner.closest('[role="tabpanel"]')).toBeNull();
  expect(screen.getByTestId("kitchen-board-empty")).toBeInTheDocument();
});

it("counts 4× Date Night as 4 sellable units, not 16 components", async () => {
  renderKitchen();
  const title = await screen.findByTestId("needs-approval-title-7");
  expect(within(title).getByText(/4/)).toBeInTheDocument();
  expect(title.textContent || "").not.toMatch(/16/);
});
