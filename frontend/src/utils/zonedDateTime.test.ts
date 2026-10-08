import {
  instantToWallTime,
  nextBookableWallTime,
  switchWallTimeZone,
  wallTimeToInstant,
} from "./zonedDateTime";

describe("zonedDateTime", () => {
  it("interprets a restaurant wall clock in the business timezone", () => {
    expect(
      wallTimeToInstant("2026-07-18T15:00", "America/New_York").toISOString(),
    ).toBe("2026-07-18T19:00:00.000Z");
  });

  it("switches entry modes without changing the instant", () => {
    expect(
      switchWallTimeZone(
        "2026-07-18T15:00",
        "America/New_York",
        "America/Argentina/Buenos_Aires",
      ),
    ).toBe("2026-07-18T16:00");
  });

  it("rejects a wall time skipped by daylight saving", () => {
    expect(() =>
      wallTimeToInstant("2026-03-08T02:30", "America/New_York"),
    ).toThrow("wall_time_does_not_exist");
  });

  it("rounds the first bookable instant before formatting in the chosen zone", () => {
    expect(
      nextBookableWallTime(
        new Date("2026-07-18T18:07:00.000Z"),
        30,
        30,
        "America/New_York",
      ),
    ).toBe("2026-07-18T15:00");
  });

  // L1-17: when now+minAdvance falls outside operating hours, roll to next open.
  it("rolls a late-night candidate forward to the next open operating slot", () => {
    // 22:07 America/New_York + 30 min advance → 22:30, past a 11:00-22:00 window.
    // Must land on the next day's open (11:00).
    expect(
      nextBookableWallTime(
        new Date("2026-07-18T02:07:00.000Z"), // 2026-07-17 22:07 EDT
        30,
        30,
        "America/New_York",
        { openHHMM: "11:00", closeHHMM: "22:00" },
      ),
    ).toBe("2026-07-18T11:00");
  });

  it("snaps a pre-open candidate up to opening time", () => {
    // 08:00 EDT + 0 advance, open at 11:00 → 11:00 same day.
    expect(
      nextBookableWallTime(
        new Date("2026-07-18T12:00:00.000Z"), // 08:00 EDT
        0,
        30,
        "America/New_York",
        { openHHMM: "11:00", closeHHMM: "22:00" },
      ),
    ).toBe("2026-07-18T11:00");
  });

  it("keeps an in-window candidate after rounding", () => {
    expect(
      nextBookableWallTime(
        new Date("2026-07-18T18:07:00.000Z"), // 14:07 EDT
        30,
        30,
        "America/New_York",
        { openHHMM: "11:00", closeHHMM: "22:00" },
      ),
    ).toBe("2026-07-18T15:00");
  });


  // R2-11: L1-17 clamped the candidate with the next OPEN day's hours but left
  // it on TODAY's date, so on a closed day the backend rejected the seeded form
  // with outside_operating_window. The window must be resolved per candidate
  // day so a closed day rolls the DATE forward too.
  describe("per-day operating windows (R2-11)", () => {
    it("rolls to the next open day when today is closed", () => {
      const closedToday = (localDate: string) =>
        localDate === "2026-07-18"
          ? null
          : { openHHMM: "11:00", closeHHMM: "22:00" };
      expect(
        nextBookableWallTime(
          new Date("2026-07-18T18:07:00.000Z"), // Sat 14:07 EDT — inside 11-22
          30,
          30,
          "America/New_York",
          closedToday,
        ),
      ).toBe("2026-07-19T11:00");
    });

    it("skips a run of closed days", () => {
      const openMondayOnly = (localDate: string) =>
        localDate === "2026-07-20"
          ? { openHHMM: "11:00", closeHHMM: "22:00" }
          : null;
      expect(
        nextBookableWallTime(
          new Date("2026-07-18T18:07:00.000Z"),
          30,
          30,
          "America/New_York",
          openMondayOnly,
        ),
      ).toBe("2026-07-20T11:00");
    });

    it("uses the candidate day's own hours, not today's", () => {
      const hours = (localDate: string) =>
        localDate === "2026-07-18"
          ? { openHHMM: "11:00", closeHHMM: "15:00" }
          : { openHHMM: "17:00", closeHHMM: "23:00" };
      expect(
        nextBookableWallTime(
          new Date("2026-07-18T18:40:00.000Z"), // 14:40 EDT → 15:30, past close
          30,
          30,
          "America/New_York",
          hours,
        ),
      ).toBe("2026-07-19T17:00");
    });

    it("still accepts a single static window for every day", () => {
      expect(
        nextBookableWallTime(
          new Date("2026-07-18T02:07:00.000Z"), // 2026-07-17 22:07 EDT
          30,
          30,
          "America/New_York",
          { openHHMM: "11:00", closeHHMM: "22:00" },
        ),
      ).toBe("2026-07-18T11:00");
    });
  });

  it("formats an instant for audit preview", () => {
    expect(
      instantToWallTime(
        new Date("2026-07-18T19:00:00.000Z"),
        "America/Argentina/Buenos_Aires",
      ),
    ).toBe("2026-07-18T16:00");
  });
});

describe("nextBookableWallTime serviceMinutes (L1-17)", () => {
  it("does not seed a start that cannot finish before close", () => {
    // Open 09:00–17:00, duration 120 + buffer 15 = 135 min → last start 14:45.
    // Candidate at 16:00 must roll to next open day rather than accept 16:00.
    const now = new Date("2026-06-01T15:00:00.000Z"); // 11:00 America/New_York EDT
    // Force candidate near close: minAdvance 0, interval 30, now just before close.
    // Use a TZ with fixed offset for determinism.
    const late = new Date("2026-06-01T20:30:00.000Z"); // 16:30 UTC
    const slot = nextBookableWallTime(
      late,
      0,
      30,
      "UTC",
      { openHHMM: "09:00", closeHHMM: "17:00" },
      { serviceMinutes: 120 + 15 },
    );
    // 16:30 cannot fit 135m before 17:00 → next day open
    expect(slot.startsWith("2026-06-02T09:")).toBe(true);
  });

  it("still accepts a start that fits duration+buffer before close", () => {
    const mid = new Date("2026-06-01T12:00:00.000Z"); // 12:00 UTC
    const slot = nextBookableWallTime(
      mid,
      0,
      30,
      "UTC",
      { openHHMM: "09:00", closeHHMM: "17:00" },
      { serviceMinutes: 120 + 15 },
    );
    expect(slot).toBe("2026-06-01T12:00");
  });
});
