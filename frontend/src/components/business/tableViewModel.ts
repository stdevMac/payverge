/**
 * Shared table list view-model helpers (L3-27 chips vs rows).
 * Counts are always derived from the same row set the filter uses.
 */

export type TableRowLike = {
  is_active: boolean;
  status: string;
};

export type StatusFilter = "all" | "available" | "occupied" | "reserved" | "inactive";

export function filterTableRows<T extends TableRowLike>(
  rows: T[],
  statusFilter: StatusFilter,
  matchesSearch: (row: T) => boolean,
): T[] {
  return rows
    .filter((t) => {
      // L3-27 audit: "Todos los estados" / "all" must include inactive tables
      // so Lista agrees with Mapa. Prior code always dropped !is_active unless
      // the filter was explicitly "inactive".
      if (statusFilter === "all") return true;
      if (statusFilter === "inactive") return !t.is_active;
      if (!t.is_active) return false;
      return t.status === statusFilter;
    })
    .filter(matchesSearch);
}

export type TableStatusCounts = {
  total: number;
  available: number;
  occupied: number;
  reserved: number;
  inactive: number;
};

export function countTableStatuses(rows: TableRowLike[]): TableStatusCounts {
  const activeRows = rows.filter((row) => row.is_active);
  return {
    // Header "total" includes inactive so it matches Lista under "all".
    total: rows.length,
    available: activeRows.filter((row) => row.status === "available").length,
    occupied: activeRows.filter((row) => row.status === "occupied").length,
    reserved: activeRows.filter((row) => row.status === "reserved").length,
    inactive: rows.filter((row) => !row.is_active).length,
  };
}

/**
 * L3-27: one memo drives the list AND the hero chips.
 *
 * Faceted counting — the search narrows both, the status facet narrows only
 * the list. Counting after the status filter would make every chip except the
 * selected one read 0, which is why the two stages are separated here rather
 * than reusing `filterTableRows` output for the counts.
 */
export function buildTableViewModel<T extends TableRowLike>(
  rows: T[],
  statusFilter: StatusFilter,
  matchesSearch: (row: T) => boolean,
): { rows: T[]; counts: TableStatusCounts } {
  const searched = rows.filter(matchesSearch);
  return {
    rows: filterTableRows(searched, statusFilter, () => true),
    counts: countTableStatuses(searched),
  };
}
