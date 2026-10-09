/** @jest-environment jsdom */
/**
 * #663: cold-load must not flash "kitchen disabled" while status is unknown.
 * #684: the live-order cap banner must not render twice inside Kitchen when
 * the parent dashboard already shows it.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const translations: Record<string, string> = {
  "businessDashboard.dashboard.kitchenManager.disabled.title":
    "Kitchen and orders are off",
  "businessDashboard.dashboard.kitchenManager.warnings.kitchenOrdersCapped":
    "Showing the oldest live tickets first.",
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
import { getAllActiveOrders, type Order } from "@/api/orders";
import { getBusiness } from "@/api/business";

const getAllActiveOrdersMock = getAllActiveOrders as jest.MockedFunction<
  typeof getAllActiveOrders
>;
const mockGetBusiness = getBusiness as jest.MockedFunction<typeof getBusiness>;

function renderKitchen(
  props: Partial<React.ComponentProps<typeof Kitchen>> = {},
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <Kitchen
        businessId={1}
        kitchenEnabled={null}
        kitchenStatusLoading
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
        {...props}
      />
    </QueryClientProvider>,
  );
}

it("does not flash the disabled empty state while kitchen status is unknown", async () => {
  renderKitchen({ kitchenEnabled: null, kitchenStatusLoading: true });
  await waitFor(() =>
    expect(
      screen.getByText(
        "businessDashboard.dashboard.kitchenManager.title",
      ),
    ).toBeInTheDocument(),
  );
  expect(screen.queryByTestId("kitchen-owner-empty-state")).not.toBeInTheDocument();
  expect(
    screen.queryByText("Kitchen and orders are off"),
  ).not.toBeInTheDocument();
  expect(screen.getByRole("status")).toBeInTheDocument();
});

it("does not flash kitchen-off when the flag is false but status is still loading", async () => {
  renderKitchen({
    kitchenEnabled: false,
    kitchenStatusLoading: true,
    businessTimezone: "America/New_York",
  });
  expect(
    screen.getByText("businessDashboard.dashboard.kitchenManager.title"),
  ).toBeInTheDocument();
  expect(screen.queryByTestId("kitchen-owner-empty-state")).not.toBeInTheDocument();
  expect(
    screen.queryByText("Kitchen and orders are off"),
  ).not.toBeInTheDocument();
  expect(screen.queryByText("Desactivado")).not.toBeInTheDocument();
  expect(screen.getByRole("status")).toBeInTheDocument();
  // Flush getBusiness / getMenu so the first-paint assertion stays honest
  // without leaving an act() warning after the status fetch is still pending.
  await waitFor(() => expect(screen.getByRole("status")).toBeInTheDocument());
  expect(screen.queryByTestId("kitchen-owner-empty-state")).not.toBeInTheDocument();
});

it("shows the disabled empty state only after status has settled off", async () => {
  renderKitchen({
    kitchenEnabled: false,
    kitchenStatusLoading: false,
    businessTimezone: "America/New_York",
  });
  expect(
    await screen.findByTestId("kitchen-owner-empty-state"),
  ).toBeInTheDocument();
  expect(screen.getByText("Kitchen and orders are off")).toBeInTheDocument();
});

it("promotes a false+loading parent flag to the live board once getBusiness says on", async () => {
  mockGetBusiness.mockResolvedValueOnce({
    default_currency: "USD",
    timezone: "America/New_York",
    kitchen_enabled: true,
    orders_enabled: true,
  } as Awaited<ReturnType<typeof getBusiness>>);
  const ticket = {
    id: 1,
    bill_id: 10,
    business_id: 1,
    order_number: "K-SEED",
    status: "approved",
    items: "[]",
    currency: "USD",
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  } as Order;
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function Harness() {
    const [enabled, setEnabled] = React.useState<boolean | null>(false);
    return (
      <Kitchen
        businessId={1}
        kitchenEnabled={enabled}
        kitchenStatusLoading
        onKitchenStatusChange={setEnabled}
        onOrderStatusChange={jest.fn()}
        globalOrders={{ 10: [ticket] }}
        globalOrdersLoaded
        businessTimezone="America/New_York"
      />
    );
  }
  render(
    <QueryClientProvider client={client}>
      <Harness />
    </QueryClientProvider>,
  );
  expect(screen.queryByTestId("kitchen-owner-empty-state")).not.toBeInTheDocument();
  expect(await screen.findByText("#K-SEED")).toBeInTheDocument();
});

it("shows the live board when kitchen is known-on even while status is refetching", async () => {
  const ticket = {
    id: 1,
    bill_id: 10,
    business_id: 1,
    order_number: "K-LIVE",
    status: "approved",
    items: "[]",
    currency: "USD",
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  } as Order;
  renderKitchen({
    kitchenEnabled: true,
    kitchenStatusLoading: true,
    globalOrders: { 10: [ticket] },
    globalOrdersLoaded: true,
  });
  expect(await screen.findByText("#K-LIVE")).toBeInTheDocument();
  expect(screen.queryByTestId("kitchen-owner-empty-state")).not.toBeInTheDocument();
  expect(
    screen.queryByText("Kitchen and orders are off"),
  ).not.toBeInTheDocument();
});

function approvedTicket(orderNumber: string): Order {
  return {
    id: 1,
    bill_id: 10,
    business_id: 1,
    order_number: orderNumber,
    status: "approved",
    items: "[]",
    currency: "USD",
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  } as Order;
}

it("does not re-render the parent live-order cap banner", async () => {
  const capCopy = "Showing the oldest live tickets first.";
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <>
        <div>{capCopy}</div>
        <Kitchen
          businessId={1}
          kitchenEnabled
          kitchenStatusLoading={false}
          onKitchenStatusChange={jest.fn()}
          onOrderStatusChange={jest.fn()}
          globalOrders={{ 10: [approvedTicket("K-1")] }}
          globalOrdersLoaded
          globalOrdersCapped
        />
      </>
    </QueryClientProvider>,
  );
  expect(await screen.findByText("#K-1")).toBeInTheDocument();
  expect(screen.queryByTestId("kitchen-order-cap-banner")).not.toBeInTheDocument();
  expect(screen.getAllByText(capCopy)).toHaveLength(1);
});

it("renders the cap banner once in standalone fallback when the live fetch is capped", async () => {
  const capCopy = "Showing the oldest live tickets first.";
  getAllActiveOrdersMock.mockResolvedValueOnce({
    items: [approvedTicket("K-SOLO")],
    metadata: { total: 1, page: 1, page_size: 100, total_pages: 1 },
    capped: true,
  });
  renderKitchen({
    kitchenEnabled: true,
    kitchenStatusLoading: false,
  });
  expect(await screen.findByText("#K-SOLO")).toBeInTheDocument();
  expect(screen.getAllByTestId("kitchen-order-cap-banner")).toHaveLength(1);
  expect(screen.getAllByText(capCopy)).toHaveLength(1);
});
