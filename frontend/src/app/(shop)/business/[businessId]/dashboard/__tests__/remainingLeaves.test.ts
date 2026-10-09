/**
 * Session P remaining leaves: L5-8, L6-2/5/39, L9-3, L5-15, F-cand-11, L3-22, L9-2.
 */
import fs from "node:fs";
import path from "node:path";
import {
  nextTabSearchParams,
  CROSS_RAIL_SHARED_PARAMS,
  TAB_PARAM_WHITELIST,
} from "../tabParams";
import { normalizeUrlParam } from "@/hooks/urlState";
import { ANALYTICS_PERIOD_VALUES } from "@/components/dashboard/Dashboard";

// __tests__ → dashboard → [businessId] → business → (shop) → app → src (6× ..)
const frontendSrc = path.resolve(__dirname, "../../../../../..");

function read(rel: string) {
  return fs.readFileSync(path.join(frontendSrc, rel), "utf-8");
}

describe("L5-8 CRM Ver cuentas → bills history + customer filter", () => {
  it("billTab survives nextTabSearchParams from CRM deep-link (wrong-tab half FIXED)", () => {
    const next = nextTabSearchParams(
      new URLSearchParams("tab=crm&sub=customers"),
      "bills",
      "billTab=history&billCustomer=12",
    );
    expect(next.get("billTab")).toBe("history");
    expect(next.get("billCustomer")).toBe("12");
  });

  it("CustomersTab still navigates with billTab=history&billCustomer", () => {
    const src = read("components/business/crm/CustomersTab.tsx");
    expect(src).toMatch(/billTab=history&billCustomer=/);
  });

  it("BillManager always passes customerFilter into getBusinessBills (unfiltered half)", () => {
    const src = read("components/business/BillManager.tsx");
    expect(src).toMatch(/customerId:\s*customerFilter/);
    // customerFilter is seeded from billCustomer
    expect(src).toMatch(/billCustomer/);
  });
});

describe("L6-2 period cross-rail shared param", () => {
  it("declares period on analytics and accounting", () => {
    expect(TAB_PARAM_WHITELIST.analytics).toContain("period");
    expect(TAB_PARAM_WHITELIST.accounting).toContain("period");
    expect(CROSS_RAIL_SHARED_PARAMS.has("period")).toBe(true);
  });

  it("carries period from analytics → accounting (cross-rail shared)", () => {
    const next = nextTabSearchParams(
      new URLSearchParams("tab=analytics&period=week&sub=revenue"),
      "accounting",
    );
    expect(next.get("period")).toBe("week");
    // sub must NOT leak
    expect(next.get("sub")).toBeNull();
  });

  it("does not carry period onto a rail that does not declare it", () => {
    const next = nextTabSearchParams(
      new URLSearchParams("tab=analytics&period=week"),
      "menu",
    );
    expect(next.get("period")).toBeNull();
  });
});

describe("L6-5 invalid period normalizes", () => {
  it("garbage period falls back to today", () => {
    expect(
      normalizeUrlParam("garbage", ANALYTICS_PERIOD_VALUES, "today"),
    ).toBe("today");
    expect(
      normalizeUrlParam("week", ANALYTICS_PERIOD_VALUES, "today"),
    ).toBe("week");
  });

  it("Dashboard uses useUrlState for period", () => {
    const src = read("components/dashboard/Dashboard.tsx");
    expect(src).toMatch(/useUrlState/);
    expect(src).toMatch(/key:\s*["']period["']/);
    expect(src).toMatch(/ANALYTICS_PERIOD_VALUES/);
  });
});

describe("L9-3 billId write-back", () => {
  it("handleViewBill writes billId to the URL", () => {
    const src = read("components/business/BillManager.tsx");
    expect(src).toMatch(/writeBillIdSearchParam/);
    expect(src).toMatch(/L9-3/);
    // close clears it
    expect(src).toMatch(/clearBillIdSearchParam\(\)/);
  });
});

describe("L5-15 staffSearch write-back + filtered empty framing", () => {
  it("StaffManagement writes staffSearch back to the URL", () => {
    const src = read("components/business/StaffManagement.tsx");
    expect(src).toMatch(/buildSearchWithParam/);
    expect(src).toMatch(/staffSearch/);
    expect(src).toMatch(/isFiltered=\{isStaffFiltered\}/);
  });

  it("StaffTable shows filtered empty when isFiltered", () => {
    const src = read("components/staff/StaffTable.tsx");
    expect(src).toMatch(/isFiltered/);
    expect(src).toMatch(/noStaffFound|filteredEmptyTitle/);
  });
});

describe("L3-22 permission gate does not silently strip ?tab=", () => {
  it("forbidden tab keeps URL and surfaces not-permitted panel", () => {
    const src = read("app/(shop)/business/[businessId]/dashboard/page.tsx");
    expect(src).toMatch(/dashboard-tab-not-permitted/);
    expect(src).toMatch(/tabNotPermittedTitle/);
    expect(src).toMatch(/L3-22 reclass/);
    expect(src).toMatch(/setUnknownTabKey\(tabFromUrl\)/);
  });

  it("no last-active-business restore mechanism exists (repro attempt)", () => {
    // Static scan of the mechanisms the scout named — none restore a business
    // over a cold-load deep link.
    const hooks = read("hooks/useBusinessDashboard.ts");
    expect(hooks).not.toMatch(/lastActiveBusiness|last_active_business/);
    expect(hooks).not.toMatch(/localStorage|sessionStorage/);
  });
});

describe("L9-2 UX half — cancel + stale banner (wiring smoke)", () => {
  it("ReservationsTodayCard uses loadReservationsToday + StaleWidgetBanner (no fabricated 0)", () => {
    const src = read("components/business/ReservationsTodayCard.tsx");
    expect(src).toMatch(/loadReservationsToday/);
    expect(src).toMatch(/StaleWidgetBanner/);
    expect(src).not.toMatch(/setTodayCount\(0\)/);
  });

  it("BusinessOverview uses loadOverviewCoreWidgets and tablesError banner", () => {
    const src = read("components/business/BusinessOverview.tsx");
    expect(src).toMatch(/loadOverviewCoreWidgets/);
    expect(src).toMatch(/tablesError/);
    expect(src).toMatch(/StaleWidgetBanner/);
    // Silent empty-array substitute on handleRefresh is gone
    expect(src).not.toMatch(/\.catch\(\(\)\s*=>\s*\(\{\s*tables:\s*\[\]\s*\}\)\)/);
  });

  it("shared StaleWidgetBanner component exists", () => {
    const src = read("components/business/shared/StaleWidgetBanner.tsx");
    expect(src).toMatch(/stale-widget-banner/);
    expect(src).toMatch(/urlState\.widgetStale/);
  });
});
