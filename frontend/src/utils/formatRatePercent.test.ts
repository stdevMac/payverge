import { formatRatePercent } from "./formatRatePercent";

describe("formatRatePercent", () => {
  it("preserves three-decimal tax rates instead of toFixed(2) rounding", () => {
    expect(formatRatePercent(8.875)).toBe("8.875%");
    expect(formatRatePercent(8.875)).not.toBe("8.88%");
  });

  it("strips trailing zeros for whole and one-decimal rates", () => {
    expect(formatRatePercent(4)).toBe("4%");
    expect(formatRatePercent(4.0)).toBe("4%");
    expect(formatRatePercent(4.5)).toBe("4.5%");
    expect(formatRatePercent(4.5)).not.toBe("4.50%");
  });

  it("handles non-finite input", () => {
    expect(formatRatePercent(Number.NaN)).toBe("0%");
    expect(formatRatePercent(Number.POSITIVE_INFINITY)).toBe("0%");
  });
});
