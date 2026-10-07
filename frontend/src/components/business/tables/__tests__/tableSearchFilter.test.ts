import { matchesTableSearch } from "../tableSearchFilter";
import type { TableRowData } from "../TableRow";

const row: TableRowData = {
  id: 1,
  name: "Patio 1",
  table_code: "CORE-T01",
  is_active: true,
  status: "available",
  capacity: 4,
  server_name: null,
  next_reservation: null,
  active_bill: null,
  last_seen: null,
};

it("matches by table_code as well as name", () => {
  expect(matchesTableSearch(row, "core-t01")).toBe(true);
  expect(matchesTableSearch(row, "patio")).toBe(true);
  expect(matchesTableSearch(row, "nope")).toBe(false);
  expect(matchesTableSearch(row, "")).toBe(true);
});
