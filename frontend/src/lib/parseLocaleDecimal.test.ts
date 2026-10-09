import {
  parseLocaleDecimal,
  tryParseLocaleDecimal,
} from "./parseLocaleDecimal";

/**
 * Exhaustive suite for the locale-aware number parser.
 * Covers the three silent data-corruption classes (L5-36 / L5-13 / L3-17)
 * plus garbage, empty, negatives, and bound overflow.
 */
describe("parseLocaleDecimal", () => {
  describe("comma decimals (es/fr/de/… — L5-36, L5-13)", () => {
    it.each([
      ["12,34", 12.34], // L5-36 cost-per-unit
      ["2,5", 2.5], // L5-13 loyalty points-rate
      ["5,50", 5.5],
      ["0,75", 0.75],
      ["0,1", 0.1],
      ["  7,9 ", 7.9],
      ["€5,50", 5.5],
      ["$12,34", 12.34],
    ])("parses %p as %p", (raw, expected) => {
      expect(parseLocaleDecimal(raw)).toBe(expected);
    });
  });

  describe("dot decimals", () => {
    it.each([
      ["12.34", 12.34],
      ["5.50", 5.5],
      ["5", 5],
      ["0.75", 0.75],
    ])("parses %p as %p", (raw, expected) => {
      expect(parseLocaleDecimal(raw)).toBe(expected);
    });
  });

  describe(">2 fractional digits must NOT strip the separator (L3-17)", () => {
    it.each([
      // Paste with 3+ decimals: keep magnitude, round to 2 places.
      ["12.345", 12.35],
      ["12,345", 12.35], // same when comma is the decimal
      ["10.999", 11],
      ["0.001", 0],
      ["99.999", 100],
      ["1.2345", 1.23],
    ])("parses %p as %p (not 1000× integer)", (raw, expected) => {
      expect(parseLocaleDecimal(raw)).toBe(expected);
      // Explicit anti-corruption: must not equal the digit-stripped integer.
      const stripped = Number(String(raw).replace(/[^\d-]/g, ""));
      expect(parseLocaleDecimal(raw)).not.toBe(stripped);
    });
  });

  describe("thousands grouping (both separators or repeated same)", () => {
    it.each([
      ["1.234,56", 1234.56], // EU thousands + comma decimal
      ["1,234.56", 1234.56], // EN thousands + dot decimal
      ["1.234.567", 1234567], // repeated dot = thousands only
      ["1,234,567", 1234567], // repeated comma = thousands only
      ["1.234.567,89", 1234567.89],
      ["1,234,567.89", 1234567.89],
    ])("parses %p as %p", (raw, expected) => {
      expect(parseLocaleDecimal(raw)).toBe(expected);
    });
  });

  describe("lone 3-digit group is a decimal (not silent thousands)", () => {
    // Single separator + 3 digits after is ambiguous (EU thousands vs 3-dp).
    // Prefer decimal so money pastes never 1000×-corrupt; operators type 1234
    // bare when they mean one thousand two hundred thirty-four.
    it.each([
      ["1.234", 1.23],
      ["1,234", 1.23],
    ])("parses %p as %p", (raw, expected) => {
      expect(parseLocaleDecimal(raw)).toBe(expected);
    });
  });

  describe("empty / garbage → 0 (legacy || 0 semantics)", () => {
    it.each([["abc"], [""], ["   "], ["€"], ["--"], [",,"], [".."]])(
      "parses %p as 0",
      (raw) => {
        expect(parseLocaleDecimal(raw)).toBe(0);
      },
    );

    it("non-string input returns 0", () => {
      // @ts-expect-error intentional
      expect(parseLocaleDecimal(null)).toBe(0);
      // @ts-expect-error intentional
      expect(parseLocaleDecimal(undefined)).toBe(0);
      // @ts-expect-error intentional
      expect(parseLocaleDecimal(12.34)).toBe(0);
    });
  });

  describe("trailing separator and partials", () => {
    it.each([
      ["5,", 5],
      ["5.", 5],
      [",5", 0.5],
      [".5", 0.5],
    ])("parses %p as %p", (raw, expected) => {
      expect(parseLocaleDecimal(raw)).toBe(expected);
    });
  });

  describe("negatives", () => {
    it.each([
      ["-3,25", -3.25],
      ["-3.25", -3.25],
      ["-12,34", -12.34],
      ["-0,5", -0.5],
    ])("parses %p as %p", (raw, expected) => {
      expect(parseLocaleDecimal(raw)).toBe(expected);
    });
  });

  describe("hyphens anywhere but a single leading minus are garbage", () => {
    it.each([["12-34"], ["1-2"], ["12--34"], ["--5"], ["5-"], ["-12-34"]])(
      "rejects %p as 0",
      (raw) => {
        expect(parseLocaleDecimal(raw)).toBe(0);
      },
    );

    it("tryParse returns null, not a stripped integer", () => {
      expect(tryParseLocaleDecimal("12-34")).toBeNull();
      expect(tryParseLocaleDecimal("5-")).toBeNull();
    });

    it("still accepts a single leading minus", () => {
      expect(parseLocaleDecimal("-12,34")).toBe(-12.34);
      expect(tryParseLocaleDecimal("-7")).toBe(-7);
    });
  });

  describe("bound / overflow", () => {
    it("clamps absurd magnitudes to Number.MAX_SAFE_INTEGER cents floor", () => {
      // Larger than what money fields should ever hold — must stay finite.
      const n = parseLocaleDecimal("999999999999999999999");
      expect(Number.isFinite(n)).toBe(true);
      expect(Math.abs(n)).toBeLessThanOrEqual(Number.MAX_SAFE_INTEGER / 100);
    });

    it("rejects mixed letter-digit garbage as 0", () => {
      expect(parseLocaleDecimal("12a34")).toBe(0);
      expect(parseLocaleDecimal("12,3a")).toBe(0);
    });
  });

  describe("maxFractionDigits option", () => {
    it("defaults to 2 places", () => {
      expect(parseLocaleDecimal("1,239")).toBe(1.24);
    });

    it("can keep more places when requested", () => {
      expect(parseLocaleDecimal("1,2345", { maxFractionDigits: 4 })).toBe(1.2345);
      expect(parseLocaleDecimal("2,5", { maxFractionDigits: 1 })).toBe(2.5);
    });
  });
});

describe("tryParseLocaleDecimal", () => {
  it("returns null for empty / whitespace", () => {
    expect(tryParseLocaleDecimal("")).toBeNull();
    expect(tryParseLocaleDecimal("   ")).toBeNull();
  });

  it("returns null for garbage", () => {
    expect(tryParseLocaleDecimal("abc")).toBeNull();
    expect(tryParseLocaleDecimal("12a")).toBeNull();
    expect(tryParseLocaleDecimal(",,")).toBeNull();
  });

  it("returns the number for valid locale decimals", () => {
    expect(tryParseLocaleDecimal("12,34")).toBe(12.34);
    expect(tryParseLocaleDecimal("2,5")).toBe(2.5);
    expect(tryParseLocaleDecimal("12.345")).toBe(12.35);
    expect(tryParseLocaleDecimal("-1,5")).toBe(-1.5);
  });

  it("does not treat 0 as null", () => {
    expect(tryParseLocaleDecimal("0")).toBe(0);
    expect(tryParseLocaleDecimal("0,00")).toBe(0);
  });
});
