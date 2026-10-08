import {
  businessLocalHour,
  formatBusinessTime,
  resolveBusinessTimeZone,
  TIME_SHORT,
} from "./businessTime";

describe("formatBusinessTime venue zone across locales (#662)", () => {
  it("renders the same venue-local hour for en, es, and es-AR", () => {
    const instant = "2026-08-19T22:12:00.000Z";
    const zone = "America/New_York";
    const en = formatBusinessTime(instant, "en", zone, {
      hour: "2-digit",
      minute: "2-digit",
    });
    const es = formatBusinessTime(instant, "es", zone, {
      hour: "2-digit",
      minute: "2-digit",
    });
    const esAR = formatBusinessTime(instant, "es-AR", zone, {
      hour: "2-digit",
      minute: "2-digit",
    });
    const utc = formatBusinessTime(instant, "en", null, {
      hour: "2-digit",
      minute: "2-digit",
      hourCycle: "h23",
    });
    expect(utc).toMatch(/22:12/);
    expect(en).not.toMatch(/22:12/);
    expect(es).not.toMatch(/22:12/);
    expect(esAR).not.toMatch(/22:12/);
    expect(en).toMatch(/6:12/);
    expect(es).toMatch(/6:12|18:12/);
    expect(esAR).toMatch(/6:12|18:12/);
  });
});

describe("businessLocalHour", () => {
  it("returns the hour in the business timezone, not the device zone", () => {
    // 2026-08-11T22:27:00Z == 18:27 in America/New_York (EDT, UTC-4)
    const instant = new Date("2026-08-11T22:27:00.000Z");
    expect(businessLocalHour(instant, "America/New_York")).toBe(18);
    // Same instant is 15:27 in Pacific — must not drive venue greetings.
    expect(businessLocalHour(instant, "America/Los_Angeles")).toBe(15);
  });

  it("falls back to UTC when the zone is missing or invalid", () => {
    expect(resolveBusinessTimeZone(null)).toBe("UTC");
    expect(resolveBusinessTimeZone("Not/AZone")).toBe("UTC");
    const instant = new Date("2026-08-11T22:27:00.000Z");
    expect(businessLocalHour(instant, null)).toBe(22);
  });
});

describe("schedule chip venue formatting (#247)", () => {
  it("maps venue-local dinner walls to evening labels, not the 4:00 AM–12:00 PM collapse", () => {
    // Correctly stored America/New_York dinner (16:00–00:00 EDT → 20:00Z–04:00Z).
    const dinnerStart = formatBusinessTime(
      "2026-08-11T20:00:00.000Z",
      "en",
      "America/New_York",
      TIME_SHORT,
    );
    const dinnerEnd = formatBusinessTime(
      "2026-08-12T04:00:00.000Z",
      "en",
      "America/New_York",
      TIME_SHORT,
    );
    expect(dinnerStart).toMatch(/4:00\s*PM/);
    expect(dinnerEnd).toMatch(/12:00\s*AM/);

    // Classic UTC-wall seed bug: 08:00–16:00Z reads as 4:00 AM–12:00 PM in NY.
    // Schedule chips must use venue TZ so this shape never masquerades as dinner.
    const bugStart = formatBusinessTime(
      "2026-08-11T08:00:00.000Z",
      "en",
      "America/New_York",
      TIME_SHORT,
    );
    const bugEnd = formatBusinessTime(
      "2026-08-11T16:00:00.000Z",
      "en",
      "America/New_York",
      TIME_SHORT,
    );
    expect(bugStart).toMatch(/4:00\s*AM/);
    expect(bugEnd).toMatch(/12:00\s*PM/);
    expect(dinnerStart).not.toEqual(bugStart);
  });
});
