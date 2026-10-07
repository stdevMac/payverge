import { cssFontString, type FontWeight } from "../artDirection/typeface";
import type { NormalizedRect, SlotAlign, SlotFont } from "../templates/types";

export interface Band {
  key: string;
  text: string;
  /** Font size as a fraction of canvas WIDTH. */
  sizePct: number;
  maxLines: number;
  /** Line-height multiplier. */
  lineHeight: number;
  /**
   * Space below this band, as a fraction of canvas WIDTH — the same dimension
   * `sizePct` above is a fraction of, and deliberately so.
   *
   * This was height-relative, which made a composition's two spacing numbers
   * two different units while reading as one: a declared `0.018` was 19.4px of
   * ink on a 1:1 post, 24.3px on 4:5 and 34.6px on 9:16. Nothing said so at the
   * declaration site, so every tuning pass really tuned the square and let the
   * other two aspects drift — `splitPanel`'s four gaps spent 138px of a 365px
   * envelope on a story, which is the actual origin of the "text is illegible
   * at 9:16" reports that earlier passes answered by widening envelopes.
   *
   * Width also happens to be the dimension that makes the number MEAN
   * something: the gap and the type it separates now scale together, so a gap
   * declared as a fraction of a headline stays that fraction on every canvas.
   * All three aspects share a 1080px width (`ASPECT_DIMS`), so one declaration
   * is now one number of device pixels everywhere.
   */
  gapPct: number;
  /**
   * Horizontal alignment of the text within the solved rect. The solver never
   * reads it — every band spans the full width of the bounds — it is carried
   * through for whoever paints the text.
   */
  align: SlotAlign;
  /**
   * The face and weight this band will be PAINTED in, mirroring `TextNode`.
   *
   * The solver reads them for one reason: a width is only a width in a given
   * font. Measuring every band in one face at one weight — as this did — silently
   * under-measures a serif headline and an overweight one alike, so `wrapText`
   * fits too many characters per line and the text overruns the bounds this
   * solver exists to enforce. Whoever fills these in must be the same code that
   * fills in the painted node's, or the two drift apart again.
   */
  font: SlotFont;
  weight: FontWeight;
}

/**
 * Text width in device pixels for `text` set in `cssFont`. Injected so the
 * solver stays canvas-free.
 *
 * The font arrives as a finished CSS string, not as role/weight/size, so the
 * measurer's whole body is `ctx.font = cssFont; return
 * ctx.measureText(text).width`. There is deliberately no second place that
 * could assemble a font differently from the one the walker paints with.
 */
export type MeasureFn = (text: string, cssFont: string) => number;

export interface SolveInput {
  bands: Band[];
  bounds: NormalizedRect;
  canvasW: number;
  canvasH: number;
  /** Stack from the top of the bounds, or pack against the bottom. */
  anchor: "top" | "bottom";
  measure: MeasureFn;
}

export interface SolvedBand {
  rect: NormalizedRect;
  /** Font size in device pixels, already including the settled type scale. */
  fontPx: number;
}

/**
 * Legibility floor for the type scale: text never shrinks below 55% of the
 * size the composition asked for. Below roughly this, body copy on a 1080px
 * social post stops being readable on a phone, so overflow past this point is
 * absorbed by dropping bands instead. Raising it drops more bands; lowering it
 * keeps more bands at a size the viewer may not be able to read.
 */
const MIN_SCALE = 0.55;

/**
 * Type-scale decrement per attempt. The search is a linear descent rather than
 * a closed-form solve or a bisection because band height is not a continuous
 * function of scale — `measure` wraps text into whole lines, so height moves in
 * steps — and there is nothing to invert. At most nine iterations over a
 * handful of bands, run once per solve.
 *
 * Repeated subtraction accumulates float drift (the descent yields
 * 0.8999999999999999 rather than 0.9), which matters because the settled scale
 * is multiplied into the font sizes callers paint with. Compare settled sizes
 * with a tolerance, never for exact equality.
 */
const SCALE_STEP = 0.05;

/**
 * Filters blank bands and floors negative numerics once, so no downstream
 * helper can forget the floor. `sizePct`, `lineHeight`, and `gapPct` are all
 * meaningless below zero — a font cannot have negative size, a line cannot
 * have negative height, and a gap must not pull the next band upward — but
 * `Band` does not enforce that at the type level, so the solver cannot trust
 * callers. Everything below assumes normalized bands.
 */
function normalizeBands(bands: Band[]): Band[] {
  return bands
    .filter((b) => b.text.trim().length > 0)
    .map((b) => ({
      ...b,
      sizePct: Math.max(0, b.sizePct),
      lineHeight: Math.max(0, b.lineHeight),
      gapPct: Math.max(0, b.gapPct),
    }));
}

/**
 * Everything the geometry helpers need except the band list itself. Omitting
 * `bands` keeps the raw, un-normalized input out of scope inside the helpers,
 * so none of them can reach past the list it was handed.
 */
type StackCtx = Omit<SolveInput, "bands">;

function fontPxFor(band: Band, scale: number, ctx: StackCtx): number {
  return band.sizePct * scale * ctx.canvasW;
}

function lineCount(band: Band, scale: number, ctx: StackCtx): number {
  const fontPx = fontPxFor(band, scale, ctx);
  const maxWidthPx = ctx.bounds.w * ctx.canvasW;
  // A zero-size band arguably occupies no lines at all; returning 1 is only
  // harmless because a zero fontPx zeroes the band height regardless.
  if (fontPx <= 0 || maxWidthPx <= 0) return 1;
  // The band's OWN face and weight, at the size it will be painted: the walker
  // builds its `ctx.font` from the same builder and the same three values.
  const textPx = ctx.measure(
    band.text,
    cssFontString({ font: band.font, weight: band.weight, px: fontPx }),
  );
  const needed = Math.ceil(textPx / maxWidthPx);
  return Math.max(1, Math.min(band.maxLines, needed));
}

/** Normalized height of one band at a given type scale. */
function bandHeight(band: Band, scale: number, ctx: StackCtx): number {
  const fontPx = fontPxFor(band, scale, ctx);
  return (fontPx * band.lineHeight * lineCount(band, scale, ctx)) / ctx.canvasH;
}

/**
 * The gap below `band`, converted from its width-relative declaration into the
 * height-normalized units the stack is built in.
 *
 * The single place the two axes meet, which is the point: `gapPct * canvasW`
 * is the gap in device pixels and dividing by `canvasH` is the only step that
 * knows the stack is measured vertically. Both call sites — the height budget
 * and the cursor advance — must agree on that conversion, and inlining it at
 * each of them is how the units drifted apart in the first place.
 *
 * Undefended against a zero-height canvas, exactly like `bandHeight` above,
 * because a positive canvas is a precondition of the whole module rather than
 * of this one helper. Guarding here alone would imply a safety the solver does
 * not have.
 */
function gapBelow(band: Band, ctx: StackCtx): number {
  return (band.gapPct * ctx.canvasW) / ctx.canvasH;
}

function totalHeight(bands: Band[], scale: number, ctx: StackCtx): number {
  return bands.reduce(
    (sum, band, index) =>
      sum +
      bandHeight(band, scale, ctx) +
      (index < bands.length - 1 ? gapBelow(band, ctx) : 0),
    0,
  );
}

/** The largest type scale, at or above MIN_SCALE, at which the stack fits. */
function settleScale(bands: Band[], ctx: StackCtx): number {
  let scale = 1;
  while (scale > MIN_SCALE && totalHeight(bands, scale, ctx) > ctx.bounds.h) {
    scale = Math.max(MIN_SCALE, scale - SCALE_STEP);
  }
  return scale;
}

/**
 * Solve a vertical stack of intrinsically-sized bands inside `bounds`.
 *
 * Bands with blank text are dropped. When the stack is taller than the bounds,
 * the type scale is reduced (down to MIN_SCALE) rather than allowing overflow;
 * if it still does not fit at MIN_SCALE the stack is clipped from the end, so
 * the most important bands — which callers list first — survive.
 *
 * Each surviving band comes back with both its rect and the font size that
 * rect was measured at, so the caller has nothing left to recompute.
 */
export function solveStack(input: SolveInput): Record<string, SolvedBand> {
  const bands = normalizeBands(input.bands);
  if (!bands.length) return {};

  const scale = settleScale(bands, input);

  // Never clip below one band: a post rendered with no words at all is worse
  // than one whose single band overruns, and the height cap below keeps that
  // band's rect inside the bounds either way.
  let kept = bands;
  while (kept.length > 1 && totalHeight(kept, scale, input) > input.bounds.h) {
    kept = kept.slice(0, -1);
  }

  const stackHeight = totalHeight(kept, scale, input);
  let cursorY =
    input.anchor === "top"
      ? input.bounds.y
      : input.bounds.y + input.bounds.h - stackHeight;
  // A single band taller than the bounds would otherwise start above them.
  cursorY = Math.max(input.bounds.y, cursorY);

  const out: Record<string, SolvedBand> = {};
  kept.forEach((band, index) => {
    // Capping at the bounds makes the rect honest about the space the band is
    // allowed to occupy and dishonest about how far its glyphs actually reach.
    // `fontPx` below is uncapped, so a caller that cares can spot the
    // difference instead of trusting the rect to describe the text.
    const h = Math.min(bandHeight(band, scale, input), input.bounds.h);
    out[band.key] = {
      rect: { x: input.bounds.x, y: cursorY, w: input.bounds.w, h },
      fontPx: fontPxFor(band, scale, input),
    };
    cursorY += h + (index < kept.length - 1 ? gapBelow(band, input) : 0);
  });
  return out;
}
