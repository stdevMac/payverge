/**
 * Grid-place assigned tables that are missing from a floor layout draft.
 * Used when a venue already has QR tables and seeds a first space.
 */

import type { SpaceTableRef } from "@/api/spaces";
import {
  DEFAULT_ROOM_HEIGHT_MM,
  DEFAULT_ROOM_WIDTH_MM,
} from "./editor/geometry";
import {
  DEFAULT_TABLE_SIZE,
  type EditorDocument,
  type EditorTable,
} from "./editor/types";

let seedKeyCounter = 0;

function nextSeedKey(): string {
  seedKeyCounter += 1;
  return `seed-tbl-${seedKeyCounter}`;
}

export function sumTableCapacities(tables: SpaceTableRef[]): number {
  return tables.reduce((sum, t) => {
    const seats =
      (typeof t.max_capacity === "number" && t.max_capacity > 0
        ? t.max_capacity
        : 0) || (t.capacity > 0 ? t.capacity : 0);
    return sum + seats;
  }, 0);
}

/**
 * Ensure room bounds exist, then append any assigned tables not already on
 * the canvas into a simple grid. Returns a new document when changes were made.
 */
export function autoPlaceMissingTables(
  doc: EditorDocument,
  assigned: SpaceTableRef[],
): { doc: EditorDocument; placed: number } {
  const placedIds = new Set(
    doc.tables
      .map((t) => t.table_id)
      .filter((id): id is number => typeof id === "number" && id > 0),
  );
  const missing = assigned.filter((t) => t.id > 0 && !placedIds.has(t.id));
  if (missing.length === 0) {
    return { doc, placed: 0 };
  }

  const next: EditorDocument = {
    ...doc,
    tables: [...doc.tables],
    regions: [...doc.regions],
    elements: [...doc.elements],
  };

  if (!next.width_mm || !next.height_mm) {
    next.width_mm = DEFAULT_ROOM_WIDTH_MM;
    next.height_mm = DEFAULT_ROOM_HEIGHT_MM;
    next.boundary = {
      points_mm: [
        { x: 0, y: 0 },
        { x: next.width_mm, y: 0 },
        { x: next.width_mm, y: next.height_mm },
        { x: 0, y: next.height_mm },
      ],
      closed: true,
    };
  }

  const size = DEFAULT_TABLE_SIZE.square;
  const gap = 400;
  const cellW = size.width_mm + gap;
  const cellH = size.height_mm + gap;
  const margin = 600;
  const cols = Math.max(
    1,
    Math.floor((next.width_mm - margin * 2 + gap) / cellW),
  );
  const startIndex = next.tables.length;

  missing.forEach((table, i) => {
    const index = startIndex + i;
    const col = index % cols;
    const row = Math.floor(index / cols);
    const capacity = table.capacity > 0 ? table.capacity : 4;
    const placed: EditorTable = {
      clientKey: nextSeedKey(),
      table_id: table.id,
      name: table.name || `T${index + 1}`,
      x_mm: margin + col * cellW,
      y_mm: margin + row * cellH,
      width_mm: size.width_mm,
      height_mm: size.height_mm,
      rotation_deg: 0,
      shape: "square",
      min_capacity: Math.min(2, capacity),
      max_capacity: capacity,
      visible_seat_count: capacity,
      is_reservable: true,
      is_combinable: false,
      is_accessible: false,
    };
    next.tables.push(placed);
  });

  return { doc: next, placed: missing.length };
}
