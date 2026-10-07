import { reservationFitsClose } from "./reservationFitsClose";

describe("reservationFitsClose L1-17", () => {
  it("rejects starts that cannot finish duration+buffer before close", () => {
    // Close 20:00, duration 90 + buffer 15 = 105 → last start 18:15.
    expect(reservationFitsClose("18:00", "20:00", 90 + 15)).toBe(true);
    expect(reservationFitsClose("18:30", "20:00", 90 + 15)).toBe(false);
    expect(reservationFitsClose("19:00", "20:00", 90 + 15)).toBe(false);
  });

  it("with zero service minutes only requires start before close", () => {
    expect(reservationFitsClose("19:59", "20:00", 0)).toBe(true);
    expect(reservationFitsClose("20:00", "20:00", 0)).toBe(true);
    expect(reservationFitsClose("20:01", "20:00", 0)).toBe(false);
  });

  it("rolls overnight close forward (bar 18:00–02:00 matches backend)", () => {
    // 23:00 + 120 min ends at 01:00 next day — before 02:00 close.
    expect(reservationFitsClose("23:00", "02:00", 120, "18:00")).toBe(true);
    // 01:00 + 90 ends at 02:30 — after overnight close.
    expect(reservationFitsClose("01:00", "02:00", 90, "18:00")).toBe(false);
    // Same-day daytime venue still rejects late starts.
    expect(reservationFitsClose("19:00", "20:00", 90, "09:00")).toBe(false);
  });
});
