/**
 * L3-26 residual: active tables that do not appear on a published layout
 * must still be reachable from map mode.
 */
export function findUnplacedActiveTables<
  T extends { table: { id: number; is_active?: boolean; name?: string }; status?: string },
>(
  liveTables: T[],
  layoutTableIds: Iterable<number>,
): T[] {
  const onLayout = new Set(layoutTableIds);
  return liveTables.filter(
    (tws) => tws.table.is_active !== false && !onLayout.has(tws.table.id),
  );
}
