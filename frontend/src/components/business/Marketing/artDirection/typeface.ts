import type { SlotFont } from "../templates/types";

/**
 * The one place a CSS font string is assembled for the scene pipeline.
 *
 * This module is a LEAF on purpose. The composition solver measures text and
 * the canvas walker paints it; they must agree on the family, the weight and
 * the size to the character, because every width the solver computes is a
 * width in a specific font. They agree by both calling this — not by both
 * happening to spell the same thing — so a change here reaches the measurer and
 * the painter in the same commit or reaches neither.
 *
 * That is why nothing here may import from `../scene/` or `../composition/`:
 * the solver has to be able to depend on it without depending on the walker.
 */

/** DM Sans ships 400–700; DM Serif Display ships 400 only. See `cssFontString`. */
export type FontWeight = 400 | 500 | 600 | 700;

export interface TypeSpec {
  /** Which face of the kit's pairing this text takes. */
  font: SlotFont;
  weight: FontWeight;
  /** Size in device pixels, unrounded; `cssFontString` rounds it. */
  px: number;
}

/**
 * Build the CSS font string for a piece of type.
 *
 * Rounding lives here rather than at the call sites so the solver — which holds
 * an exact `fontPx` — and the walker — which holds `sizePct * canvasW`, the
 * same number after a divide and a multiply — cannot round to different
 * integers and produce two strings for one piece of text.
 */
/**
 * The family name alone, for callers that build their own CSS rather than a
 * shorthand (print walker font-family declarations).
 *
 * Same two strings `cssFontString` embeds — quoted, without fallbacks — so a
 * face the canvas walker paints and the print walker does not is a single-file
 * edit rather than two lists drifting apart.
 */
export function cssFontFamily(font: SlotFont): string {
  return font === "serif" ? '"DM Serif Display"' : '"DM Sans"';
}

export function cssFontString(spec: TypeSpec): string {
  const px = Math.round(spec.px);
  const serif = spec.font === "serif";
  const family = serif
    ? "'DM Serif Display', Georgia, serif"
    : "'DM Sans', sans-serif";
  // DM Serif Display ships a single weight; forcing 400 avoids synthetic bold.
  // `spec.weight` is therefore deliberately ignored for serif type — and it is
  // ignored HERE, in the shared builder, so the solver measures the same 400
  // the walker paints. A serif node still carries its authored weight so that a
  // future face with real weights needs no change upstream.
  const weight = serif ? 400 : spec.weight;
  return `${weight} ${px}px ${family}`;
}
