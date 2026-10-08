/**
 * Spaces overview KPI mapper.
 *
 * Tables stay the Live View / tables-API entity. This only classifies existing
 * rows as placed vs waiting-to-place so the strip cannot read 0/0/0 when the
 * venue already has QR tables.
 */

import {
  layoutTableStats,
  parseLayoutDocument,
  type Space,
  type SpaceSummary,
  type SpaceTableRef,
} from "@/api/spaces";
import { sumTableCapacities } from "./autoPlaceTables";
import { placedTablesCount } from "./placedTablesCount";

/** Minimal table row from Live View / GET .../tables (same `tables` entity). */
export type ExistingTableLike = {
  id: number;
  business_id?: number;
  name?: string;
  table_code?: string;
  capacity?: number;
  is_active?: boolean;
  space_id?: number | null;
  max_capacity?: number | null;
};

export type SpacesOverviewKpis = {
  spaces: number;
  tablesPlaced: number;
  maxSeats: number;
  unassigned: number;
  waitingToPlace: boolean;
  unassignedTables: SpaceTableRef[];
};

function asPositiveInt(value: unknown): number {
  const n = Number(value);
  return Number.isFinite(n) && n > 0 ? Math.trunc(n) : 0;
}

export function tableHasSpaceAssignment(table: ExistingTableLike): boolean {
  return asPositiveInt(table.space_id) > 0;
}

export function tableSeatCapacity(table: ExistingTableLike): number {
  return (
    asPositiveInt(table.max_capacity) ||
    asPositiveInt(table.capacity) ||
    0
  );
}

export function toSpaceTableRef(table: ExistingTableLike): SpaceTableRef {
  return {
    id: table.id,
    business_id: asPositiveInt(table.business_id),
    name: typeof table.name === "string" ? table.name : "",
    table_code: typeof table.table_code === "string" ? table.table_code : "",
    capacity: tableSeatCapacity(table),
    is_active: table.is_active !== false,
    space_id: tableHasSpaceAssignment(table)
      ? asPositiveInt(table.space_id)
      : null,
  };
}

function isActiveExistingTable(table: ExistingTableLike): boolean {
  return table.is_active !== false && asPositiveInt(table.id) > 0;
}

function rowExposesSpaceId(table: ExistingTableLike): boolean {
  return Object.prototype.hasOwnProperty.call(table, "space_id");
}

function layoutSeatTotal(spaces: Space[]): number {
  let seats = 0;
  for (const space of spaces) {
    const layout =
      parseLayoutDocument(space.published_layout_json) ||
      parseLayoutDocument(space.draft_layout_json);
    seats += layoutTableStats(layout).maxSeats;
  }
  return seats;
}

/**
 * Prefer the spaces-summary unassigned list. When that list is empty but Live
 * View already has tables (and none are assigned to a space), treat those
 * existing tables as waiting-to-place.
 */
export function resolveUnassignedTables(
  unassignedFromSpaces: SpaceTableRef[],
  existingTables: ExistingTableLike[],
  assignedCount: number,
): SpaceTableRef[] {
  const fromSpaces = unassignedFromSpaces.filter((t) => asPositiveInt(t.id) > 0);
  const liveActive = existingTables.filter(isActiveExistingTable);
  const liveExposesSpaceId =
    liveActive.length > 0 && liveActive.every(rowExposesSpaceId);

  if (liveExposesSpaceId) {
    return liveActive.filter((t) => !tableHasSpaceAssignment(t)).map(toSpaceTableRef);
  }
  if (fromSpaces.length > 0) {
    return fromSpaces.map(toSpaceTableRef);
  }
  if (assignedCount === 0 && liveActive.length > 0) {
    return liveActive.map(toSpaceTableRef);
  }
  return [];
}

export function computeSpacesOverviewKpis(input: {
  spaces: Space[];
  summary: SpaceSummary | null;
  unassignedFromSpaces: SpaceTableRef[];
  existingTables: ExistingTableLike[];
}): SpacesOverviewKpis {
  const tablesPlaced = placedTablesCount(input.summary);
  const unassignedTables = resolveUnassignedTables(
    input.unassignedFromSpaces,
    input.existingTables,
    tablesPlaced,
  );
  const summaryUnassigned = asPositiveInt(input.summary?.unassigned_tables);
  const unassigned = Math.max(summaryUnassigned, unassignedTables.length);
  const layoutSeats = layoutSeatTotal(input.spaces);
  const tableSeats = sumTableCapacities(unassignedTables);
  const maxSeats = layoutSeats > 0 ? layoutSeats : tableSeats;

  return {
    spaces: input.summary?.total_spaces ?? input.spaces.length,
    tablesPlaced,
    maxSeats,
    unassigned,
    waitingToPlace: unassigned > 0 && tablesPlaced === 0,
    unassignedTables,
  };
}
