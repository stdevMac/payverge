import type { Space } from "@/api/spaces";
import {
  preferredSpaceLayout,
  spaceCardStats,
  tablesAssignedToSpace,
} from "./spaceCardStats";

function space(overrides: Partial<Space> = {}): Space {
  return {
    id: 7,
    business_id: 1,
    name: "Main dining",
    space_type: "indoor",
    floor_level: 0,
    sort_order: 0,
    measurement_unit: "m",
    status: "draft",
    layout_schema_version: 1,
    draft_revision: 1,
    published_revision: 0,
    has_unpublished_changes: false,
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-07-01T00:00:00Z",
    ...overrides,
  };
}

function liveTable(
  id: number,
  extras: { space_id?: number | null; capacity?: number } = {},
) {
  return {
    id,
    business_id: 1,
    name: `Table ${id}`,
    table_code: `T${id}`,
    capacity: extras.capacity ?? 4,
    is_active: true,
    ...(Object.prototype.hasOwnProperty.call(extras, "space_id")
      ? { space_id: extras.space_id ?? null }
      : {}),
  };
}

const tenAssigned = Array.from({ length: 10 }, (_, i) =>
  liveTable(i + 1, { space_id: 7 }),
);

describe("preferredSpaceLayout", () => {
  it("does not let an empty published {} mask draft tables", () => {
    const layout = preferredSpaceLayout({
      published_layout_json: {},
      draft_layout_json: {
        schema_version: 1,
        tables: [
          {
            table_id: 1,
            x_mm: 0,
            y_mm: 0,
            width_mm: 900,
            height_mm: 900,
            shape: "square",
            max_capacity: 4,
          },
        ],
      },
    });
    expect(layout?.tables).toHaveLength(1);
  });

  it("treats stringified empty published as empty", () => {
    const layout = preferredSpaceLayout({
      published_layout_json: "{}",
      draft_layout_json: {
        schema_version: 1,
        tables: [
          {
            table_id: 2,
            x_mm: 0,
            y_mm: 0,
            width_mm: 900,
            height_mm: 900,
            shape: "square",
            max_capacity: 2,
          },
        ],
      },
    });
    expect(layout?.tables).toHaveLength(1);
    expect(layout?.tables?.[0].table_id).toBe(2);
  });
});

describe("tablesAssignedToSpace", () => {
  it("filters by space_id when present", () => {
    const rows = [
      liveTable(1, { space_id: 7 }),
      liveTable(2, { space_id: 8 }),
      liveTable(3, { space_id: null }),
    ];
    expect(tablesAssignedToSpace(7, rows, 2).map((t) => t.id)).toEqual([1]);
  });

  it("attributes all live tables on a single-space venue when space_id is omitted", () => {
    const rows = Array.from({ length: 10 }, (_, i) => liveTable(i + 1));
    expect(tablesAssignedToSpace(7, rows, 1)).toHaveLength(10);
  });
});

describe("spaceCardStats", () => {
  it("counts assigned tables when layout JSON is empty (canvas auto-place source)", () => {
    const stats = spaceCardStats(space(), tenAssigned, 1);
    expect(stats.tableCount).toBe(10);
    expect(stats.maxSeats).toBe(40);
  });

  it("does not report 0 when published is {} and draft has tables", () => {
    const stats = spaceCardStats(
      space({
        published_layout_json: {},
        draft_layout_json: {
          schema_version: 1,
          tables: Array.from({ length: 3 }, (_, i) => ({
            table_id: i + 1,
            x_mm: i * 1000,
            y_mm: 0,
            width_mm: 900,
            height_mm: 900,
            shape: "square" as const,
            max_capacity: 4,
          })),
        },
      }),
      [],
      1,
    );
    expect(stats.tableCount).toBe(3);
    expect(stats.maxSeats).toBe(12);
  });

  it("unions layout tables with assigned-but-unplaced rows", () => {
    const stats = spaceCardStats(
      space({
        draft_layout_json: {
          schema_version: 1,
          tables: [
            {
              table_id: 1,
              x_mm: 0,
              y_mm: 0,
              width_mm: 900,
              height_mm: 900,
              shape: "square",
              max_capacity: 4,
            },
          ],
        },
      }),
      [
        liveTable(1, { space_id: 7, capacity: 4 }),
        liveTable(2, { space_id: 7, capacity: 6 }),
      ],
      1,
    );
    expect(stats.tableCount).toBe(2);
    expect(stats.maxSeats).toBe(10);
  });
});
