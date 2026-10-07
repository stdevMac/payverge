/** @jest-environment jsdom */
/**
 * Related to #70 / #229 — Overview extras must not hide two tiles behind a
 * nested "More metrics" disclosure with no obvious collapse. Two-or-fewer
 * extras render inline; three-or-more use a button + chevron disclosure.
 */

import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const renderWithQuery = (ui: React.ReactElement) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
  );
};

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: jest.fn(),
}));

jest.mock("@/api/analytics", () => ({
  analyticsApi: { getDashboardSummary: jest.fn() },
}));

jest.mock("@/api/business", () => ({
  businessApi: { getBusinessTables: jest.fn() },
  getMenu: jest.fn(),
}));

jest.mock("@/api/inventory", () => ({
  inventoryApi: { getSummary: jest.fn() },
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

jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: () => ({
    degraded: false,
    blocked: false,
    reconnect: jest.fn(),
  }),
}));

jest.mock("@/components/common/CurrencyConverter", () => ({
  __esModule: true,
  CurrencyPrice: ({ amount }: { amount: number }) => (
    <span data-testid="currency-price">${amount.toFixed(2)}</span>
  ),
}));

jest.mock("../overview/ProactiveInsights", () => ({
  __esModule: true,
  default: () => null,
}));

jest.mock("../ReservationsTodayCard", () => ({
  __esModule: true,
  default: () => null,
}));

jest.mock("../staff/LiveTableGrid", () => ({
  __esModule: true,
  default: () => <div data-testid="live-table-grid" />,
}));

import BusinessOverview from "@/components/business/BusinessOverview";
import { analyticsApi } from "@/api/analytics";
import { businessApi, getMenu } from "@/api/business";
import { inventoryApi } from "@/api/inventory";
import { pluginAPI } from "@/api/plugins";
import { getDirectorProactiveInsights } from "@/api/directorConsole";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

const fakeBusiness = {
  id: 42,
  name: "Taqueria Verge",
  default_currency: "USD",
  display_currency: "USD",
} as const;

const baseDashboard = {
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

function primeMocks(dashboard: typeof baseDashboard) {
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
  (analyticsApi.getDashboardSummary as jest.Mock).mockResolvedValue(dashboard);
  (businessApi.getBusinessTables as jest.Mock).mockResolvedValue({
    tables: [{ id: 1 }, { id: 2 }],
  });
  (getMenu as jest.Mock).mockResolvedValue({
    parsed_categories: [{ items: [{ id: "a" }] }],
  });
  (inventoryApi.getSummary as jest.Mock).mockResolvedValue({
    out_of_stock_items: 1,
    low_stock_items: 0,
    settings: { low_stock_warnings_enabled: true },
  });
  (pluginAPI.protected.getAllPlugins as jest.Mock).mockResolvedValue({
    plugins: [],
  });
  (pluginAPI.business.getBusinessPlugins as jest.Mock).mockResolvedValue({
    plugins: [],
  });
  (getDirectorProactiveInsights as jest.Mock).mockResolvedValue({
    insights: [],
  });
}

async function waitForOverview() {
  await screen.findByTestId("overview-secondary-strip");
  await waitFor(() =>
    expect(
      (analyticsApi.getDashboardSummary as jest.Mock).mock.calls.length,
    ).toBeGreaterThan(0),
  );
}

describe("BusinessOverview — more metrics (#70 / #229)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("surfaces two extras inline without a nested More metrics disclosure", async () => {
    primeMocks(baseDashboard);
    renderWithQuery(<BusinessOverview business={fakeBusiness as never} />);
    await waitForOverview();

    const inline = await screen.findByTestId("overview-more-metrics-inline");
    expect(inline.children.length).toBe(2);
    expect(screen.queryByTestId("overview-more-metrics")).not.toBeInTheDocument();
    expect(inline.textContent).toMatch(/tables/i);
    expect(inline.textContent).toMatch(/inventory/i);
  });

  it("uses a chevron disclosure when three or more extras exist, and collapse is obvious", async () => {
    primeMocks({
      ...baseDashboard,
      today: {
        ...baseDashboard.today,
        by_current_staff: { tips_from_paid: 12, bills_created: 4 },
      },
    } as typeof baseDashboard & {
      today: {
        by_current_staff: { tips_from_paid: number; bills_created: number };
      };
    });
    renderWithQuery(<BusinessOverview business={fakeBusiness as never} />);
    await waitForOverview();

    expect(
      screen.queryByTestId("overview-more-metrics-inline"),
    ).not.toBeInTheDocument();
    const more = await screen.findByTestId("overview-more-metrics");
    expect(more.tagName.toLowerCase()).toBe("button");
    expect(more).toHaveAttribute("aria-expanded", "false");
    expect(more.querySelector("svg")).not.toBeNull();

    fireEvent.click(more);
    expect(more).toHaveAttribute("aria-expanded", "true");
    const panel = document.getElementById(more.getAttribute("aria-controls")!);
    expect(panel).toBeTruthy();
    expect(panel!.children.length).toBeGreaterThanOrEqual(3);

    fireEvent.click(more);
    expect(more).toHaveAttribute("aria-expanded", "false");
    expect(
      document.getElementById(more.getAttribute("aria-controls")!),
    ).toBeNull();
  });
});
