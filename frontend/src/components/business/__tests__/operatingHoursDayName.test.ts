import { getDayName } from "@/components/business/operatingHoursDayName";

describe("getDayName", () => {
  it("returns the English day name for a known index", () => {
    expect(getDayName(0, "en")).toBe("Sunday");
    expect(getDayName(1, "en")).toBe("Monday");
  });

  it("returns Unknown for an out-of-range index", () => {
    expect(getDayName(99, "en")).toBe("Unknown");
  });
});
