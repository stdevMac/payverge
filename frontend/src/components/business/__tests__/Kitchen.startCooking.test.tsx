/** @jest-environment jsdom */
/**
 * #660: starting a ticket must land it in En Cocina, not drop it off the board.
 */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const translations: Record<string, string> = {
  "businessDashboard.dashboard.kitchenManager.buttons.startCooking":
    "Start Cooking",
  "businessDashboard.dashboard.kitchenManager.tabs.approved": "Approved",
  "businessDashboard.dashboard.kitchenManager.tabs.inKitchen": "In Kitchen",
  "businessDashboard.dashboard.kitchenManager.tabs.ready": "Ready",
  "businessDashboard.dashboard.kitchenManager.tabs.allOrders": "All Orders",
  "businessDashboard.dashboard.kitchenManager.emptyStates.in_kitchen.title":
    "No orders in kitchen",
};

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => translations[key] ?? key,
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

jest.mock("@/api/business", () => ({
  getBusiness: jest.fn(() =>
    Promise.resolve({ default_currency: "USD", timezone: "America/New_York" }),
  ),
  getMenu: jest.fn(() => Promise.resolve({ parsed_categories: [] })),
}));

import Kitchen from "@/components/business/Kitchen";
import type { Order } from "@/api/orders";

const approvedOrder: Order = {
  id: 86,
  bill_id: 10,
  business_id: 1,
  order_number: "G86-36604192",
  status: "approved",
  items: JSON.stringify([
    {
      id: "i1",
      menu_item_name: "Steak",
      quantity: 1,
      price: 24,
      options: [],
      special_requests: "",
      subtotal: 24,
    },
  ]),
  currency: "USD",
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
} as Order;

it("moves an approved ticket into En Cocina after Start Cooking", async () => {
  const onOrderStatusChange = jest.fn().mockResolvedValue(undefined);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <Kitchen
        businessId={1}
        globalOrders={{ 10: [approvedOrder] }}
        globalOrdersLoaded
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={onOrderStatusChange}
        businessTimezone="America/New_York"
      />
    </QueryClientProvider>,
  );

  expect(await screen.findByText("#G86-36604192")).toBeInTheDocument();
  fireEvent.click(screen.getByText("Start Cooking"));

  await waitFor(() =>
    expect(onOrderStatusChange).toHaveBeenCalledWith(
      86,
      "in_kitchen",
      "kitchen",
    ),
  );

  const kitchenTab = screen.getByRole("tab", { name: /In Kitchen/ });
  expect(kitchenTab).toHaveAttribute("aria-selected", "true");
  expect(screen.getByText("#G86-36604192")).toBeInTheDocument();
  expect(screen.queryByText("No orders in kitchen")).not.toBeInTheDocument();
});
