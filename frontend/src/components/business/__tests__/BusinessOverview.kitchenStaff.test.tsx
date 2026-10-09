/** @jest-environment jsdom */
/**
 * Kitchen staff hold bills:read but not the analytics summary permission, so
 * the overview never fetches the summary that the Active bills and Today's
 * orders tiles read. Those tiles used to render a confident "0" while orders
 * were pending; they must not render at all without the summary.
 */

import React from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// BusinessOverview reads the menu-item count via React Query now — wrap renders.
const renderWithQuery = (ui: React.ReactElement) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
  );
};

// --- Mocks -----------------------------------------------------------------

// Translation provider — return the key itself so assertions are deterministic
// regardless of locale state. Matches the pattern used by the AiWaiter tests.
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

// Business tier hook — pretend owner has full access.
jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: jest.fn(),
}));

// Analytics + business APIs used by the overview.
jest.mock("@/api/analytics", () => ({
  analyticsApi: {
    getDashboardSummary: jest.fn(),
  },
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getBusinessTables: jest.fn(),
  },
  getMenu: jest.fn(),
}));

jest.mock("@/api/inventory", () => ({
  inventoryApi: {
    getSummary: jest.fn(),
  },
}));

jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    protected: { getAllPlugins: jest.fn() },
    business: { getBusinessPlugins: jest.fn() },
  },
}));

jest.mock("@/api/directorConsole", () => ({
  getDirectorProactiveInsights: jest.fn(),
}));

// BusinessOverview now loads proactive insights via useProactiveInsights, which
// subscribes to the shared SSE stream. Stub the hook so the test doesn't open a
// real EventSource.
jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: () => ({ degraded: false, blocked: false, reconnect: jest.fn() }),
}));

// Children that make their own API calls — stub them out.
jest.mock("@/components/common/CurrencyConverter", () => ({
  __esModule: true,
  CurrencyPrice: ({ amount }: { amount: number }) => (
    <span data-testid="currency-price">${amount.toFixed(2)}</span>
  ),
}));

jest.mock("../overview/ProactiveInsights", () => ({
  __esModule: true,
  default: () => <div data-testid="proactive-insights" />,
}));

jest.mock("../ReservationsTodayCard", () => ({ __esModule: true, default: () => null }));
jest.mock("@/contexts/StaffPermissionsContext", () => ({
  useStaffPermissionsContext: () => ({
    // The kitchen role's default grants (backend/internal/server/rbac.go):
    // no overview:kpi / analytics, no tables:read.
    permissions: [
      "business:read",
      "menu:read",
      "orders:read",
      "orders:write",
      "orders:status",
      "orders:kitchen",
      "bills:read",
      "staff:read",
      "delivery:dispatch:read",
    ],
    rolePermissions: [],
    customGrants: [],
    isLoading: false,
    isError: false,
    refetch: () => {},
  }),
}));

jest.mock("../staff/LiveTableGrid", () => ({
  __esModule: true,
  default: () => <div data-testid="live-table-grid" />,
}));

// --- Imports (after mocks) -------------------------------------------------

import BusinessOverview from "@/components/business/BusinessOverview";
import { analyticsApi } from "@/api/analytics";
import { businessApi, getMenu } from "@/api/business";
import { inventoryApi } from "@/api/inventory";
import { pluginAPI } from "@/api/plugins";
import { getDirectorProactiveInsights } from "@/api/directorConsole";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { resetInstanceCacheForTests } from "@/hooks/useInstance";

// --- Fixtures --------------------------------------------------------------

const fakeBusiness = {
  id: 42,
  name: "Taqueria Verge",
  default_currency: "USD",
  display_currency: "USD",
} as any;

const dashboardFixture = {
  today: { revenue: 1234, tips: 50, transactions: 12, bills: 15 },
  week: {
    revenue: 8400,
    tips: 420,
    transactions: 90,
    bills: 110,
    unique_customers: 75,
    average_ticket: 28.5,
  },
  live: { active_bills: 3, active_bills_by_table: {} },
  top_items: [],
};

function primeMocks() {
  (useBusinessAccess as jest.Mock).mockReturnValue({
    access: null,
    loading: false,
    error: null,
    hasAccess: true,
    isSuspended: false,
    lockState: "active",
    aiConfigured: false,
    refetch: jest.fn(),
  });
  (analyticsApi.getDashboardSummary as jest.Mock).mockResolvedValue(
    dashboardFixture,
  );
  (businessApi.getBusinessTables as jest.Mock).mockResolvedValue({
    tables: [{ id: 1 }, { id: 2 }, { id: 3 }],
  });
  (getMenu as jest.Mock).mockResolvedValue({
    parsed_categories: [{ items: [{ id: "a" }, { id: "b" }] }],
  });
  (inventoryApi.getSummary as jest.Mock).mockResolvedValue({
    out_of_stock_items: 0,
    low_stock_items: 0,
    settings: { low_stock_warnings_enabled: true },
  });
  (pluginAPI.protected.getAllPlugins as jest.Mock).mockResolvedValue({
    plugins: [],
  });
  (pluginAPI.business.getBusinessPlugins as jest.Mock).mockResolvedValue({
    plugins: [],
  });
  (getDirectorProactiveInsights as jest.Mock).mockResolvedValue({ insights: [] });
}

// --- Tests ---------------------------------------------------------------

describe("BusinessOverview — kitchen staff without the analytics summary", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    primeMocks();
  });

  afterEach(() => resetInstanceCacheForTests());

  it("does not fetch the summary and shows no zero bills/orders tiles", async () => {
    renderWithQuery(
      <BusinessOverview business={fakeBusiness} isStaffUser staffRole="kitchen" />,
    );
    const strip = await screen.findByTestId("overview-secondary-strip");
    await waitFor(() =>
      expect(
        within(strip).getByText(
          "businessDashboard.overview.roleSpecific.kitchen.stats.kitchenStatus",
        ),
      ).toBeInTheDocument(),
    );
    expect(analyticsApi.getDashboardSummary).not.toHaveBeenCalled();
    expect(
      within(strip).queryByText("businessDashboard.overview.activeBills"),
    ).not.toBeInTheDocument();
    expect(
      within(strip).queryByText("businessDashboard.overview.todayOrders"),
    ).not.toBeInTheDocument();
  });
});
