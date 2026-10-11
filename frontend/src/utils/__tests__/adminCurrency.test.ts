import { formatAdminCurrency } from "../adminCurrency";

describe("formatAdminCurrency", () => {
  it("formats a number as 2-decimal USD by default", () => {
    expect(formatAdminCurrency(1234.5)).toBe("$1,234.50");
  });

  it("parses a numeric string", () => {
    expect(formatAdminCurrency("29")).toBe("$29.00");
  });

  it("returns $0.00 for null/undefined", () => {
    expect(formatAdminCurrency(null)).toBe("$0.00");
    expect(formatAdminCurrency(undefined)).toBe("$0.00");
  });

  it("honors a 0-fraction-digits option for whole-dollar KPI tiles", () => {
    expect(formatAdminCurrency(1234.5, { fractionDigits: 0 })).toBe("$1,235");
  });

  it("honors a currency override (fiscal ARS receipts)", () => {
    // Intl separates the code and number with U+00A0.
    expect(formatAdminCurrency(1234.5, { currency: "ARS" })).toBe("ARS 1,234.50");
    expect(formatAdminCurrency(null, { currency: "ARS" })).toBe("ARS 0.00");
  });

  it("shortens a million or more in compact mode so KPI tiles stay on one line", () => {
    expect(formatAdminCurrency(38336450, { currency: "ARS", fractionDigits: 0, compact: true })).toBe("ARS\u00a038.3M");
    expect(formatAdminCurrency(38336450, { fractionDigits: 0, compact: true })).toBe("$38.3M");
    expect(formatAdminCurrency(999999, { fractionDigits: 0, compact: true })).toBe("$999,999");
  });
});
