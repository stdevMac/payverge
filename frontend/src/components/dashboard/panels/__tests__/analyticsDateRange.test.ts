import { localDateKey } from "@/lib/localDate";

/**
 * Regression test: the analytics from/to date-range computation MUST use
 * local calendar dates, not UTC. In a negative-offset timezone (the
 * Americas), toISOString().slice(0,10) at 23:30 local on Jan 15 would
 * return "2026-01-16" — skipping the current day and fetching tomorrow's
 * empty bucket. localDateKey prevents that.
 */
describe("analytics date-range uses local calendar dates", () => {
  const computeRange = (now: Date) => {
    const from = localDateKey(new Date(now.getTime() - 29 * 864e5));
    const to = localDateKey(now);
    return { from, to };
  };

  it("at 23:30 local, to stays on the current day (not the next UTC day)", () => {
    // Jan 15 at 23:30 local. toISOString() in a -5 offset would be 2026-01-16T04:30:00Z
    // → .slice(0,10) = "2026-01-16" — the WRONG day. localDateKey must yield "2026-01-15".
    const late = new Date(2026, 0, 15, 23, 30, 0);
    const { from, to } = computeRange(late);
    expect(to).toBe("2026-01-15");
    // from should be exactly 29 days earlier in the local calendar
    expect(from).toBe("2025-12-17");
  });

  it("at 00:05 local, to stays on the new day (not the previous UTC day)", () => {
    const early = new Date(2026, 5, 7, 0, 5, 0);
    const { from, to } = computeRange(early);
    expect(to).toBe("2026-06-07");
    expect(from).toBe("2026-05-09");
  });

  it("mid-month midday yields correct 29-day span", () => {
    const noon = new Date(2026, 3, 15, 12, 0, 0);
    const { from, to } = computeRange(noon);
    expect(to).toBe("2026-04-15");
    expect(from).toBe("2026-03-17");
  });
});
