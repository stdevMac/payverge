/** @jest-environment jsdom */
/**
 * #775: En Cocina leftover cards must expose order number and bill label as
 * separate innerText tokens. CSS wrap/gap is not a text node.
 */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const translations: Record<string, string> = {
  "businessDashboard.dashboard.kitchenManager.order.billPrefix":
    "Cuenta n.º {billId}",
  "businessDashboard.dashboard.kitchenManager.order.viewBillAria":
    "Ver cuenta #{billId}",
  "businessDashboard.dashboard.kitchenManager.order.viewOrderAria":
    "Ver pedido #{orderNumber}",
  "businessDashboard.dashboard.kitchenManager.tabs.approved": "Aprobados",
  "businessDashboard.dashboard.kitchenManager.tabs.inKitchen": "En Cocina",
  "businessDashboard.dashboard.kitchenManager.tabs.ready": "Listo",
  "businessDashboard.dashboard.kitchenManager.tabs.allOrders":
    "Todos los Pedidos",
};

jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: jest.fn() }),
  usePathname: () => "/business/demo-admin-8-ai-pro/dashboard",
  useSearchParams: () => new URLSearchParams(),
  useParams: () => ({}),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "es", setLocale: jest.fn() }),
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
import { kitchenTicketIdentityText } from "@/components/business/kitchenTicketIdentity";

const leftover = (orderNumber: string, billId: number): Order =>
  ({
    id: billId,
    bill_id: billId,
    business_id: 1,
    order_number: orderNumber,
    status: "in_kitchen",
    items: JSON.stringify([
      {
        id: `i${billId}`,
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
  }) as Order;

const leftovers: Order[] = [
  leftover("G86-36604192", 1132),
  leftover("G86-36604193", 1134),
  leftover("G86-36604194", 1141),
];

function renderKitchen() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <Kitchen
        businessId={1}
        globalOrders={{
          1132: [leftovers[0]],
          1134: [leftovers[1]],
          1141: [leftovers[2]],
        }}
        globalOrdersLoaded
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
        businessTimezone="America/New_York"
      />
    </QueryClientProvider>,
  );
}

function identityText(el: HTMLElement): string {
  return (el.innerText || el.textContent || "").replace(/\u00a0/g, " ");
}

it("keeps En Cocina leftover identities as readable tokens (#775)", async () => {
  renderKitchen();

  fireEvent.click(await screen.findByRole("tab", { name: /En Cocina/ }));

  const identities = await screen.findAllByTestId("kitchen-ticket-identity");
  expect(identities).toHaveLength(3);

  const expected = leftovers.map((order) =>
    kitchenTicketIdentityText(
      order.order_number,
      `Cuenta n.º ${order.bill_id}`,
    ),
  );

  const actual = identities.map((el) => identityText(el));
  expect(actual).toEqual(expected);
  for (const text of actual) {
    expect(text).toMatch(/#G86-\d+ · Cuenta n\.º \d+/);
    expect(text).not.toMatch(/#G86-\d+Cuenta/);
  }
});
