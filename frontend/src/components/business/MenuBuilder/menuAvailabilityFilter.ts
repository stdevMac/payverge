import type { OrderabilityState } from "@/api/orders";

/** Inventory states that mean the dish is 86'd / OOS for operator filters. */
const INVENTORY_EMPTY_STATES: ReadonlySet<OrderabilityState> = new Set([
  "inventory_out",
]);

function isInventoryEightySixState(
  state?: OrderabilityState | string | null,
): boolean {
  return !!state && INVENTORY_EMPTY_STATES.has(state as OrderabilityState);
}

/**
 * Authoritative Menu Builder 86 signal. When the menu endpoint stamped
 * `orderability_state`, that map is the only sellability source — the same
 * one guest QR reads. A leftover catalog `inventory_status` from #766
 * (`available` + `out_of_stock` on Harvest Bowl) must not 86 the dish.
 * `inventory_status` is a fallback only for rows with no orderability stamp.
 */
export function isMenuItemInventoryOut(item: {
  orderability_state?: OrderabilityState | string | null;
  inventory_status?: string | null;
}): boolean {
  const state = item.orderability_state;
  if (state != null && state !== "") {
    return isInventoryEightySixState(state);
  }
  return item.inventory_status === "out_of_stock";
}

/**
 * The operator's own 86 switch, independent of inventory.
 *
 * Operator menu reads report `is_available` as the EFFECTIVE answer (manual off
 * OR inventory blocked), matching what guests see, and carry the stored switch
 * in `manual_available` (#727). Every control that writes availability has to
 * round-trip this one, or saving an inventory-blocked dish would persist a
 * manual 86 that survives the restock. Payloads without the field (legacy
 * caches, the add-item form, optimistic local rows) fall back to is_available.
 */
export function menuItemManualAvailable(item: {
  is_available?: boolean | null;
  manual_available?: boolean | null;
}): boolean {
  if (typeof item.manual_available === "boolean") return item.manual_available;
  return item.is_available !== false;
}

/**
 * Whether a menu item should appear under the "unavailable" filter.
 * Includes manual off and inventory_out (L3-8).
 */
export function isManualEightySix(item: {
  is_available?: boolean | null;
  manual_available?: boolean | null;
  orderability_state?: OrderabilityState | string | null;
}): boolean {
  return !menuItemManualAvailable(item) || item.orderability_state === "manual_disabled";
}

export function isUnavailableForFilter(item: {
  is_available?: boolean;
  manual_available?: boolean | null;
  orderability_state?: OrderabilityState | string;
  inventory_status?: string | null;
}): boolean {
  return isManualEightySix(item) || isMenuItemInventoryOut(item);
}
