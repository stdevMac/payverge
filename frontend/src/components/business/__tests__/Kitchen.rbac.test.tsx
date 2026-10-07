/** @jest-environment jsdom */
/**
 * When kitchen/orders is disabled, the activation card is owner/manager-only.
 * Non-manager staff see a placeholder so the owner-only CTA never reaches them.
 */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

type KitchenOrderFixture = ReturnType<typeof buildOrder>;
type MockOrdersResponse = { orders: KitchenOrderFixture[]; total: number };
type MockAllActiveOrdersResponse = {
  items: KitchenOrderFixture[];
  metadata: { total: number; pageSize: number; pagesFetched: number };
  capped: boolean;
  warning?: string;
};
type MockGetAllActiveOrdersOptions = {
  statuses?: string[];
  activeBillsOnly?: boolean;
};

const translations: Record<string, string> = {
  "kitchenDisplay.launch": "Launch Kitchen Display",
  "businessDashboard.dashboard.kitchenManager.buttons.startCooking": "Start Cooking",
  "businessDashboard.dashboard.kitchenManager.modal.orderNumber": "Order {orderNumber}",
  "businessDashboard.dashboard.kitchenManager.modal.startCooking": "Start Cooking in modal",
};

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => translations[key] ?? key,
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    access: null,
    loading: false,
    error: null,
    hasAccess: true,
    isSuspended: false,
    lockState: "active",
    aiConfigured: false,
    refetch: jest.fn(),
  }),
}));

jest.mock("@/api/kitchenOrders", () => ({
  getKitchenOrdersStatus: jest.fn(() =>
    Promise.resolve({ kitchen_enabled: false, orders_enabled: false }),
  ),
  toggleKitchenAndOrders: jest.fn(),
}));

const mockGetOrders = jest.fn<Promise<MockOrdersResponse>, [number, string?]>(() =>
  Promise.resolve({ orders: [], total: 0 }),
);
const mockGetAllActiveOrders = jest.fn<
  Promise<MockAllActiveOrdersResponse>,
  [number, MockGetAllActiveOrdersOptions?]
>(() =>
  Promise.resolve({
    items: [],
    metadata: { total: 0, pageSize: 100, pagesFetched: 0 },
    capped: false,
  }),
);
const mockUseSSEEvents = jest.fn();

jest.mock("@/api/orders", () => ({
  getOrders: (businessId: number, status?: string) =>
    mockGetOrders(businessId, status),
  getAllActiveOrders: (businessId: number, options?: MockGetAllActiveOrdersOptions) =>
    mockGetAllActiveOrders(businessId, options),
  getOrderStatusText: jest.fn((status: string) => status),
  parseOrderItems: jest.fn((items: string) => JSON.parse(items || "[]")),
}));

jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: (options: unknown) => {
    mockUseSSEEvents(options);
    return { retriesExhausted: false, reconnect: jest.fn() };
  },
}));

const mockUseAuth = jest.fn();
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => mockUseAuth(),
}));

// Kitchen derives its RBAC capabilities from the staff-permissions context
// (settings:write → canActivate, orders:status → canUpdateOrderStatus), not
// from the auth role alone. Drive that context per-test so RBAC assertions
// keep proving "no permission → no control".
let mockStaffPermissions: string[] = [];
jest.mock("@/contexts/StaffPermissionsContext", () => ({
  useStaffPermissionsContext: () => ({
    permissions: mockStaffPermissions,
    rolePermissions: mockStaffPermissions,
    customGrants: [],
    isLoading: false,
    isError: false,
    refetch: jest.fn(),
  }),
}));

// Kitchen loads business + menu on mount via axios; without these mocks the
// requests hit real XHR in jsdom and spew AggregateError noise.
jest.mock("../../../api/business", () => ({
  getBusiness: jest.fn(() => Promise.resolve({ default_currency: "USD" })),
  getMenu: jest.fn(() => Promise.resolve({ parsed_categories: [] })),
}));

import Kitchen from "@/components/business/Kitchen";

const baseStaff = {
  id: 1,
  name: "Test User",
  email: "test@example.com",
  business_id: 1,
  is_active: true,
};

const buildOrder = (status: "approved" | "in_kitchen" | "ready" = "approved") => ({
  id: 7,
  bill_id: 10,
  business_id: 1,
  order_number: "K-7",
  status,
  created_by: "server",
  approved_by: "server",
  notes: "",
  items: JSON.stringify([
    {
      id: "item-1",
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
});

// Effective permissions per staff role, mirroring the backend RBAC role
// defaults exercised by these assertions: only manager can activate
// (settings:write); manager/server/kitchen can move tickets (orders:status);
// host is read-only.
const rolePermissions: Record<string, string[]> = {
  manager: ["settings:write", "orders:status"],
  server: ["orders:status"],
  kitchen: ["orders:status"],
  host: [],
};

const renderWithRole = (
  role: string | null,
  options: {
    kitchenEnabled?: boolean;
    kitchenStatusLoading?: boolean;
    onOrderStatusChange?: jest.Mock;
  } = {},
) => {
  mockStaffPermissions = role === null ? [] : (rolePermissions[role] ?? []);
  mockUseAuth.mockReturnValue({
    isWeb3User: role === null,
    isStaffUser: role !== null,
    isOAuthUser: false,
    oauthData: null,
    staffData: role === null ? null : { ...baseStaff, role },
    refreshStaffData: jest.fn(),
    walletLinked: false,
    walletAddress: null,
    linkWallet: jest.fn(),
    unlinkWallet: jest.fn(),
    refreshOAuthUser: jest.fn(),
    isLoading: false,
    isInitialized: true,
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <Kitchen
        businessId={1}
        kitchenEnabled={options.kitchenEnabled ?? false}
        kitchenStatusLoading={options.kitchenStatusLoading ?? false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={options.onOrderStatusChange || jest.fn()}
      />
    </QueryClientProvider>,
  );
};

describe("Kitchen feature-off RBAC", () => {
  beforeEach(() => {
    mockStaffPermissions = [];
    mockUseAuth.mockReset();
    mockGetOrders.mockClear();
    mockGetOrders.mockResolvedValue({ orders: [], total: 0 });
    mockGetAllActiveOrders.mockClear();
    mockGetAllActiveOrders.mockResolvedValue({
      items: [],
      metadata: { total: 0, pageSize: 100, pagesFetched: 0 },
      capped: false,
    });
    mockUseSSEEvents.mockClear();
  });

  it("renders activation toggle for manager", async () => {
    renderWithRole("manager");
    await waitFor(() =>
      expect(screen.getByTestId("kitchen-activation-toggle")).toBeInTheDocument(),
    );
    expect(screen.queryByTestId("kitchen-disabled-placeholder")).not.toBeInTheDocument();
  });

  it("keeps the page header visible while the activation card is shown", async () => {
    renderWithRole("manager");
    await waitFor(() =>
      expect(screen.getByTestId("kitchen-activation-toggle")).toBeInTheDocument(),
    );
    expect(
      screen.getByText("businessDashboard.dashboard.kitchenManager.title"),
    ).toBeInTheDocument();
  });

  // Kitchen/counter cluster consolidation: the owner's inline toggle
  // duplicated the single source of truth in Settings → Kitchen. The owner
  // now gets a directed empty state with an "Open settings" CTA instead;
  // manager keeps the inline activation card (still tested above).
  it("renders a directed 'open settings' empty state for owner (staffData=null), not the inline toggle", async () => {
    renderWithRole(null);
    await waitFor(() =>
      expect(screen.getByTestId("kitchen-owner-empty-state")).toBeInTheDocument(),
    );
    expect(screen.queryByTestId("kitchen-activation-toggle")).not.toBeInTheDocument();
    expect(screen.queryByTestId("kitchen-disabled-placeholder")).not.toBeInTheDocument();
  });

  it.each(["host", "server", "kitchen"] as const)(
    "renders contact-owner placeholder for %s",
    async (role) => {
      renderWithRole(role);
      await waitFor(() =>
        expect(screen.getByTestId("kitchen-disabled-placeholder")).toBeInTheDocument(),
      );
      expect(screen.queryByTestId("kitchen-activation-toggle")).not.toBeInTheDocument();
    },
  );

  it.each(["host", "server", "kitchen"] as const)(
    "does not fetch orders or enable SSE for disabled %s placeholder",
    async (role) => {
      renderWithRole(role, { kitchenEnabled: false });

      await waitFor(() =>
        expect(screen.getByTestId("kitchen-disabled-placeholder")).toBeInTheDocument(),
      );
      expect(mockUseSSEEvents).toHaveBeenCalled();
      const sseEnabledStates = mockUseSSEEvents.mock.calls.map(([options]) => {
        return (options as { enabled?: boolean }).enabled;
      });
      expect({
        orderFetches: mockGetAllActiveOrders.mock.calls.length,
        sseEnabledStates,
        allSSECallsDisabled: sseEnabledStates.every((enabled) => enabled === false),
      }).toEqual({
        orderFetches: 0,
        sseEnabledStates: sseEnabledStates.map(() => false),
        allSSECallsDisabled: true,
      });
    },
  );

  it("renders host Kitchen as read-only when enabled", async () => {
    const onOrderStatusChange = jest.fn().mockResolvedValue(undefined);
    mockGetAllActiveOrders.mockResolvedValue({
      items: [buildOrder("approved")],
      metadata: { total: 1, pageSize: 100, pagesFetched: 1 },
      capped: false,
    });
    renderWithRole("host", { kitchenEnabled: true, onOrderStatusChange });

    const orderHeading = await screen.findByText("#K-7");
    fireEvent.click(orderHeading);
    await waitFor(() => expect(screen.getByText("Order K-7")).toBeInTheDocument());
    expect(screen.queryByText("Start Cooking in modal")).not.toBeInTheDocument();
    expect(screen.queryByText("Start Cooking")).not.toBeInTheDocument();
    expect(screen.queryByText("Launch Kitchen Display")).not.toBeInTheDocument();
    expect(onOrderStatusChange).not.toHaveBeenCalled();
  });

  it("scopes standalone fallback order loads to active bills", async () => {
    mockGetAllActiveOrders.mockResolvedValue({
      items: [buildOrder("approved")],
      metadata: { total: 1, pageSize: 100, pagesFetched: 1 },
      capped: false,
    });

    renderWithRole("manager", { kitchenEnabled: true });

    await waitFor(() => expect(screen.getByText("#K-7")).toBeInTheDocument());
    // Pending is fetched too so the Needs-approval strip can surface brand-new
    // orders; the board itself still only renders approved+ tickets.
    expect(mockGetAllActiveOrders).toHaveBeenCalledWith(1, {
      statuses: [
        "pending",
        "approved",
        "in_kitchen",
        "ready",
        "delivered",
        "cancelled",
      ],
      activeBillsOnly: true,
    });
  });

  it.each(["manager", "server", "kitchen"] as const)(
    "renders Kitchen mutation controls for %s when enabled",
    async (role) => {
      const onOrderStatusChange = jest.fn().mockResolvedValue(undefined);
      mockGetAllActiveOrders.mockResolvedValue({
        items: [buildOrder("approved")],
        metadata: { total: 1, pageSize: 100, pagesFetched: 1 },
        capped: false,
      });
      renderWithRole(role, { kitchenEnabled: true, onOrderStatusChange });

      await waitFor(() => expect(screen.getByText("#K-7")).toBeInTheDocument());
      const startCooking = screen.getByText("Start Cooking");
      expect(startCooking).toBeInTheDocument();
      expect(screen.getByText("Launch Kitchen Display")).toBeInTheDocument();

      fireEvent.click(startCooking);

      await waitFor(() =>
        expect(onOrderStatusChange).toHaveBeenCalledWith(7, "in_kitchen", "kitchen"),
      );
    },
  );
});
