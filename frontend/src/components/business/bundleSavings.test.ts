import {
  computeBundleSavings,
  bundleSavingsTone,
  isBundlePriceAtOrAboveRegular,
} from "./bundleSavings";

describe("L3-18 signed bundle savings", () => {
  it("keeps negative savings (does not clamp to 0)", () => {
    expect(computeBundleSavings(10, 15)).toBe(-5);
    expect(bundleSavingsTone(-5)).toBe("negative");
  });

  it("positive savings stay green-tone", () => {
    expect(computeBundleSavings(20, 15)).toBe(5);
    expect(bundleSavingsTone(5)).toBe("positive");
  });

  it("flags price >= regularTotal for S-5 warning", () => {
    expect(isBundlePriceAtOrAboveRegular(10, 10)).toBe(true);
    expect(isBundlePriceAtOrAboveRegular(10, 12)).toBe(true);
    expect(isBundlePriceAtOrAboveRegular(10, 8)).toBe(false);
  });
});
