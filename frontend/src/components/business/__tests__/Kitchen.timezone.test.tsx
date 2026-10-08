/** @jest-environment jsdom */
/**
 * #662: Kitchen must settle the venue zone from getBusiness when the staff
 * dashboard passes no timezone. The filed stamp is 22:12Z — EN/es used to
 * print 22:12 (UTC); the venue clock is 6:12 PM / 18:12 / 6:12 p. m.
 */
import React from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  FILED_NOW,
  FILED_UTC_FIRE,
  expectVenueNyFireClock,
  type OperatorLocale,
} from "./_kdsVenueClock";

let mockLocale: OperatorLocale = "en";
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: mockLocale, setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    if (key.endsWith("time.elapsedAgo")) return "{duration} ago";
    return key;
  },
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
  getOrderStatusText: jest.fn((status: string) => status),
  parseOrderItems: (items: string) => JSON.parse(items || "[]"),
}));

jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: () => ({ retriesExhausted: false, reconnect: jest.fn() }),
}));

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    isWeb3User: false,
    isStaffUser: true,
    staffData: { id: 7, role: "kitchen", business_id: 1 },
    isLoading: false,
    isInitialized: true,
  }),
}));

jest.mock("@/contexts/StaffPermissionsContext", () => ({
  useStaffPermissionsContext: () => ({
    permissions: ["kitchen:read", "orders:read", "orders:status"],
    rolePermissions: ["kitchen:read", "orders:read", "orders:status"],
    customGrants: [],
    isLoading: false,
    isError: false,
    refetch: jest.fn(),
  }),
}));

jest.mock("@/api/business", () => ({
  getBusiness: jest.fn(() =>
    Promise.resolve({
      default_currency: "USD",
      timezone: "America/New_York",
    }),
  ),
  getMenu: jest.fn(() => Promise.resolve({ parsed_categories: [] })),
}));

import Kitchen from "@/components/business/Kitchen";
import type { Order } from "@/api/orders";

beforeEach(() => {
  jest.useFakeTimers();
  jest.setSystemTime(new Date(FILED_NOW));
});

afterEach(() => {
  cleanup();
  jest.useRealTimers();
});

it.each(["en", "es", "es-AR"] as const)(
  "settles the filed 22:12Z stamp to the New York clock in %s without a timezone prop",
  async (locale) => {
    mockLocale = locale;
    const order = {
      id: 1,
      bill_id: 10,
      business_id: 1,
      order_number: "K-NY",
      status: "approved",
      items: "[]",
      currency: "USD",
      created_at: FILED_UTC_FIRE,
      updated_at: FILED_UTC_FIRE,
    } as Order;

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

    expect(await screen.findByText("#K-NY")).toBeInTheDocument();
    expectVenueNyFireClock(
      screen.getByTestId("kitchen-ticket-fire-time").textContent ?? "",
      locale,
    );
    const elapsed =
      screen.getByTestId("kitchen-ticket-elapsed").textContent ?? "";
    expect(elapsed).toMatch(/5m/);
    expect(elapsed).not.toMatch(/4h/);
  },
);
