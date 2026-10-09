import { formatGuestClockTime, guestHourCycle } from "./guestClockTime";

describe("guestClockTime (#949)", () => {
  it("uses 24-hour time for es-AR, not a. m. / p. m.", () => {
    expect(guestHourCycle("es-AR")).toBe("h23");
    expect(guestHourCycle("es-ar")).toBe("h23");
    const formatted = formatGuestClockTime("23:00", "es-AR");
    expect(formatted).toMatch(/23/);
    expect(formatted).not.toMatch(/p\.?\s*m/i);
    expect(formatted).not.toMatch(/a\.?\s*m/i);
  });

  it("keeps 12-hour English for en", () => {
    expect(guestHourCycle("en")).toBe("h12");
    expect(formatGuestClockTime("23:00", "en")).toMatch(/11:00\s*PM/i);
  });

  it("follows Intl 24-hour for es", () => {
    expect(guestHourCycle("es")).toBe("h23");
    expect(formatGuestClockTime("17:00", "es")).toMatch(/17/);
  });
});
