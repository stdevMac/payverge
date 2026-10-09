/** @jest-environment jsdom */
import { fireEvent, render, screen, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { BusinessOverviewList } from "./BusinessOverviewList";
import type { Business } from "@/api/business";
import type { DashboardSummary } from "@/api/analytics";
import { asDollars } from "@/types/money";

jest.mock("./BusinessOverviewPanel", () => ({
  __esModule: true,
  BusinessOverviewPanel: ({ business }: { business: Business }) => (
    <div data-testid="business-detail-panel">{business.name}</div>
  ),
  default: ({ business }: { business: Business }) => (
    <div data-testid="business-detail-panel">{business.name}</div>
  ),
}));

jest.mock("./CombinedOverviewPanel", () => ({
  __esModule: true,
  CombinedOverviewPanel: ({
    todayHasActivity,
  }: {
    todayHasActivity: boolean;
  }) => (
    <div data-testid="combined-overview-panel">
      {todayHasActivity ? "combined-series" : "noActivityToday"}
    </div>
  ),
  default: ({ todayHasActivity }: { todayHasActivity: boolean }) => (
    <div data-testid="combined-overview-panel">
      {todayHasActivity ? "combined-series" : "noActivityToday"}
    </div>
  ),
}));

const t = (key: string) => key.split(".").pop() || key;

const wrapper = ({ children }: { children: React.ReactNode }) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
};

function biz(
  id: number,
  name: string,
  currency: string,
  active = true,
  extras: Partial<Business> = {},
): Business {
  return {
    id,
    name,
    logo: "",
    is_active: active,
    default_currency: currency,
    owner_address: "0x0",
    address: { street: "", city: "", state: "", postal_code: "", country: "" },
    settlement_address: "",
    tipping_address: "",
    tax_rate: 0,
    service_fee_rate: 0,
    tax_inclusive: false,
    service_inclusive: false,
    ...extras,
  } as Business;
}

function summary(opts: {
  todayRevenue: number;
  todayTips: number;
  todayBills: number;
  activeBills: number;
  weekRevenue: number;
  weekBills: number;
  avgTicket: number;
}): DashboardSummary {
  return {
    today: {
      revenue: asDollars(opts.todayRevenue),
      tips: asDollars(opts.todayTips),
      transactions: opts.todayBills,
      bills: opts.todayBills,
    },
    week: {
      revenue: asDollars(opts.weekRevenue),
      tips: asDollars(0),
      transactions: opts.weekBills,
      bills: opts.weekBills,
      unique_customers: 0,
      average_ticket: asDollars(opts.avgTicket),
    },
    live: { active_bills: opts.activeBills },
    top_items: [],
  };
}

describe("BusinessOverviewList — portfolio master/detail", () => {
  const businesses = [
    biz(1, "La Parrilla", "USD"),
    biz(2, "Cafe Norte", "USD", false),
  ];
  const stats: Record<number, DashboardSummary> = {
    1: summary({
      todayRevenue: 840,
      todayTips: 50,
      todayBills: 8,
      activeBills: 2,
      weekRevenue: 5000,
      weekBills: 200,
      avgTicket: 25,
    }),
    2: summary({
      todayRevenue: 400,
      todayTips: 20,
      todayBills: 4,
      activeBills: 1,
      weekRevenue: 2000,
      weekBills: 100,
      avgTicket: 20,
    }),
  };

  function renderList({
    onManage = jest.fn(),
    statsOverride = stats,
    statsLoading = false,
  }: {
    onManage?: jest.Mock;
    statsOverride?: Record<number, DashboardSummary>;
    statsLoading?: boolean;
  } = {}) {
    return render(
      <BusinessOverviewList
        businesses={businesses}
        stats={statsOverride}
        statsLoading={statsLoading}
        onManage={onManage}
        navigatingBusinessId={null}
        t={t}
      />,
      { wrapper },
    );
  }

  it("defaults to combined portfolio metrics using revenue and paid bills today", () => {
    renderList();

    expect(
      screen.getByRole("button", { name: /allBusinesses/i }),
    ).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByText(/\$1,240\.00/)).toBeInTheDocument();
    expect(screen.getByTestId("portfolio-today-bills")).toHaveTextContent("12");
    expect(screen.queryByText(/^3$/)).toBeNull();
  });

  it("labels today vs this-week grain on hub cards and does not ask to select All", () => {
    renderList();

    expect(screen.getByText("todayRevenue")).toBeInTheDocument();
    expect(screen.getByText("weekRevenue")).toBeInTheDocument();
    expect(screen.getByTestId("overview-week-total")).toHaveTextContent(
      "$7,000.00",
    );
    expect(screen.getByTestId("portfolio-week-bills")).toHaveTextContent("300");
    expect(screen.getByTestId("combined-overview-panel")).toHaveTextContent(
      "combined-series",
    );
    expect(screen.queryByText(/selectBusinessHint/i)).toBeNull();

    const rows = screen.getAllByTestId("portfolio-business-row");
    expect(
      within(rows[0]).getByTestId("portfolio-row-grain-today"),
    ).toHaveTextContent("grainToday");
    expect(
      within(rows[0]).getByTestId("portfolio-row-grain-week"),
    ).toHaveTextContent(/this week|grainWeek/i);
  });

  it("says No activity today on the combined view when today is quiet", () => {
    renderList({
      statsOverride: {
        1: summary({
          todayRevenue: 0,
          todayTips: 0,
          todayBills: 0,
          activeBills: 0,
          weekRevenue: 5000,
          weekBills: 200,
          avgTicket: 25,
        }),
        2: summary({
          todayRevenue: 0,
          todayTips: 0,
          todayBills: 0,
          activeBills: 0,
          weekRevenue: 2000,
          weekBills: 100,
          avgTicket: 20,
        }),
      },
    });

    expect(screen.getByTestId("portfolio-today-bills")).toHaveTextContent("0");
    expect(screen.getByTestId("overview-week-total")).toHaveTextContent(
      "$7,000.00",
    );
    expect(screen.getByTestId("combined-overview-panel")).toHaveTextContent(
      "noActivityToday",
    );
    expect(screen.queryByText(/selectBusinessHint/i)).toBeNull();
  });

  it("shows revenue and paid bill volume for every business selector", () => {
    renderList();

    const rows = screen.getAllByTestId("portfolio-business-row");
    expect(rows).toHaveLength(2);
    expect(within(rows[0]).getByText("La Parrilla")).toBeInTheDocument();
    expect(within(rows[0]).getByText(/\$840\.00/)).toBeInTheDocument();
    expect(within(rows[0]).getByText("8")).toBeInTheDocument();
    expect(within(rows[1]).getByText("Cafe Norte")).toBeInTheDocument();
    expect(within(rows[1]).getByText("4")).toBeInTheDocument();
  });

  it("selects a business without navigating and shows its analytics panel", () => {
    renderList();

    fireEvent.click(
      screen.getByRole("button", { name: /viewMetrics.*La Parrilla/i }),
    );

    expect(screen.getByTestId("business-detail-panel")).toHaveTextContent(
      "La Parrilla",
    );
    expect(
      screen.getByRole("button", { name: /viewMetrics.*La Parrilla/i }),
    ).toHaveAttribute("aria-pressed", "true");
  });

  it("keeps direct business access separate from metric selection", () => {
    const onManage = jest.fn();
    renderList({ onManage });

    fireEvent.click(
      screen.getByRole("button", { name: /openBusiness.*La Parrilla/i }),
    );

    expect(onManage).toHaveBeenCalledTimes(1);
    expect(onManage).toHaveBeenCalledWith(businesses[0]);
    expect(screen.queryByTestId("business-detail-panel")).toBeNull();
  });

  it("distinguishes same-name venues in visible copy and accessible names", () => {
    const twins: Business[] = [
      biz(8, "Payverge Core Demo Kitchen", "USD", true, {
        custom_url: "demo-admin-8-core",
      }),
      biz(1, "Payverge Core Demo Kitchen", "USD", true, {
        custom_url: "demo-admin-1-core",
      }),
    ];
    render(
      <BusinessOverviewList
        businesses={twins}
        stats={{
          8: summary({
            todayRevenue: 10,
            todayTips: 0,
            todayBills: 1,
            activeBills: 0,
            weekRevenue: 20,
            weekBills: 2,
            avgTicket: 10,
          }),
          1: summary({
            todayRevenue: 4,
            todayTips: 0,
            todayBills: 1,
            activeBills: 0,
            weekRevenue: 8,
            weekBills: 1,
            avgTicket: 4,
          }),
        }}
        statsLoading={false}
        onManage={jest.fn()}
        navigatingBusinessId={null}
        t={t}
      />,
      { wrapper },
    );

    const diffs = screen.getAllByTestId("portfolio-row-differentiator");
    expect(diffs.map((el) => el.textContent)).toEqual([
      "demo-admin-8-core",
      "demo-admin-1-core",
    ]);
    expect(
      screen.getByRole("button", {
        name: /viewMetrics.*Payverge Core Demo Kitchen \(demo-admin-8-core\)/i,
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: /viewMetrics.*Payverge Core Demo Kitchen \(demo-admin-1-core\)/i,
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: /openBusiness.*Payverge Core Demo Kitchen \(demo-admin-8-core\)/i,
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: /openBusiness.*Payverge Core Demo Kitchen \(demo-admin-1-core\)/i,
      }),
    ).toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", {
        name: /viewMetrics.*demo-admin-1-core/i,
      }),
    );
    expect(screen.getByTestId("portfolio-detail-differentiator")).toHaveTextContent(
      "demo-admin-1-core",
    );
    expect(
      screen.getAllByRole("button", {
        name: /openBusiness.*Payverge Core Demo Kitchen \(demo-admin-1-core\)/i,
      }).length,
    ).toBeGreaterThanOrEqual(2);
  });

  it("links Add business to the register wizard with ?new=1 so owners are not bounced back to their first venue", () => {
    // The register page redirects any owner who already has a business to
    // that business's dashboard unless ?new=1 is present (MIN-3). This list
    // only renders for owners with businesses, so the bare path is a dead end.
    renderList();

    expect(screen.getByRole("link", { name: /addBusiness/i })).toHaveAttribute(
      "href",
      "/business/register?new=1",
    );
  });

  it("always shows venue name and Manage while summary metrics load", () => {
    renderList({ statsOverride: {}, statsLoading: true });

    expect(screen.queryByTestId("business-overview-skeleton")).toBeNull();
    const rows = screen.getAllByTestId("portfolio-business-row");
    expect(rows).toHaveLength(2);
    expect(within(rows[0]).getByText("La Parrilla")).toBeInTheDocument();
    expect(
      within(rows[0]).getByRole("button", { name: /openBusiness.*La Parrilla/i }),
    ).toBeInTheDocument();
    expect(within(rows[0]).getAllByText("—").length).toBeGreaterThan(0);
    expect(within(rows[1]).getByText("Cafe Norte")).toBeInTheDocument();
    expect(
      within(rows[1]).getByRole("button", { name: /openBusiness.*Cafe Norte/i }),
    ).toBeInTheDocument();
  });
});

describe("BusinessOverviewList add-business entry", () => {
  const venues = [biz(1, "Cafe Sur", "USD"), biz(2, "Cafe Norte", "USD")];

  function renderWith(canAddBusiness?: boolean) {
    return render(
      <BusinessOverviewList
        businesses={venues}
        stats={{}}
        statsLoading={false}
        onManage={jest.fn()}
        navigatingBusinessId={null}
        t={t}
        canAddBusiness={canAddBusiness}
      />,
      { wrapper },
    );
  }

  it("offers add business by default", () => {
    renderWith();
    expect(screen.getByRole("link", { name: /addBusiness/ })).toHaveAttribute(
      "href",
      "/business/register?new=1",
    );
  });

  it("hides add business on the public demo, where venue creation is refused", () => {
    renderWith(false);
    expect(screen.queryByRole("link", { name: /addBusiness/ })).toBeNull();
  });
});
