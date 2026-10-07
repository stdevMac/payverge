import type { NormalizedRect } from "../templates/types";
import type { FormatDef, PrintSpec } from "./formats";

/**
 * The pixel-to-physical bridge, and a LEAF module on purpose — the same reason
 * `scene/geometry.ts` is one.
 *
 * The canvas walker works in device pixels and the print walker works in
 * millimetres, and there is exactly one arithmetic relation between them. Stated
 * once here, it cannot be re-derived (and transposed, or fixed at the wrong dpi)
 * at each call site. Nothing in this file may import from `./printWalker`,
 * `../scene/` or `../composition/`; `./formats` and `../templates/types` are the
 * only dependencies allowed.
 *
 * Pure arithmetic. No DOM, no HTML, no strings that are markup.
 */

/** Millimetres to the inch. */
const MM_PER_INCH = 25.4;

/** PostScript points to the inch. */
const PT_PER_INCH = 72;

/**
 * Crop-mark stroke width. 0.25pt is the prepress hairline convention; expressed
 * in mm because every other length here is.
 */
export const HAIRLINE_MM = (0.25 / PT_PER_INCH) * MM_PER_INCH;

/** A rectangle in page-space millimetres, measured from the page's top-left. */
export interface MmRect {
  leftMm: number;
  topMm: number;
  wMm: number;
  hMm: number;
}

/** Device pixels at `dpi`, in millimetres. */
export function pxToMm(px: number, dpi: number): number {
  if (dpi <= 0) return 0;
  return (px * MM_PER_INCH) / dpi;
}

/**
 * Device pixels at `dpi`, in points.
 *
 * Type sizes are emitted in `pt` rather than `mm` because that is the unit print
 * CSS and every operator's mental model of type use; box geometry stays in `mm`.
 */
export function pxToPt(px: number, dpi: number): number {
  if (dpi <= 0) return 0;
  return (px * PT_PER_INCH) / dpi;
}

/**
 * How far the artwork sits from the page edge on every side: one mark length, so
 * the marks have somewhere to live outside the bleed.
 */
function gutterMm(spec: PrintSpec): number {
  return spec.markLengthMm;
}

/**
 * The paper the piece prints on: trim, plus bleed on every side, plus a mark
 * gutter on every side.
 *
 * Both dimensions are explicit numbers and neither is `auto`. That is not
 * incidental: `@page { size: 80mm auto }` is not a valid page-size declaration in
 * Chromium and silently falls back to A4 — a lesson from the 2026-07-17 bill
 * alert print stabilization, paid for once already by the thermal bill formatter.
 */
export function pageBoxMm(spec: PrintSpec): { wMm: number; hMm: number } {
  const outer = 2 * (spec.bleedMm + gutterMm(spec));
  return { wMm: spec.trimWMm + outer, hMm: spec.trimHMm + outer };
}

/** The trim box's top-left corner, in page-space millimetres. */
export function trimOriginMm(spec: PrintSpec): {
  leftMm: number;
  topMm: number;
} {
  const inset = spec.bleedMm + gutterMm(spec);
  return { leftMm: inset, topMm: inset };
}

/** The bleed box: what the scene's normalized 0..1 space actually covers. */
export function bleedBoxMm(spec: PrintSpec): MmRect {
  const gutter = gutterMm(spec);
  return {
    leftMm: gutter,
    topMm: gutter,
    wMm: spec.trimWMm + 2 * spec.bleedMm,
    hMm: spec.trimHMm + 2 * spec.bleedMm,
  };
}

/**
 * A scene rect, resolved onto the page.
 *
 * `x`/`w` go against the bleed box's WIDTH and `y`/`h` against its HEIGHT —
 * never the other way round, and never both against the same axis. This is the
 * print walker's `resolveRect`, and it exists for the same reason: a transposed
 * axis is a plain argument swap with nothing to catch it.
 */
export function rectToMm(rect: NormalizedRect, format: FormatDef): MmRect {
  const spec = format.print;
  if (!spec) {
    // A screen format has no page. Returning zeros rather than throwing keeps
    // the walker's node loop total; the walker itself refuses a non-print format
    // before it gets here.
    return { leftMm: 0, topMm: 0, wMm: 0, hMm: 0 };
  }
  const box = bleedBoxMm(spec);
  return {
    leftMm: box.leftMm + rect.x * box.wMm,
    topMm: box.topMm + rect.y * box.hMm,
    wMm: rect.w * box.wMm,
    hMm: rect.h * box.hMm,
  };
}

/**
 * The eight crop marks — two per corner — as filled hairline rectangles.
 *
 * Each mark sits ON the trim line it indicates and runs OUTWARD from the page
 * edge, stopping `bleedMm` short of the trim corner. That offset is the whole
 * convention: a mark that ran all the way to the corner would print on top of
 * the bleed, and a printer trimming to it would cut through ink it was meant to
 * align against.
 *
 * Rectangles rather than borders or an SVG so the walker can place them with the
 * same absolute-position machinery it uses for every other node, and so the
 * output stays assertable as a string.
 */
export function cropMarkRects(spec: PrintSpec): MmRect[] {
  const { leftMm: trimLeft, topMm: trimTop } = trimOriginMm(spec);
  const trimRight = trimLeft + spec.trimWMm;
  const trimBottom = trimTop + spec.trimHMm;
  const length = spec.markLengthMm;
  const page = pageBoxMm(spec);
  const t = HAIRLINE_MM;
  // Centre each hairline on its trim line so the mark reads as the cut, not as
  // a line beside it.
  const half = t / 2;

  const horizontal = (topMm: number, leftMm: number): MmRect => ({
    leftMm,
    topMm: topMm - half,
    wMm: length,
    hMm: t,
  });
  const vertical = (leftMm: number, topMm: number): MmRect => ({
    leftMm: leftMm - half,
    topMm,
    wMm: t,
    hMm: length,
  });

  return [
    // Top-left
    horizontal(trimTop, 0),
    vertical(trimLeft, 0),
    // Top-right
    horizontal(trimTop, page.wMm - length),
    vertical(trimRight, 0),
    // Bottom-left
    horizontal(trimBottom, 0),
    vertical(trimLeft, page.hMm - length),
    // Bottom-right
    horizontal(trimBottom, page.wMm - length),
    vertical(trimRight, page.hMm - length),
  ];
}
