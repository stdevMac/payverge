/**
 * Space-card table/seat counts must match the editor canvas.
 *
 * The card used to read `published || draft` layout JSON only. An empty
 * published `{}` parses as a truthy document, so draft tables were ignored.
 * The editor then auto-places assigned QR tables that were never written
 * into that JSON — Properties showed 10 while the card said 0.
 * Counts stay honest without an editor-open draft PUT.
 */

import {
  layoutTableStats,
  parseLayoutDocument,
  type LayoutDocument,
  type Space,
} from "@/api/spaces";
import { autoPlaceMissingTables } from "./autoPlaceTables";
import { editorToLayout, layoutToEditor } from "./editor/types";
import {
  tableHasSpaceAssignment,
  toSpaceTableRef,
  type ExistingTableLike,
} from "./spacesOverviewKpis";

function asPositiveInt(value: unknown): number {
  const n = Number(value);
  return Number.isFinite(n) && n > 0 ? Math.trunc(n) : 0;
}

function isActiveTable(table: ExistingTableLike): boolean {
  return table.is_active !== false && asPositiveInt(table.id) > 0;
}

function layoutTableCount(layout: LayoutDocument | null): number {
  return layout?.tables?.length ?? 0;
}

function layoutHasRoom(layout: LayoutDocument | null): boolean {
  if (!layout) return false;
  return (layout.width_mm ?? 0) > 0 || Boolean(layout.boundary);
}

/** Draft first (what the editor loads). Empty published `{}` must not win. */
export function preferredSpaceLayout(space: {
  published_layout_json?: unknown;
  draft_layout_json?: unknown;
}): LayoutDocument | null {
  const published = parseLayoutDocument(space.published_layout_json);
  const draft = parseLayoutDocument(space.draft_layout_json);
  if (layoutTableCount(draft) > 0) return draft;
  if (layoutTableCount(published) > 0) return published;
  if (layoutHasRoom(draft)) return draft;
  if (layoutHasRoom(published)) return published;
  return draft ?? published;
}

/**
 * Tables the editor would auto-place onto this space.
 * Prefer explicit `space_id`. On a single-space venue whose live rows omit
 * `space_id`, attribute every active table (demo / pre-assignment payload).
 */
export function tablesAssignedToSpace(
  spaceId: number,
  tables: ExistingTableLike[],
  spaceCount: number,
): ExistingTableLike[] {
  const active = tables.filter(isActiveTable);
  const matching = active.filter(
    (t) => asPositiveInt(t.space_id) === spaceId,
  );
  if (matching.length > 0) return matching;
  if (active.some(tableHasSpaceAssignment)) return [];
  if (spaceCount === 1) return active;
  return [];
}

export function spaceCardStats(
  space: Space,
  assignedTables: ExistingTableLike[],
  spaceCount: number,
): { tableCount: number; maxSeats: number } {
  const layout = preferredSpaceLayout(space);
  const doc = layoutToEditor(layout, {
    width_mm: space.width_mm,
    height_mm: space.height_mm,
    measurement_unit: space.measurement_unit,
  });
  const assigned = tablesAssignedToSpace(space.id, assignedTables, spaceCount);
  const seeded = autoPlaceMissingTables(doc, assigned.map(toSpaceTableRef));
  return layoutTableStats(editorToLayout(seeded.doc));
}
