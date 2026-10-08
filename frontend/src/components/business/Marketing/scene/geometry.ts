import type { NormalizedRect } from "../templates/types";

/**
 * The one place the scene pipeline turns normalized coordinates into device
 * pixels.
 *
 * This module is a LEAF on purpose, for the same reason
 * `../artDirection/typeface.ts` is one. The canvas walker resolves photo, shape,
 * text and logo boxes, and the motif drawers resolve their anchor rect; all of
 * them must agree that `x`/`w` go against WIDTH and `y`/`h` against HEIGHT,
 * because a transposed axis is a plain argument swap with nothing to catch it.
 * They agree by both calling this — not by both happening to spell the same two
 * multiplications — so a change here reaches every call site in one commit or
 * reaches none.
 *
 * That is why nothing here may import from `./motifs`, `./renderScene` or
 * `./buildScene`. `resolveRect` used to live in `./motifs`, which made the
 * walker depend on the decorative-motif module for its core coordinate maths and
 * sent anyone tracing logo placement into a file about flourishes. Importing
 * upward from here would rebuild exactly that tangle. `../templates/types` (the
 * home of `NormalizedRect`, itself importless) is the only dependency this file
 * is allowed.
 *
 * Pure arithmetic: no `CanvasRenderingContext2D`, so the planned print and
 * motion walkers resolve rects through the same function the canvas walker does.
 */

/** A `NormalizedRect` resolved to device pixels. */
export interface Px {
  x: number;
  y: number;
  w: number;
  h: number;
}

/**
 * The one normalized→pixel conversion in the scene pipeline. `x`/`w` resolve
 * against canvas WIDTH and `y`/`h` against canvas HEIGHT — never the other way
 * round, and never both against the same axis. Every walker site goes through
 * here so the axis pairing is stated once instead of being re-derived (and
 * possibly transposed) at each call site.
 */
export function resolveRect(
  rect: NormalizedRect,
  canvasW: number,
  canvasH: number,
): Px {
  return {
    x: rect.x * canvasW,
    y: rect.y * canvasH,
    w: rect.w * canvasW,
    h: rect.h * canvasH,
  };
}
