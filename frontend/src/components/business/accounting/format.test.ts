import { formatDay, formatMoney, formatPeriod } from "./format";

describe("formatMoney", () => {
  it("formats USD amounts for en", () => {
    expect(formatMoney(1850, "USD", "en")).toBe("$1,850.00");
  });

  it("formats USD amounts for es", () => {
    // Intl "es" uses comma decimal and a currency suffix (NBSP possible).
    const result = formatMoney(1850, "USD", "es");
    expect(result.replace(/\u00a0|\u202f/g, " ")).toBe("1850,00 US$");
  });

  it("defaults locale to en when omitted", () => {
    expect(formatMoney(1850, "USD")).toBe("$1,850.00");
  });

  it("treats non-finite amounts as 0", () => {
    expect(formatMoney(Number.NaN, "USD", "en")).toBe("$0.00");
  });

  it("falls back for invalid currency codes", () => {
    expect(formatMoney(10, "NOTREAL", "en")).toBe("NOTREAL 10.00");
  });
});

describe("formatDay", () => {
  it("formats a calendar day for en", () => {
    expect(formatDay("2026-07-13", "en")).toBe("Jul 13, 2026");
  });

  it("formats a calendar day for es", () => {
    expect(formatDay("2026-07-13", "es")).toBe("13 jul 2026");
  });

  it("returns empty string for empty input", () => {
    expect(formatDay("", "en")).toBe("");
  });

  it("pins ISO timestamps to the calendar day (no TZ shift)", () => {
    expect(formatDay("2026-07-13T23:00:00Z", "en")).toBe("Jul 13, 2026");
  });
});

describe("formatPeriod", () => {
  it("formats a range for en", () => {
    expect(formatPeriod("2026-07-13", "2026-07-19", "en")).toBe(
      "Jul 13, 2026 → Jul 19, 2026",
    );
  });

  it("formats a range for es", () => {
    expect(formatPeriod("2026-07-13", "2026-07-19", "es")).toBe(
      "13 jul 2026 → 19 jul 2026",
    );
  });

  it("collapses to a single day when start equals end", () => {
    expect(formatPeriod("2026-07-13", "2026-07-13", "en")).toBe(
      "Jul 13, 2026",
    );
  });

  it("returns end day when start is empty", () => {
    expect(formatPeriod("", "2026-07-19", "en")).toBe("2026-07-19");
  });
});
