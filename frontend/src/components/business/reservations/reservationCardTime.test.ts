/** @jest-environment node */
import {
  rangeSpansMultipleDays,
  reservationCardTimeMode,
} from "./reservationCardTime";

describe("reservationCardTime (L1-14)", () => {
  it("uses time-only for a single-day today window", () => {
    expect(rangeSpansMultipleDays("2026-06-01", "2026-06-01")).toBe(false);
    expect(reservationCardTimeMode("2026-06-01", "2026-06-01")).toBe(
      "time_only",
    );
  });

  it("uses date+time when the range spans multiple days", () => {
    expect(rangeSpansMultipleDays("2026-06-01", "2026-07-01")).toBe(true);
    expect(reservationCardTimeMode("2026-06-01", "2026-07-01")).toBe(
      "date_time",
    );
  });

  it("treats open/partial ranges as multi-day for card display", () => {
    expect(rangeSpansMultipleDays(undefined, undefined)).toBe(true);
    expect(rangeSpansMultipleDays("2026-06-01", undefined)).toBe(true);
    expect(reservationCardTimeMode(undefined, "2026-06-01")).toBe("date_time");
  });
});
