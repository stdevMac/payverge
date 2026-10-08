import type { InventoryMenuItemStatus } from "@/api/inventory";
import {
  combineCostHealthCoverage,
  dishRecipeCoverageFromStatuses,
  type DishRecipeCoverage,
} from "./dishRecipeCoverage";

function status(
  overrides: Partial<InventoryMenuItemStatus> & { has_recipe: boolean },
): InventoryMenuItemStatus {
  return {
    menu_item_id: overrides.menu_item_id ?? "item",
    menu_item_name: overrides.menu_item_name ?? "Dish",
    category_name: "Mains",
    manual_available: true,
    status: "ok",
    max_possible_servings: 10,
    recommended_available: true,
    shows_warning: false,
    blocks_sale: false,
    affected_inventory: [],
    warning_inventory: [],
    ...overrides,
  };
}

describe("dishRecipeCoverageFromStatuses", () => {
  it("flags 3 of 6 mapped dishes as incomplete (issue 181)", () => {
    const statuses = [
      status({ menu_item_id: "1", has_recipe: true }),
      status({ menu_item_id: "2", has_recipe: true }),
      status({ menu_item_id: "3", has_recipe: true }),
      status({ menu_item_id: "4", has_recipe: false }),
      status({ menu_item_id: "5", has_recipe: false }),
      status({ menu_item_id: "6", has_recipe: false }),
    ];
    const coverage = dishRecipeCoverageFromStatuses(statuses);
    expect(coverage).toEqual({
      mapped: 3,
      total: 6,
      ratio: 0.5,
      incomplete: true,
    });
  });

  it("is complete only when every status has a recipe", () => {
    const coverage = dishRecipeCoverageFromStatuses([
      status({ menu_item_id: "1", has_recipe: true }),
      status({ menu_item_id: "2", has_recipe: true }),
    ]);
    expect(coverage.incomplete).toBe(false);
    expect(coverage.ratio).toBe(1);
  });

  it("does not invent incompleteness when inventory summary has no dishes", () => {
    expect(dishRecipeCoverageFromStatuses([])).toEqual({
      mapped: 0,
      total: 0,
      ratio: null,
      incomplete: false,
    });
    expect(dishRecipeCoverageFromStatuses(undefined).incomplete).toBe(false);
  });
});

describe("combineCostHealthCoverage", () => {
  const incomplete: DishRecipeCoverage = {
    mapped: 3,
    total: 6,
    ratio: 0.5,
    incomplete: true,
  };

  it("caveats a healthy sales-weighted ratio when half the menu is unmapped", () => {
    expect(combineCostHealthCoverage(false, incomplete)).toEqual({
      lowCoverage: true,
      kind: "dish",
    });
  });

  it("keeps the sales caveat when dish mapping is complete", () => {
    expect(
      combineCostHealthCoverage(true, {
        mapped: 6,
        total: 6,
        ratio: 1,
        incomplete: false,
      }),
    ).toEqual({ lowCoverage: true, kind: "sales" });
  });

  it("does not caveat when both signals are healthy", () => {
    expect(
      combineCostHealthCoverage(false, {
        mapped: 6,
        total: 6,
        ratio: 1,
        incomplete: false,
      }),
    ).toEqual({ lowCoverage: false, kind: "none" });
  });
});
