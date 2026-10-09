/** @jest-environment jsdom */
/**
 * Wave 4 Task 11: one-tap 86 from kitchen ticket detail modal.
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: jest.fn() }),
  usePathname: () => "/business/1/dashboard",
  useSearchParams: () => new URLSearchParams(),
  useParams: () => ({}),
}));

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
    Promise.resolve({ default_currency: "USD", timezone: "UTC" }),
  ),
  getMenu: jest.fn(() =>
    Promise.resolve({
      version: 3,
      categories: [
        {
          id: "c1",
          name: "Mains",
          items: [
            {
              id: "m1",
              name: "Soup",
              description: "",
              price: 12,
              is_available: true,
            },
          ],
        },
      ],
    }),
  ),
  updateMenuItem: jest.fn(() =>
    Promise.resolve({ message: "ok", version: 4 }),
  ),
}));

import Kitchen from "@/components/business/Kitchen";
import type { Order } from "@/api/orders";
import { updateMenuItem } from "@/api/business";

const mockUpdateMenuItem = updateMenuItem as jest.MockedFunction<
  typeof updateMenuItem
>;

const order = {
  id: 9,
  bill_id: 10,
  business_id: 1,
  order_number: "K-9",
  status: "approved",
  items: JSON.stringify([
    {
      id: "i1",
      menu_item_id: "m1",
      menu_item_name: "Soup",
      quantity: 1,
      price: 12,
      options: [],
      special_requests: "",
      subtotal: 12,
    },
  ]),
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
} as Order;

it("marks a ticket line item 86 via the menu-item update API", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <Kitchen
        businessId={1}
        globalOrders={{ 10: [order] }}
        globalOrdersLoaded
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />
    </QueryClientProvider>,
  );
  // open the ticket detail modal
  fireEvent.click(await screen.findByText("#K-9"));
  // press the 86 action on the Soup row
  const eightySix = await screen.findByRole("button", {
    name: /kitchenManager.eightySix.action/,
  });
  fireEvent.click(eightySix);
  // confirm in the shared ConfirmationModal
  fireEvent.click(
    await screen.findByRole("button", {
      name: /kitchenManager.eightySix.confirmLabel/,
    }),
  );
  await waitFor(() => expect(mockUpdateMenuItem).toHaveBeenCalled());
  const call = mockUpdateMenuItem.mock.calls[0] as unknown[];
  const [businessId, , , updatedItem, version, categoryId, itemId] = call;
  expect(businessId).toBe(1);
  expect((updatedItem as { is_available: boolean }).is_available).toBe(false);
  expect(version).toBe(3);
  expect(categoryId).toBe("c1");
  expect(itemId).toBe("m1");
});
