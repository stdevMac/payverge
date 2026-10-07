/** @jest-environment node */
import {
  shouldFlipToUpcomingAfterCreate,
  shouldScheduleRefreshAfterCreate,
} from "./reservationPostCreate";

describe("reservationPostCreate (L1-6)", () => {
  const now = new Date(2026, 5, 1, 12, 0, 0); // 2026-06-01 local

  it("does not flip when the new reservation is on local today", () => {
    expect(
      shouldFlipToUpcomingAfterCreate({
        reservationTime: new Date(2026, 5, 1, 20, 0, 0).toISOString(),
        dateFilter: "today",
        now,
      }),
    ).toBe(false);
  });

  it("flips from today to upcoming when the reservation is another day", () => {
    expect(
      shouldFlipToUpcomingAfterCreate({
        reservationTime: new Date(2026, 5, 3, 20, 0, 0).toISOString(),
        dateFilter: "today",
        now,
      }),
    ).toBe(true);
  });

  it("does not flip when already on upcoming/all/all_time", () => {
    for (const dateFilter of ["upcoming", "all", "all_time"]) {
      expect(
        shouldFlipToUpcomingAfterCreate({
          reservationTime: new Date(2026, 5, 3, 20, 0, 0).toISOString(),
          dateFilter,
          now,
        }),
      ).toBe(false);
    }
  });

  it("skips manual refresh when the filter flip re-keys loaders (avoids race)", () => {
    expect(shouldScheduleRefreshAfterCreate(true)).toBe(false);
    expect(shouldScheduleRefreshAfterCreate(false)).toBe(true);
  });
});
