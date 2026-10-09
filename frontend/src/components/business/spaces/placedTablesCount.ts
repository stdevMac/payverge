/**
 * L3-23: "Mesas colocadas" / total tables on the floor uses assigned only.
 * Unassigned tables have their own aggregate chip.
 */
export function placedTablesCount(summary: {
  assigned_tables?: number;
  unassigned_tables?: number;
} | null | undefined): number {
  if (!summary) return 0;
  return Number(summary.assigned_tables) || 0;
}
