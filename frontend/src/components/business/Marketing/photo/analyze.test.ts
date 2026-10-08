import type { ImageDataLike } from "./types";
import { DOMINANT_COLOR_COUNT, analyzePhoto, photoSignalsFrom } from "./analyze";

function build(
  width: number,
  height: number,
  at: (x: number, y: number) => [number, number, number],
): ImageDataLike {
  const data = new Uint8ClampedArray(width * height * 4);
  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      const [r, g, b] = at(x, y);
      const o = (y * width + x) * 4;
      data[o] = r;
      data[o + 1] = g;
      data[o + 2] = b;
      data[o + 3] = 255;
    }
  }
  return { data, width, height };
}

/** Bottom third flat, top two thirds hard stripes: a quiet bottom band. */
const quietBottom = build(64, 64, (x, y) =>
  y >= 42 ? [30, 30, 30] : Math.floor(x / 2) % 2 === 0 ? [0, 0, 0] : [255, 255, 255],
);

const flat = build(64, 64, () => [120, 120, 120]);

describe("analyzePhoto", () => {
  it("derives every field from one image", () => {
    const analysis = analyzePhoto(quietBottom);
    expect(analysis.luma.size).toBe(analysis.edges.size);
    expect(analysis.luma.values).toHaveLength(analysis.luma.size ** 2);
    expect(analysis.blurScore).toBeGreaterThan(0);
    expect(analysis.exposure.histogram.reduce((s, v) => s + v, 0)).toBeCloseTo(
      1,
      6,
    );
    expect(analysis.negativeSpace).toBe("bottom");
    expect(analysis.busy).toBe(true);
    expect(analysis.focal.x).toBeGreaterThanOrEqual(0);
    expect(analysis.focal.y).toBeLessThanOrEqual(1);
    expect(analysis.dominantColors.length).toBeGreaterThan(0);
    expect(analysis.dominantColors.length).toBeLessThanOrEqual(
      DOMINANT_COLOR_COUNT,
    );
    analysis.dominantColors.forEach((hex) =>
      expect(hex).toMatch(/^#[0-9a-f]{6}$/),
    );
  });

  it("keeps the focal point at the subject box centre", () => {
    const analysis = analyzePhoto(quietBottom);
    expect(analysis.focal).toEqual({
      x: analysis.subject.x + analysis.subject.w / 2,
      y: analysis.subject.y + analysis.subject.h / 2,
    });
  });

  it("calls a flat frame calm and centres its focal point", () => {
    const analysis = analyzePhoto(flat);
    expect(analysis.busy).toBe(false);
    expect(analysis.negativeSpace).toBe("none");
    expect(analysis.focal).toEqual({ x: 0.5, y: 0.5 });
    expect(analysis.blurScore).toBeCloseTo(0, 6);
  });

  it("is deterministic", () => {
    expect(analyzePhoto(quietBottom)).toEqual(analyzePhoto(quietBottom));
  });
});

describe("photoSignalsFrom", () => {
  it("narrows the analysis to what the chooser reads", () => {
    expect(photoSignalsFrom(analyzePhoto(quietBottom))).toEqual({
      negativeSpace: "bottom",
      busy: true,
    });
  });

  it("passes null through, so a failed analysis is not a fabricated signal", () => {
    expect(photoSignalsFrom(null)).toBeNull();
  });
});
