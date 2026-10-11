import {
  DEFAULT_WEEK_START_DAY,
  resolveWeekStartDay,
  weekStartDayName,
  weekStartKey,
  weekdayInTimeZone,
} from "./scheduleWeek";

describe("resolveWeekStartDay", () => {
  it("keeps Sunday (0) instead of falling back to Monday", () => {
    expect(resolveWeekStartDay(0)).toBe(0);
    expect(resolveWeekStartDay("0")).toBe(0);
    expect(resolveWeekStartDay(undefined)).toBe(DEFAULT_WEEK_START_DAY);
    expect(resolveWeekStartDay(null)).toBe(DEFAULT_WEEK_START_DAY);
  });
});

describe("#664 weekStartDayName", () => {
  const originalTz = process.env.TZ;

  beforeAll(() => {
    process.env.TZ = "America/Buenos_Aires";
  });

  afterAll(() => {
    process.env.TZ = originalTz;
  });

  it("labels Monday as lunes (not domingo) on a device behind UTC", () => {
    expect(weekStartDayName(1, "es")).toMatch(/^lunes$/i);
    expect(weekStartDayName(0, "es")).toMatch(/^domingo$/i);
    expect(weekStartDayName(1, "en")).toMatch(/^monday$/i);
    expect(weekStartDayName(0, "en")).toMatch(/^sunday$/i);
  });

  it("does not slide a persisted Monday back to Sunday in es-AR", () => {
    expect(weekStartDayName(1, "es-AR")).not.toMatch(/domingo/i);
    expect(weekStartDayName(1, "es-AR")).toMatch(/lunes/i);
  });
});

describe("weekStartKey venue timezone", () => {
  it("aligns Sunday-start weeks to the venue calendar day, not UTC", () => {
    // Wednesday 19 Aug 2026 02:00 UTC = Tuesday 18 Aug 22:00 in New York.
    const instant = new Date("2026-08-19T02:00:00.000Z");
    expect(weekdayInTimeZone(instant, "America/New_York")).toBe(2);
    expect(weekStartKey(instant, 0, "America/New_York")).toBe("2026-08-16");
    expect(weekStartKey(instant, 1, "America/New_York")).toBe("2026-08-17");
    expect(weekStartKey(instant, 0, "America/New_York")).not.toBe(
      weekStartKey(instant, 1, "America/New_York"),
    );
  });
});
