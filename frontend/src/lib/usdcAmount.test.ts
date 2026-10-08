import { formatUsdcMicrounits } from "./usdcAmount";

describe("formatUsdcMicrounits", () => {
  it.each([
    [55_004_919, "55.004919"],
    [55_000_000, "55.000000"],
    [1, "0.000001"],
    [9_999, "0.009999"],
    [0, "0.000000"],
    [108_004_213, "108.004213"],
    [-1_500_000, "-1.500000"],
    [Number.MAX_SAFE_INTEGER, "9007199254.740991"],
  ])("renders %d micro-USDC as %s", (microunits, expected) => {
    expect(formatUsdcMicrounits(microunits)).toBe(expected);
  });

  it("keeps the sub-cent binding offset instead of rounding to cents", () => {
    // 12.34 USD + offset 4567 must not display as 12.35 or 12.34.
    expect(formatUsdcMicrounits(12_344_567)).toBe("12.344567");
  });

  it.each([0.5, Number.NaN, Number.POSITIVE_INFINITY, 2 ** 53])(
    "rejects a non-safe-integer amount (%p)",
    (value) => {
      expect(() => formatUsdcMicrounits(value)).toThrow(RangeError);
    },
  );
});
