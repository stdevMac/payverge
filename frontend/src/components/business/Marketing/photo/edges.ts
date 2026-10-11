import { lumaPlane, GRID_CELLS } from "./grid";
import type { CellGrid, ImageDataLike } from "./types";

/**
 * The largest magnitude a 3×3 Sobel pair can produce on 0–255 input.
 *
 * Each kernel sums to ±4·255 = ±1020 at a full-contrast step, and the magnitude
 * is `hypot(Gx, Gy)`, so the bound is `hypot(1020, 1020)`. Dividing by it makes
 * every cell value a 0–1 fraction that means the same thing whatever the source
 * image's bit depth or size, which is what lets `BUSY_EDGE` in `regions.ts` be
 * a single constant rather than a per-image threshold.
 */
const SOBEL_MAX = Math.hypot(4 * 255, 4 * 255);

/**
 * Laplacian variance at or above which a downsampled photo counts as sharp.
 *
 * 120 is calibrated on synthetic patterns, not on photographs: a linear ramp
 * (zero second derivative, the softest thing that is not flat) scores under 1,
 * and 4px-period stripes score in the hundreds of thousands. `edges.test.ts`
 * pins the bracket on both sides, so this constant cannot drift silently.
 *
 * It is NOT yet calibrated against real menu photography, and it is the one
 * number in this wave that reaches an operator as a verdict (`readiness.ts`).
 * Before the readiness panel ships to operators, run it over one real
 * business's menu library and confirm the soft/sharp split matches human
 * judgement; if it does not, retune HERE — a single constant with a pinned
 * bracket — rather than adding a second threshold downstream.
 *
 * Note the scale dependence: this is measured on the ~64×64 analysis
 * downsample, not the source photo, so the numbers are far lower than the
 * "variance of Laplacian > 100" figures quoted for full-resolution images.
 * Changing `ANALYSIS_SIZE` in `downsample.ts` invalidates this constant.
 */
export const BLUR_FLOOR = 120;

/**
 * Mean normalized Sobel magnitude per cell, 0–1.
 *
 * Border pixels have no full 3×3 neighbourhood and are skipped rather than
 * clamped or mirrored: on a 64×64 downsample they are 1.5% of the image, and
 * either fix-up invents an edge (mirroring) or flattens one (clamping) exactly
 * at the frame edge, which is where the negative-space bands are measured.
 * A cell containing only border pixels reports 0.
 */
export function edgeGrid(img: ImageDataLike, cells = GRID_CELLS): CellGrid {
  const size = Math.max(1, Math.floor(cells));
  const values = new Array<number>(size * size).fill(0);
  const counts = new Array<number>(size * size).fill(0);
  const { width, height } = img;
  if (width < 3 || height < 3) return { size, values };

  const plane = lumaPlane(img);
  const at = (x: number, y: number) => plane[y * width + x];

  for (let y = 1; y < height - 1; y += 1) {
    for (let x = 1; x < width - 1; x += 1) {
      const tl = at(x - 1, y - 1);
      const tc = at(x, y - 1);
      const tr = at(x + 1, y - 1);
      const ml = at(x - 1, y);
      const mr = at(x + 1, y);
      const bl = at(x - 1, y + 1);
      const bc = at(x, y + 1);
      const br = at(x + 1, y + 1);
      const gx = tr + 2 * mr + br - (tl + 2 * ml + bl);
      const gy = bl + 2 * bc + br - (tl + 2 * tc + tr);
      const magnitude = Math.hypot(gx, gy) / SOBEL_MAX;

      const row = Math.min(size - 1, Math.floor((y * size) / height));
      const col = Math.min(size - 1, Math.floor((x * size) / width));
      const cell = row * size + col;
      values[cell] += magnitude;
      counts[cell] += 1;
    }
  }

  for (let cell = 0; cell < values.length; cell += 1) {
    values[cell] = counts[cell] > 0 ? values[cell] / counts[cell] : 0;
  }
  return { size, values };
}

/**
 * Population variance of the 4-neighbour Laplacian — the standard cheap
 * focus measure. A blurred image has little high-frequency content, so its
 * second derivative is small everywhere and its variance collapses.
 *
 * The 4-neighbour kernel (`4c − up − down − left − right`) is used rather than
 * the 8-neighbour one because it is what "variance of Laplacian" conventionally
 * means and because the diagonal terms add cost without adding separation at
 * this resolution. Border pixels are skipped, as in `edgeGrid`.
 *
 * Population (not sample) variance, so a 3×3 image and a 64×64 image are
 * directly comparable and the divisor can never be zero for a valid input.
 */
export function laplacianVariance(img: ImageDataLike): number {
  const { width, height } = img;
  if (width < 3 || height < 3) return 0;

  const plane = lumaPlane(img);
  const at = (x: number, y: number) => plane[y * width + x];

  let sum = 0;
  let sumSquares = 0;
  let count = 0;
  for (let y = 1; y < height - 1; y += 1) {
    for (let x = 1; x < width - 1; x += 1) {
      const value =
        4 * at(x, y) -
        at(x, y - 1) -
        at(x, y + 1) -
        at(x - 1, y) -
        at(x + 1, y);
      sum += value;
      sumSquares += value * value;
      count += 1;
    }
  }
  if (count === 0) return 0;
  const mean = sum / count;
  return Math.max(0, sumSquares / count - mean * mean);
}
