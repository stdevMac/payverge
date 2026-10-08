import { formatTrendLabel } from "./chartDates";

describe("formatTrendLabel", () => {
  it("formats ISO day strings as short localized dates", () => {
    expect(formatTrendLabel("2026-06-03", "en")).toBe("Jun 3");
  });

  it("localizes for Spanish operators", () => {
    expect(formatTrendLabel("2026-06-03", "es").toLowerCase()).toContain(
      "jun",
    );
  });

  it("does not UTC-shift the day for western timezones", () => {
    // Regardless of the test machine's TZ, a plain ISO day must render that
    // calendar day, not the previous one.
    expect(formatTrendLabel("2026-01-01", "en")).toBe("Jan 1");
  });

  it("passes non-ISO labels through untouched", () => {
    expect(formatTrendLabel("13:00", "en")).toBe("13:00");
    expect(formatTrendLabel("Week 23", "en")).toBe("Week 23");
    expect(formatTrendLabel("2026-06-03T10:00:00Z", "en")).toBe(
      "2026-06-03T10:00:00Z",
    );
  });
});
