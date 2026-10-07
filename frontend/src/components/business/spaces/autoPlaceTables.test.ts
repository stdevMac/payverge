import { autoPlaceMissingTables, sumTableCapacities } from "./autoPlaceTables";
import type { EditorDocument } from "./editor/types";
import type { SpaceTableRef } from "@/api/spaces";

function emptyDoc(): EditorDocument {
  return {
    schema_version: 1,
    width_mm: 0,
    height_mm: 0,
    measurement_unit: "m",
    regions: [],
    elements: [],
    tables: [],
  };
}

const tables: SpaceTableRef[] = [
  {
    id: 1,
    business_id: 1,
    name: "Table 1",
    table_code: "T1",
    capacity: 4,
    is_active: true,
    space_id: 9,
  },
  {
    id: 2,
    business_id: 1,
    name: "Table 2",
    table_code: "T2",
    capacity: 2,
    is_active: true,
    space_id: 9,
  },
];

describe("autoPlaceTables", () => {
  it("sums capacities for max-seats framing", () => {
    expect(sumTableCapacities(tables)).toBe(6);
  });

  it("falls back to max_capacity when capacity is missing", () => {
    expect(
      sumTableCapacities([
        { id: 3, business_id: 1, name: "T3", table_code: "T3", capacity: 0, is_active: true, max_capacity: 8 },
      ]),
    ).toBe(8);
  });

  it("grids assigned tables missing from an empty draft", () => {
    const { doc, placed } = autoPlaceMissingTables(emptyDoc(), tables);
    expect(placed).toBe(2);
    expect(doc.width_mm).toBeGreaterThan(0);
    expect(doc.height_mm).toBeGreaterThan(0);
    expect(doc.tables.map((t) => t.table_id)).toEqual([1, 2]);
    expect(doc.tables[0].max_capacity).toBe(4);
  });

  it("skips tables already on the canvas", () => {
    const seeded = autoPlaceMissingTables(emptyDoc(), tables).doc;
    const again = autoPlaceMissingTables(seeded, tables);
    expect(again.placed).toBe(0);
    expect(again.doc.tables).toHaveLength(2);
  });
});
