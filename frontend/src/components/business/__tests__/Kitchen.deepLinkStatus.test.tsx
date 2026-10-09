/** @jest-environment jsdom */
/**
 * Related to issue 794: leftover-kitchen tables link to
 * ?tab=kitchen&kitchenStatus=in_kitchen. The Kitchen tab must honor that
 * param on landing and open the requested queue instead of the default
 * Approved queue — otherwise the table row's escape hatch lands the
 * operator on the wrong list.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

let mockSearchParams = new URLSearchParams();
const mockPush = jest.fn();
jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush, replace: jest.fn() }),
  usePathname: () => "/business/1/dashboard",
  useSearchParams: () => mockSearchParams,
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
  getMenu: jest.fn(() => Promise.resolve({ parsed_categories: [] })),
}));

import Kitchen from "@/components/business/Kitchen";
import type { Order } from "@/api/orders";

const mkOrder = (id: number, status: string): Order =>
  ({
    id,
    bill_id: 10,
    business_id: 1,
    order_number: `K-${id}`,
    status,
    items: "[]",
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  }) as Order;

function renderKitchen() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <Kitchen
        businessId={1}
        globalOrders={{
          10: [mkOrder(9, "approved"), mkOrder(11, "in_kitchen")],
        }}
        globalOrdersLoaded
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  jest.clearAllMocks();
  sessionStorage.clear();
  mockSearchParams = new URLSearchParams();
});

it("opens the queue named by ?kitchenStatus= (table-row escape hatch)", async () => {
  mockSearchParams = new URLSearchParams("kitchenStatus=in_kitchen");
  renderKitchen();
  const tab = await screen.findByRole("tab", {
    name: /tabs\.inKitchen/,
  });
  expect(tab).toHaveAttribute("aria-selected", "true");
});

it("ignores an invalid kitchenStatus value and stays on the default queue", async () => {
  mockSearchParams = new URLSearchParams("kitchenStatus=drop_table");
  renderKitchen();
  const tab = await screen.findByRole("tab", {
    name: /tabs\.approved/,
  });
  expect(tab).toHaveAttribute("aria-selected", "true");
});
