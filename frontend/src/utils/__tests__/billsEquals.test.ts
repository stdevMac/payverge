import { billsEquals } from "@/utils/billsEquals";
import type { Bill } from "@/api/bills";
import type { Dollars } from "@/types/money";

const d = (n: number) => n as Dollars;

const mk = (over: Partial<Bill>): Bill =>
  ({
    id: 1,
    status: "open",
    paid_amount: d(0),
    total_amount: d(10),
    updated_at: "2026-07-20T10:00:00.000Z",
    ...over,
  }) as Bill;

describe("billsEquals", () => {
  it("returns true for identical reference", () => {
    const list = [mk({})];
    expect(billsEquals(list, list)).toBe(true);
  });

  it("returns true for equal board-relevant fields across fresh arrays", () => {
    expect(billsEquals([mk({})], [mk({})])).toBe(true);
  });

  it("returns false when length differs", () => {
    expect(billsEquals([mk({})], [mk({}), mk({ id: 2 })])).toBe(false);
  });

  it("returns false when a board-relevant field changed", () => {
    expect(billsEquals([mk({})], [mk({ status: "paid" })])).toBe(false);
    expect(billsEquals([mk({})], [mk({ paid_amount: d(5) })])).toBe(false);
    expect(billsEquals([mk({})], [mk({ updated_at: "2026-07-20T11:00:00Z" })])).toBe(
      false,
    );
  });

  it("ignores non-board fields (e.g. bill_number)", () => {
    expect(
      billsEquals([mk({ bill_number: "A" })], [mk({ bill_number: "B" })]),
    ).toBe(true);
  });
});
