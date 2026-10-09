import {
  buildTableViewModel,
  countTableStatuses,
  filterTableRows,
} from "./tableViewModel";

const rows = [
  { id: 1, is_active: true, status: "available" },
  { id: 2, is_active: true, status: "occupied" },
  { id: 3, is_active: true, status: "reserved" },
  { id: 4, is_active: false, status: "available" },
];

describe("L3-27 table view-model", () => {
  it("counts include inactive in total so header matches Lista under all", () => {
    expect(countTableStatuses(rows)).toEqual({
      total: 4,
      available: 1,
      occupied: 1,
      reserved: 1,
      inactive: 1,
    });
  });

  it('filter "all" includes inactive tables (audit: Todos los estados)', () => {
    const filtered = filterTableRows(rows, "all", () => true);
    expect(filtered.map((r) => r.id).sort()).toEqual([1, 2, 3, 4]);
    expect(filtered.some((r) => !r.is_active)).toBe(true);
  });

  it("filters inactive out of status-specific active views", () => {
    const filtered = filterTableRows(rows, "available", () => true);
    expect(filtered.map((r) => r.id)).toEqual([1]);
  });

  it('filter "inactive" returns only inactive rows', () => {
    const filtered = filterTableRows(rows, "inactive", () => true);
    expect(filtered.map((r) => r.id)).toEqual([4]);
  });

  it("buildTableViewModel all includes inactive in rows and total", () => {
    const vm = buildTableViewModel(rows, "all", () => true);
    expect(vm.rows).toHaveLength(4);
    expect(vm.counts.total).toBe(4);
    expect(vm.counts.inactive).toBe(1);
  });
});
