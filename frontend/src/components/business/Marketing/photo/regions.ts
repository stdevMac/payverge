import type { CellGrid, NegativeSpace, Rect01 } from "./types";

/** The whole frame, in 0..1. */
export const FULL_FRAME: Rect01 = { x: 0, y: 0, w: 1, h: 1 };

/**
 * The three bands a composition can actually exploit. Each is a third of the
 * frame with the centre inset horizontally, because a centred stack sits in a
 * column rather than across the full width and measuring the corners with it
 * would let two busy corners veto a genuinely quiet middle.
 */
const TOP_BAND: Rect01 = { x: 0, y: 0, w: 1, h: 0.34 };
const BOTTOM_BAND: Rect01 = { x: 0, y: 0.66, w: 1, h: 0.34 };
const CENTER_BAND: Rect01 = { x: 0.2, y: 0.33, w: 0.6, h: 0.34 };

/**
 * How much quieter than the whole frame a band must be to count as negative
 * space. 0.7 asks for a 30% reduction in edge energy, which is enough to
 * exclude "this band happens to be marginally calmer" without demanding an
 * empty band — most food photography has a plate somewhere in every third.
 *
 * Raising it makes the chooser reach for `photoTopStack`/`photoBottomStack`
 * more often; lowering it makes the chooser fall through to the dish-name
 * length rule. Pinned in both directions by `regions.test.ts`.
 */
const QUIET_RATIO = 0.7;

/**
 * Mean frame edge energy at or above which a photo counts as visually busy,
 * which routes the chooser to `cornerCard` — a family that puts type in a small
 * panel rather than across the photograph.
 *
 * 0.16 of the Sobel maximum. A single full-contrast step scores 0.707 in the
 * cells it crosses, so 0.16 across the WHOLE frame means roughly a quarter of
 * the frame is hard edge. Exported so the test brackets it from both sides
 * rather than restating the number.
 */
export const BUSY_EDGE = 0.16;

/**
 * Fraction of the peak cell energy at or above which a cell is part of the
 * subject. Relative, not absolute, because a flatly-lit dish and a
 * high-contrast one both have a subject — the question is which cells stand out
 * within THIS photograph.
 */
const SUBJECT_EDGE_FRACTION = 0.35;

function clamp01(value: number): number {
  if (!Number.isFinite(value)) return 0.5;
  return Math.max(0, Math.min(1, value));
}

/**
 * Mean cell value over `rect`, weighted by each cell's overlap AREA.
 *
 * Area weighting rather than "cells whose centre falls inside" because the band
 * rects above do not land on cell boundaries — `y: 0.66` cuts row 5 of 8 at
 * 72% — and centre-membership would silently round a band up or down by a whole
 * eighth of the frame. A rect with no overlap at all (degenerate, or entirely
 * outside) falls back to the grid mean rather than 0: zero would read as
 * "perfectly quiet" and win every comparison it appears in.
 */
export function meanOfCells(grid: CellGrid, rect: Rect01): number {
  const { size, values } = grid;
  if (size <= 0 || values.length === 0) return 0;

  const step = 1 / size;
  const x0 = Math.max(0, rect.x);
  const x1 = Math.min(1, rect.x + rect.w);
  const y0 = Math.max(0, rect.y);
  const y1 = Math.min(1, rect.y + rect.h);

  let weighted = 0;
  let weight = 0;
  for (let row = 0; row < size; row += 1) {
    const overlapH = Math.min((row + 1) * step, y1) - Math.max(row * step, y0);
    if (overlapH <= 0) continue;
    for (let col = 0; col < size; col += 1) {
      const overlapW = Math.min((col + 1) * step, x1) - Math.max(col * step, x0);
      if (overlapW <= 0) continue;
      const area = overlapW * overlapH;
      weighted += values[row * size + col] * area;
      weight += area;
    }
  }

  if (weight <= 0) {
    return values.reduce((sum, value) => sum + value, 0) / values.length;
  }
  return weighted / weight;
}

/**
 * Which band of the frame is quiet enough to set type on.
 *
 * A flat photograph returns "none", not "top". That reads oddly — a flat frame
 * is *all* negative space — but "none" here means "no band is distinctively
 * quieter than the others", which is exactly true, and it lets the chooser fall
 * through to its content rules instead of being pushed to a photo-geometry
 * family by a photograph with no geometry.
 */
export function negativeSpaceFrom(edges: CellGrid): NegativeSpace {
  const frame = meanOfCells(edges, FULL_FRAME);
  if (frame <= 0) return "none";

  const candidates: Array<{ band: NegativeSpace; energy: number }> = [
    { band: "top", energy: meanOfCells(edges, TOP_BAND) },
    { band: "bottom", energy: meanOfCells(edges, BOTTOM_BAND) },
    { band: "center", energy: meanOfCells(edges, CENTER_BAND) },
  ];

  // Strict `<` keeps source order as the tie-break, so a photo whose top and
  // bottom are equally quiet always resolves to the same band.
  let best = candidates[0];
  candidates.forEach((candidate) => {
    if (candidate.energy < best.energy) best = candidate;
  });

  return best.energy <= frame * QUIET_RATIO ? best.band : "none";
}

/** Whether the frame carries enough edge energy to crowd type laid over it. */
export function busyFrom(edges: CellGrid): boolean {
  return meanOfCells(edges, FULL_FRAME) >= BUSY_EDGE;
}

/**
 * The tightest CELL-ALIGNED box containing every cell at or above
 * `SUBJECT_EDGE_FRACTION` of the peak.
 *
 * Cell-aligned, so the box is coarse by construction — an eighth of the frame
 * in each direction. That is deliberate: its only consumer is the focal point,
 * and a crop focus does not want sub-cell precision derived from a 64×64
 * thumbnail. A frame with no peak at all (flat, or empty) reports the full
 * frame, which puts the focal point back at dead centre.
 */
export function subjectBoxFrom(edges: CellGrid): Rect01 {
  const { size, values } = edges;
  const peak = values.reduce((max, value) => (value > max ? value : max), 0);
  if (size <= 0 || peak <= 0) return { ...FULL_FRAME };

  const threshold = peak * SUBJECT_EDGE_FRACTION;
  let minRow = size;
  let maxRow = -1;
  let minCol = size;
  let maxCol = -1;
  for (let row = 0; row < size; row += 1) {
    for (let col = 0; col < size; col += 1) {
      if (values[row * size + col] < threshold) continue;
      if (row < minRow) minRow = row;
      if (row > maxRow) maxRow = row;
      if (col < minCol) minCol = col;
      if (col > maxCol) maxCol = col;
    }
  }
  if (maxRow < 0) return { ...FULL_FRAME };

  const step = 1 / size;
  return {
    x: minCol * step,
    y: minRow * step,
    w: (maxCol - minCol + 1) * step,
    h: (maxRow - minRow + 1) * step,
  };
}

/** The centre of a box, clamped into the frame — `fitCover`'s focal input. */
export function focalPointFrom(box: Rect01): { x: number; y: number } {
  return {
    x: clamp01(box.x + box.w / 2),
    y: clamp01(box.y + box.h / 2),
  };
}

/**
 * Map a rect in CANVAS coordinates onto the SOURCE photo, in source
 * coordinates, so a canvas band can be measured against the analysis grids.
 *
 * Three frames meet here and mixing them is the whole hazard:
 *   - `rect` and `imageArea` are 0..1 of the CANVAS
 *   - `source` is the cover-fit window in SOURCE PIXELS, straight out of
 *     `fitCover` — it already carries the operator's focal point and zoom
 *   - the result is 0..1 of the SOURCE, which is what `lumaGrid` describes
 *
 * Skipping this mapping and measuring the canvas rect against the grids
 * directly is correct only for an uncropped, unzoomed, full-bleed photo whose
 * aspect happens to match the canvas — i.e. almost never.
 *
 * Returns null when the rect misses the image area entirely (`splitPanel`'s
 * text band, which sits below the photo panel) or when the photo has no
 * dimensions. Null means "there are no pixels under this band", which is a
 * different answer from "the pixels under it average zero".
 */
export function mapCanvasRectToSource(
  rect: Rect01,
  imageArea: Rect01,
  source: { sx: number; sy: number; sw: number; sh: number },
  natural: { w: number; h: number },
): Rect01 | null {
  if (natural.w <= 0 || natural.h <= 0) return null;
  if (imageArea.w <= 0 || imageArea.h <= 0) return null;

  const left = Math.max(rect.x, imageArea.x);
  const right = Math.min(rect.x + rect.w, imageArea.x + imageArea.w);
  const top = Math.max(rect.y, imageArea.y);
  const bottom = Math.min(rect.y + rect.h, imageArea.y + imageArea.h);
  if (right <= left || bottom <= top) return null;

  // Fractions of the image area, which is the part of the canvas the cover-fit
  // window maps onto.
  const u0 = (left - imageArea.x) / imageArea.w;
  const u1 = (right - imageArea.x) / imageArea.w;
  const v0 = (top - imageArea.y) / imageArea.h;
  const v1 = (bottom - imageArea.y) / imageArea.h;

  return {
    x: (source.sx + u0 * source.sw) / natural.w,
    y: (source.sy + v0 * source.sh) / natural.h,
    w: ((u1 - u0) * source.sw) / natural.w,
    h: ((v1 - v0) * source.sh) / natural.h,
  };
}
