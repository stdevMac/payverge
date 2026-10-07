/** @jest-environment node */
import fs from "node:fs";
import path from "node:path";
import {
  nextTabSearchParams,
  TAB_PARAM_WHITELIST,
} from "@/app/(shop)/business/[businessId]/dashboard/tabParams";

const srcDir = path.resolve(__dirname, "..");

describe("Reservations and Inventory nested tabs are URL-backed (#455)", () => {
  it("whitelists sub on both rails", () => {
    expect(TAB_PARAM_WHITELIST.reservations).toContain("sub");
    expect(TAB_PARAM_WHITELIST.inventory).toContain("sub");
  });

  it("ReservationManager and InventoryManager use useSubTab", () => {
    const reservations = fs.readFileSync(
      path.join(srcDir, "ReservationManager.tsx"),
      "utf8",
    );
    const inventory = fs.readFileSync(
      path.join(srcDir, "InventoryManager.tsx"),
      "utf8",
    );
    expect(reservations).toMatch(/useSubTab/);
    expect(reservations).toMatch(/"settings"/);
    expect(inventory).toMatch(/useSubTab/);
    expect(inventory).toMatch(/"items"/);
  });

  it("preserves reservationsView when switching nested reservation destinations", () => {
    const next = nextTabSearchParams(
      new URLSearchParams(
        "tab=reservations&reservationsView=board&sub=reservations",
      ),
      "reservations",
      "sub=settings",
    );
    expect(next.get("tab")).toBe("reservations");
    expect(next.get("sub")).toBe("settings");
    expect(next.get("reservationsView")).toBe("board");
  });

  it("round-trips inventory ?sub=settings on same-rail navigation", () => {
    const next = nextTabSearchParams(
      new URLSearchParams("tab=inventory&sub=settings"),
      "inventory",
    );
    expect(next.get("sub")).toBe("settings");
  });
});
