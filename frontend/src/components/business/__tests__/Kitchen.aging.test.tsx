/** @jest-environment jsdom */
/**
 * F16: the default Kitchen list view must escalate the elapsed-time color the
 * same way the full-screen KDS does, so a stale order is visually distinct
 * from a fresh one.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const translations: Record<string, string> = {
  "businessDashboard.dashboard.kitchenManager.time.minutesAgo": "{minutes}m ago",
  "businessDashboard.dashboard.kitchenManager.time.hoursMinutesAgo":
    "{hours}h {minutes}m ago",
  "businessDashboard.dashboard.kitchenManager.time.elapsedAgo":
    "{duration} ago",
  "businessDashboard.dashboard.kitchenManager.order.billPrefix": "Bill #{billId}",
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

const mockGetAllActiveOrders = jest.fn();
jest.mock("@/api/orders", () => ({
  getOrders: jest.fn(() => Promise.resolve({ orders: [], total: 0 })),
  getAllActiveOrders: (...a: unknown[]) => mockGetAllActiveOrders(...a),
  getOrderStatusText: jest.fn((status: string) => status),
  parseOrderItems: jest.fn((items: string) => JSON.parse(items || "[]")),
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

const oldOrder = {
  id: 7,
  bill_id: 10,
  business_id: 1,
  order_number: "K-7",
  status: "approved",
  created_by: "server",
  approved_by: "server",
  notes: "",
  items: JSON.stringify([
    { id: "i1", menu_item_name: "Soup", quantity: 1, price: 12, options: [], subtotal: 12 },
  ]),
  // 40 minutes ago -> should trigger the red urgency class.
  created_at: new Date(Date.now() - 40 * 60_000).toISOString(),
  updated_at: new Date().toISOString(),
};

beforeEach(() => {
  jest.clearAllMocks();
  mockGetAllActiveOrders.mockResolvedValue({
    items: [oldOrder],
    metadata: { total: 1, pageSize: 100, pagesFetched: 1 },
    capped: false,
  });
});

it("color-codes a stale (40m) order's elapsed time with the red urgency class", async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <Kitchen
        businessId={1}
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />
    </QueryClientProvider>,
  );

  // Wait for the order card to render.
  await waitFor(() => expect(screen.getByText("#K-7")).toBeInTheDocument());

  // The elapsed-time text ("40m ago") must be wrapped in the urgency span.
  const elapsed = screen.getByText("40m ago");
  expect(elapsed.className).toContain("text-rose-600");
});

it("does not rose-pulse an 18-day-old order (L1-24 stale cap)", async () => {
  const ancient = {
    ...oldOrder,
    id: 99,
    order_number: "K-99",
    // ~18 days ago — must humanize and neutralize urgency
    created_at: new Date(Date.now() - 450 * 60 * 60_000).toISOString(),
  };
  mockGetAllActiveOrders.mockResolvedValue({
    items: [ancient],
    metadata: { total: 1, pageSize: 100, pagesFetched: 1 },
    capped: false,
  });

  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <Kitchen
        businessId={1}
        kitchenEnabled
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />
    </QueryClientProvider>,
  );

  await waitFor(() => expect(screen.getByText("#K-99")).toBeInTheDocument());

  const elapsed = screen.getByText("18d ago");
  expect(elapsed.className).not.toContain("text-rose-600");
  expect(elapsed.className).not.toContain("animate-pulse");
  expect(elapsed.className).toContain("text-ink-500");
  expect(screen.queryByText(/450h/)).not.toBeInTheDocument();
});
