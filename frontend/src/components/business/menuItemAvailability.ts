import type { OrderabilityState } from "@/api/orders";

/** Inventory states that mean the dish is 86'd / OOS for bundle composition. */
const BUNDLE_INVENTORY_86: ReadonlySet<string> = new Set(["inventory_out"]);

/**
 * L3-16: items eligible for new bundle composition (not 86'd).
 * Manual `is_available: false` *and* inventory-driven 86 states are excluded —
 * the first fix only filtered the manual flag, so OOS dishes stayed pickable.
 */
export function isMenuItemAvailableForBundle(item: {
  is_available?: boolean | null;
  orderability_state?: OrderabilityState | string | null;
}): boolean {
  // Treat missing/undefined as available (legacy rows); explicit false is 86'd.
  if (item.is_available === false) return false;
  const state = item.orderability_state;
  if (state && BUNDLE_INVENTORY_86.has(state)) return false;
  return true;
}

export function filterAvailableMenuItems<
  T extends {
    is_available?: boolean | null;
    orderability_state?: OrderabilityState | string | null;
  },
>(items: T[]): T[] {
  return items.filter(isMenuItemAvailableForBundle);
}
