/** @jest-environment node */
import {
  computeReservationDateRange,
  normalizeReservationDateFilter,
} from "../reservationDateRange";
import { localDateKey } from "@/lib/localDate";

describe("computeReservationDateRange (audit L6 #16 — local-day keys)", () => {
  it("today range uses the LOCAL calendar day, not the UTC day", () => {
    // An evening instant that, in any timezone west of UTC, is already the next
    // UTC calendar day. The range must follow the LOCAL day so tonight's
    // reservations are not hidden.
    const eveningLocal = new Date(2026, 2, 14, 22, 30, 0); // Mar 14 2026 22:30 local
    const { startDate, endDate } = computeReservationDateRange({
      dateFilter: "today",
      now: eveningLocal,
    });
    expect(startDate).toBe(localDateKey(eveningLocal));
    expect(endDate).toBe(localDateKey(eveningLocal));
    // Local-day key derives from local getters, never toISOString slicing.
    expect(startDate).toBe("2026-03-14");
  });

  it("upcoming range spans local today .. today+30", () => {
    const now = new Date(2026, 5, 1, 12, 0, 0);
    const { startDate, endDate } = computeReservationDateRange({
      dateFilter: "upcoming",
      now,
    });
    expect(startDate).toBe(localDateKey(now));
    const plus30 = new Date(now);
    plus30.setDate(plus30.getDate() + 30);
    expect(endDate).toBe(localDateKey(plus30));
  });

  it("past range spans today-30 .. local today", () => {
    const now = new Date(2026, 5, 1, 12, 0, 0);
    const { startDate, endDate } = computeReservationDateRange({
      dateFilter: "past",
      now,
    });
    expect(endDate).toBe(localDateKey(now));
    const minus30 = new Date(now);
    minus30.setDate(minus30.getDate() - 30);
    expect(startDate).toBe(localDateKey(minus30));
  });

  it("truly unknown filter yields an open (undefined) range", () => {
    expect(
      computeReservationDateRange({ dateFilter: "something_unknown" }),
    ).toEqual({
      startDate: undefined,
      endDate: undefined,
    });
  });

  it("custom range honors explicit from/to YYYY-MM-DD bounds", () => {
    expect(
      computeReservationDateRange({
        dateFilter: "custom",
        customStartDate: "2025-01-01",
        customEndDate: "2025-01-31",
      }),
    ).toEqual({ startDate: "2025-01-01", endDate: "2025-01-31" });
  });

  it("custom range swaps inverted bounds", () => {
    expect(
      computeReservationDateRange({
        dateFilter: "custom",
        customStartDate: "2025-06-01",
        customEndDate: "2025-05-01",
      }),
    ).toEqual({ startDate: "2025-05-01", endDate: "2025-06-01" });
  });

  it("custom range with only start is from-only", () => {
    expect(
      computeReservationDateRange({
        dateFilter: "custom",
        customStartDate: "2024-12-01",
      }),
    ).toEqual({ startDate: "2024-12-01", endDate: undefined });
  });

  it("custom range with empty bounds is open", () => {
    expect(
      computeReservationDateRange({
        dateFilter: "custom",
        customStartDate: "",
        customEndDate: "  ",
      }),
    ).toEqual({ startDate: undefined, endDate: undefined });
  });

  it("upcoming range stretches to the bookable horizon when max_advance_days > 30", () => {
    // Guests can book up to max_advance_days out (45 here); the Upcoming view
    // capping at +30 hid those bookings from the dashboard entirely.
    const now = new Date(2026, 5, 1, 12, 0, 0);
    const { startDate, endDate } = computeReservationDateRange({
      dateFilter: "upcoming",
      now,
      maxAdvanceDays: 45,
    });
    expect(startDate).toBe(localDateKey(now));
    const plus45 = new Date(now);
    plus45.setDate(plus45.getDate() + 45);
    expect(endDate).toBe(localDateKey(plus45));
  });

  it("upcoming range keeps the 30-day floor for short horizons", () => {
    const now = new Date(2026, 5, 1, 12, 0, 0);
    const { endDate } = computeReservationDateRange({
      dateFilter: "upcoming",
      now,
      maxAdvanceDays: 7,
    });
    const plus30 = new Date(now);
    plus30.setDate(plus30.getDate() + 30);
    expect(endDate).toBe(localDateKey(plus30));
  });

  // L1-28: "Todo (incluye pasadas)". Both bounds must be EXPLICIT — the server
  // defaults an absent start_date to today (hiding all past rows) and computes
  // an absent end_date from start_date (2000-01-01 + horizon ≈ year 2000).
  it("all_time spans the far-past floor through the bookable horizon", () => {
    const now = new Date(2026, 5, 1, 12, 0, 0);
    const { startDate, endDate } = computeReservationDateRange({
      dateFilter: "all_time",
      now,
      maxAdvanceDays: 45,
    });
    expect(startDate).toBe("2000-01-01");
    const plus45 = new Date(now);
    plus45.setDate(plus45.getDate() + 45);
    expect(endDate).toBe(localDateKey(plus45));
  });

  it("all_time keeps the 30-day forward floor for short horizons", () => {
    const now = new Date(2026, 5, 1, 12, 0, 0);
    const { endDate } = computeReservationDateRange({
      dateFilter: "all_time",
      now,
      maxAdvanceDays: 7,
    });
    const plus30 = new Date(now);
    plus30.setDate(plus30.getDate() + 30);
    expect(endDate).toBe(localDateKey(plus30));
  });

  // L1-7: "all" / open horizon must not send undefined bounds (BE invents
  // today→horizon, making the option identical to "upcoming" and dishonest
  // about including the past). Align with all_time: explicit far-past floor
  // through the bookable horizon.
  it("all filter uses explicit all_time-style bounds (not open/undefined)", () => {
    const now = new Date(2026, 5, 1, 12, 0, 0);
    const range = computeReservationDateRange({
      dateFilter: "all",
      now,
      maxAdvanceDays: 45,
    });
    expect(range.startDate).toBe("2000-01-01");
    const plus45 = new Date(now);
    plus45.setDate(plus45.getDate() + 45);
    expect(range.endDate).toBe(localDateKey(plus45));
  });
});

describe("normalizeReservationDateFilter (R2-B6)", () => {
  // After L1-7 the picker showed two entries with identical labels and
  // identical bounds ("all" and "all_time"). The "all" option is gone; any
  // value still carrying it must resolve to the one surviving key, otherwise
  // the Select has no matching item and renders blank.
  it("maps the retired 'all' key onto 'all_time'", () => {
    expect(normalizeReservationDateFilter("all")).toBe("all_time");
  });

  it("leaves every live filter key untouched", () => {
    for (const key of ["today", "upcoming", "past", "custom", "all_time"]) {
      expect(normalizeReservationDateFilter(key)).toBe(key);
    }
  });

  it("passes unknown values through unchanged", () => {
    expect(normalizeReservationDateFilter("something_unknown")).toBe(
      "something_unknown",
    );
  });
});
