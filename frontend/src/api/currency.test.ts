import { formatCurrency } from "@/api/currency";

describe("formatCurrency", () => {
  it("formats USD with cents", () => {
    expect(formatCurrency(1234.56, "USD")).toBe("$1,234.56");
  });

  it("formats zero-decimal currencies without forced cents", () => {
    expect(formatCurrency(1234.56, "JPY")).toBe("¥1,235");
  });

  it("honors the supplied locale for grouping/decimal separators", () => {
    // German locale swaps the comma/period roles: 1.234,56 €
    const de = formatCurrency(1234.56, "EUR", undefined, "de-DE");
    // Non-breaking spaces / glyph placement vary by ICU build, so assert the
    // separator semantics rather than the exact string.
    expect(de).toContain("1.234,56");
    expect(de).toContain("€");
  });

  it("defaults to en-US grouping when no locale is given", () => {
    expect(formatCurrency(1234.56, "EUR")).toBe("€1,234.56");
  });

  it("formats USDC with the diner locale decimal convention (not toFixed)", () => {
    // Issue #561: chip sat next to "34,99 $" as locale-blind "USDC 34.99".
    const de = formatCurrency(34.99, "USDC", undefined, "de");
    const deDE = formatCurrency(34.99, "USDC", undefined, "de-DE");
    const fr = formatCurrency(34.99, "USDC", undefined, "fr");
    expect(de).toContain("USDC");
    expect(de).toContain("34,99");
    expect(de).not.toContain("34.99");
    expect(deDE).toContain("34,99");
    expect(deDE).not.toContain("34.99");
    expect(fr).toContain("34,99");
    expect(fr).not.toContain("34.99");
    expect(formatCurrency(34.99, "USDC", undefined, "en")).toContain("34.99");
  });

  it("formats grouped USDC amounts with locale separators", () => {
    const de = formatCurrency(1234.5, "USDC", undefined, "de");
    expect(de).toContain("1.234,50");
    expect(de).not.toMatch(/1234\.50/);
  });
});
