import {
  contrastRatio,
  validateTextOnBackground,
  validatePrimaryBrandColor,
  validateSecondaryBrandColor,
  SHIPPED_BRAND_PRESETS,
  MIN_AA_CONTRAST,
} from "./contrast";

describe("contrastRatio", () => {
  it("matches WCAG reference pairs", () => {
    // Black on white is the WCAG reference maximum.
    expect(contrastRatio("#000000", "#ffffff")).toBeCloseTo(21, 10);
    expect(contrastRatio("#FFFFFF", "#000000")).toBeCloseTo(21, 10);

    // #767676 on white is the classic AA boundary example (~4.54:1).
    expect(contrastRatio("#767676", "#ffffff")).toBeCloseTo(4.54, 1);
    // One step lighter fails AA for normal text (~4.48:1).
    expect(contrastRatio("#777777", "#ffffff")).toBeLessThan(4.5);
  });

  it("rejects non-#RRGGBB input", () => {
    expect(() => contrastRatio("#fff", "#ffffff")).toThrow(TypeError);
    expect(() => contrastRatio("ffffff", "#ffffff")).toThrow(TypeError);
    expect(() => contrastRatio("#gggggg", "#ffffff")).toThrow(TypeError);
  });
});

describe("validateTextOnBackground", () => {
  it("raises a blocking validation error naming the ratio when below 4.5:1", () => {
    // Light amber body text on white — well under AA.
    const result = validateTextOnBackground("#f59e0b", "#ffffff");
    expect(result.ok).toBe(false);
    expect(result.ratio).toBeLessThan(MIN_AA_CONTRAST);
    expect(result.error).toBeDefined();
    expect(result.error).toMatch(/4\.5\s*:?\s*1/i);
    // Named ratio appears in the message (one decimal place is fine).
    expect(result.error).toMatch(
      new RegExp(result.ratio.toFixed(1).replace(".", "\\.")),
    );
  });

  it("passes when text-on-background clears WCAG AA", () => {
    const result = validateTextOnBackground("#000000", "#ffffff");
    expect(result.ok).toBe(true);
    expect(result.ratio).toBeGreaterThanOrEqual(MIN_AA_CONTRAST);
    expect(result.error).toBeUndefined();
  });
});

describe("shipped brand presets", () => {
  it("exports exactly eight presets", () => {
    expect(SHIPPED_BRAND_PRESETS).toHaveLength(8);
  });

  it.each(SHIPPED_BRAND_PRESETS.map((p) => [p.key, p] as const))(
    "preset %s passes AA for primary (white text) and secondary (text on white)",
    (_key, preset) => {
      const primary = validatePrimaryBrandColor(preset.primary);
      const secondary = validateSecondaryBrandColor(preset.secondary);
      expect(primary.ok).toBe(true);
      expect(secondary.ok).toBe(true);
    },
  );
});
