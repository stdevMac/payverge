import { LUMA_B, LUMA_G, LUMA_R, perceptualLuma } from "./luma";

describe("perceptualLuma", () => {
  it("uses the Rec.601 weighting the scrim threshold is expressed in", () => {
    expect(LUMA_R).toBe(0.299);
    expect(LUMA_G).toBe(0.587);
    expect(LUMA_B).toBe(0.114);
  });

  it("maps white to 255 and black to 0", () => {
    expect(perceptualLuma(255, 255, 255)).toBeCloseTo(255, 6);
    expect(perceptualLuma(0, 0, 0)).toBe(0);
  });

  it("weights green far above blue", () => {
    expect(perceptualLuma(0, 255, 0)).toBeCloseTo(149.685, 3);
    expect(perceptualLuma(0, 0, 255)).toBeCloseTo(29.07, 3);
    expect(perceptualLuma(255, 0, 0)).toBeCloseTo(76.245, 3);
  });

  // This is NOT WCAG relative luminance. Mid grey is 128 here and ~0.216 there;
  // neither is a rescaling of the other, and confusing them silently breaks the
  // adaptive scrim.
  it("is not normalized to 0..1", () => {
    expect(perceptualLuma(128, 128, 128)).toBeCloseTo(128, 6);
  });
});
