/** @jest-environment jsdom */
import * as fs from "node:fs";
import * as path from "node:path";
import {
  TAB_REGISTRY,
  TAB_KEYS,
  isDeepLinkPageGated,
  createTabPrefetcher,
  prefetchTab,
} from "../tabRegistry";

/**
 * The registry is the FE definition point for a dashboard rail (loader, icon,
 * deep-link page gate). Membership stays owned by config/dashboard-tabs.json
 * — this suite pins the two so a declared-but-unwired tab (#64 fiscal) fails
 * here instead of reaching production as an orphaned route.
 */

const MIRROR_PATH = path.resolve(
  __dirname,
  "../../../../config/dashboard-tabs.json",
);

type ConfigRow = { key: string; route_kind: string };

function configDashboardTabKeys(): string[] {
  const rows = JSON.parse(fs.readFileSync(MIRROR_PATH, "utf8")) as ConfigRow[];
  return rows
    .filter((r) => r.route_kind === "dashboard_tab")
    .map((r) => r.key);
}

describe("tabRegistry — parity with config/dashboard-tabs.json", () => {
  it("declares every dashboard_tab key from the JSON, plus printers (standalone)", () => {
    const expected = [...configDashboardTabKeys(), "printers"].sort();
    expect([...TAB_KEYS].sort()).toEqual(expected);
  });

  it("gives every tab a rail icon component", () => {
    for (const key of TAB_KEYS) {
      const icon = TAB_REGISTRY[key].icon;
      // lucide-react glyphs are forwardRef components (object, not function).
      expect(["function", "object"]).toContain(typeof icon);
      expect(icon).toBeTruthy();
    }
  });

  it("assigns a distinct icon glyph to every tab (collapsed-rail ambiguity guard)", () => {
    const icons = TAB_KEYS.map((k) => TAB_REGISTRY[k].icon);
    expect(new Set(icons).size).toBe(icons.length);
  });
});

describe("isDeepLinkPageGated", () => {
  it("gates the tabs whose panels do not render their own lock notice", () => {
    expect(isDeepLinkPageGated("menu")).toBe(true);
    expect(isDeepLinkPageGated("tables")).toBe(true);
    expect(isDeepLinkPageGated("schedule")).toBe(true);
    expect(isDeepLinkPageGated("cash-register")).toBe(true);
  });

  it("returns false for tabs that self-gate and unknown keys", () => {
    expect(isDeepLinkPageGated("overview")).toBe(false);
    expect(isDeepLinkPageGated("bills")).toBe(false);
    expect(isDeepLinkPageGated("analytics")).toBe(false);
    expect(isDeepLinkPageGated("not-a-tab")).toBe(false);
  });
});

describe("createTabPrefetcher", () => {
  it("memoizes one promise per key (no duplicate chunk fetches)", async () => {
    const loaders = {
      a: jest.fn(() => Promise.resolve("A")),
      b: jest.fn(() => Promise.resolve("B")),
    };
    const prefetch = createTabPrefetcher(loaders);

    const p1 = prefetch("a");
    const p2 = prefetch("a");
    expect(p1).toBe(p2);
    await expect(p1).resolves.toBe("A");
    expect(loaders.a).toHaveBeenCalledTimes(1);

    await prefetch("b");
    expect(loaders.b).toHaveBeenCalledTimes(1);
  });

  it("swallows a failed chunk load and caches it (no retry storms)", async () => {
    const loaders = {
      bad: jest.fn(() => Promise.reject(new Error("chunk gone"))),
    };
    const prefetch = createTabPrefetcher(loaders);

    await expect(prefetch("bad")).resolves.toBeUndefined();
    await expect(prefetch("bad")).resolves.toBeUndefined();
    expect(loaders.bad).toHaveBeenCalledTimes(1);
  });
});

describe("prefetchTab", () => {
  it("is wired to the registry loaders", () => {
    expect(typeof prefetchTab).toBe("function");
  });
});
