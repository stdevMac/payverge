import type { InventoryItemHealth } from "@/api/inventory";

export interface ReorderRow {
  id: number;
  name: string;
  unit: string;
  onHand: number;
  threshold: number;
  /** Minimum order to get back to the reorder threshold. The item model has
   *  no par/target level, so this is a floor, not a full restock target. */
  suggested: number;
}

export function buildReorderRows(
  lowStock: InventoryItemHealth[],
  outOfStock: InventoryItemHealth[],
): ReorderRow[] {
  const byId = new Map<number, InventoryItemHealth>();
  [...outOfStock, ...lowStock].forEach((item) => byId.set(item.id, item));
  return Array.from(byId.values())
    .map((item) => ({
      id: item.id,
      name: item.name,
      unit: item.unit,
      onHand: item.current_quantity,
      threshold: item.reorder_threshold,
      suggested: Math.max(item.reorder_threshold - item.current_quantity, 0),
    }))
    .sort((a, b) => b.suggested - a.suggested);
}

const FORMULA_PREFIX_RE = /^[=+\-@\t\r]/;

function cell(value: string | number): string {
  const s = String(value ?? "");
  const safe = FORMULA_PREFIX_RE.test(s) ? `'${s}` : s;
  if (/[",\n]/.test(safe)) return `"${safe.replace(/"/g, '""')}"`;
  return safe;
}

export function buildReorderCsv(rows: ReorderRow[], headers: string[]): string {
  const lines = [headers.map(cell).join(",")];
  rows.forEach((row) => {
    lines.push(
      [row.name, row.unit, row.onHand, row.threshold, row.suggested]
        .map(cell)
        .join(","),
    );
  });
  return lines.join("\n");
}
