/** @jest-environment jsdom */
/**
 * #662: the KDS must never present a UTC-derived clock as the venue clock.
 *
 * Staff (cooks / expo) reach the Kitchen tab through the dashboard's
 * synthesized staff business row, which carries no `timezone`, so the
 * `businessTimezone` prop is null and Kitchen has to resolve the venue zone
 * itself. `resolveBusinessTimeZone(null)` collapses "unknown" to "UTC", so
 * every paint before that round trip lands showed a wrong-but-plausible fire
 * time (22:12 instead of 18:12 for America/New_York) — exactly the reported
 * symptom. Hold the existing skeleton until the venue zone settles instead.
 */
import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "es", setLocale: jest.fn() }),
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
    permissions: ["kitchen:read", "orders:read"],
    rolePermissions: ["kitchen:read", "orders:read"],
    customGrants: [],
    isLoading: false,
    isError: false,
    refetch: jest.fn(),
  }),
}));

let resolveBusiness: (value: unknown) => void = () => {};
jest.mock("@/api/business", () => ({
  getBusiness: jest.fn(
    () =>
      new Promise((resolve) => {
        resolveBusiness = resolve;
      }),
  ),
  getMenu: jest.fn(() => Promise.resolve({ parsed_categories: [] })),
}));

import Kitchen from "@/components/business/Kitchen";
import type { Order } from "@/api/orders";

const CREATED_AT = "2026-08-19T22:12:00.000Z"; // 18:12 in America/New_York

function renderKitchen() {
  const order = {
    id: 1,
    bill_id: 10,
    business_id: 1,
    order_number: "K-NY",
    status: "approved",
    items: "[]",
    currency: "USD",
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
  } as Order;

  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      {/* No businessTimezone prop — the staff dashboard path. */}
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
}

it("does not paint a UTC fire time while the venue timezone is unresolved", async () => {
  renderKitchen();

  // Let every already-settled promise (tier, kitchen status, menu) flush. The
  // business lookup that carries the venue timezone is still in flight.
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });

  // The UTC rendering of CREATED_AT for `es` is "22:12"; the venue rendering
  // is "18:12". Neither the wrong clock nor the ticket it sits on may paint.
  expect(screen.queryByText(/22:12/)).not.toBeInTheDocument();
  expect(screen.queryByText("#K-NY")).not.toBeInTheDocument();
  expect(screen.getByRole("status")).toBeInTheDocument();

  await act(async () => {
    resolveBusiness({ default_currency: "USD", timezone: "America/New_York" });
    await Promise.resolve();
  });

  expect(await screen.findByText("#K-NY")).toBeInTheDocument();
  await waitFor(() => {
    expect(screen.getByTestId("kitchen-ticket-fire-time").textContent).toMatch(
      /18:12/,
    );
  });
  expect(screen.getByTestId("kitchen-ticket-fire-time").textContent).not.toMatch(
    /22:12/,
  );
});

it("still renders the board when the venue lookup fails, without blocking on it", async () => {
  renderKitchen();

  await act(async () => {
    resolveBusiness(undefined);
    await Promise.resolve();
  });

  expect(await screen.findByText("#K-NY")).toBeInTheDocument();
});
