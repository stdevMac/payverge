/** @jest-environment jsdom */
/**
 * Related to issue 832: the owner hub listed four cards carrying only two
 * distinct names. Two of them are demo venues owned by ANOTHER admin account,
 * surfaced only because the viewer holds the platform-admin role (backend
 * ListBusinessesForInsideUser adds `OR is_demo` for admins). The hub said
 * nothing about that, and folded the foreign venues' money into a combined
 * panel whose own subtitle says "across your businesses".
 */
import { render, screen, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import fs from "fs";
import path from "path";
import {
  BusinessOverviewList,
  isAdminScopedVenue,
} from "./BusinessOverviewList";
import type { Business } from "@/api/business";
import type { DashboardSummary } from "@/api/analytics";
import { asDollars } from "@/types/money";

jest.mock("./BusinessOverviewPanel", () => ({
  __esModule: true,
  BusinessOverviewPanel: () => <div data-testid="business-detail-panel" />,
  default: () => <div data-testid="business-detail-panel" />,
}));

jest.mock("./CombinedOverviewPanel", () => ({
  __esModule: true,
  CombinedOverviewPanel: ({ businesses }: { businesses: Business[] }) => (
    <div data-testid="combined-overview-panel">
      {businesses.map((b) => b.id).join(",")}
    </div>
  ),
  default: ({ businesses }: { businesses: Business[] }) => (
    <div data-testid="combined-overview-panel">
      {businesses.map((b) => b.id).join(",")}
    </div>
  ),
}));

// Mirrors the runtime interpolation so the count assertion is meaningful.
const t = (key: string) =>
  key === "overview.scope.combinedNote"
    ? "excludes {count}"
    : key.split(".").pop() || key;

const wrapper = ({ children }: { children: React.ReactNode }) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
};

function biz(
  id: number,
  name: string,
  extras: Partial<Business> = {},
): Business {
  return {
    id,
    name,
    logo: "",
    is_active: true,
    default_currency: "USD",
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

function summary(
  todayRevenue: number,
  weekRevenue: number,
  bills: number,
): DashboardSummary {
  return {
    today: {
      revenue: asDollars(todayRevenue),
      tips: asDollars(0),
      transactions: bills,
      bills,
    },
    week: {
      revenue: asDollars(weekRevenue),
      tips: asDollars(0),
      transactions: bills,
      bills,
      unique_customers: 0,
      average_ticket: asDollars(todayRevenue),
    },
    live: { active_bills: 0 },
    top_items: [],
  };
}

// The live shape: viewer is user 8; 85/86 are theirs, 1/2 are admin 1's clones
// of the same demo and carry the same two display names.
const MINE = biz(85, "Payverge AI Pro Demo Lounge", {
  business_id: "demo-admin-8-ai-pro",
  custom_url: "payverge-ai-pro-demo-lounge",
  user_id: 8,
  is_demo: true,
  demo_owner_user_id: 8,
});
const THEIRS = biz(2, "Payverge AI Pro Demo Lounge", {
  business_id: "demo-admin-1-ai-pro",
  custom_url: "demo-admin-1-ai-pro-demo-lounge",
  user_id: 1,
  is_demo: true,
  demo_owner_user_id: 1,
});

const STATS: Record<number, DashboardSummary> = {
  85: summary(100, 700, 3),
  2: summary(50, 350, 2),
};

function renderHub(viewerUserId?: number | null, businesses = [MINE, THEIRS]) {
  return render(
    <BusinessOverviewList
      businesses={businesses}
      stats={STATS}
      statsLoading={false}
      onManage={jest.fn()}
      navigatingBusinessId={null}
      t={t}
      viewerUserId={viewerUserId}
    />,
    { wrapper },
  );
}

describe("isAdminScopedVenue", () => {
  it("treats a venue owned by the viewer as theirs", () => {
    expect(isAdminScopedVenue(MINE, 8)).toBe(false);
  });

  it("treats a demo venue scoped to the viewer as theirs", () => {
    expect(
      isAdminScopedVenue(biz(9, "X", { user_id: 1, demo_owner_user_id: 8 }), 8),
    ).toBe(false);
  });

  it("flags a venue belonging to another account", () => {
    expect(isAdminScopedVenue(THEIRS, 8)).toBe(true);
  });

  it("claims nothing when the viewer id is unknown", () => {
    expect(isAdminScopedVenue(THEIRS, null)).toBe(false);
    expect(isAdminScopedVenue(THEIRS, undefined)).toBe(false);
  });
});

describe("BusinessOverviewList admin-scoped venues (issue 832)", () => {
  it("groups and labels venues the viewer does not own", () => {
    renderHub(8);

    expect(
      screen.getByTestId("portfolio-scope-owned-label"),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId("portfolio-scope-admin-label"),
    ).toBeInTheDocument();

    // Exactly one row is chipped, and it is the foreign one.
    const chips = screen.getAllByTestId("portfolio-row-admin-chip");
    expect(chips).toHaveLength(1);
    const chippedRow = chips[0].closest(
      '[data-testid="portfolio-business-row"]',
    );
    expect(
      within(chippedRow as HTMLElement).getByTestId(
        "portfolio-row-differentiator",
      ),
    ).toHaveTextContent("demo-admin-1-ai-pro-demo-lounge");
  });

  it("carries the scope in the accessible name so twins are told apart", () => {
    renderHub(8);

    expect(
      screen.getByRole("button", {
        name: /openBusiness.*demo-admin-1-ai-pro-demo-lounge.*adminChip/i,
      }),
    ).toBeInTheDocument();
    // The viewer's own venue keeps a clean name.
    expect(
      screen.getByRole("button", {
        name: /openBusiness Payverge AI Pro Demo Lounge \(payverge-ai-pro-demo-lounge\)$/i,
      }),
    ).toBeInTheDocument();
  });

  it("keeps another admin's demo money out of the combined portfolio totals", () => {
    renderHub(8);

    // 100 today / 700 this week is the viewer's venue alone; adding the foreign
    // venue would read 150 / 1050 and 5 bills.
    expect(screen.getByTestId("overview-currency-total")).toHaveTextContent(
      "$100.00",
    );
    expect(screen.getByTestId("overview-week-total")).toHaveTextContent(
      "$700.00",
    );
    expect(screen.getByTestId("portfolio-today-bills")).toHaveTextContent("3");
    expect(screen.getByTestId("portfolio-week-bills")).toHaveTextContent("3");
    expect(screen.getByTestId("combined-overview-panel")).toHaveTextContent(
      "85",
    );
    expect(
      screen.getByTestId("portfolio-combined-scope-note"),
    ).toHaveTextContent("excludes 1");
  });

  it("leaves an ordinary owner's hub untouched", () => {
    renderHub(8, [MINE]);

    expect(screen.queryByTestId("portfolio-scope-owned-label")).toBeNull();
    expect(screen.queryByTestId("portfolio-scope-admin-label")).toBeNull();
    expect(screen.queryByTestId("portfolio-row-admin-chip")).toBeNull();
    expect(screen.queryByTestId("portfolio-combined-scope-note")).toBeNull();
    expect(screen.getAllByTestId("portfolio-business-row")).toHaveLength(1);
  });

  it("labels nothing for a session with no numeric user id (web3)", () => {
    renderHub(null);

    expect(screen.queryByTestId("portfolio-row-admin-chip")).toBeNull();
    expect(screen.queryByTestId("portfolio-scope-admin-label")).toBeNull();
    // Both venues still count toward the combined view.
    expect(screen.getByTestId("overview-currency-total")).toHaveTextContent(
      "$150.00",
    );
  });

  it("ships the scope copy in both operator locales with the count slot", () => {
    for (const locale of ["en", "es"]) {
      const messages = JSON.parse(
        fs.readFileSync(
          path.join(__dirname, `../../i18n/messages/${locale}/dashboard.json`),
          "utf8",
        ),
      );
      const scope = messages.overview.scope;
      expect(scope.ownedLabel).toBeTruthy();
      expect(scope.adminLabel).toBeTruthy();
      expect(scope.adminHint).toBeTruthy();
      expect(scope.adminChip).toBeTruthy();
      // Dropping the slot would silently print a note with no number in it.
      expect(scope.combinedNote).toContain("{count}");
    }
  });

  it("dashboard page passes the signed-in account id to the hub", () => {
    const pageSrc = fs.readFileSync(
      path.join(__dirname, "../../app/(shop)/dashboard/page.tsx"),
      "utf8",
    );
    expect(pageSrc).toMatch(/viewerUserId=\{oauthData\?\.userId \?\? null\}/);
  });
});
