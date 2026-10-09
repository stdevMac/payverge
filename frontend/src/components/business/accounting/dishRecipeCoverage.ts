import type { InventoryMenuItemStatus } from "@/api/inventory";

export type DishRecipeCoverage = {
  mapped: number;
  total: number;
  /** mapped/total when total > 0, otherwise null. */
  ratio: number | null;
  /** True when some sellable dishes have no recipe — food% is not a healthy KPI. */
  incomplete: boolean;
};

/**
 * Dish-level recipe coverage from inventory summary menu_item_statuses.
 * Sales-weighted recipe_coverage_pct can look "healthy" while half the menu
 * has no recipe (issue 181). Incomplete mapping is mapped < total.
 */
export function dishRecipeCoverageFromStatuses(
  statuses: InventoryMenuItemStatus[] | null | undefined,
): DishRecipeCoverage {
  const list = Array.isArray(statuses) ? statuses : [];
  const total = list.length;
  const mapped = list.filter((row) => row.has_recipe).length;
  if (total <= 0) {
    return { mapped: 0, total: 0, ratio: null, incomplete: false };
  }
  return {
    mapped,
    total,
    ratio: mapped / total,
    incomplete: mapped < total,
  };
}

export type CostHealthCoverageKind = "none" | "sales" | "dish";

/**
 * Prefer dish-mapping honesty over sales-weighted coverage. A 10% food-cost
 * headline can look "healthy" when mapped dishes dominate sales while half
 * the menu still has no recipe (issue 181).
 */
export function combineCostHealthCoverage(
  salesLowCoverage: boolean,
  dish: DishRecipeCoverage | undefined,
): { lowCoverage: boolean; kind: CostHealthCoverageKind } {
  if (dish?.incomplete) {
    return { lowCoverage: true, kind: "dish" };
  }
  if (salesLowCoverage) {
    return { lowCoverage: true, kind: "sales" };
  }
  return { lowCoverage: false, kind: "none" };
}
