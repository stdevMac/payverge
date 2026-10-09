import { getTablesWithStatus } from "@/api/business";
import {
  countOccupiedFromFloor,
  countOccupiedTables,
  fetchOccupiedTablesCount,
} from "./occupiedTablesCount";

jest.mock("@/api/business", () => ({
  getTablesWithStatus: jest.fn(),
}));

const getFloor = getTablesWithStatus as jest.MockedFunction<
  typeof getTablesWithStatus
>;

/** Live AI Pro 86: 5 occupied including leftover-kitchen T2/T3/T5 (0 open bills). */
const leftoverKitchenFloor = [
  {
    status: "occupied",
    table: { is_active: true },
    active_bills_count: 1,
    active_bills: [{ id: 1143 }],
  },
  {
    status: "occupied",
    table: { is_active: true },
    active_bills_count: 0,
    active_bills: [],
  },
  {
    status: "occupied",
    table: { is_active: true },
    active_bills_count: 0,
    active_bills: [],
  },
  {
    status: "available",
    table: { is_active: true },
    active_bills_count: 0,
    active_bills: [],
  },
  {
    status: "occupied",
    table: { is_active: true },
    active_bills_count: 0,
    active_bills: [],
  },
  {
    status: "available",
    table: { is_active: true },
    active_bills_count: 0,
    active_bills: [],
  },
  {
    status: "available",
    table: { is_active: true },
    active_bills_count: 0,
    active_bills: [],
  },
  {
    status: "available",
    table: { is_active: true },
    active_bills_count: 0,
    active_bills: [],
  },
  {
    status: "occupied",
    table: { is_active: true },
    active_bills_count: 1,
    active_bills: [{ id: 761 }],
  },
  {
    status: "available",
    table: { is_active: true },
    active_bills_count: 0,
    active_bills: [],
  },
];

describe("countOccupiedTables (#774 leftover-kitchen occupancy)", () => {
  it("matches a 5-table occupied floor when two checks and three leftover kitchen tables are live", () => {
    // Live AI Pro 86: T1 #1143, T9 partial #761, T2/T3/T5 leftover kitchen / 0 bills.
    expect(
      countOccupiedTables(
        [
          { status: "open", table_id: 1 },
          { status: "partial", table_id: 9 },
          { status: "closed", table_id: 2 },
        ],
        {
          1143: [{ status: "in_kitchen", bill: { table_id: 1 } }],
          9002: [{ status: "in_kitchen", bill: { table_id: 2 } }],
          9003: [{ status: "approved", bill: { table_id: 3 } }],
          9005: [{ status: "ready", table_id: 5 }],
        },
      ),
    ).toBe(5);
  });

  it("does not treat unique open checks as occupancy when leftover kitchen is absent", () => {
    expect(
      countOccupiedTables(
        [
          { status: "open", table_id: 1 },
          { status: "partial", table_id: 9 },
        ],
        {},
      ),
    ).toBe(2);
  });

  it("counts leftover kitchen tables with no open check", () => {
    expect(
      countOccupiedTables(
        [{ status: "closed", table_id: 2 }],
        [{ status: "in_kitchen", bill: { table_id: 2 } }],
      ),
    ).toBe(1);
  });

  it("does not inflate from paid leftovers, delivered tickets, or table_id 0", () => {
    expect(
      countOccupiedTables(
        [
          { status: "paid", table_id: 4 },
          { status: "open", table_id: 0 },
        ],
        [
          { status: "delivered", bill: { table_id: 4 } },
          { status: "cancelled", bill: { table_id: 5 } },
          { status: "in_kitchen", bill: { table_id: 0 } },
        ],
      ),
    ).toBe(0);
  });

  it("dedupes a live check and its kitchen tickets onto one table", () => {
    expect(
      countOccupiedTables(
        [{ status: "open", table_id: 1 }],
        [{ status: "approved", bill: { table_id: 1 } }],
      ),
    ).toBe(1);
  });

  it("returns 0 for empty or missing inputs", () => {
    expect(countOccupiedTables([], {})).toBe(0);
    expect(countOccupiedTables(undefined, undefined)).toBe(0);
    expect(countOccupiedTables(null, null)).toBe(0);
  });
});

describe("countOccupiedFromFloor (#774 leftover-kitchen on closed/abandoned bills)", () => {
  it("matches the Tables header: 5 occupied when T2/T3/T5 have leftover kitchen and 0 open bills", () => {
    // After bill.closed, the kitchen activeBillsOnly feed only keeps T1 + T9.
    expect(
      countOccupiedTables(
        [
          { status: "open", table_id: 1 },
          { status: "partial", table_id: 9 },
        ],
        {},
      ),
    ).toBe(2);
    expect(countOccupiedFromFloor(leftoverKitchenFloor)).toBe(5);
  });

  it("does not count reserved or inactive leftover rows as occupied", () => {
    expect(
      countOccupiedFromFloor([
        { status: "reserved", table: { is_active: true } },
        { status: "occupied", table: { is_active: false } },
      ]),
    ).toBe(0);
  });
});

describe("fetchOccupiedTablesCount uses /tables/status, not activeBillsOnly", () => {
  beforeEach(() => {
    getFloor.mockReset();
  });

  it("reads leftover-kitchen occupied tables from the status board", async () => {
    getFloor.mockResolvedValue({ tables: leftoverKitchenFloor as any });

    await expect(fetchOccupiedTablesCount(86)).resolves.toBe(5);
    expect(getFloor).toHaveBeenCalledWith(86);
  });
});
