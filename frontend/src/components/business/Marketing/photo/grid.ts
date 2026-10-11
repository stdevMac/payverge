import { perceptualLuma } from "./luma";
import type { CellGrid, ImageDataLike } from "./types";

/**
 * Cells per side of every analysis grid.
 *
 * 8 over a 64×64 downsample makes each cell an 8×8 block of the downsample —
 * coarse enough that one bright plate does not read as a quiet band, fine
 * enough that thirds of the frame are three whole rows rather than 2.67. Every
 * band rect in `regions.ts` is expressed in 0..1 and area-weighted across
 * cells, so changing this number changes precision, not meaning.
 */
export const GRID_CELLS = 8;

/**
 * Alpha at or above which a pixel counts. Below it the RGB channels of a fully
 * transparent pixel are meaningless (commonly zero), and averaging them in
 * would read a transparent PNG as a black photograph.
 */
const ALPHA_FLOOR = 128;

/**
 * One perceptual luma per pixel, row-major, as a `Float64Array`.
 *
 * The Sobel and Laplacian passes both need random access to neighbouring
 * lumas, and recomputing the weighting per neighbour would triple the
 * multiplications. Transparent pixels resolve to 0 here rather than being
 * skipped: a plane has to stay indexable by `y * width + x`.
 */
export function lumaPlane(img: ImageDataLike): Float64Array {
  const { data, width, height } = img;
  const total = Math.max(0, width) * Math.max(0, height);
  const plane = new Float64Array(total);
  for (let index = 0; index < total; index += 1) {
    const o = index * 4;
    plane[index] =
      data[o + 3] < ALPHA_FLOOR
        ? 0
        : perceptualLuma(data[o], data[o + 1], data[o + 2]);
  }
  return plane;
}

/**
 * Mean perceptual luma per cell, 0–255.
 *
 * Cell edges are computed with `floor(i * extent / cells)` and floored to at
 * least one pixel wide, so an image smaller than the grid still fills every
 * cell instead of leaving some as `0 / 0`. Cells that end up empty anyway
 * (a zero-sized image) report 0 rather than NaN — a NaN would propagate
 * silently through every mean downstream.
 */
export function lumaGrid(img: ImageDataLike, cells = GRID_CELLS): CellGrid {
  const size = Math.max(1, Math.floor(cells));
  const values = new Array<number>(size * size).fill(0);
  const { data, width, height } = img;
  if (width <= 0 || height <= 0) return { size, values };

  const edge = (index: number, extent: number) =>
    Math.floor((index * extent) / size);

  for (let row = 0; row < size; row += 1) {
    const y0 = Math.min(edge(row, height), height - 1);
    const y1 = Math.min(Math.max(y0 + 1, edge(row + 1, height)), height);
    for (let col = 0; col < size; col += 1) {
      const x0 = Math.min(edge(col, width), width - 1);
      const x1 = Math.min(Math.max(x0 + 1, edge(col + 1, width)), width);
      let sum = 0;
      let count = 0;
      for (let y = y0; y < y1; y += 1) {
        for (let x = x0; x < x1; x += 1) {
          const o = (y * width + x) * 4;
          if (data[o + 3] < ALPHA_FLOOR) continue;
          sum += perceptualLuma(data[o], data[o + 1], data[o + 2]);
          count += 1;
        }
      }
      values[row * size + col] = count > 0 ? sum / count : 0;
    }
  }
  return { size, values };
}
