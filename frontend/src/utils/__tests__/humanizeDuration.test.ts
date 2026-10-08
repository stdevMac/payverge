import {
  humanizeDurationMinutes,
  elapsedUrgency,
} from "../humanizeDuration";

describe("humanizeDurationMinutes", () => {
  it("formats under an hour as Nm", () => {
    expect(humanizeDurationMinutes(0)).toBe("0m");
    expect(humanizeDurationMinutes(1)).toBe("1m");
    expect(humanizeDurationMinutes(45)).toBe("45m");
    expect(humanizeDurationMinutes(59)).toBe("59m");
  });

  it("formats under a day as Xh Ym (drops zero minutes)", () => {
    expect(humanizeDurationMinutes(60)).toBe("1h");
    expect(humanizeDurationMinutes(75)).toBe("1h 15m");
    expect(humanizeDurationMinutes(135)).toBe("2h 15m");
    expect(humanizeDurationMinutes(23 * 60 + 5)).toBe("23h 5m");
  });

  it("formats one day with optional hours (1d / 1d 2h)", () => {
    expect(humanizeDurationMinutes(24 * 60)).toBe("1d");
    expect(humanizeDurationMinutes(24 * 60 + 120)).toBe("1d 2h");
    expect(humanizeDurationMinutes(47 * 60 + 30)).toBe("1d 23h");
  });

  it("formats multi-day ages as compact days (never raw minute or hour walls)", () => {
    // L1-9: LiveTableGrid regression — 38858 min occupancy
    expect(humanizeDurationMinutes(38858)).toBe("26d");
    // L1-24: Kitchen regression — ~450h delivered order age
    expect(humanizeDurationMinutes(450 * 60)).toBe("18d");
    expect(humanizeDurationMinutes(26 * 24 * 60)).toBe("26d");
    expect(humanizeDurationMinutes(2 * 24 * 60)).toBe("2d");
  });

  it("treats negative and non-finite as 0m", () => {
    expect(humanizeDurationMinutes(-5)).toBe("0m");
    expect(humanizeDurationMinutes(Number.NaN)).toBe("0m");
    expect(humanizeDurationMinutes(Number.POSITIVE_INFINITY)).toBe("0m");
  });

  it("floors fractional minutes", () => {
    expect(humanizeDurationMinutes(45.9)).toBe("45m");
    expect(humanizeDurationMinutes(90.1)).toBe("1h 30m");
  });

  it("accepts es locale without changing compact units (language-neutral)", () => {
    expect(humanizeDurationMinutes(135, "es")).toBe("2h 15m");
    expect(humanizeDurationMinutes(38858, "es")).toBe("26d");
  });
});

describe("elapsedUrgency", () => {
  it("returns none for fresh ages", () => {
    expect(elapsedUrgency(0)).toBe("none");
    expect(elapsedUrgency(10)).toBe("none");
    expect(elapsedUrgency(15)).toBe("none");
  });

  it("warns past 15m and critical past 30m by default", () => {
    expect(elapsedUrgency(16)).toBe("warn");
    expect(elapsedUrgency(30)).toBe("warn");
    expect(elapsedUrgency(31)).toBe("critical");
    expect(elapsedUrgency(90)).toBe("critical");
  });

  it("marks ages past calmAfter as stale (no urgent red pulse)", () => {
    expect(elapsedUrgency(24 * 60)).toBe("stale");
    expect(elapsedUrgency(450 * 60)).toBe("stale");
    expect(elapsedUrgency(38858)).toBe("stale");
  });

  it("honors custom thresholds", () => {
    expect(
      elapsedUrgency(20, {
        warnAfterMinutes: 10,
        criticalAfterMinutes: 25,
        calmAfterMinutes: 60,
      }),
    ).toBe("warn");
    expect(
      elapsedUrgency(40, {
        warnAfterMinutes: 10,
        criticalAfterMinutes: 25,
        calmAfterMinutes: 60,
      }),
    ).toBe("critical");
    expect(
      elapsedUrgency(90, {
        warnAfterMinutes: 10,
        criticalAfterMinutes: 25,
        calmAfterMinutes: 60,
      }),
    ).toBe("stale");
  });

  it("returns none for negative/non-finite", () => {
    expect(elapsedUrgency(-1)).toBe("none");
    expect(elapsedUrgency(Number.NaN)).toBe("none");
  });
});
