import { InventoryItem } from "@/api/inventory";

export type InventoryStatus = "ok" | "low_stock" | "out_of_stock";

/**
 * The one definition of an item's stock status. Out beats low beats ok.
 * A zero `reorder_threshold` means "no reorder point set" and never triggers
 * low_stock. Consumed by the KPI tiles, status chips, table column, reorder
 * filter, and detail drawer so they can never disagree.
 */
export function itemStatus(item: InventoryItem): InventoryStatus {
  if (item.current_quantity <= 0) return "out_of_stock";
  if (item.reorder_threshold > 0 && item.current_quantity <= item.reorder_threshold) {
    return "low_stock";
  }
  return "ok";
}

/**
 * Stock value of one item in business currency. cost_per_unit is already
 * dollars. A negative (oversold, warn-mode) quantity is clamped to 0 — you
 * cannot hold negative shelf value, and an unclamped negative would subtract
 * from the total stock-value tile (INV-L3). The negative quantity itself stays
 * visible via itemStatus (out_of_stock) and the quantity column.
 */
export function selectItemValue(item: InventoryItem): number {
  // Round to cents — binary float residue (36 * 4.2 → 151.20000000000002)
  // must not surface as money on KPI tiles. (FIND-043)
  const raw = Math.max(0, item.current_quantity || 0) * (item.cost_per_unit || 0);
  return Math.round(raw * 100) / 100;
}

/** Total stock value across all items. */
export function selectTotalStockValue(items: InventoryItem[]): number {
  const sum = items.reduce((acc, it) => acc + selectItemValue(it), 0);
  return Math.round(sum * 100) / 100;
}
