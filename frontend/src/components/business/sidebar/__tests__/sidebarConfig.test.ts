/**
 * Sidebar config guards the operator-app information architecture.
 *
 * Membership is generated from config/dashboard-tabs.json so every
 * route_kind: "dashboard_tab" key appears in exactly one group. FE-only
 * ordering and the finance/setup presentation overrides sit on top.
 */
import fs from "node:fs";
import path from "node:path";
import {
  PRIMARY_TABS,
  PRIMARY_TAB_GROUPS,
  SECONDARY_TABS,
  TODAY_TABS,
  allDashboardTabKeysFromConfig,
  sidebarGroupForTab,
  type TabKey,
} from "../sidebarConfig";

const CONFIG_PATH = path.resolve(
  __dirname,
  "../../../../../../config/dashboard-tabs.json",
);

const MIRROR_PATH = path.resolve(
  __dirname,
  "../../../../config/dashboard-tabs.json",
);

describe("sidebar config — generated from dashboard-tabs.json", () => {
  it("keeps the frontend mirror byte-identical to the repo-root source of truth", () => {
    // The root file is outside the ./frontend Docker build context, so the
    // bundle imports a tracked mirror. If this fails, re-copy:
    //   cp config/dashboard-tabs.json frontend/src/config/dashboard-tabs.json
    expect(fs.readFileSync(MIRROR_PATH, "utf8")).toBe(
      fs.readFileSync(CONFIG_PATH, "utf8"),
    );
  });

  it("places every dashboard_tab key in exactly one sidebar group (incl. fiscal)", () => {
    const configKeys = allDashboardTabKeysFromConfig();
    // printers is standalone in the JSON but still in PRIMARY; exclude it from
    // this assertion — only dashboard_tab keys are required.
    const sidebar = new Map<string, string>();
    for (const t of TODAY_TABS) sidebar.set(t, "today");
    for (const g of PRIMARY_TAB_GROUPS) {
      for (const t of g.tabs) {
        expect(sidebar.has(t)).toBe(false);
        sidebar.set(t, g.id);
      }
    }
    for (const t of SECONDARY_TABS) {
      expect(sidebar.has(t)).toBe(false);
      sidebar.set(t, "setup");
    }

    for (const key of configKeys) {
      expect(sidebar.has(key)).toBe(true);
    }
    // fiscal specifically — was orphaned before Task 32.
    expect(sidebar.get("fiscal")).toBe("setup");
    expect(SECONDARY_TABS).toContain("fiscal");
  });

  it("does not duplicate a tab across primary and secondary", () => {
    const both = PRIMARY_TABS.filter((t) => SECONDARY_TABS.includes(t));
    expect(both).toEqual([]);
  });

  it("assigns every primary (non-today) tab to exactly one section group", () => {
    const grouped = PRIMARY_TAB_GROUPS.flatMap((g) => g.tabs);
    const expected = PRIMARY_TABS.filter(
      (t) => !(TODAY_TABS as readonly string[]).includes(t),
    );
    expect(new Set(grouped).size).toBe(grouped.length);
    expect([...grouped].sort()).toEqual([...expected].sort());
  });

  it("surfaces daily-use operator tabs at the primary level", () => {
    for (const k of [
      "overview",
      "bills",
      "cash-register",
      "printers",
      "kitchen",
      "reservations",
      "menu",
      "tables",
      "analytics",
      "crm",
      "delivery",
      "inventory",
      "staff",
    ]) {
      expect(PRIMARY_TABS).toContain(k);
    }
  });

  it("keeps Counters under Setup — setup-only, not a live ops pickup board", () => {
    expect(SECONDARY_TABS).toContain("counter");
    expect(sidebarGroupForTab("counter")).toBe("setup");
    const operations = PRIMARY_TAB_GROUPS.find((g) => g.id === "operations");
    expect(operations?.tabs).not.toContain("counter");
    expect(PRIMARY_TABS).not.toContain("counter");
  });

  it("places cash-register immediately after bills in primary navigation", () => {
    expect(PRIMARY_TABS.slice(0, 4)).toEqual([
      "overview",
      "bills",
      "cash-register",
      "printers",
    ]);
  });

  it("groups printers with operations, not setup", () => {
    const operations = PRIMARY_TAB_GROUPS.find((g) => g.id === "operations");
    expect(operations?.tabs).toContain("printers");
    expect(SECONDARY_TABS).not.toContain("printers");
  });

  it("puts Accounting under a real Finance group with Analytics", () => {
    const finance = PRIMARY_TAB_GROUPS.find((g) => g.id === "finance");
    expect(finance).toBeDefined();
    expect(finance?.tabs).toEqual(
      expect.arrayContaining(["analytics", "accounting"]),
    );
    // No more financeAndSettings mislabel holding analytics+inventory.
    expect(PRIMARY_TAB_GROUPS.map((g) => g.id)).not.toContain(
      "financeAndSettings",
    );
    expect(sidebarGroupForTab("inventory")).toBe("management");
    expect(SECONDARY_TABS).not.toContain("accounting");
  });

  it("keeps genuine configuration under a labeled Setup group (not MORE)", () => {
    for (const k of [
      "counter",
      "business-page",
      "fiscal",
      "plugins",
      "settings",
    ] as TabKey[]) {
      expect(SECONDARY_TABS).toContain(k);
      expect(sidebarGroupForTab(k)).toBe("setup");
    }
  });

  it("matches the live config file on disk (no stale embedded copy)", () => {
    const onDisk = JSON.parse(fs.readFileSync(CONFIG_PATH, "utf8")) as Array<{
      key: string;
      route_kind: string;
    }>;
    const diskKeys = onDisk
      .filter((r) => r.route_kind === "dashboard_tab")
      .map((r) => r.key)
      .sort();
    expect(allDashboardTabKeysFromConfig().sort()).toEqual(diskKeys);
  });
});
