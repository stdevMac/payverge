import { placedTablesCount } from "./placedTablesCount";

describe("L3-23 placedTablesCount", () => {
  it("counts assigned only (does not sum unassigned)", () => {
    expect(
      placedTablesCount({ assigned_tables: 3, unassigned_tables: 7 }),
    ).toBe(3);
  });

  it("returns 0 when summary missing", () => {
    expect(placedTablesCount(null)).toBe(0);
  });
});
