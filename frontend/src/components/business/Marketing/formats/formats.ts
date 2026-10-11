import type { MarketingAspectRatio, NormalizedRect } from "../templates/types";

/**
 * What canvas a post is being laid out on.
 *
 * This replaces `AspectRatio` as the pipeline's unit of "which canvas". The
 * difference is not cosmetic. `AspectRatio` was a closed three-value union that
 * every piece of composition geometry keyed an exhaustive `Record` on, so adding
 * a fourth canvas was 20 compile errors and 20 hand-authored literals across
 * nine composition families. A `FormatDef` carries the four facts the geometry
 * actually needs — pixel dimensions, a platform-safe envelope, a medium, and a
 * physical trim size where one exists — so a composition can COMPUTE its layout
 * instead of looking it up per aspect.
 *
 * The three original ids keep their exact legacy spelling. They are the ids, not
 * aliases for them, which is what makes Wave 4 a zero-migration change: every
 * `creative_snapshot.aspect` already on disk is a valid `FormatId`.
 */
export type FormatId = "1:1" | "4:5" | "9:16" | "wide" | "strip" | "5:7";

/**
 * How a format leaves the browser. "screen" formats raster to PNG through the
 * canvas walker; "print" formats emit `@page` HTML through the print walker.
 * Nothing else in the pipeline branches on this — `buildScene` produces the same
 * `Scene` either way, which is the whole point of the IR.
 */
type FormatMedium = "screen" | "print";

/**
 * The physical facts a print format needs and a screen format has no meaning for.
 *
 * Every length is in millimetres and every one describes the PAPER, not the
 * artwork. The artwork's own coordinate space is the BLEED box (trim plus bleed
 * on every side), because that is the rectangle full-bleed art must actually
 * cover; `FormatDef.px` is that same bleed box rasterized at `dpi`.
 */
export interface PrintSpec {
  /** Finished size after cutting. */
  trimWMm: number;
  trimHMm: number;
  /** How far art extends past each trim edge, so a cut that drifts still lands on ink. */
  bleedMm: number;
  /** Length of each crop-mark line. */
  markLengthMm: number;
  /** Raster resolution the artwork is authored at. */
  dpi: number;
}

export interface FormatDef {
  id: FormatId;
  /** Operator-facing name: `formats.<id>.name` in marketingDashboard.json. */
  labelKey: string;
  /** Operator-facing use case: `formats.<id>.use`. */
  useKey: string;
  medium: FormatMedium;
  /**
   * Export dimensions in device pixels. For a print format this is the BLEED box
   * at `print.dpi`, not the trim box — see `PrintSpec`.
   *
   * Note that `w` is NOT 1080 for every format any more. Every scalar in the
   * scene IR is a fraction of canvas WIDTH, so a 1920-wide format renders type
   * 1.78x larger in pixels than a 1080-wide one — which is correct, because it
   * is also 1.78x more pixels wide. Nothing needs a per-format type scale.
   */
  px: { w: number; h: number };
  /**
   * The platform-safe content envelope, normalized against `px`.
   *
   * This is data, not derivation: 9:16's floor is at 0.82 because Instagram
   * paints its own UI over the bottom of a Story, and no ratio arithmetic can
   * know that. Each format brings its own — one literal per format, which is the
   * cost this whole registry exists to bound.
   */
  safe: NormalizedRect;
  /**
   * The legacy aspect string this format is, or null for a format that did not
   * exist before Wave 4.
   *
   * Read by exactly two kinds of caller: the three legacy composition families,
   * whose per-aspect tables are keyed by it and which refuse any format where it
   * is null; and `templates/templates.ts`, whose `TemplateDef.layouts` is still a
   * `Record<MarketingAspectRatio, TemplateLayout>`. It is deliberately NOT a
   * general-purpose downcast — anything else that reaches for it is asking a
   * format to pretend to be one of the original three.
   */
  legacyAspect: MarketingAspectRatio | null;
  print: PrintSpec | null;
}

/**
 * The three formats that predate Wave 4.
 *
 * `px` and `safe` are copied verbatim from `ASPECT_DIMS` and
 * `PLATFORM_CONTENT_BOUNDS` in ../templates/types.ts, and `formats.test.ts`
 * holds them to those tables so the two cannot drift while both exist.
 */
const LEGACY_FORMATS: Record<MarketingAspectRatio, FormatDef> = {
  "1:1": {
    id: "1:1",
    labelKey: "formats.1:1.name",
    useKey: "formats.1:1.use",
    medium: "screen",
    px: { w: 1080, h: 1080 },
    safe: { x: 0.06, y: 0.06, w: 0.88, h: 0.88 },
    legacyAspect: "1:1",
    print: null,
  },
  "4:5": {
    id: "4:5",
    labelKey: "formats.4:5.name",
    useKey: "formats.4:5.use",
    medium: "screen",
    px: { w: 1080, h: 1350 },
    safe: { x: 0.06, y: 0.07, w: 0.88, h: 0.86 },
    legacyAspect: "4:5",
    print: null,
  },
  "9:16": {
    id: "9:16",
    labelKey: "formats.9:16.name",
    useKey: "formats.9:16.use",
    medium: "screen",
    px: { w: 1080, h: 1920 },
    safe: { x: 0.06, y: 0.12, w: 0.88, h: 0.7 },
    legacyAspect: "9:16",
    print: null,
  },
};

/**
 * Storefront hero. 16:9 at 1920 wide, the size a website hero band is authored
 * at and the only format in the registry wider than it is tall.
 *
 * `safe` is a plain 0.06 horizontal inset (115px, matching the other formats'
 * optical margin) and 0.10 vertically (108px) — there is no platform chrome to
 * dodge here, unlike a Story, so the envelope is a design margin rather than a
 * UI allowance.
 *
 * Type is width-relative, so a 0.075 headline is 144px here against 81px on a
 * 1080-wide feed post. That is correct: the canvas is 1.78x wider, so the
 * headline occupies the same fraction of it.
 */
const WIDE_FORMAT: FormatDef = {
  id: "wide",
  labelKey: "formats.wide.name",
  useKey: "formats.wide.use",
  medium: "screen",
  px: { w: 1920, h: 1080 },
  safe: { x: 0.06, y: 0.1, w: 0.88, h: 0.8 },
  legacyAspect: null,
  print: null,
};

/**
 * Email and web footer strip. 4:1 at 1200 wide, which halves cleanly to the
 * 600px an email client renders a full-width block at.
 *
 * NOT a thermal receipt. The spec's "receipt and email footer" conflates two
 * machines: a thermal receipt is 58mm or 80mm of ESC/POS driven by
 * backend/internal/services/print/formatters/receipt.go, and no browser PNG
 * reaches it. Branding a receipt is a backend formatter change and is out of
 * scope for this wave.
 *
 * A 4:1 canvas is the one place in the registry where the solver has to work
 * hard: five bands at their declared sizes need roughly 2.3x the height this
 * envelope has, so the stack settles at MIN_SCALE and clips to its three most
 * important bands. That is the solver doing its job, and
 * `formats.strip.solve.test.ts` pins the result rather than leaving it to chance.
 */
const STRIP_FORMAT: FormatDef = {
  id: "strip",
  labelKey: "formats.strip.name",
  useKey: "formats.strip.use",
  medium: "screen",
  px: { w: 1200, h: 300 },
  safe: { x: 0.04, y: 0.1, w: 0.92, h: 0.8 },
  legacyAspect: null,
  print: null,
};

/**
 * Table tent, 5in x 7in, 300 dpi, with bleed and crop marks.
 *
 * Coordinate space: the scene's normalized 0..1 covers the BLEED box, not the
 * trim box, because full-bleed art has to run past the cut. `px` is therefore the
 * bleed box at 300 dpi — 133.35mm x 184.15mm, which is exactly 1575 x 2175 px —
 * and `printGeometry.ts` converts back with `px * 25.4 / dpi`.
 *
 * `safe` is 0.08 / 0.06 of the bleed box, which puts the content envelope
 * 7.5mm inside the trim on the sides and 4.4mm inside it top and bottom: a real
 * print margin, not a platform-UI allowance.
 *
 * bleed = 3.175mm = 1/8in, the standard commercial bleed. markLength = 5mm, and
 * the marks are offset outward from the trim by exactly the bleed so they never
 * sit on live art.
 */
const TABLE_TENT_FORMAT: FormatDef = {
  id: "5:7",
  labelKey: "formats.5:7.name",
  useKey: "formats.5:7.use",
  medium: "print",
  px: { w: 1575, h: 2175 },
  safe: { x: 0.08, y: 0.06, w: 0.84, h: 0.88 },
  legacyAspect: null,
  print: {
    trimWMm: 127,
    trimHMm: 177.8,
    bleedMm: 3.175,
    markLengthMm: 5,
    dpi: 300,
  },
};

/**
 * Closed registry of every campaign-kit format. Six entries: three legacy
 * screen canvases, two Wave 4 screen canvases, and one print tent.
 */
export const FORMATS: Record<FormatId, FormatDef> = {
  ...LEGACY_FORMATS,
  wide: WIDE_FORMAT,
  strip: STRIP_FORMAT,
  "5:7": TABLE_TENT_FORMAT,
};

/**
 * Display and export order. `4:5` leads because it is the default hero and the
 * one PostCard renders; the rest follow in descending expected use.
 */
export const FORMAT_ORDER: readonly FormatId[] = [
  "4:5",
  "9:16",
  "1:1",
  "wide",
  "strip",
  "5:7",
];

export const SCREEN_FORMATS: readonly FormatId[] = FORMAT_ORDER.filter(
  (id) => FORMATS[id].medium === "screen",
);

export const PRINT_FORMATS: readonly FormatId[] = FORMAT_ORDER.filter(
  (id) => FORMATS[id].medium === "print",
);

/**
 * Narrow a value of unknown provenance — a `creative_snapshot.aspect` off the
 * wire, a URL parameter — to a `FormatId` guaranteed to have a definition.
 * Exact match only: no trimming, no case folding, because the backend whitelist
 * in marketing_activity_handlers.go stores the canonical id and anything else is
 * a value this build cannot render.
 *
 * Use this instead of `as FormatId`. The cast turns a server-side typo into an
 * undefined registry lookup and a zero-sized canvas; the guard turns it into a
 * fall back to the default format.
 */
export function isFormatId(value: unknown): value is FormatId {
  return (
    typeof value === "string" && (FORMAT_ORDER as readonly string[]).includes(value)
  );
}

/**
 * The format for one of the three original aspect strings.
 *
 * Total by construction — `MarketingAspectRatio` has exactly the three members
 * `LEGACY_FORMATS` defines — so this needs no fallback and callers need no
 * null check.
 */
export function formatForLegacyAspect(aspect: MarketingAspectRatio): FormatDef {
  return LEGACY_FORMATS[aspect];
}

/** The format a post gets when nothing else has been chosen. */
export const DEFAULT_FORMAT_ID: FormatId = "4:5";
