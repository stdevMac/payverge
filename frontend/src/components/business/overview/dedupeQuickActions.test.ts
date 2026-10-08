/** @jest-environment node */
import {
  filterQuickActionsDuplicatingInsights,
  quickActionKeysSuppressedByInsights,
} from "./dedupeQuickActions";

describe("quickActionKeysSuppressedByInsights", () => {
  it("suppresses inventory Quick Action when a stock briefing is already shown", () => {
    const keys = quickActionKeysSuppressedByInsights([
      { type: "inventory_out_of_stock" },
    ]);
    expect(keys.has("inventory-alerts")).toBe(true);
    expect(keys.has("active-bills")).toBe(false);
  });

  it("suppresses active-bills when stale_open_bills is briefed", () => {
    const keys = quickActionKeysSuppressedByInsights([
      { type: "stale_open_bills" },
    ]);
    expect(keys.has("active-bills")).toBe(true);
  });
});

describe("filterQuickActionsDuplicatingInsights", () => {
  it("drops the inventory Quick Action when Premium Beef is already in briefings", () => {
    const actions = [
      { key: "enable-card-payments" },
      { key: "inventory-alerts" },
      { key: "active-bills" },
    ];
    const next = filterQuickActionsDuplicatingInsights(actions, [
      { type: "inventory_out_of_stock" },
    ]);
    expect(next.map((a) => a.key)).toEqual([
      "enable-card-payments",
      "active-bills",
    ]);
  });
});
