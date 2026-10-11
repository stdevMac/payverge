import type { MarketingCrop } from "@/api/marketing";
import type { MotifId } from "../artDirection/kits";
import type { NormalizedRect, SlotAlign, SlotFont } from "../templates/types";

/**
 * The Scene IR is deliberately renderer-agnostic: it is produced once by
 * `buildScene` and consumed by a canvas walker today, with motion (video) and
 * print walkers planned against the same node list. Nothing here may assume a
 * `CanvasRenderingContext2D`, and no constructor may supply a default — every
 * visual decision belongs to art direction upstream, not to this module.
 */

interface BaseNode {
  kind: string;
  /** Painting order, ascending. */
  z: number;
  rect: NormalizedRect;
  /**
   * Alpha multiplier, 0–1. Omitted means fully opaque.
   *
   * MULTIPLIED onto whatever alpha the walker already has, not assigned —
   * `renderScene` is the one place that applies it, inside a `save`/`restore`
   * pair so it cannot compound across nodes. It lives on `BaseNode` rather than
   * on the one node that used to declare it (`MotifNode`) because every fade in
   * the Wave 3 motion vocabulary — text reveal, badge pop, logo settle — needs
   * it, and a per-kind alpha field would be five ways to spell one thing.
   */
  opacity?: number;
  /**
   * Clip rect, in the same normalized space as `rect`. Omitted means no clip.
   *
   * Nodes are painted, not composited into layers, so a clip is the only way
   * to express a partial reveal — a text mask wipe, a scrim wipe — without a
   * second walker. The walker resolves it through the same `resolveRect` every
   * other rect goes through, installs it with `roundRect(..., 0)` + `ctx.clip()`
   * (NOT `ctx.rect()`, which neither test stub implements), and restores
   * immediately after the node, so a clip can never leak onto its neighbour.
   *
   * A zero-area clip paints nothing. That is the intended value at t=0 of a
   * reveal, not a degenerate case to guard against.
   */
  clip?: NormalizedRect;
}

export interface PhotoNode extends BaseNode {
  kind: "photo";
  url: string;
  crop?: MarketingCrop;
  /**
   * Color grade from the kit's grade preset, in CSS filter-function syntax
   * (e.g. `saturate(1.04) contrast(1.03)`). Deliberately not a canvas-specific
   * value: the same string is assignable to `ctx.filter`, to an SVG/CSS
   * `filter` property for print, and to a CSS filter for video. A walker that
   * cannot apply filters must drop it rather than approximate it.
   */
  filter?: string;
  /**
   * A colour to `multiply`-blend over the photo rect, `#rrggbb`, correcting a
   * colour cast. Separate from `filter` because the CSS filter set has no
   * per-channel gain — see `Grade` in `../photo/grade.ts`. A walker that cannot
   * multiply-blend must drop it rather than approximate it with a hue rotation,
   * which is a different operation entirely.
   */
  whiteBalance?: string;
}

export interface ShapeNode extends BaseNode {
  kind: "shape";
  fill: string;
  /**
   * Corner radius as a fraction of canvas WIDTH, matching `TextNode.sizePct`
   * and the pill padding below — every scalar length in this IR is
   * width-relative so a node keeps its proportions across all three aspects.
   * Omitted means square corners.
   */
  radiusPct?: number;
}

export interface TextNode extends BaseNode {
  kind: "text";
  key: string;
  text: string;
  font: SlotFont;
  weight: 400 | 500 | 600 | 700;
  /** Font size as a fraction of canvas WIDTH, matching the existing renderer. */
  sizePct: number;
  align: SlotAlign;
  color: string;
  maxLines: number;
  lineHeight: number;
  uppercase?: boolean;
  /**
   * Letter-spacing as a fraction of the resolved font size (not of canvas
   * width, and not in px), the same unit as `TypePairing.tracking` in
   * `../artDirection/kits.ts` that produces it. Omitted means the face's
   * natural spacing.
   */
  tracking?: number;
  /**
   * Rounded plate painted behind the text. Both `padX` and `padY` are
   * fractions of canvas WIDTH — `padY` does NOT scale with height. That is
   * intentional (it keeps the plate's optical padding identical across 1:1,
   * 4:5 and 9:16) and it is what the existing renderer does; see the
   * `padX * width` / `padY * width` pair in `../templates/renderPost.ts`.
   */
  pill?: { fill: string; padX: number; padY: number };
}

export interface MotifNode extends BaseNode {
  kind: "motif";
  motif: MotifId;
  color: string;
}

/**
 * A logo's frame: an origin and a DIAMETER. Deliberately not a
 * `NormalizedRect`.
 *
 * The mark is painted as a disc — `drawLogo` in `../templates/renderPost.ts`
 * takes `{ x, y, size }`, rounds `size * canvasWidth`, and paints a circle of
 * that diameter — so there is no second dimension for a walker to honour.
 * `LogoNode` used to carry a `NormalizedRect` anyway, with an `h` `buildScene`
 * computed from the aspect ratio and every consumer discarded.
 *
 * A field the walker ignores is worse than waste in this IR specifically: the
 * node list is the contract the planned motion and print walkers are being
 * written against, and an `h` on the node reads to those authors as a height
 * they are expected to respect. `w` is the whole geometry, on both axes, which
 * is also what keeps a square mark square across 1:1, 4:5 and 9:16 without an
 * aspect correction anywhere.
 */
interface LogoRect {
  x: number;
  y: number;
  /** Diameter, as a fraction of canvas WIDTH — matching `LogoAnchor.size`. */
  w: number;
}

export interface LogoNode extends Omit<BaseNode, "rect"> {
  kind: "logo";
  rect: LogoRect;
  url: string;
}

export interface ScrimNode extends BaseNode {
  kind: "scrim";
  from: string;
  to: string;
  /**
   * When true the walker must measure what it has already painted underneath
   * `rect` and strengthen or skip the scrim accordingly — on canvas that means
   * a raster readback (`ctx.getImageData`). A non-raster walker (print, video)
   * cannot do that, so it must either resolve `adaptive` to a fixed scrim
   * before walking or ignore the flag and paint `from`→`to` as authored.
   * When `boost` is present, adaptive has already been resolved upstream and
   * the walker must not read the canvas back.
   */
  adaptive: boolean;
  /**
   * Cutoff for the `adaptive` measurement, in 0–255 perceptual luma from the
   * `0.299 R + 0.587 G + 0.114 B` weighting used by `sampleRegionLuminance` in
   * `../templates/renderPost.ts`. Underlying luma at or below this value is
   * dark enough that the scrim is left as authored.
   *
   * This is emphatically NOT the WCAG `relativeLuminance` (0–1, sRGB-linear,
   * `0.2126/0.7152/0.0722`) exported from `../artDirection/color.ts`. This
   * feature calls two different quantities "luminance"; they are not
   * interchangeable and neither is a rescaling of the other.
   */
  luminanceThreshold: number;
  /**
   * A second gradient, decided upstream from photo analysis, painted over the
   * authored one. Its presence means `adaptive` has already been resolved: the
   * walker must NOT read the canvas back, and a non-raster walker (print,
   * motion) gets a correct adaptive scrim for the first time. Absent with
   * `adaptive: true` still means "measure it yourself".
   */
  boost?: { from: string; to: string };
}

export type SceneNode =
  | PhotoNode
  | ShapeNode
  | TextNode
  | MotifNode
  | LogoNode
  | ScrimNode;

/** Discriminant union, so a walker can type a `Record<SceneNodeKind, Painter>`. */
export type SceneNodeKind = SceneNode["kind"];

export interface Scene {
  width: number;
  height: number;
  /** Painted before any node; the brand surface for no-photo compositions. */
  background: string;
  /**
   * Already sorted ascending by `z` — the assembler sorts once (via
   * `sortNodes`) and walkers paint in array order without re-sorting. Ties keep
   * authoring order. Anyone constructing a `Scene` by hand owes that ordering.
   */
  nodes: SceneNode[];
}

export function photoNode(node: Omit<PhotoNode, "kind">): PhotoNode {
  return { kind: "photo", ...node };
}

export function shapeNode(node: Omit<ShapeNode, "kind">): ShapeNode {
  return { kind: "shape", ...node };
}

/**
 * Every field is required of the caller by design. `buildScene` resolves
 * `color` through `pickReadableColor` (WCAG AA) and `font`/`weight` through the
 * kit's `typePairing`; a constructor default would silently override art
 * direction, and — because `exactOptionalPropertyTypes` is off — an explicit
 * `undefined` would defeat it anyway and paint nothing.
 */
export function textNode(node: Omit<TextNode, "kind">): TextNode {
  return { kind: "text", ...node };
}

export function motifNode(node: Omit<MotifNode, "kind">): MotifNode {
  return { kind: "motif", ...node };
}

export function logoNode(node: Omit<LogoNode, "kind">): LogoNode {
  return { kind: "logo", ...node };
}

export function scrimNode(node: Omit<ScrimNode, "kind">): ScrimNode {
  return { kind: "scrim", ...node };
}

/**
 * Ascending by z. Stable (ES2019+), so authoring order breaks ties. Never
 * mutates `nodes`.
 *
 * Constrained to `{ z: number }` rather than to `BaseNode` because `z` is all
 * it reads, and because `LogoNode` no longer satisfies `BaseNode` — its rect is
 * a `LogoRect`. Asking for the whole base node here would make the sort refuse
 * a node whose geometry it never looks at.
 */
export function sortNodes<T extends { z: number }>(nodes: readonly T[]): T[] {
  return [...nodes].sort((a, b) => a.z - b.z);
}
