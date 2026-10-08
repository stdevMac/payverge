import type { Space, SpaceSummary, SpaceTableRef } from "@/api/spaces";
import {
  computeSpacesOverviewKpis,
  resolveUnassignedTables,
  tableHasSpaceAssignment,
  tableSeatCapacity,
  toSpaceTableRef,
} from "./spacesOverviewKpis";

function liveTable(
  id: number,
  extras: Partial<{
    capacity: number;
    space_id: number | null;
    is_active: boolean;
    name: string;
  }> = {},
): {
  id: number;
  business_id: number;
  name: string;
  table_code: string;
  capacity: number;
  is_active: boolean;
  space_id?: number | null;
} {
  const row = {
    id,
    business_id: 1,
    name: extras.name ?? `Table ${id}`,
    table_code: `T${id}`,
    capacity: extras.capacity ?? 4,
    is_active: extras.is_active ?? true,
  };
  if (Object.prototype.hasOwnProperty.call(extras, "space_id")) {
    return { ...row, space_id: extras.space_id ?? null };
  }
  return row;
}

function tenLiveTables(): ReturnType<typeof liveTable>[] {
  return Array.from({ length: 10 }, (_, i) => liveTable(i + 1));
}

const emptySummary: SpaceSummary = {
  total_spaces: 0,
  draft_spaces: 0,
  published_spaces: 0,
  archived_spaces: 0,
  unassigned_tables: 0,
  assigned_tables: 0,
};

describe("spacesOverviewKpis", () => {
  it("treats Live View tables as waiting-to-place when spaces summary is empty", () => {
    const kpis = computeSpacesOverviewKpis({
      spaces: [],
      summary: emptySummary,
      unassignedFromSpaces: [],
      existingTables: tenLiveTables(),
    });

    expect(kpis.waitingToPlace).toBe(true);
    expect(kpis.unassigned).toBe(10);
    expect(kpis.tablesPlaced).toBe(0);
    expect(kpis.spaces).toBe(0);
    expect(kpis.maxSeats).toBe(40);
    expect(kpis.unassignedTables).toHaveLength(10);
  });

  it("keeps max seats from existing table capacity when the unassigned list is missing", () => {
    const kpis = computeSpacesOverviewKpis({
      spaces: [],
      summary: { ...emptySummary, unassigned_tables: 10 },
      unassignedFromSpaces: [],
      existingTables: tenLiveTables(),
    });

    expect(kpis.waitingToPlace).toBe(true);
    expect(kpis.unassigned).toBe(10);
    expect(kpis.maxSeats).toBe(40);
  });

  it("uses spaces-summary unassigned rows when the tables API omits space_id", () => {
    const fromSpaces: SpaceTableRef[] = [
      {
        id: 1,
        business_id: 1,
        name: "T1",
        table_code: "T1",
        capacity: 4,
        is_active: true,
        space_id: null,
      },
    ];
    const kpis = computeSpacesOverviewKpis({
      spaces: [],
      summary: { ...emptySummary, unassigned_tables: 1, assigned_tables: 3 },
      unassignedFromSpaces: fromSpaces,
      existingTables: tenLiveTables(),
    });

    expect(kpis.tablesPlaced).toBe(3);
    expect(kpis.waitingToPlace).toBe(false);
    expect(kpis.unassigned).toBe(1);
    expect(kpis.unassignedTables.map((t) => t.id)).toEqual([1]);
  });

  it("honors space_id on the existing-tables payload when present", () => {
    const existing = [
      liveTable(1, { space_id: null, capacity: 4 }),
      liveTable(2, { space_id: 9, capacity: 6 }),
      { ...liveTable(3, { capacity: 2 }), space_id: 0 },
    ];

    const resolved = resolveUnassignedTables([], existing, 1);
    expect(resolved.map((t) => t.id)).toEqual([1, 3]);
    expect(tableHasSpaceAssignment(existing[1])).toBe(true);
  });

  it("does not invent unassigned tables when some are already assigned and space_id is unknown", () => {
    const resolved = resolveUnassignedTables([], tenLiveTables(), 4);
    expect(resolved).toEqual([]);
  });

  it("prefers layout seat counts once tables are on a floor plan", () => {
    const space = {
      id: 1,
      business_id: 1,
      name: "Main",
      space_type: "indoor",
      floor_level: 0,
      sort_order: 0,
      measurement_unit: "m",
      status: "published",
      layout_schema_version: 1,
      draft_revision: 1,
      published_revision: 1,
      has_unpublished_changes: false,
      created_at: "2026-07-01T00:00:00Z",
      updated_at: "2026-07-01T00:00:00Z",
      published_layout_json: {
        schema_version: 1,
        tables: [
          {
            table_id: 1,
            x_mm: 0,
            y_mm: 0,
            width_mm: 800,
            height_mm: 800,
            shape: "round",
            max_capacity: 6,
          },
        ],
      },
    } as Space;

    const kpis = computeSpacesOverviewKpis({
      spaces: [space],
      summary: { ...emptySummary, total_spaces: 1, assigned_tables: 1 },
      unassignedFromSpaces: [],
      existingTables: [liveTable(1, { space_id: 1, capacity: 4 })],
    });

    expect(kpis.waitingToPlace).toBe(false);
    expect(kpis.tablesPlaced).toBe(1);
    expect(kpis.maxSeats).toBe(6);
  });

  it("maps seat capacity from max_capacity when capacity is missing", () => {
    expect(tableSeatCapacity({ id: 1, max_capacity: 8 })).toBe(8);
    expect(toSpaceTableRef({ id: 9, name: "Patio 1", max_capacity: 5 }).capacity).toBe(
      5,
    );
  });
});
