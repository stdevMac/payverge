import type { MotifId } from "../artDirection/kits";
import type { FormatDef, FormatId } from "../formats/formats";
import type {
  LogoAnchor,
  MarketingAspectRatio,
  NormalizedRect,
  TemplateStyle,
} from "../templates/types";
import type { Band } from "./solver";

export type CompositionId =
  | "photoBottomStack"
  | "photoTopStack"
  | "splitPanel"
  | "badgeHero"
  | "cornerCard"
  | "posterStack"
  | "legacyEditorial"
  | "legacyBold"
  | "legacyMinimal";

/**
 * A band without the three fields a composition cannot know: the runtime
 * `text`, and the `font`/`weight` that come from the art-direction kit a
 * composition is paired with rather than from the layout. `buildScene` fills
 * all three in — and fills the type fields in once, before solving, so the
 * solver measures the face the walker will paint.
 */
type BandTemplate = Omit<Band, "text" | "font" | "weight">;

export interface CompositionDef {
  id: CompositionId;
  labelKey: string;
  /** False means the composition is designed to work with no photo at all. */
  requiresPhoto: boolean;
  /**
   * Can this family lay itself out on this canvas at all?
   *
   * Separate from `requiresPhoto` and orthogonal to it. The six choosable
   * families compute their geometry from `format.safe` and so accept anything in
   * the registry; the three legacy families reproduce a pre-rewrite template
   * that only ever had three layouts, and there is no honest answer for a fourth.
   * `chooseComposition` and `usableCompositions` both filter on this, so a
   * legacy id can never be selected for a Wave 4 format.
   */
  supportsFormat: (format: FormatDef) => boolean;
  /** Where the photo sits. Full bleed unless the composition splits the canvas. */
  imageAreaFor: (format: FormatDef) => NormalizedRect;
  /** Where the text stack sits. Always inside `format.safe`. */
  boundsFor: (format: FormatDef) => NormalizedRect;
  /** Ordered most-important-first; the solver clips from the end. */
  bandsFor: (format: FormatDef) => BandTemplate[];
  anchor: "top" | "bottom";
  motifs: MotifId[];
  /**
   * Where the mark sits.
   *
   * A function of the format for the same reason its three siblings are: a flat
   * anchor cannot survive canvases of different proportions. `LogoAnchor.size` is
   * WIDTH-relative, so the same `size` is the same fraction of the width
   * everywhere while `y` means a different pixel offset on every canvas — which
   * is how a flat anchor put every family's mark outside the platform-safe area
   * on 9:16.
   */
  logoAnchorFor: (format: FormatDef) => LogoAnchor;
}

const FULL_BLEED: NormalizedRect = { x: 0, y: 0, w: 1, h: 1 };

/**
 * Every number this file declares is a fraction of canvas WIDTH — `sizePct` and
 * `gapPct` alike (see `Band` in solver.ts). All three aspects are 1080px wide
 * (`ASPECT_DIMS`), so one declaration is one number of device pixels on every
 * canvas, and the 1:1 post is the canvas the numbers were tuned on.
 *
 * `gapPct` was height-relative until the unit was corrected, which is why none
 * of the values below moved when it changed: they already meant what they say
 * on the square, and the fix is what makes 4:5 and 9:16 agree with it instead
 * of spending 1.25x and 1.78x the ink on the same declaration.
 * `compositions.test.ts` pins the resulting device pixels for every family.
 */
const band = (
  key: string,
  sizePct: number,
  // `key` and `sizePct` are positional for a reason — an override blob that
  // could replace them would let a caller silently contradict the call it
  // just wrote.
  over: Omit<Partial<BandTemplate>, "key" | "sizePct"> = {},
): BandTemplate => ({
  key,
  sizePct,
  maxLines: 1,
  lineHeight: 1.15,
  gapPct: 0.018,
  align: "left",
  ...over,
});

/** Clamp a rect inside the format's own platform-safe area. */
function safeBounds(format: FormatDef, rect: NormalizedRect): NormalizedRect {
  const safe = format.safe;
  const x = Math.max(safe.x, rect.x);
  const y = Math.max(safe.y, rect.y);
  const right = Math.min(safe.x + safe.w, rect.x + rect.w);
  const bottom = Math.min(safe.y + safe.h, rect.y + rect.h);
  return { x, y, w: Math.max(0, right - x), h: Math.max(0, bottom - y) };
}

/**
 * The legacy aspect a legacy family's tables are keyed by.
 *
 * `supportsFormat` already refuses any format whose `legacyAspect` is null, so
 * the fallback is unreachable through the pipeline. It exists because a table
 * lookup must be total — an `undefined` rect propagates as `NaN` coordinates and
 * paints nothing, which is a far worse failure than laying out as 4:5.
 */
function legacyKeyFor(format: FormatDef): MarketingAspectRatio {
  return format.legacyAspect ?? "4:5";
}

/**
 * What the three legacy* families can and cannot reproduce.
 *
 * They exist for one reason: a `creative_snapshot` saved before the scene
 * rewrite must keep rendering the way it did. `compositions.test.ts` holds all
 * three to `templates.ts` — none of the numbers below may be tuned by eye.
 *
 * REPRODUCIBLE, and asserted:
 *  - the slot set and its order (`bandsFor` keys vs the template's slots)
 *  - every type field: `sizePct`, `align`, `maxLines`, `lineHeight`
 *  - `imageAreaFor`, against the template's own `imageArea`
 *  - the canvas edge the type is pinned to: a bottom-anchored family's last
 *    band ends where the template's lowest slot ended, a top-anchored family's
 *    first band starts where the template's highest slot started. One entry
 *    deviates by 0.005 and says why — see LEGACY_EDITORIAL_BOUNDS.
 *  - that the free (unanchored) edge leaves room for the template's type at
 *    full size, so the solver never shrinks what the template declared
 *
 * NOT reproducible, by construction:
 *  - the absolute y of each individual slot. EDITORIAL and BOLD are two-cluster
 *    layouts — a badge in one corner, the rest of the type in the opposite one
 *    — and the solver packs ONE contiguous stack. Whichever cluster the anchor
 *    pins lands correctly; the other floats to meet it.
 *  - inter-slot spacing. The templates spaced slots by absolute rects; the
 *    solver spaces bands by intrinsic height plus `gapPct`. There is no slot
 *    field `gapPct` could be derived from, so every legacy band keeps `band()`'s
 *    default and the stacks read denser than the originals, most visibly on
 *    9:16 where the template spread five slots over 0.66 of a 1920px canvas.
 *    Correcting `gapPct` to width-relative tightened that further — 34.6px per
 *    gap to 19.4px on 9:16, 24.3px to 19.4px on 4:5 — and left 1:1 untouched,
 *    which is the aspect these families' fidelity is actually asserted on.
 *    Widening the gap back would be the wrong repair: the anchored edge, the
 *    type sizes and the alignments are the fidelity these families can offer,
 *    and all three are pinned below and unaffected by spacing.
 *  - EDITORIAL's price/cta row. The template sat them side by side (price left
 *    at x 0.07, cta right at x 0.5); a solved stack can only stack, so they
 *    become two rows. The cta keeps the template's `align: "right"`, which is
 *    what still puts its glyphs against the right edge of the column.
 */

/**
 * Content envelope of the pre-rewrite "minimal" template's dishName -> price
 * / handle row, read directly off `minimalSlots` in templates.ts (top edge =
 * dishName's top, bottom edge = max of dishName/price/handle bottoms per
 * aspect). legacyMinimal's boundsFor must vary per aspect — the flat rect this
 * replaced, `{ x: 0.08, y: 0.76, w: 0.84, h: 0.16 }`, ran to 0.92 while the
 * 9:16 safe area (PLATFORM_CONTENT_BOUNDS) stops at 0.82, so `safeBounds`
 * clamped it to a degenerate 0.06 height there. The old template positioned
 * this stack much higher up on the tall aspect (dishName at y 0.59, not 0.75).
 */
const LEGACY_MINIMAL_BOUNDS: Record<MarketingAspectRatio, NormalizedRect> = {
  "1:1": { x: 0.08, y: 0.75, w: 0.84, h: 0.18 },
  "4:5": { x: 0.08, y: 0.72, w: 0.84, h: 0.19 },
  "9:16": { x: 0.08, y: 0.59, w: 0.84, h: 0.21 },
};

/**
 * Photo area of the pre-rewrite "minimal" template, read off `MINIMAL.layouts`
 * in templates.ts. Same per-aspect reasoning as LEGACY_MINIMAL_BOUNDS above:
 * the flat `{ x: 0.04, y: 0.04, w: 0.92, h: 0.7 }` this replaced was only ever
 * close on the square, and on 9:16 it started 154px too high and ended 269px
 * too low, leaving 288px of photo under the text stack.
 *
 * The template itself overlapped its own text, slightly and deliberately: the
 * photo ends at 0.60 on 9:16 while dishName starts at 0.59, and at 0.76 on 1:1
 * while dishName starts at 0.75. The table below reproduces that — 19.2px of
 * overlap at 9:16, 10.8px at 1:1, none at 4:5, pinned as expected values by
 * `compositions.test.ts`. It is not a defect to be tidied away: MINIMAL's
 * `lightBottom` scrim starts at 0.54 / 0.66, well above either overlap, and
 * `buildScene` lays its own scrim from 0.08 above `boundsFor`, so the band that
 * overlaps the photo is painted over a settled cream field either way.
 */
const LEGACY_MINIMAL_IMAGE_AREA: Record<MarketingAspectRatio, NormalizedRect> = {
  "1:1": { x: 0.04, y: 0.04, w: 0.92, h: 0.72 },
  "4:5": { x: 0.04, y: 0.04, w: 0.92, h: 0.68 },
  "9:16": { x: 0.04, y: 0.12, w: 0.92, h: 0.48 },
};

/**
 * Text envelope of the pre-rewrite "editorial" template, from `editorialSlots`
 * in templates.ts. Bottom-anchored, so the BOTTOM edge is the load-bearing one:
 * it is the template's lowest slot bottom (handle, at 0.915/0.89/0.785 plus its
 * 0.025 height) bar one declared 0.005 exception at 4:5, and
 * `compositions.test.ts` pins the solved stack against it. The flat
 * `{ x: 0.07, y: 0.5, w: 0.86, h: 0.44 }` this replaced — copied verbatim from
 * `photoBottomStack` — ran to 0.94 on all three aspects, which is right only on
 * the square; `safeBounds` then clamped it to 0.93 on 4:5 and 0.82 on 9:16,
 * sitting the type 20px and 19px below where the template had it.
 *
 * x/w are the template's text column (0.07 wide 0.86) verbatim on every aspect.
 *
 * The top edge is a budget, not a position, so it is derived from the
 * template's scrim rather than from its type: `buildScene` lays its gradient
 * from 0.08 of canvas height above these bounds, so y 0.51/0.50/0.50
 * reproduces EDITORIAL's own scrim tops of 0.43/0.42/0.42. A deeper envelope
 * would darken photograph the template left alone. It is also well above the
 * ~0.57/0.61/0.57 at which the five bands would start to crowd the envelope at
 * template sizes with a two-line dishName, so the solver never has to shrink.
 *
 * ONE DELIBERATE DEVIATION, at 4:5: the bottom edge is 0.92, not the template's
 * 0.915 text bottom. The same 0.08 bleed runs BELOW the envelope, and
 * EDITORIAL's 4:5 scrim is declared to reach the bottom of the canvas
 * (`darkBottom({ x: 0, y: 0.42, w: 1, h: 0.58 })`). An envelope ending at 0.915
 * puts the gradient's last stop at 0.995 and leaves a 6.75px strip of
 * unscrimmed photograph along the bottom edge — a visible hairline under
 * light-on-dark type. 0.92 is the shallowest edge that closes it, at the cost
 * of sitting the stack 6.75px lower than the template did.
 * `templates/renderPost.test.ts` pins that scrim band to the pixel; this table
 * is the other half of that contract.
 */
const LEGACY_EDITORIAL_BOUNDS: Record<MarketingAspectRatio, NormalizedRect> = {
  "1:1": { x: 0.07, y: 0.51, w: 0.86, h: 0.43 },
  "4:5": { x: 0.07, y: 0.5, w: 0.86, h: 0.42 },
  "9:16": { x: 0.07, y: 0.5, w: 0.86, h: 0.31 },
};

/**
 * Text envelope of the pre-rewrite "bold" template, from `boldSlots` in
 * templates.ts. Top-anchored, so the TOP edge is the load-bearing one: it is
 * the template's highest slot top (badge, at 0.07/0.08/0.15).
 *
 * BOLD is the family the old flat rect got most wrong. It was
 * `{ x: 0.08, y: 0.46, w: 0.84, h: 0.46 }` with `anchor: "bottom"` — copied
 * from `badgeHero` — while the template spreads its slots from 0.07 down to
 * 0.945, opening on a pill badge and a full-width uppercase headline in the
 * TOP third. Bottom-anchoring that stack buried the two elements BOLD is built
 * around in the lower half of the canvas.
 *
 * The bottom edge is the template's lowest slot bottom (handle at
 * 0.92/0.895/0.79 plus 0.025). On 1:1 that is 0.945, past the 0.94 safe floor,
 * so `safeBounds` trims it to 0.94; since the stack is top-anchored the trim
 * costs budget only, never position. The resulting envelope is far deeper than
 * the bands need, which is the point — nothing here may force a shrink.
 *
 * x/w are 0.06 wide 0.88, the column of BOLD's widest slots. Every BOLD slot is
 * centred on x 0.5 (badge 0.22+0.56, cta 0.2+0.6, the rest 0.06+0.88) and every
 * band is `align: "center"`, so one column reproduces all five horizontally.
 */
const LEGACY_BOLD_BOUNDS: Record<MarketingAspectRatio, NormalizedRect> = {
  "1:1": { x: 0.06, y: 0.07, w: 0.88, h: 0.875 },
  "4:5": { x: 0.06, y: 0.08, w: 0.88, h: 0.84 },
  "9:16": { x: 0.06, y: 0.15, w: 0.88, h: 0.665 },
};

/**
 * Five bands copied slot for slot from `editorialSlots` in templates.ts.
 *
 * `standardBands(1)` stood here and was wrong in three of the five: it billed
 * the price at 0.042 against the template's 0.048 (-12.5%), the handle at 0.022
 * against 0.021, and — the one that moves glyphs rather than resizing them —
 * left the cta `align: "left"` where every EDITORIAL layout sets `align:
 * "right"`. It also gave dishName three lines at line-height 1.06 instead of
 * the template's two at 1.05, and put every band on `band()`'s 1.15 line-height
 * default where the template's supporting slots use 1.1.
 */
function legacyEditorialBands(): BandTemplate[] {
  return [
    band("badge", 0.026, { lineHeight: 1.1 }),
    band("dishName", 0.075, { maxLines: 2, lineHeight: 1.05 }),
    band("price", 0.048, { lineHeight: 1.1 }),
    band("cta", 0.029, { lineHeight: 1.1, align: "right" }),
    band("handle", 0.021, { lineHeight: 1.1 }),
  ];
}

/**
 * Five bands copied slot for slot from `boldSlots` in templates.ts.
 *
 * `standardBands(1.1)` stood here — a generic set scaled by a hand-picked 1.1 —
 * and got all five alignments wrong on top of four of the five sizes: BOLD is
 * a centred poster, every one of its slots is `align: "center"`, and
 * `standardBands` leaves `band()`'s "left" default in place. So every `bold`
 * post an operator had already saved rendered ragged-left. The sizes were off
 * by +24% (badge 0.0286 vs 0.023), -16% (price 0.0462 vs 0.055), +21% (handle
 * 0.0242 vs 0.02) and +2.7% (cta), with the headline within 0.6%.
 */
function legacyBoldBands(): BandTemplate[] {
  return [
    band("badge", 0.023, { lineHeight: 1.1, align: "center" }),
    band("dishName", 0.082, { maxLines: 3, lineHeight: 1.02, align: "center" }),
    band("price", 0.055, { lineHeight: 1.1, align: "center" }),
    band("cta", 0.03, { lineHeight: 1.1, align: "center" }),
    band("handle", 0.02, { lineHeight: 1.1, align: "center" }),
  ];
}

/**
 * Three bands, not the five of `standardBands`, because the pre-rewrite
 * "minimal" template painted exactly three slots — see `minimalSlots` in
 * templates.ts, which defines dishName, price and handle and nothing else.
 * Sizes, line heights, max lines and alignments are copied from those slots so
 * the family reproduces the type it is supposed to reproduce; the handle keeps
 * the template's right alignment, since the old layout sat it at the right edge
 * of the price's row and a solved stack can only stack.
 *
 * Inheriting `standardBands` here was wrong twice over. It packed five bands
 * into LEGACY_MINIMAL_BOUNDS, an envelope measured from those three slots, so
 * the solver bottomed out at MIN_SCALE on every 1:1 post (fit margin 1.008x)
 * and painted a 11.8px handle where the template painted 23.8px. And it drew
 * elements the template never drew: `defaultSlots` in postContent.ts fills
 * slots by *play*, not by template, so a stored `win_back` snapshot — whose
 * default template is `minimal` — carries badge and cta text that the old
 * renderer silently dropped and this one would have shown.
 */
function legacyMinimalBands(): BandTemplate[] {
  return [
    band("dishName", 0.058, { maxLines: 2, lineHeight: 1.05 }),
    band("price", 0.034, { lineHeight: 1.1 }),
    band("handle", 0.022, { lineHeight: 1.1, align: "right" }),
  ];
}

/**
 * Which corner of the safe area a family's mark takes, how big it is, and how
 * far it clears the edge.
 *
 * `insetPct` is WIDTH-relative on both axes, and that is the point. The disc
 * itself is width-relative (`LogoAnchor.size`), and `drawLogo` paints its ring at
 * `size / 2 + round(width * 0.008)` — also width-relative. A height-relative
 * inset would therefore be a different number of pixels of clearance on every
 * canvas, which is how a flat table came to need a hand-tuned entry per aspect.
 * Measured against each format's own safe area, every choosable family's
 * horizontal inset below is exactly what the pre-Wave-4 table used at all three
 * aspects — see `compositions.test.ts`, which asserts it.
 */
interface LogoPlacement {
  corner:
    | "topLeft"
    | "topRight"
    | "topCenter"
    | "bottomLeft"
    | "bottomRight"
    | "bottomCenter";
  /** Disc width, as a fraction of canvas WIDTH. */
  sizePct: number;
  /** Clearance from the safe edge, as a fraction of canvas WIDTH on BOTH axes. */
  insetPct: number;
}

/**
 * The disc never eats more than this fraction of the canvas HEIGHT.
 *
 * A width-relative size is proportionate on a square and absurd on a 4:1 strip:
 * `cornerCard`'s 0.12 of a 1200px width is 144px on a 300px-tall canvas, nearly
 * half of it. The cap converts to a width-relative ceiling per format.
 *
 * It cannot change any pre-Wave-4 render. At 1:1 the ceiling is 0.22, at 4:5
 * 0.275 and at 9:16 0.391 — all above every declared `sizePct` — and those three
 * formats take the override path regardless.
 */
const MAX_LOGO_HEIGHT_FRACTION = 0.22;

const LOGO_PLACEMENTS: Record<CompositionId, LogoPlacement> = {
  // Text sits from y 0.5 down, so the mark takes the empty top-left corner.
  photoBottomStack: { corner: "topLeft", sizePct: 0.11, insetPct: 0.01 },
  // Mirror image: the stack owns the top, so the mark drops to bottom-right.
  photoTopStack: { corner: "bottomRight", sizePct: 0.1, insetPct: 0.02 },
  // Inside the photo panel (y 0..0.58), clear of the text panel below it.
  splitPanel: { corner: "topRight", sizePct: 0.08, insetPct: 0.04 },
  // The badge hero's stack starts at 0.46; the mark holds the top-left.
  badgeHero: { corner: "topLeft", sizePct: 0.11, insetPct: 0.02 },
  // The card occupies the lower left, so the mark balances it top-right.
  cornerCard: { corner: "topRight", sizePct: 0.12, insetPct: 0.04 },
  // Centred, matching the poster's centred type.
  posterStack: { corner: "bottomCenter", sizePct: 0.08, insetPct: 0.01 },
  // Legacy families never reach the derived path (`supportsFormat` refuses every
  // non-legacy format and all three legacy formats are overridden below), but a
  // placement is still required of them. These describe where their overrides
  // already put the mark.
  legacyEditorial: { corner: "topRight", sizePct: 0.1, insetPct: 0.01 },
  legacyBold: { corner: "bottomRight", sizePct: 0.1, insetPct: 0.02 },
  legacyMinimal: { corner: "topRight", sizePct: 0.1, insetPct: 0.02 },
};

/**
 * Every anchor the pre-Wave-4 `LOGO_ANCHORS` table held, verbatim.
 *
 * These are hand-tuned values from three canvases the design was drawn against,
 * and they win over the derivation wherever they exist — which is what makes the
 * format refactor pixel-identical on 1:1, 4:5 and 9:16. `compositions.test.ts`
 * pins all 27 and fails if one is dropped or edited.
 *
 * A Wave 4 format has no entry here and takes the derived anchor. That is the
 * whole cost of a new format at this call site: zero literals.
 */
const LOGO_OVERRIDES: Record<
  CompositionId,
  Partial<Record<FormatId, LogoAnchor>>
> = {
  photoBottomStack: {
    "1:1": { x: 0.07, y: 0.07, size: 0.11 },
    "4:5": { x: 0.07, y: 0.078, size: 0.11 },
    "9:16": { x: 0.07, y: 0.13, size: 0.11 },
  },
  photoTopStack: {
    "1:1": { x: 0.82, y: 0.83, size: 0.1 },
    "4:5": { x: 0.82, y: 0.84, size: 0.1 },
    "9:16": { x: 0.82, y: 0.75, size: 0.1 },
  },
  splitPanel: {
    "1:1": { x: 0.82, y: 0.07, size: 0.08 },
    "4:5": { x: 0.82, y: 0.078, size: 0.08 },
    "9:16": { x: 0.82, y: 0.13, size: 0.08 },
  },
  badgeHero: {
    "1:1": { x: 0.08, y: 0.08, size: 0.11 },
    "4:5": { x: 0.08, y: 0.08, size: 0.11 },
    "9:16": { x: 0.08, y: 0.13, size: 0.11 },
  },
  cornerCard: {
    "1:1": { x: 0.78, y: 0.08, size: 0.12 },
    "4:5": { x: 0.78, y: 0.08, size: 0.12 },
    "9:16": { x: 0.78, y: 0.13, size: 0.12 },
  },
  /*
   * Sized 0.08 rather than the 0.12 a flat anchor asked for: the lane the
   * composition leaves below its envelope is 0.84 -> 0.94 on 1:1, i.e. 108px,
   * and a 0.12 mark needs 148px with its ring. On 9:16 there is no lane at all,
   * so the mark sits at 0.72, inside the envelope but well below the type.
   */
  posterStack: {
    "1:1": { x: 0.46, y: 0.85, size: 0.08 },
    "4:5": { x: 0.46, y: 0.85, size: 0.08 },
    "9:16": { x: 0.46, y: 0.72, size: 0.08 },
  },
  // EDITORIAL in templates.ts: {0.83,0.065}, {0.83,0.075}, {0.84,0.145}. The
  // first two y values put the ring 3.6px and 2.3px above the safe top.
  legacyEditorial: {
    "1:1": { x: 0.83, y: 0.07, size: 0.1 },
    "4:5": { x: 0.83, y: 0.078, size: 0.1 },
    "9:16": { x: 0.84, y: 0.145, size: 0.09 },
  },
  // BOLD in templates.ts: {0.82,0.84}, {0.82,0.075}, {0.84,0.72} — the corner
  // change per-format geometry exists for. On 1:1, y 0.84 with size 0.1 puts the
  // disc's own bottom exactly on the 0.94 safe floor, so the whole 9px ring
  // hangs past it; 0.075 puts the ring 2.3px above the safe top on 4:5.
  legacyBold: {
    "1:1": { x: 0.82, y: 0.83, size: 0.1 },
    "4:5": { x: 0.82, y: 0.078, size: 0.1 },
    "9:16": { x: 0.84, y: 0.72, size: 0.09 },
  },
  // MINIMAL in templates.ts: {0.82,0.065}, {0.82,0.065}, {0.84,0.145}. The 1:1
  // entry repeats EDITORIAL's 3.6px top overrun; the 4:5 one does not.
  legacyMinimal: {
    "1:1": { x: 0.82, y: 0.07, size: 0.1 },
    "4:5": { x: 0.82, y: 0.078, size: 0.1 },
    "9:16": { x: 0.84, y: 0.145, size: 0.09 },
  },
};

/** The anchor a placement resolves to on a given canvas. Pure geometry. */
function deriveLogoAnchor(id: CompositionId, format: FormatDef): LogoAnchor {
  const placement = LOGO_PLACEMENTS[id];
  const safe = format.safe;
  // Normalized height units are shorter than normalized width units by exactly
  // this ratio, so it is what converts a width-relative length onto the y axis.
  const vScale = format.px.h > 0 ? format.px.w / format.px.h : 1;
  const size = Math.min(
    placement.sizePct,
    MAX_LOGO_HEIGHT_FRACTION / (vScale || 1),
  );
  const inset = placement.insetPct;
  const left = safe.x + inset;
  const right = safe.x + safe.w - size - inset;
  const center = safe.x + (safe.w - size) / 2;
  const top = safe.y + inset * vScale;
  const bottom = safe.y + safe.h - (size + inset) * vScale;
  switch (placement.corner) {
    case "topLeft":
      return { x: left, y: top, size };
    case "topRight":
      return { x: right, y: top, size };
    case "topCenter":
      return { x: center, y: top, size };
    case "bottomLeft":
      return { x: left, y: bottom, size };
    case "bottomRight":
      return { x: right, y: bottom, size };
    case "bottomCenter":
      return { x: center, y: bottom, size };
  }
}

/** Hand-tuned override if one exists for this canvas, otherwise the derivation. */
function resolveLogoAnchor(id: CompositionId, format: FormatDef): LogoAnchor {
  return LOGO_OVERRIDES[id][format.id] ?? deriveLogoAnchor(id, format);
}

/**
 * Test seam. `resolveLogoAnchor` always prefers the override, so there is no way
 * to observe the derivation on the three formats that have one — and the claim
 * that the derivation reproduces their x is exactly what needs asserting.
 */
export function deriveLogoAnchorForTests(
  id: CompositionId,
  format: FormatDef,
): LogoAnchor {
  return deriveLogoAnchor(id, format);
}

/**
 * The panel `splitPanel` gives the photo: a fixed 58% of the canvas, text
 * below it, at every aspect.
 *
 * The last flat constant among the compositions, and deliberately so — unlike
 * the legacy families, whose geometry has to track three per-aspect template
 * layouts, this one IS the idea: the composition is named for the split and the
 * split does not move. It is also the only choosable family that carves the
 * photo out of the canvas rather than bleeding it, which is why the text
 * envelope below has to be checked against it (see `compositions.test.ts`) —
 * swelling this to 0.95 would swallow the text envelope whole.
 */
const SPLIT_PANEL_IMAGE_AREA: NormalizedRect = { x: 0, y: 0, w: 1, h: 0.58 };

/**
 * `splitPanel`'s text envelope: the whole lane between the photo panel above and
 * the platform-safe floor below, less a 0.01 gutter under the panel.
 *
 * One rule, and no per-format table, because only the floor moves and
 * `safeBounds` already clamps to it. The pre-Wave-4 table declared
 * `h 0.35 / 0.34 / 0.23` — which is exactly `safe.y + safe.h - 0.59` at
 * `0.94 / 0.93 / 0.82`. Declaring `h: 1` and letting the clamp do the work
 * reproduces all three effective envelopes identically (pinned in
 * compositions.test.ts) and costs a new format nothing.
 *
 * The 0.01 gutter is the grid the rest of this file is declared on, and 10.8px
 * (19.2px on 9:16) of clear space is enough to read the split as a split. Since
 * the stack is top-anchored, the gutter IS the position of the first band; the
 * floor below is only budget.
 */
const SPLIT_PANEL_BOUNDS: NormalizedRect = {
  x: 0.08,
  y: 0.59,
  w: 0.84,
  h: 1,
};

/**
 * `cornerCard`'s text envelope. Bottom-anchored, so the 0.92 bottom edge is the
 * position and the 0.54 top edge is only a budget — raising the top moves no
 * glyph, it just buys the solver room before it starts shrinking.
 *
 * That budget was 0.58, and it was too tight for the narrowest column any family
 * declares (w 0.6, against 0.84-0.88 elsewhere): a long dish name — 50 characters
 * is already enough — wraps to the full three lines there, and in the boldest kit
 * (`typePairing.scale` 1.15) the stack settled at a type scale that put the
 * handle at 16.0px, on the wrong side of the 16px floor. 0.54 keeps the card
 * where it is and lets the same copy settle a step larger.
 *
 * The pre-Wave-4 table kept 0.58 at 9:16, where nothing had been breached. That
 * distinction does not survive flattening and does not need to: every band's
 * height is `fontPx * lineHeight / canvasH` and `fontPx` is width-relative, so
 * the same stack is 0.5625x as tall in normalized units on a 1920px canvas as on
 * a 1080px one, and the extra budget is budget nothing uses. compositions.test.ts
 * asserts that by solving the same copy against both envelopes and comparing the
 * settled stack, not the declared rect.
 */
const CORNER_CARD_BOUNDS: NormalizedRect = {
  x: 0.08,
  y: 0.54,
  w: 0.6,
  h: 0.38,
};

/** The standard editorial band set: badge, dish name, price, CTA, handle. */
function standardBands(scale: number): BandTemplate[] {
  return [
    band("badge", 0.026 * scale),
    band("dishName", 0.075 * scale, { maxLines: 3, lineHeight: 1.06 }),
    band("price", 0.042 * scale),
    band("cta", 0.028 * scale),
    band("handle", 0.022 * scale),
  ];
}

export const COMPOSITIONS: Record<CompositionId, CompositionDef> = {
  photoBottomStack: {
    id: "photoBottomStack",
    labelKey: "compositions.photoBottomStack",
    requiresPhoto: true,
    supportsFormat: () => true,
    imageAreaFor: () => FULL_BLEED,
    boundsFor: (format) =>
      safeBounds(format, { x: 0.07, y: 0.5, w: 0.86, h: 0.44 }),
    bandsFor: () => standardBands(1),
    anchor: "bottom",
    motifs: ["rule"],
    logoAnchorFor: (format) => resolveLogoAnchor("photoBottomStack", format),
  },
  photoTopStack: {
    id: "photoTopStack",
    labelKey: "compositions.photoTopStack",
    requiresPhoto: true,
    supportsFormat: () => true,
    imageAreaFor: () => FULL_BLEED,
    boundsFor: (format) =>
      safeBounds(format, { x: 0.07, y: 0.08, w: 0.86, h: 0.4 }),
    bandsFor: () => standardBands(1),
    anchor: "top",
    motifs: ["rule"],
    logoAnchorFor: (format) => resolveLogoAnchor("photoTopStack", format),
  },
  splitPanel: {
    id: "splitPanel",
    labelKey: "compositions.splitPanel",
    requiresPhoto: true,
    supportsFormat: () => true,
    imageAreaFor: () => SPLIT_PANEL_IMAGE_AREA,
    boundsFor: (format) => safeBounds(format, SPLIT_PANEL_BOUNDS),
    bandsFor: () => standardBands(0.95),
    anchor: "top",
    motifs: ["rule", "cornerBrackets"],
    logoAnchorFor: (format) => resolveLogoAnchor("splitPanel", format),
  },
  badgeHero: {
    id: "badgeHero",
    labelKey: "compositions.badgeHero",
    requiresPhoto: true,
    supportsFormat: () => true,
    imageAreaFor: () => FULL_BLEED,
    boundsFor: (format) =>
      safeBounds(format, { x: 0.08, y: 0.46, w: 0.84, h: 0.46 }),
    bandsFor: () => [
      band("badge", 0.058, { gapPct: 0.03 }),
      band("dishName", 0.062, { maxLines: 2, lineHeight: 1.08 }),
      band("cta", 0.03),
      band("handle", 0.022),
    ],
    anchor: "bottom",
    motifs: ["rule", "seal"],
    logoAnchorFor: (format) => resolveLogoAnchor("badgeHero", format),
  },
  cornerCard: {
    id: "cornerCard",
    labelKey: "compositions.cornerCard",
    requiresPhoto: true,
    supportsFormat: () => true,
    imageAreaFor: () => FULL_BLEED,
    boundsFor: (format) => safeBounds(format, CORNER_CARD_BOUNDS),
    bandsFor: () => standardBands(0.9),
    anchor: "bottom",
    /*
     * `ticketNotch` is here because this is the family that reads as a card:
     * the narrowest column any composition declares (w 0.6) sitting in one
     * corner, which is the shape a stub is. The two discs bite the left and
     * right edges of that column at mid-height, so they land on the card's own
     * silhouette rather than floating in the photograph.
     *
     * It also fixes a family that stamped NOTHING under the ticket kit: that
     * kit carries `ticketNotch`, `rule` and `tape`, and this composition
     * allowed only `cornerBrackets`, so the intersection `buildScene` takes was
     * empty. `rule` is deliberately still not allowed — this stack has no
     * headline lane to underline, it has a card edge — so the notch is what
     * gives the kit its one mark here.
     *
     * Costs 8.64px of inward clamp on the left at every aspect (the disc's
     * radius is 30.24px and the column starts 21.6px inside the safe area);
     * `buildScene` applies it from `MOTIF_OUTSET`.
     */
    motifs: ["cornerBrackets", "ticketNotch"],
    logoAnchorFor: (format) => resolveLogoAnchor("cornerCard", format),
  },
  posterStack: {
    id: "posterStack",
    labelKey: "compositions.posterStack",
    requiresPhoto: false,
    supportsFormat: () => true,
    imageAreaFor: () => FULL_BLEED,
    boundsFor: (format) =>
      safeBounds(format, { x: 0.1, y: 0.16, w: 0.8, h: 0.68 }),
    bandsFor: () => [
      band("badge", 0.03, { align: "center", gapPct: 0.05 }),
      band("dishName", 0.115, { maxLines: 4, lineHeight: 1.02, align: "center" }),
      band("price", 0.05, { align: "center", gapPct: 0.04 }),
      band("cta", 0.03, { align: "center" }),
      band("handle", 0.022, { align: "center" }),
    ],
    anchor: "top",
    /*
     * `tape` is here and nowhere else because a strip of tape is what holds a
     * POSTER up, and because this is the only family with the headroom for it.
     * The strip paints entirely above the rect it is handed — 72.71px at
     * 1080px wide, the second largest outset in `MOTIF_OUTSET` — and this
     * envelope starts at y 0.16, which leaves 100.1px / 143.3px / 76.8px of
     * clearance above it at 1:1 / 4:5 / 9:16. So it needs no correction at any
     * aspect.
     *
     * `photoTopStack` was the obvious alternative and is the wrong one: its
     * envelope is already clamped onto the safe top by `safeBounds`, so a tape
     * strip there would be pushed a full 72.71px DOWN by `buildScene`'s
     * safe-area clamp and land on the first line of type. The clamp is a
     * guardrail against a few pixels of overshoot, not a placement strategy.
     */
    motifs: ["rule", "seal", "tape"],
    logoAnchorFor: (format) => resolveLogoAnchor("posterStack", format),
  },
  // EDITORIAL's layouts declare no `imageArea`, so `layout()` gave them the full
  // bleed, and no motif geometry, so `motifs` stays empty: a rule or a seal the
  // template never drew would be new decoration on an old post.
  legacyEditorial: {
    id: "legacyEditorial",
    labelKey: "compositions.legacyEditorial",
    requiresPhoto: false,
    supportsFormat: (format) => format.legacyAspect !== null,
    imageAreaFor: () => FULL_BLEED,
    boundsFor: (format) =>
      safeBounds(format, LEGACY_EDITORIAL_BOUNDS[legacyKeyFor(format)]),
    bandsFor: () => legacyEditorialBands(),
    anchor: "bottom",
    motifs: [],
    logoAnchorFor: (format) => resolveLogoAnchor("legacyEditorial", format),
  },
  // Full bleed and motif-free for the same two reasons as legacyEditorial.
  // `anchor: "top"` because BOLD opens on a badge at y 0.07 and a headline at
  // 0.16; see LEGACY_BOLD_BOUNDS for why bottom-anchoring was the wrong half.
  legacyBold: {
    id: "legacyBold",
    labelKey: "compositions.legacyBold",
    requiresPhoto: false,
    supportsFormat: (format) => format.legacyAspect !== null,
    imageAreaFor: () => FULL_BLEED,
    boundsFor: (format) =>
      safeBounds(format, LEGACY_BOLD_BOUNDS[legacyKeyFor(format)]),
    bandsFor: () => legacyBoldBands(),
    anchor: "top",
    motifs: [],
    logoAnchorFor: (format) => resolveLogoAnchor("legacyBold", format),
  },
  // The only legacy family that needs a photo: `imageAreaFor` carves a discrete
  // inset panel out of the canvas, and an empty inset panel is a hole, not a
  // design.
  legacyMinimal: {
    id: "legacyMinimal",
    labelKey: "compositions.legacyMinimal",
    requiresPhoto: true,
    supportsFormat: (format) => format.legacyAspect !== null,
    imageAreaFor: (format) => LEGACY_MINIMAL_IMAGE_AREA[legacyKeyFor(format)],
    boundsFor: (format) =>
      safeBounds(format, LEGACY_MINIMAL_BOUNDS[legacyKeyFor(format)]),
    bandsFor: () => legacyMinimalBands(),
    anchor: "top",
    motifs: [],
    logoAnchorFor: (format) => resolveLogoAnchor("legacyMinimal", format),
  },
};

export const COMPOSITION_ORDER: readonly CompositionId[] = [
  "photoBottomStack",
  "photoTopStack",
  "splitPanel",
  "badgeHero",
  "cornerCard",
  "posterStack",
  "legacyEditorial",
  "legacyBold",
  "legacyMinimal",
];

/** Compositions the chooser may select. Legacy families are never chosen. */
export const CHOOSABLE_COMPOSITIONS: readonly CompositionId[] = [
  "photoBottomStack",
  "photoTopStack",
  "splitPanel",
  "badgeHero",
  "cornerCard",
  "posterStack",
];

export const LEGACY_COMPOSITION_FOR_TEMPLATE: Record<
  TemplateStyle,
  CompositionId
> = {
  editorial: "legacyEditorial",
  bold: "legacyBold",
  minimal: "legacyMinimal",
};
