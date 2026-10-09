import type { TableRowData } from "./TableRow";

/** Wave 4: list filter matches the operator-facing name OR the table_code
 *  (deep-links from AI Waiter carry the code, e.g. "CORE-T01"). */
export function matchesTableSearch(row: TableRowData, query: string): boolean {
  const q = query.toLowerCase().trim();
  if (!q) return true;
  return (
    row.name.toLowerCase().includes(q) ||
    row.table_code.toLowerCase().includes(q)
  );
}
