import type { ImageDataLike } from "./types";
import { GRID_CELLS, lumaGrid, lumaPlane } from "./grid";

/** Build an image from a per-pixel colour function. Opaque everywhere. */
function image(
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

const BLACK: [number, number, number] = [0, 0, 0];
const WHITE: [number, number, number] = [255, 255, 255];

describe("lumaPlane", () => {
  it("returns one luma per pixel, row-major", () => {
    const plane = lumaPlane(image(2, 2, (x) => (x === 0 ? BLACK : WHITE)));
    expect(Array.from(plane)).toEqual([0, 255, 0, 255]);
  });

  it("returns an empty plane for a zero-sized image", () => {
    expect(lumaPlane({ data: [], width: 0, height: 0 }).length).toBe(0);
  });
});

describe("lumaGrid", () => {
  it("averages each cell", () => {
    const grid = lumaGrid(
      image(4, 4, (x) => (x < 2 ? BLACK : WHITE)),
      2,
    );
    expect(grid.size).toBe(2);
    expect(grid.values).toEqual([0, 255, 0, 255]);
  });

  it("defaults to the shared cell count", () => {
    expect(lumaGrid(image(64, 64, () => WHITE)).size).toBe(GRID_CELLS);
    expect(lumaGrid(image(64, 64, () => WHITE)).values).toHaveLength(
      GRID_CELLS * GRID_CELLS,
    );
  });

  // Cells never collapse to zero width even when the image has fewer pixels
  // than cells, so no cell is ever left as an unwritten NaN.
  it("survives an image smaller than the grid", () => {
    const grid = lumaGrid(image(2, 2, () => WHITE), 8);
    expect(grid.values).toHaveLength(64);
    grid.values.forEach((value) => expect(Number.isFinite(value)).toBe(true));
  });

  it("ignores fully transparent pixels rather than reading them as black", () => {
    const data = new Uint8ClampedArray(2 * 1 * 4);
    // Pixel 0: opaque white. Pixel 1: transparent (channels are garbage).
    data.set([255, 255, 255, 255], 0);
    data.set([0, 0, 0, 0], 4);
    const grid = lumaGrid({ data, width: 2, height: 1 }, 1);
    expect(grid.values[0]).toBeCloseTo(255, 6);
  });

  it("returns zeros for a degenerate image", () => {
    expect(lumaGrid({ data: [], width: 0, height: 0 }, 2).values).toEqual([
      0, 0, 0, 0,
    ]);
  });
});
