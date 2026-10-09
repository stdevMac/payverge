import { extractDominantColors, harmonizePalette, type ImageDataLike } from "./palette";

/** Build a w×h RGBA buffer from a per-pixel color function. */
function makeImage(
  width: number,
  height: number,
  at: (x: number, y: number) => [number, number, number, number],
): ImageDataLike {
  const data = new Uint8ClampedArray(width * height * 4);
  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      const [r, g, b, a] = at(x, y);
      const i = (y * width + x) * 4;
      data[i] = r;
      data[i + 1] = g;
      data[i + 2] = b;
      data[i + 3] = a;
    }
  }
  return { data, width, height };
}

describe("extractDominantColors", () => {
  it("recovers the two colors of a half-red half-blue image", () => {
    const img = makeImage(8, 8, (x) =>
      x < 4 ? [255, 0, 0, 255] : [0, 0, 255, 255],
    );
    const colors = extractDominantColors(img, 2);
    expect(colors).toHaveLength(2);
    expect(colors.sort()).toEqual(["#0000ff", "#ff0000"]);
  });

  it("ignores transparent pixels", () => {
    const img = makeImage(4, 4, (x) =>
      x < 2 ? [255, 0, 0, 255] : [0, 255, 0, 0],
    );
    expect(extractDominantColors(img, 1)).toEqual(["#ff0000"]);
  });

  it("returns an empty array when every pixel is transparent", () => {
    const img = makeImage(2, 2, () => [1, 2, 3, 0]);
    expect(extractDominantColors(img, 3)).toEqual([]);
  });

  it("never returns more colors than requested", () => {
    const img = makeImage(16, 16, (x, y) => [x * 16, y * 16, 128, 255]);
    expect(extractDominantColors(img, 3).length).toBeLessThanOrEqual(3);
  });

  it("caps output at the number of available pixels when count exceeds them", () => {
    // Only 2 opaque pixels exist; there is nothing left to split once every
    // bucket is down to one pixel, so the loop exits via the "no splittable
    // bucket" path rather than by reaching `count`.
    const img = makeImage(2, 1, (x) =>
      x === 0 ? [255, 0, 0, 255] : [0, 0, 255, 255],
    );
    expect(extractDominantColors(img, 5).length).toBeLessThanOrEqual(2);
  });

  it("can repeat the same hex value when a flat region outnumbers count", () => {
    // A uniform-color region has no distinct color to split toward, but the
    // median-cut loop still bisects by pixel position until it reaches
    // `count` buckets, so the result is `count` copies of one averaged hex —
    // not a shorter, deduplicated list. Downstream consumers that need
    // visually distinct swatches must dedupe themselves.
    const img = makeImage(4, 4, () => [10, 20, 30, 255]);
    const colors = extractDominantColors(img, 3);
    expect(colors).toEqual(["#0a141e", "#0a141e", "#0a141e"]);
  });
});

describe("harmonizePalette", () => {
  const brand = { primary: "#1a6b6a", secondary: "#0f3d3c" };

  it("keeps brand primary and secondary intact", () => {
    const out = harmonizePalette(brand, ["#c2410c", "#fed7aa"]);
    expect(out.primary).toBe("#1a6b6a");
    expect(out.secondary).toBe("#0f3d3c");
  });

  it("exposes an accent drawn from the photo", () => {
    const out = harmonizePalette(brand, ["#c2410c", "#fed7aa"]);
    expect(["#c2410c", "#fed7aa"]).toContain(out.accent);
  });

  it("falls back to the brand primary as accent with no photo colors", () => {
    expect(harmonizePalette(brand, []).accent).toBe("#1a6b6a");
  });

  it("picks onAccent for AA contrast against the accent", () => {
    const out = harmonizePalette(brand, ["#111111"]);
    expect(out.onAccent).toBe("#ffffff");
  });
});
