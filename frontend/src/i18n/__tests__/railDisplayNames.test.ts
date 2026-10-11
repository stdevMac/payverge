/**
 * Root E / F-cand-5 — bills rail = Cuentas, fiscal rail = Facturas.
 * Sidebar must read RAIL_LABEL_KEYS so a future rename is one key.
 */
import { readFileSync } from "fs";
import { join } from "path";
import esBusinessDashboard from "../messages/es/businessDashboard.json";
import {
  RAIL_CANONICAL_ES,
  RAIL_LABEL_KEYS,
} from "../railDisplayNames";

describe("rail display names (Root E / F-cand-5)", () => {
  test("Spanish canonical rail labels are Cuentas (bills) and Facturas (fiscal)", () => {
    const tabs = (
      esBusinessDashboard as {
        tabs: Record<string, string>;
      }
    ).tabs;
    expect(tabs.bills).toBe(RAIL_CANONICAL_ES.bills);
    expect(tabs.fiscal).toBe(RAIL_CANONICAL_ES.fiscal);
    // The collision the audit named: same string on both rails must not return.
    expect(tabs.bills).not.toBe(tabs.fiscal);
  });

  test("DashboardSidebar reads RAIL_LABEL_KEYS for bills and fiscal labels", () => {
    const sidebarPath = join(
      __dirname,
      "../../components/business/DashboardSidebar.tsx",
    );
    const src = readFileSync(sidebarPath, "utf8");
    expect(src).toMatch(/RAIL_LABEL_KEYS/);
    expect(src).toMatch(/RAIL_LABEL_KEYS\.bills/);
    expect(src).toMatch(/RAIL_LABEL_KEYS\.fiscal/);
  });

  test("RAIL_LABEL_KEYS point at the tabs.* paths under businessDashboard", () => {
    expect(RAIL_LABEL_KEYS.bills).toBe("tabs.bills");
    expect(RAIL_LABEL_KEYS.fiscal).toBe("tabs.fiscal");
  });
});
