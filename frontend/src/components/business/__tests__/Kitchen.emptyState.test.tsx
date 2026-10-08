/** @jest-environment jsdom */
/**
 * Wave 4 Task 13: kitchen board empty state uses shared EmptyState.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
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
  getMenu: jest.fn(() => Promise.resolve({ parsed_categories: [] })),
}));

import Kitchen from "@/components/business/Kitchen";

it("renders the shared EmptyState when the board is empty", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <Kitchen
        businessId={1}
        globalOrders={{}}
        globalOrdersLoaded
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />
    </QueryClientProvider>,
  );
  expect(
    await screen.findByTestId("kitchen-board-empty"),
  ).toBeInTheDocument();
  expect(
    screen.getByText(
      "businessDashboard.dashboard.kitchenManager.emptyStates.approved.title",
    ),
  ).toBeInTheDocument();
});
