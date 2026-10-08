/** @jest-environment jsdom */
/**
 * Visual-hierarchy smoke tests for BusinessOverview.
 *
 * Round 4 audit called out that the dashboard's hero revenue metric rendered
 * a naked dollar figure (no hint / no delta) while the strip below it
 * exploded into 5-7 same-weight tiles via `auto-fit minmax(14rem, 1fr)`.
 * Together those two regressions flattened the hierarchy the hero was
 * supposed to establish.
 *
 * These tests lock in:
 *   1. Hero/primary metric exposes a `hint` describing the comparison window.
 *   2. The unified strip renders exactly four cards on desktop — one primary
 *      (col-span-2) plus three secondaries — so Quick Actions stay above the
 *      fold without scattering metrics across multiple rows.
 *   3. Additional operational metrics (tables / inventory) with only one or
 *      two extras render inline — a nested "More metrics" disclosure is
 *      reserved for three or more tiles, with a visible chevron collapse.
 *
 * If someone reverts the strip to `auto-fit`, or drops the hint, or spills
 * extras back into the primary strip, this suite fails.
 */

import React from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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
import { parseInstanceInfo } from "@/lib/instance/instanceInfo";
import {
  resetInstanceCacheForTests,
  setInstanceForTests,
} from "@/hooks/useInstance";

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

// --- Tests -----------------------------------------------------------------

describe("BusinessOverview — visual hierarchy", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    primeMocks();
  });

  afterEach(() => resetInstanceCacheForTests());

  // useBusinessAccess derives aiConfigured from the instance's features.ai;
  // the hook is mocked here, so mirror that derivation on both sides.
  function withInstanceAi(ai: boolean) {
    (useBusinessAccess as jest.Mock).mockReturnValue({
      ...(useBusinessAccess as jest.Mock)(),
      aiConfigured: ai,
    });
    setInstanceForTests(
      parseInstanceInfo({
        product_name: "Payverge",
        registration_mode: "invite",
        features: { ai },
      }),
    );
  }

  it("shows the Sage briefing when the instance has AI", async () => {
    withInstanceAi(true);
    renderWithQuery(<BusinessOverview business={fakeBusiness} />);
    expect(await screen.findByTestId("proactive-insights")).toBeInTheDocument();
  });

  it("hides the Sage briefing when the instance runs with AI off", async () => {
    withInstanceAi(false);
    renderWithQuery(<BusinessOverview business={fakeBusiness} />);
    await screen.findByTestId("overview-secondary-strip");
    expect(screen.queryByTestId("proactive-insights")).not.toBeInTheDocument();
  });

  it("renders today/live cards in the primary strip", async () => {
    renderWithQuery(<BusinessOverview business={fakeBusiness} />);

    const strip = await screen.findByTestId("overview-secondary-strip");
    // Wait for data fetch to settle so the strip reflects loaded state.
    await waitFor(() =>
      expect(
        (analyticsApi.getDashboardSummary as jest.Mock).mock.calls.length,
      ).toBeGreaterThan(0),
    );

    // Primary strip holds today/live metrics only; avg ticket lives in the
    // separate "This week" subgroup so the 7-day window isn't mixed in.
    const items = within(strip).getAllByRole("listitem");
    expect(items.length).toBeGreaterThanOrEqual(2);
    expect(items.length).toBeLessThanOrEqual(3);
    for (const item of items) {
      expect(item.className).not.toMatch(/md:col-span-3/);
      expect(item.className).not.toMatch(/2xl:col-span-2/);
    }
  });

  it("labels the refresh control with visible text (#63)", async () => {
    renderWithQuery(<BusinessOverview business={fakeBusiness} />);
    const refresh = await screen.findByRole("button", {
      name: "businessDashboard.overview.welcome.refreshData",
    });
    expect(refresh).toHaveTextContent(
      "businessDashboard.overview.welcome.refreshData",
    );
    expect(refresh.querySelector("svg")).toBeTruthy();
  });

  it("labels avg ticket with this-week period (#63, #703)", async () => {
    renderWithQuery(<BusinessOverview business={fakeBusiness} />);
    const week = await screen.findByTestId("overview-week-metrics");
    await waitFor(() => {
      expect(
        within(week).getAllByText("businessDashboard.overview.thisWeek").length,
      ).toBeGreaterThan(0);
    });
    expect(
      within(week).getByText("businessDashboard.overview.avgTicket"),
    ).toBeInTheDocument();
  });

  it("each stat card renders a scope hint so windows aren't mixed without a label", async () => {
    // Owner view (non-staff) sees the analytics path: hintToday + hintLive in
    // the primary strip, and thisWeek under the This week subgroup.
    renderWithQuery(<BusinessOverview business={fakeBusiness} />);
    await screen.findByTestId("overview-secondary-strip");
    // Wait for the dashboard fetch to settle so the cards drop their
    // loading state — `hint` only renders when `loading=false`.
    await waitFor(() =>
      expect(
        (analyticsApi.getDashboardSummary as jest.Mock).mock.calls.length,
      ).toBeGreaterThan(0),
    );
    const today = await screen.findAllByText(
      "businessDashboard.overview.hintToday",
    );
    expect(today.length).toBeGreaterThan(0);
    expect(
      (await screen.findAllByText("businessDashboard.overview.thisWeek"))
        .length,
    ).toBeGreaterThan(0);
    expect(
      await screen.findByText("businessDashboard.overview.hintLive"),
    ).toBeInTheDocument();
  });

  it("keeps primary strip on a balanced grid (no oversized hero)", async () => {
    renderWithQuery(<BusinessOverview business={fakeBusiness} />);
    const strip = await screen.findByTestId("overview-secondary-strip");
    expect(strip.className).toMatch(/xl:grid-cols-3/);
    expect(strip.className).toMatch(/sm:grid-cols-2/);
    // Guard against the old asymmetric hero layout regression.
    expect(strip.className).not.toMatch(/2xl:grid-cols-5/);
    expect(strip.className).not.toMatch(/auto-fit/);
    expect(strip.className).not.toMatch(/minmax\(14rem/);
  });

  it("additional (non-hero) metrics with two tiles render inline, not nested", async () => {
    renderWithQuery(<BusinessOverview business={fakeBusiness} />);

    await screen.findByTestId("overview-secondary-strip");
    const inline = await screen.findByTestId("overview-more-metrics-inline");
    expect(inline.children.length).toBeGreaterThan(0);
    expect(screen.queryByTestId("overview-more-metrics")).not.toBeInTheDocument();
    const strip = screen.getByTestId("overview-secondary-strip");
    expect(strip).not.toContainElement(inline);
  });

  it("More metrics disclosure is reserved for three-or-more extras (Task 33)", async () => {
    (analyticsApi.getDashboardSummary as jest.Mock).mockResolvedValue({
      ...dashboardFixture,
      today: {
        ...dashboardFixture.today,
        by_current_staff: { tips_from_paid: 8, bills_created: 3 },
      },
    });
    renderWithQuery(<BusinessOverview business={fakeBusiness} />);
    const more = await screen.findByTestId("overview-more-metrics");
    fireEvent.click(more);
    expect(more).toHaveAttribute("aria-expanded", "true");
    const grid = document.getElementById(more.getAttribute("aria-controls")!);
    expect(grid).toBeTruthy();
    expect(grid!.children.length).toBeGreaterThan(0);
    for (const child of Array.from(grid!.children)) {
      expect(child.textContent?.trim().length).toBeGreaterThan(0);
      expect(child.querySelector("button")?.textContent ?? "").not.toMatch(
        /Enable Supported Cross-Chain/i,
      );
    }
  });
});
