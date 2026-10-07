/** @jest-environment jsdom */
/**
 * K-1 regression: KitchenDisplayMode must stay mounted across a simulated
 * dashboard poll tick that delivers a NEW empty globalOrders reference. The
 * old code re-ran loadOrders(true) on every parent reference change, fell
 * into the API fallback (empty map), flipped `loading`, swapped in the
 * skeleton, and unmounted the KDS — dropping fullscreen on real tablets.
 */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const translations: Record<string, string> = {
  "kitchenDisplay.launch": "Launch KDS Mode",
  "kitchenDisplay.title": "Kitchen Display System",
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

const mockGetAllActiveOrders = jest.fn(() =>
  Promise.resolve({
    items: [],
    metadata: { total: 0, pageSize: 100, pagesFetched: 0 },
    capped: false,
  }),
);
jest.mock("@/api/orders", () => ({
  getOrders: jest.fn(() => Promise.resolve({ orders: [], total: 0 })),
  getAllActiveOrders: (...a: unknown[]) =>
    mockGetAllActiveOrders(...(a as [])),
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

jest.mock(
  "@/components/business/operational-alerts/EnableAlertSoundButton",
  () => ({ __esModule: true, default: () => null }),
);
jest.mock(
  "@/components/business/operational-alerts/OperationalAlertClaimStatus",
  () => ({ __esModule: true, default: () => null }),
);

import Kitchen from "@/components/business/Kitchen";
import type { Order } from "@/api/orders";

it("keeps KitchenDisplayMode mounted across a poll tick delivering a new empty orders map", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const baseProps = {
    businessId: 1,
    globalOrdersCapped: false,
    kitchenEnabled: true,
    kitchenStatusLoading: false,
    onKitchenStatusChange: jest.fn(),
    onOrderStatusChange: jest.fn(),
  };
  const tree = (orders: Record<number, Order[]>) => (
    <QueryClientProvider client={client}>
      <Kitchen {...baseProps} globalOrders={orders} globalOrdersLoaded />
    </QueryClientProvider>
  );

  const { rerender } = render(tree({}));

  const launchButtons = await screen.findAllByText("Launch KDS Mode");
  fireEvent.click(launchButtons[0]);
  await waitFor(() =>
    expect(screen.getByText("Kitchen Display System")).toBeInTheDocument(),
  );

  rerender(tree({}));

  expect(screen.getByText("Kitchen Display System")).toBeInTheDocument();

  expect(mockGetAllActiveOrders).not.toHaveBeenCalled();
});
