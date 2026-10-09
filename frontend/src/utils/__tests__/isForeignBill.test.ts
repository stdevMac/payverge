import { isForeignBill } from "@/utils/isForeignBill";

describe("isForeignBill (L9-1 tenant isolation)", () => {
  it("returns false when bill business_id matches the current business", () => {
    expect(isForeignBill(42, 42)).toBe(false);
  });

  it("returns true when bill business_id belongs to another business", () => {
    expect(isForeignBill(99, 42)).toBe(true);
  });

  it("fails closed when business_id is missing", () => {
    expect(isForeignBill(undefined, 42)).toBe(true);
    expect(isForeignBill(null, 42)).toBe(true);
  });

  it("coerces numeric string-like values consistently via Number()", () => {
    // Defensive: API is typed as number, but runtime JSON can surprise.
    expect(isForeignBill(Number("42"), 42)).toBe(false);
    expect(isForeignBill(Number("99"), 42)).toBe(true);
  });
});
