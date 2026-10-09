import type { FormatDef } from "../formats/formats";
import type { CompositionDef } from "./compositions";

/**
 * Which band of the frame a composition's type occupies, and therefore which
 * band an image generator must leave visually quiet.
 *
 * `left` and `right` are in the union but no current family produces one: every
 * choosable composition spans most of the width and differs only in where it
 * sits vertically. They are here for Wave 4's wide and strip formats, where a
 * side-reserved band is the natural shape. A test pins that nothing returns
 * them today, so adding one is a deliberate act rather than a drift.
 */
export type ReservedBand =
  | "top"
  | "bottom"
  | "center"
  | "left"
  | "right"
  | "none";

/** Above this the stack reads as living in the top of the frame. */
const TOP_LIMIT = 0.4;
/** Below this it reads as living in the bottom. */
const BOTTOM_LIMIT = 0.6;

/**
 * Reserve nothing when the type never lands on the photograph.
 *
 * This is the same predicate `imageCoversText` applies in `scene/buildScene.ts`
 * — deliberately, so the module that decides "is the backdrop under this type
 * knowable?" and the module that decides "must the generator keep this band
 * quiet?" cannot disagree about whether the photo reaches the type. Touching
 * edges do not count: an image area ending exactly where the text begins shares
 * a zero-area boundary and no pixel.
 */
function typeSitsOnPhoto(
  composition: CompositionDef,
  format: FormatDef,
): boolean {
  const image = composition.imageAreaFor(format);
  const text = composition.boundsFor(format);
  const width =
    Math.min(image.x + image.w, text.x + text.w) - Math.max(image.x, text.x);
  const height =
    Math.min(image.y + image.h, text.y + text.h) - Math.max(image.y, text.y);
  return width > 0 && height > 0;
}

/**
 * The band of the frame this composition reserves for overlay copy, at this
 * format. Vertical only — see the note on `ReservedBand`.
 *
 * Decided from the text envelope's vertical CENTRE rather than its edges,
 * because every envelope is a budget with slack at the free end: `cornerCard`
 * declares 0.54 → 0.92 purely so the solver has room before it shrinks, and
 * reading its top edge would call a bottom-anchored card "centre".
 */
export function reservedBandFor(
  composition: CompositionDef,
  format: FormatDef,
): ReservedBand {
  if (!typeSitsOnPhoto(composition, format)) return "none";
  const bounds = composition.boundsFor(format);
  const centre = bounds.y + bounds.h / 2;
  if (centre <= TOP_LIMIT) return "top";
  if (centre >= BOTTOM_LIMIT) return "bottom";
  return "center";
}
