/** @jest-environment node */
import { computeReservationDateRange } from "../reservationDateRange";
import { businessDateKey } from "@/utils/businessTime";

describe("L1-10 computeReservationDateRange uses business TZ when provided", () => {
  it("today follows the restaurant day, not the device day", () => {
    // 2026-03-15T03:30:00Z is still March 14 evening in America/Los_Angeles
    // and already March 15 morning in UTC/Europe.
    const instant = new Date("2026-03-15T03:30:00.000Z");
    const la = computeReservationDateRange({
      dateFilter: "today",
      now: instant,
      businessTimeZone: "America/Los_Angeles",
    });
    expect(la.startDate).toBe(businessDateKey(instant, "America/Los_Angeles"));
    expect(la.startDate).toBe("2026-03-14");

    const utc = computeReservationDateRange({
      dateFilter: "today",
      now: instant,
      businessTimeZone: "UTC",
    });
    expect(utc.startDate).toBe("2026-03-15");
  });

  it("without businessTimeZone keeps device-local behavior", () => {
    const localEvening = new Date(2026, 2, 14, 22, 30, 0);
    const range = computeReservationDateRange({
      dateFilter: "today",
      now: localEvening,
    });
    expect(range.startDate).toBe("2026-03-14");
  });
});
