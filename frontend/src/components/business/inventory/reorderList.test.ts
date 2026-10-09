import { buildReorderRows, buildReorderCsv } from "./reorderList";
import type { InventoryItemHealth } from "@/api/inventory";

const low: InventoryItemHealth[] = [
  {
    id: 1,
    name: "Flour",
    unit: "kg",
    current_quantity: 2,
    reorder_threshold: 10,
    status: "low_stock",
  },
  {
    id: 2,
    name: "=EVIL()",
    unit: "u",
    current_quantity: 0,
    reorder_threshold: 5,
    status: "out_of_stock",
  },
];

it("computes suggested minimum order back to threshold", () => {
  const rows = buildReorderRows(low, []);
  expect(rows[0].suggested).toBe(8); // 10 - 2
  expect(rows[1].suggested).toBe(5); // 5 - 0
});

it("escapes formula-prefixed names in the CSV", () => {
  const csv = buildReorderCsv(buildReorderRows(low, []), [
    "Name",
    "Unit",
    "On hand",
    "Reorder at",
    "Order at least",
  ]);
  expect(csv).toContain("'=EVIL()");
  expect(csv.split("\n")).toHaveLength(3); // header + 2 rows
});
