import type { ImageDataLike } from "./types";
import { EXPOSURE_BUCKETS, exposureStats } from "./exposure";

function pixels(
  colors: Array<[number, number, number, number]>,
): ImageDataLike {
  const data = new Uint8ClampedArray(colors.length * 4);
  colors.forEach(([r, g, b, a], index) => {
    data.set([r, g, b, a], index * 4);
  });
  return { data, width: colors.length, height: 1 };
}

const WHITE: [number, number, number, number] = [255, 255, 255, 255];
const BLACK: [number, number, number, number] = [0, 0, 0, 255];
const GREY: [number, number, number, number] = [128, 128, 128, 255];

describe("exposureStats", () => {
  it("puts white in the last bucket and calls it clipped", () => {
    const stats = exposureStats(pixels([WHITE, WHITE]));
    expect(stats.histogram).toHaveLength(EXPOSURE_BUCKETS);
    expect(stats.histogram[EXPOSURE_BUCKETS - 1]).toBeCloseTo(1, 6);
    expect(stats.meanLuma).toBeCloseTo(255, 6);
    expect(stats.highlightClipping).toBeCloseTo(1, 6);
    expect(stats.shadowClipping).toBeCloseTo(0, 6);
  });

  it("puts black in the first bucket and calls it clipped", () => {
    const stats = exposureStats(pixels([BLACK, BLACK]));
    expect(stats.histogram[0]).toBeCloseTo(1, 6);
    expect(stats.meanLuma).toBeCloseTo(0, 6);
    expect(stats.shadowClipping).toBeCloseTo(1, 6);
    expect(stats.highlightClipping).toBeCloseTo(0, 6);
  });

  it("clips neither end on mid grey", () => {
    const stats = exposureStats(pixels([GREY, GREY]));
    expect(stats.histogram[8]).toBeCloseTo(1, 6);
    expect(stats.shadowClipping).toBe(0);
    expect(stats.highlightClipping).toBe(0);
    expect(stats.meanLuma).toBeCloseTo(128, 6);
  });

  it("normalizes the histogram to a distribution", () => {
    const stats = exposureStats(pixels([BLACK, WHITE, GREY, GREY]));
    expect(stats.histogram.reduce((sum, v) => sum + v, 0)).toBeCloseTo(1, 6);
    expect(stats.shadowClipping).toBeCloseTo(0.25, 6);
    expect(stats.highlightClipping).toBeCloseTo(0.25, 6);
  });

  it("reports per-channel means for grey-world white balance", () => {
    const stats = exposureStats(pixels([[200, 100, 50, 255], [100, 50, 25, 255]]));
    expect(stats.channelMeans).toEqual({ r: 150, g: 75, b: 37.5 });
  });

  it("ignores transparent pixels", () => {
    const stats = exposureStats(pixels([WHITE, [0, 0, 0, 0]]));
    expect(stats.meanLuma).toBeCloseTo(255, 6);
    expect(stats.shadowClipping).toBe(0);
  });

  it("returns a neutral, non-NaN result for an empty image", () => {
    const stats = exposureStats({ data: [], width: 0, height: 0 });
    expect(stats.meanLuma).toBe(128);
    expect(stats.channelMeans).toEqual({ r: 128, g: 128, b: 128 });
    expect(stats.histogram.reduce((sum, v) => sum + v, 0)).toBe(0);
  });
});
