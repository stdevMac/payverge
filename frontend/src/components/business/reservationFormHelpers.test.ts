import {
  reservationErrorMessage,
  coversOnBusinessDay,
  formatNextArrivalInsight,
  mergeReservationBooks,
  nextUpcomingArrival,
  reconcileTableForPartySize,
  reservationWallTimeToISO,
} from "./reservationFormHelpers";
import enApiErrors from "@/i18n/locales/en/apiErrors.json";

// The shared axiosInstance interceptor delivers a sanitized plain Error with
// the axios markers copied but NO `isAxiosError` flag.
const sanitized = (
  status: number | undefined,
  error?: string,
  code?: string,
) =>
  Object.assign(new Error("request failed"), {
    status,
    code,
    response:
      status === undefined
        ? undefined
        : { status, data: { error, code } },
  });

describe("reservationErrorMessage (L1-4 via surfaceBackendError)", () => {
  it("surfaces the backend's display reason for 400/409/422 (sanitized error)", () => {
    expect(
      reservationErrorMessage(
        sanitized(409, "the selected time is no longer available"),
        "Could not save reservation",
      ),
    ).toBe("the selected time is no longer available");

    expect(
      reservationErrorMessage(
        sanitized(400, "reservations must be made at least 30 minutes in advance"),
        "fallback",
      ),
    ).toBe("reservations must be made at least 30 minutes in advance");
  });

  it("localizes a known catalog code instead of collapsing to English generic", () => {
    const err = sanitized(
      400,
      "Validation failed",
      "VALIDATION_INVALID_INPUT",
    );
    expect(reservationErrorMessage(err, "fallback", "en")).toBe(
      enApiErrors.VALIDATION_INVALID_INPUT,
    );
  });

  it("does not surface raw reasons for other statuses", () => {
    const msg = reservationErrorMessage(sanitized(500, "db exploded internals"), "fallback");
    expect(msg).not.toBe("db exploded internals");
  });

  it("falls back for non-axios throws", () => {
    expect(reservationErrorMessage(new TypeError("boom"), "fallback")).not.toBe("");
  });
});

describe("reconcileTableForPartySize (L1-11)", () => {
  const tables = [
    { id: 1, capacity: 2 },
    { id: 2, capacity: 6 },
  ];

  it("clears selection when party size exceeds selected table capacity", () => {
    expect(reconcileTableForPartySize(1, 4, tables)).toEqual({
      tableId: undefined,
      cleared: true,
    });
  });

  it("keeps selection when capacity still fits", () => {
    expect(reconcileTableForPartySize(2, 4, tables)).toEqual({
      tableId: 2,
      cleared: false,
    });
  });

  it("keeps selection when table is not yet in options (loading)", () => {
    expect(reconcileTableForPartySize(9, 4, tables)).toEqual({
      tableId: 9,
      cleared: false,
    });
  });
});

describe("nextUpcomingArrival", () => {
  const now = new Date("2026-07-02T22:51:00");
  const make = (time: string, status = "confirmed") => ({
    reservation_time: time,
    status,
  });

  it("skips arrivals that already happened", () => {
    const past = make("2026-07-02T20:00:00");
    expect(nextUpcomingArrival([past], now)).toBeUndefined();
  });

  it("returns the earliest future active reservation", () => {
    const past = make("2026-07-02T20:00:00");
    const later = make("2026-07-02T23:30:00");
    const soon = make("2026-07-02T23:00:00");
    expect(nextUpcomingArrival([later, past, soon], now)).toBe(soon);
  });

  it("ignores cancelled/seated reservations", () => {
    const cancelled = make("2026-07-02T23:00:00", "cancelled");
    const seated = make("2026-07-02T23:15:00", "seated");
    expect(nextUpcomingArrival([cancelled, seated], now)).toBeUndefined();
  });

  it("keeps a confirmed booking inside the no-show grace as next arrival", () => {
    const justLate = make("2026-07-02T22:46:00");
    expect(nextUpcomingArrival([justLate], now)).toBe(justLate);
  });

  it("does not advertise a confirmed booking hours past grace", () => {
    const hoursLate = make("2026-07-02T15:00:00");
    expect(nextUpcomingArrival([hoursLate], now)).toBeUndefined();
  });

  it("returns a confirmed tomorrow booking as the next arrival", () => {
    const tomorrow = make("2026-07-03T23:00:00.000Z");
    expect(nextUpcomingArrival([tomorrow], now)).toBe(tomorrow);
  });

  it("keeps a listed confirmed row the device clock already expired", () => {
    const listed = {
      id: 262,
      reservation_time: "2026-07-02T15:00:00.000Z",
      status: "confirmed",
    };
    expect(nextUpcomingArrival([listed], now)).toBeUndefined();
    expect(
      nextUpcomingArrival([listed], now, 15, { listedReservations: [listed] }),
    ).toBe(listed);
  });
});

describe("coversOnBusinessDay", () => {
  it("sums confirmed party size on the venue day", () => {
    expect(
      coversOnBusinessDay(
        [
          {
            reservation_time: "2026-08-21T23:00:00.000Z",
            status: "confirmed",
            party_size: 2,
          },
          {
            reservation_time: "2026-08-22T23:00:00.000Z",
            status: "confirmed",
            party_size: 4,
          },
        ],
        "2026-08-21",
        "America/New_York",
      ),
    ).toBe(2);
  });
});

describe("formatNextArrivalInsight", () => {
  it("names the party next to the time", () => {
    expect(
      formatNextArrivalInsight(
        { customer_name: "QA Test Franky", party_size: 2 },
        "19:00",
      ),
    ).toBe("19:00 · QA Test Franky");
  });

  it("falls back to party size when the name is missing", () => {
    expect(formatNextArrivalInsight({ party_size: 4 }, "19:00")).toBe(
      "19:00 · 4",
    );
  });
});

describe("mergeReservationBooks", () => {
  it("keeps a confirmed upcoming row when a leftover today book is nonempty", () => {
    const seatedToday = { id: 1, reservation_time: "2026-08-19T15:00:00.000Z", status: "seated" };
    const tomorrow = { id: 262, reservation_time: "2026-08-20T23:00:00.000Z", status: "confirmed" };
    const merged = mergeReservationBooks([tomorrow], [seatedToday]);
    expect(merged).toEqual([tomorrow, seatedToday]);
    expect(
      nextUpcomingArrival(merged, new Date("2026-08-19T15:00:00.000Z")),
    ).toBe(tomorrow);
  });
});

describe("reservationWallTimeToISO", () => {
  it("stores America/New_York 7pm as 23:00Z in EDT, not 19:00Z", () => {
    expect(
      reservationWallTimeToISO("2026-08-19T19:00", "America/New_York"),
    ).toBe("2026-08-19T23:00:00.000Z");
  });

  it("stores America/New_York 7pm as 00:00Z the next day in EST", () => {
    expect(
      reservationWallTimeToISO("2026-01-15T19:00", "America/New_York"),
    ).toBe("2026-01-16T00:00:00.000Z");
  });
});
