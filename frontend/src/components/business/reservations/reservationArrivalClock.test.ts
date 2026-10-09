process.env.TZ = "America/Argentina/Buenos_Aires";

import {
  DEFAULT_NO_SHOW_GRACE_MINUTES,
  isNextArrivalCandidate,
  reservationArrivalPhase,
  reservationDisplayStatus,
} from "./reservationArrivalClock";

const now = new Date("2026-08-13T23:48:00.000Z"); // ~19:48 ET
const fifteen = "2026-08-13T19:00:00.000Z"; // 15:00 ET

describe("reservationArrivalClock", () => {
  it("defaults grace to 15 minutes", () => {
    expect(DEFAULT_NO_SHOW_GRACE_MINUTES).toBe(15);
  });

  it("marks a 15:00 confirmed booking Late hours later", () => {
    const row = { reservation_time: fifteen, status: "confirmed" };
    expect(reservationArrivalPhase(row, now, 15)).toBe("expired");
    expect(reservationDisplayStatus(row, now, 15)).toBe("late");
    expect(isNextArrivalCandidate(row, now, 15)).toBe(false);
  });

  it("keeps a just-late confirmed booking as next arrival", () => {
    const lateNow = new Date("2026-08-13T19:10:00.000Z");
    const row = { reservation_time: fifteen, status: "confirmed" };
    expect(reservationArrivalPhase(row, lateNow, 15)).toBe("late");
    expect(reservationDisplayStatus(row, lateNow, 15)).toBe("late");
    expect(isNextArrivalCandidate(row, lateNow, 15)).toBe(true);
  });

  it("does not treat seated or no_show as late", () => {
    expect(
      reservationDisplayStatus(
        { reservation_time: fifteen, status: "seated" },
        now,
        15,
      ),
    ).toBe("seated");
    expect(
      isNextArrivalCandidate(
        { reservation_time: fifteen, status: "no_show" },
        now,
        15,
      ),
    ).toBe(false);
  });

  it("reads a naive 19:00 wall time in the venue zone, not the device TZ", () => {
    // 22:30Z is 19:30 ART / 18:30 NY. Naive 19:00 is already past (+grace)
    // if Date() uses the device zone, but still upcoming in America/New_York.
    const lateAfternoon = new Date("2026-08-21T22:30:00.000Z");
    const row = { reservation_time: "2026-08-21T19:00:00", status: "confirmed" };
    expect(isNextArrivalCandidate(row, lateAfternoon, 15)).toBe(false);
    expect(
      isNextArrivalCandidate(row, lateAfternoon, 15, "America/New_York"),
    ).toBe(true);
    expect(
      reservationArrivalPhase(row, lateAfternoon, 15, "America/New_York"),
    ).toBe("upcoming");
  });
});
