import { formatDisplayDate } from "../displayDate";

describe("formatDisplayDate", () => {
  const iso = "2026-07-02T15:00:00Z";

  it("renders a month-name date, never ambiguous numeric m/d vs d/m", () => {
    const en = formatDisplayDate(iso, "en");
    expect(en).toMatch(/Jul/);
    expect(en).toMatch(/2026/);
    expect(en).not.toMatch(/^\d+\/\d+\/\d+$/);
  });

  it("localizes the month name for es", () => {
    expect(formatDisplayDate(iso, "es").toLowerCase()).toMatch(/jul/);
  });

  it("returns -- for empty and invalid input", () => {
    expect(formatDisplayDate(undefined, "en")).toBe("--");
    expect(formatDisplayDate("", "en")).toBe("--");
    expect(formatDisplayDate("not-a-date", "en")).toBe("--");
  });
});
