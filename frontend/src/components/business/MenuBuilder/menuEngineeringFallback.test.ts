/**
 * #834 — live evidence from business 86 on sha-0024205b6c01 (2026-08-22):
 *
 *   GET .../accounting/menu-engineering?period=day   → has_sales false, 3 dishes
 *   GET .../accounting/menu-engineering?period=week  → has_sales false, 3 dishes
 *   GET .../accounting/menu-engineering?period=month → has_sales true,  3 dishes,
 *                                                      median_qty_sold 28
 *   GET .../analytics/sales?period=week  → total_revenue 0    (Aug 17 → now)
 *   GET .../analytics/sales?period=month → total_revenue 11809.32 (Aug 1 → now)
 *
 * The panel defaults to `week`, so the owner lands on an empty matrix with no
 * indication that one tab over holds the whole history.
 */
import {
  WIDEST_ME_PERIOD,
  reportHasNoSales,
  shouldProbeWiderPeriod,
  widerPeriodWithSales,
} from "./menuEngineeringFallback";
import type { MenuEngineeringReport } from "@/api/menuEngineering";

function report(over: Partial<MenuEngineeringReport>): MenuEngineeringReport {
  return {
    period: "week",
    median_food_cost_pct: 0,
    median_qty_sold: 0,
    dishes: [],
    rollups: [],
    items_needing_cost: 0,
    sparse: true,
    has_sales: false,
    ...over,
  };
}

describe("#834 menu-engineering wider-period fallback", () => {
  it("probes month when the selected window reports no sales", () => {
    expect(shouldProbeWiderPeriod(report({ has_sales: false }), "week")).toBe(true);
    expect(shouldProbeWiderPeriod(report({ has_sales: false }), "day")).toBe(true);
  });

  it("never probes when already on the widest window", () => {
    expect(shouldProbeWiderPeriod(report({ has_sales: false }), "month")).toBe(false);
  });

  it("never probes a window that does have sales", () => {
    expect(shouldProbeWiderPeriod(report({ has_sales: true }), "week")).toBe(false);
  });

  it("does not probe on a failed load or on a backend without the flag", () => {
    expect(shouldProbeWiderPeriod(null, "week")).toBe(false);
    const legacy = report({});
    delete (legacy as Partial<MenuEngineeringReport>).has_sales;
    expect(reportHasNoSales(legacy)).toBe(false);
    expect(shouldProbeWiderPeriod(legacy, "week")).toBe(false);
  });

  it("offers month only when the probe actually found sales", () => {
    expect(widerPeriodWithSales(report({ has_sales: true }))).toBe(WIDEST_ME_PERIOD);
    expect(widerPeriodWithSales(report({ has_sales: false }))).toBeNull();
    expect(widerPeriodWithSales(null)).toBeNull();
  });

  it("keeps the offer honest when a costed dish list hides zero sales", () => {
    // Business 86's week report carries 3 dishes, all qty_sold 0. A dishes-based
    // heuristic would call that "has data"; has_sales is the honest signal.
    const week = report({
      dishes: [
        {
          menu_item_id: "demo-steak",
          menu_item_name: "Steak Plate",
          food_cost_pct: 0,
          qty_sold: 0,
          avg_price: 0 as MenuEngineeringReport["dishes"][number]["avg_price"],
          unit_cost: 0 as MenuEngineeringReport["dishes"][number]["unit_cost"],
          margin_per_unit: 0 as MenuEngineeringReport["dishes"][number]["margin_per_unit"],
          quadrant: "star",
          action: "protect",
          suggested_price: 0 as MenuEngineeringReport["dishes"][number]["suggested_price"],
        },
      ],
    });
    expect(reportHasNoSales(week)).toBe(true);
    expect(shouldProbeWiderPeriod(week, "week")).toBe(true);
  });
});

describe("#834 MenuBuilder wires the probe", () => {
  it("index.tsx probes on an empty window and feeds the matrix", async () => {
    const fs = await import("fs");
    const path = require("path") as typeof import("path");
    const src = fs.readFileSync(path.join(__dirname, "index.tsx"), "utf8");
    // The probe is gated (never fires on a window that has sales)...
    expect(src).toMatch(/shouldProbeWiderPeriod\(r, period\)/);
    // ...asks for the widest preset...
    expect(src).toMatch(/getMenuEngineering\(String\(businessId\), "month"\)/);
    // ...and the answer reaches the matrix rather than dying in state.
    expect(src).toMatch(/widerPeriodWithSales=\{meWiderPeriod\}/);
  });
});
