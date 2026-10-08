import fs from "fs";
import path from "path";
import { findUnplacedActiveTables } from "./unplacedTables";

describe("L3-26 unplaced active tables", () => {
  it("lists active tables missing from the layout (incl. occupied)", () => {
    const live = [
      { table: { id: 1, is_active: true, name: "T1" }, status: "occupied" },
      { table: { id: 2, is_active: true, name: "T2" }, status: "available" },
      { table: { id: 3, is_active: false, name: "T3" }, status: "available" },
    ];
    // Only table 2 is drawn on the layout.
    const unplaced = findUnplacedActiveTables(live, [2]);
    expect(unplaced.map((t) => t.table.id)).toEqual([1]);
  });

  it("returns empty when every active table is on the layout", () => {
    const live = [
      { table: { id: 1, is_active: true }, status: "available" },
      { table: { id: 2, is_active: true }, status: "reserved" },
    ];
    expect(findUnplacedActiveTables(live, [1, 2])).toEqual([]);
  });

  it("TablesLiveMap mounts the unplaced list (not a dead helper)", () => {
    // L3-26: helper-only tests pass when the panel is never rendered.
    const src = fs.readFileSync(
      path.join(__dirname, "TablesLiveMap.tsx"),
      "utf8",
    );
    expect(src).toMatch(/findUnplacedActiveTables/);
    expect(src).toMatch(/data-testid="live-map-unplaced"/);
    expect(src).toMatch(/data-testid=\{`live-map-unplaced-\$\{tws\.table\.id\}`\}/);
  });
});
