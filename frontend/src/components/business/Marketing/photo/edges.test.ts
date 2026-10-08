import type { ImageDataLike } from "./types";
import { BLUR_FLOOR, edgeGrid, laplacianVariance } from "./edges";

function grey(
  width: number,
  height: number,
  at: (x: number, y: number) => number,
): ImageDataLike {
  const data = new Uint8ClampedArray(width * height * 4);
  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      const value = at(x, y);
      const o = (y * width + x) * 4;
      data[o] = value;
      data[o + 1] = value;
      data[o + 2] = value;
      data[o + 3] = 255;
    }
  }
  return { data, width, height };
}

const flat = (size: number, value = 128) => grey(size, size, () => value);
const verticalEdge = (size: number) =>
  grey(size, size, (x) => (x < size / 2 ? 0 : 255));
const checker = (size: number) =>
  grey(size, size, (x, y) => ((x + y) % 2 === 0 ? 0 : 255));
/** A linear ramp: second derivative zero everywhere, so maximally "soft". */
const ramp = (size: number) =>
  grey(size, size, (x) => Math.round((x / (size - 1)) * 255));
/** 4px-period stripes: hard edges, so unambiguously "sharp". */
const stripes = (size: number) =>
  grey(size, size, (x) => (Math.floor(x / 2) % 2 === 0 ? 0 : 255));

describe("edgeGrid", () => {
  it("reports no energy on a flat field", () => {
    expect(edgeGrid(flat(16), 4).values.every((v) => v === 0)).toBe(true);
  });

  it("normalizes a full-contrast step to the Sobel maximum ratio", () => {
    // A single black/white step gives |Gx| = 4 * 255 = 1020 and Gy = 0, so the
    // magnitude is 1020 against a theoretical maximum of hypot(1020, 1020).
    edgeGrid(verticalEdge(4), 2).values.forEach((value) =>
      expect(value).toBeCloseTo(1020 / Math.hypot(1020, 1020), 4),
    );
  });

  it("is bounded to 0..1", () => {
    edgeGrid(checker(16), 4).values.forEach((value) => {
      expect(value).toBeGreaterThanOrEqual(0);
      expect(value).toBeLessThanOrEqual(1);
    });
  });

  it("returns zeros for a degenerate image", () => {
    expect(edgeGrid({ data: [], width: 0, height: 0 }, 2).values).toEqual([
      0, 0, 0, 0,
    ]);
  });
});

describe("laplacianVariance", () => {
  it("is zero on a flat field", () => {
    expect(laplacianVariance(flat(16))).toBeCloseTo(0, 6);
  });

  it("is zero on a linear ramp — a ramp has no second derivative", () => {
    expect(laplacianVariance(ramp(64))).toBeLessThan(1);
  });

  it("is the squared response on a one-pixel checkerboard", () => {
    // Every interior pixel sits opposite its four neighbours, so the 4-neighbour
    // Laplacian is ±(8v - 1020) = ±1020 and the mean is zero.
    expect(laplacianVariance(checker(8))).toBeCloseTo(1020 * 1020, 0);
  });

  it("brackets BLUR_FLOOR: a ramp is below it, hard stripes above", () => {
    expect(laplacianVariance(ramp(64))).toBeLessThan(BLUR_FLOOR);
    expect(laplacianVariance(stripes(64))).toBeGreaterThan(BLUR_FLOOR);
  });
});
