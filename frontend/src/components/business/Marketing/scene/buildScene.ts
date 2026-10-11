import type { MarketingCrop } from "@/api/marketing";
import {
  compositeOver,
  conditionSurface,
  pickReadableColor,
  type SurfaceOverlay,
} from "../artDirection/color";
import type { CreativeKit, MotifId, TextureId } from "../artDirection/kits";
import type { ResolvedPalette } from "../artDirection/palette";
import type { FontWeight } from "../artDirection/typeface";
import type { CompositionDef } from "../composition/compositions";
import { solveStack, type Band, type MeasureFn } from "../composition/solver";
import type { FormatDef } from "../formats/formats";
import { gradeFor } from "../photo/grade";
import type { PhotoAnalysis } from "../photo/types";
import type { NormalizedRect, SlotFont } from "../templates/types";
import { MOTIF_OUTSET } from "./motifs";
import { adaptiveScrimBoost } from "./scrim";
import {
  logoNode,
  motifNode,
  photoNode,
  scrimNode,
  sortNodes,
  textNode,
  type Scene,
  type SceneNode,
} from "./types";

/**
 * Assemble a creative concept into a flat, z-ordered `Scene`.
 *
 * Pure and canvas-free by construction: text width arrives through the injected
 * `measure`, never from a `CanvasRenderingContext2D`. That is what lets the
 * planned motion and print walkers consume the same output as the canvas
 * walker, and what lets this module be tested under Jest, which maps `canvas`
 * to a mock.
 *
 * This is also where WCAG 2.1 AA is enforced rather than eyeballed: every text
 * foreground is chosen by `pickReadableColor` at a 4.5:1 floor against the
 * bitmap it will sit on — surface plus texture, not surface alone. Wherever the
 * photo does not reach the type that floor is guaranteed, not hoped for: the
 * surface is conditioned until the composite is out of the luminance band where
 * no foreground token can reach it. See `imageCoversText` for which path a
 * composition takes and `resolveForegrounds` for what, over a photo, is only
 * assumed.
 */

/*
 * The five values below are painted, not classed. A canvas fill takes a CSS
 * color string, so `bg-warm-50` never reaches the bitmap and the repo's
 * inline-hex ban has nothing to offer here.
 *
 * Four of them are literal shades of the warm-neutral scale that
 * `tailwind.config.ts` declares the single source of truth for text and
 * surfaces, so a shade change there has to be mirrored here — that is the cost
 * of painting instead of classing, and the reason each constant is named for
 * its token rather than inlined. The suppression is re-enabled immediately
 * after the block so a stray hex anywhere else in this file is still caught.
 */
/* eslint-disable no-restricted-syntax -- warm-scale design tokens as canvas paint values; see the comment above */

/** warm-50: the cream field a light-surface kit's scrim lays down. */
const SURFACE_CREAM = "#faf9f6";
/** ink-900: the field a dark kit's scrim lays down, and the dark foreground. */
const INK_900 = "#1c1917";
/**
 * The light foreground. Deliberately pure white and not warm-50: type over a
 * photograph wants no tint of its own, and the extra contrast is free.
 */
const WHITE = "#ffffff";
/** warm-200: the muted foreground on a dark field. */
const WARM_200 = "#e4e0d8";
/** warm-700: the muted foreground on a light field. */
const WARM_700 = "#403b34";

/* eslint-enable no-restricted-syntax */

export interface BuildSceneInput {
  kit: CreativeKit;
  composition: CompositionDef;
  /**
   * The canvas this scene is for. `canvasW`/`canvasH` stay separate because a
   * thumbnail solves at its own scaled size, not at `format.px`.
   */
  format: FormatDef;
  canvasW: number;
  canvasH: number;
  photoUrl: string;
  logoUrl: string;
  crop?: MarketingCrop;
  slots: Partial<Record<string, string>>;
  palette: ResolvedPalette;
  measure: MeasureFn;
  /**
   * The photo's analysis, or null when there is no photo or it could not be
   * read back. Null keeps every photo-derived decision at its Wave 1 value, so
   * an unanalysable photo renders exactly as it did before this wave.
   */
  analysis?: PhotoAnalysis | null;
  /**
   * Mean perceptual luma (0–255) of the SOURCE pixels under the scrim band,
   * measured by the caller through `mapCanvasRectToSource` because only the
   * caller knows the cover-fit window. Null (or absent) leaves the scrim
   * adaptive and the walker reads the canvas back as it did in Wave 1.
   */
  scrimLuma?: number | null;
}

/**
 * Painting order, ascending. The gaps are deliberate: a later wave can slot a
 * layer between two of these without renumbering the ones already shipped.
 *
 * This is NOT the order nodes are pushed in. `texture` is emitted with the
 * motifs, after the text, because that is where the code that emits motifs
 * lives; it paints underneath the text all the same. `sortNodes` at the end is
 * what reconciles the two, and it is load-bearing for exactly this reason.
 */
const Z = {
  photo: 0,
  scrim: 10,
  texture: 15,
  text: 30,
  motif: 40,
  logo: 50,
} as const;

/** The headline slot: it alone takes the kit's display face and heaviest weight. */
const DISPLAY_SLOT = "dishName";

/** The headline's weight. Ignored for a serif face — see `cssFontString`. */
const DISPLAY_WEIGHT: FontWeight = 700;

/** Every other band: medium, so supporting copy reads as supporting copy. */
const BODY_WEIGHT: FontWeight = 500;

/**
 * The face and the weight a band takes. The ONLY place either is decided, and
 * the kit is the ONLY input to the decision.
 *
 * THE BRAND `design_settings.font_family` IS DELIBERATELY NOT AN INPUT HERE.
 * That is a decision, not an omission, so here is the reasoning in full.
 *
 * The pre-rewrite renderer had a `slotFontString` whose rule was: a slot that
 * asked for serif renders serif only if the brand `fontFamily` is absent or is
 * literally "serif". Every caller supplies one and defaults it to `"Inter"` —
 * `usePostComposer`, `PostCard`, `MarketingLibrary` and `ActivityDetailDrawer`
 * all spell `?? "Inter"` — and `"Inter"` is not `"serif"`. So the rule did not
 * express a brand preference at all: it was a global serif kill-switch that
 * only a business explicitly on `font_family: "Serif"` could turn off.
 *
 * Reinstating it would therefore make the serif display face of `editorial`,
 * `chalkboard` and `linen` unreachable for every business on the default, which
 * on the measured population is every business there is. It would also break
 * the kit path outright: an operator who picks the kit labelled Editorial in
 * the drawer, sees a serif specimen, and gets DM Sans because a settings field
 * they never touched still says "Inter" is a bug on its face.
 *
 * The setting is NOT inert and is NOT being removed. `design_settings.
 * font_family` still drives the public business page's typography
 * (`ConvertingBusinessLandingPage`, `PublicMenuDisplay`, `BusinessAboutTab`,
 * `designClasses.ts`), is still edited in `DesignCustomization`, is still
 * validated against {Inter, Sans, Serif} by `business_design_validation.go`,
 * and still steers AI image generation through `derivedMarketingVisualMood` in
 * `marketing_handlers.go`. What it is no longer is an input to THIS pipeline:
 * since Wave 1 the typeface is art direction's decision, carried by the kit,
 * which on the legacy path is itself derived from the stored template by
 * `kitForLegacyTemplate`.
 *
 * KNOWN DELTA, recorded rather than papered over. On the legacy path a stored
 * `template: "editorial"` snapshot painted a DM Sans headline before the
 * rewrite (serif slot, `font_family: "Inter"`, kill-switch fires) and paints DM
 * Serif Display now. `legacyBold` is unaffected (no serif slot) and
 * `legacyMinimal` is unaffected on the default (its kit's display face is sans,
 * which is what the kill-switch produced). See the legacy-face test in
 * buildScene.test.ts.
 *
 * The guard against this being quietly undone is structural rather than
 * conventional: `BuildSceneInput.palette` is a `ResolvedPalette`, which carries
 * no `fontFamily` at all, so honouring one would require widening the input
 * type first. buildScene.test.ts pins both halves — the type has no such key,
 * and the face of every band across every kit x family x aspect is exactly the
 * kit's.
 */
function bandTypeFor(
  kit: CreativeKit,
  key: string,
): { font: SlotFont; weight: FontWeight } {
  const isDisplay = key === DISPLAY_SLOT;
  return {
    font: isDisplay ? kit.typePairing.display : kit.typePairing.body,
    weight: isDisplay ? DISPLAY_WEIGHT : BODY_WEIGHT,
  };
}

/** Read as secondary information, so they take the muted foreground. */
const MUTED_SLOTS = new Set<string>(["handle", "cta"]);

/** Rendered inside a filled pill using the accent color. */
const PILL_SLOTS = new Set<string>(["badge"]);

/**
 * The motif that paints each kit texture, or null where none can.
 *
 * A texture is not a motif: `TextureId` and `MotifId` are separate unions, and
 * a texture is a full-bleed surface treatment the kit owns rather than an
 * accent a composition places. They meet here because `MotifId` happens to name
 * two of the textures and `MOTIF_DRAWERS` already knows how to paint them, so
 * the IR needs no new node kind to carry one.
 *
 * `paper` has no drawer and no MotifId to name it, so the linen kit's texture
 * stays unrepresented until one lands — better a documented gap than a paper
 * grain faked with the grain field.
 *
 * This is what makes `Z.texture` reachable at all. It was previously spelled
 * `BEHIND_MOTIFS`, a `Set<MotifId>` of "grain" and "halftone" applied to the
 * kit-and-composition motif intersection — where it could never match, because
 * no composition lists either one and no kit carries either in its `motifSet`.
 * A whole z-layer was unreachable and the sort that ordered it untestable.
 */
const TEXTURE_MOTIF: Record<TextureId, MotifId | null> = {
  none: null,
  grain: "grain",
  halftone: "halftone",
  paper: null,
};

/**
 * Alpha for the texture field. Low by necessity: it is painted in the accent
 * color over the whole canvas, and anything approaching the motif opacity would
 * be a wash rather than a texture.
 */
const TEXTURE_OPACITY = 0.12;

/** Textures cover the canvas; they are a surface, not a mark on the surface. */
const FULL_BLEED: NormalizedRect = { x: 0, y: 0, w: 1, h: 1 };

/** Slots set in all caps, where the kit's tracking earns its keep. */
const UPPERCASE_SLOTS = new Set<string>(["badge", "cta"]);

/** Pill padding, both fractions of canvas WIDTH — see `TextNode.pill`. */
const PILL_PAD_X = 0.022;
const PILL_PAD_Y = 0.012;

/**
 * How far the scrim reaches past the text bounds, as a fraction of canvas
 * HEIGHT, so the gradient has run its course before the first glyph.
 */
const SCRIM_BLEED = 0.08;

/**
 * The gradient band behind the text stack.
 *
 * The top and the bottom are clamped as a pair, not independently: clamping `y`
 * to 0 and `h` to 1 in isolation lets `y + h` land past 1 for any low-sitting
 * stack (bounds of y 0.5 / h 0.44 give y 0.42 and h 0.60, a bottom edge at
 * 1.02), painting a gradient off the bottom of the canvas.
 */
function scrimRect(bounds: NormalizedRect): NormalizedRect {
  const y = Math.max(0, bounds.y - SCRIM_BLEED);
  const bottom = Math.min(1, bounds.y + bounds.h + SCRIM_BLEED);
  return { x: 0, y, w: 1, h: Math.max(0, bottom - y) };
}

/**
 * The band a composition's scrim covers, for this format.
 *
 * Exported because `renderPost` has to measure the photo under exactly this
 * rect and cannot re-derive it: `scrimRect` folds in `SCRIM_BLEED` and clamps
 * the top and bottom as a pair. A caller measuring `boundsFor(format)` instead
 * would sample a band 8% of the canvas shorter than the one being painted.
 */
export function scrimRectFor(
  composition: CompositionDef,
  format: FormatDef,
): NormalizedRect {
  return scrimRect(composition.boundsFor(format));
}

/**
 * The rect to hand a motif drawer so its INK — not its rect — lands inside the
 * platform-safe area.
 *
 * Four of the seven drawers paint outside the rect they are given, by design: a
 * rule floats `0.02W` above it, a seal `2.4` radii above it, a notch straddles
 * the left and right edges. `MOTIF_OUTSET` in ./motifs.ts is the per-motif,
 * per-side table of how far, and this is its consumer. Handing the drawers the
 * raw text bounds put the rule 23.76px above the safe top on every 9:16
 * `photoTopStack` render, the seal 13.92px above it on `posterStack`, and the
 * bracket stroke 1.62px below the safe floor wherever a family's envelope ends
 * on it — 28 (kit, family, aspect) combinations in all.
 *
 * CLAMP EACH EDGE INWARD, rather than translating the whole rect or insetting
 * it unconditionally:
 *
 *  - a TRANSLATION preserves the mark's size but moves the edge that fits as
 *    well as the edge that does not, and it cannot resolve a breach on two
 *    opposite sides at once. `cornerBrackets` breaches all four sides by half a
 *    stroke, so there is no translation that fixes it.
 *  - an UNCONDITIONAL INSET taxes every motif on every family, including the
 *    ones with room to spare, and would move `photoBottomStack`'s rule 23.76px
 *    down its stack to fix a breach that does not exist there.
 *  - clamping per edge is a no-op wherever the ink already fits, and where it
 *    does not it moves the offending edge by exactly the overshoot, landing the
 *    ink flush with the safe boundary. It is also the ONLY one of the three
 *    that is correct for both a positioned accent and a field: `rule` keeps its
 *    length (the untouched opposite edge still sets `px.w`) and `halftone`
 *    keeps its origin.
 *
 * One rule for all seven is therefore the right shape, but it is not free: a
 * clamped rule sits flush with the safe edge instead of `0.02W` above the
 * stack, which is why `posterStack` is where `tape` lives and `photoTopStack`
 * is not (see the note on those `motifs` arrays). A composition with no
 * headroom should not be given a motif that needs some — the clamp keeps the
 * ink legal, it does not make the placement good.
 *
 * NOT applied to the kit texture. That is a full-bleed surface treatment, and
 * the safe area governs content, not the surface content sits on; insetting a
 * grain field would leave four bare margins.
 *
 * AXES. Every `MotifOutset` value is device pixels derived from canvas WIDTH,
 * but a rect is normalized against BOTH dimensions — x/w on width, y/h on
 * height — so the horizontal pair is divided by `canvasW` and the vertical pair
 * by `canvasH`. Dividing all four by the same dimension silently mis-corrects
 * the vertical edges by the aspect ratio, which on 9:16 is 1.78x.
 */
function safeMotifRect(
  motif: MotifId,
  rect: NormalizedRect,
  format: FormatDef,
  canvasW: number,
  canvasH: number,
): NormalizedRect {
  // A zero dimension is a state this module models rather than rejects (see the
  // `canvasW > 0 ?` guards below and MIN_PITCH_PX in ./motifs.ts). There is no
  // safe area to clamp to on a zero canvas, and dividing by it would return a
  // rect of NaNs.
  if (canvasW <= 0 || canvasH <= 0) return rect;

  const safe = format.safe;
  const outset = MOTIF_OUTSET[motif](canvasW);
  const left = Math.max(rect.x, safe.x + outset.left / canvasW);
  const right = Math.min(
    rect.x + rect.w,
    safe.x + safe.w - outset.right / canvasW,
  );
  const top = Math.max(rect.y, safe.y + outset.top / canvasH);
  const bottom = Math.min(
    rect.y + rect.h,
    safe.y + safe.h - outset.bottom / canvasH,
  );

  // Degenerate only if a motif's outsets exceed the safe area itself, which
  // would already be outside the preconditions `MOTIF_OUTSET` documents (its
  // values assume a rect large enough to contain the mark). Floored rather than
  // inverted so such a motif paints nothing instead of painting backwards.
  return {
    x: left,
    y: top,
    w: Math.max(0, right - left),
    h: Math.max(0, bottom - top),
  };
}

/**
 * Do the photo's pixels actually land under the text stack?
 *
 * This, and not `photoUrl !== ""`, is what decides whether the backdrop under
 * the type is knowable, AND — since the scrim was scoped to match — whether one
 * is painted at all. `splitPanel` is the counter-example the url test gets
 * wrong: it requires a photo, but its image area stops at 0.58 and its stack
 * starts below that, so not one glyph sits over a photograph. Scoping by url
 * left it picking a foreground against an assumed ink-900 scrim field, which
 * for a light brand primary is white type on a near-white surface.
 *
 * A KNIFE EDGE, deliberately left alone. `legacyMinimal` at 4:5 ends its photo
 * panel exactly where its stack begins — 0.04 + 0.68 against 0.72 — which by
 * the paragraph below is a miss. In IEEE-754 it is 0.7200000000000001, so the
 * overlap is 1.1e-16 and the family stays on the photo path. That is the render
 * worth keeping (its cream scrim over a cream surface paints nothing but the
 * wash across the bottom of the panel, which the pre-rewrite MINIMAL template
 * painted too), so no epsilon is applied here. `buildScene.test.ts` pins it so
 * the accident cannot become a silent change.
 *
 * Any intersection at all counts as covered. That is deliberately conservative:
 * a stack that straddles the edge of a photo panel has genuinely unknowable
 * pixels under part of it, and conditioning the surface would restyle the brand
 * to fix a backdrop only some of the type sits on. Only a clean miss earns the
 * poster treatment.
 *
 * Touching edges do not intersect: an image area ending exactly where the text
 * bounds begin shares a zero-area boundary and no pixel.
 */
function imageCoversText(
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

interface Foregrounds {
  /** The literal surface painted under every node. */
  background: string;
  primary: string;
  muted: string;
}

/** The WCAG 2.1 AA floor for body-sized text. */
const AA_RATIO = 4.5;

/** The two foreground tokens every non-pill slot picks from. */
const PRIMARY_CANDIDATES = [WHITE, INK_900];
const MUTED_CANDIDATES = [WARM_200, WARM_700];

/**
 * Resolve the surface and the two text foregrounds that read on it.
 *
 * With no photo UNDER THE TYPE the backdrop is exact — it is the color this
 * scene paints. With a photo under it the backdrop is an *assumption*: the flat
 * dark or light field the kit's scrim is meant to lay down. Whether a given
 * photograph actually lets the scrim reach that field depends on the pixels
 * underneath, which this module cannot see; resolving it from photo analysis is
 * Wave 2's job (see `ScrimNode.adaptive`). Which of the two applies is
 * `imageCoversText`'s question, not `photoUrl`'s — see there.
 *
 * The knowable path used to be the weak one: its backdrop is the operator's
 * brand primary, an arbitrary color, and white / ink-900 both fail 4.5:1 across
 * a 0.0368-wide luminance band that real brands land in (`#6366f1`, `#8b5cf6`
 * and `#a855f7` among them). `pickReadableColor` degraded silently there. The
 * surface is now conditioned out of that band first — see `conditionSurface` —
 * so the pick that follows always has a passing candidate. Brands inside the
 * band therefore render on a slightly darker or lighter version of their own
 * primary, at the same hue and saturation; brands outside it are untouched.
 *
 * The muted pair is looser than the primary pair — warm-200 / warm-700 fail
 * over a much wider band, (0.1273, 0.3760) — so conditioning for the primary
 * pair does not rescue them. Rather than condition twice (which would drag
 * mid-tone brands halfway to black to satisfy a deliberately low-contrast
 * token) the muted pick falls back to the primary foreground, which is already
 * guaranteed. Secondary slots lose their tint on those brands; they do not lose
 * their legibility.
 *
 * Neither backdrop is the surface on its own. `Z.texture` sits ABOVE the
 * surface and the scrim and BELOW the type, so a kit that carries a texture
 * puts a full-bleed layer of `palette.accent` between the two and the glyphs
 * read against that composite. It is a texture, not a wash, but it is not an
 * area average either: `drawHalftone` paints 8.6px discs on a 32.4px pitch at
 * 1080px wide, wide enough to sit whole behind a stem. Conditioning the surface
 * alone was therefore self-defeating — `#a855f7` conditioned to `#ab57fb` at
 * 4.5702:1, and the accent at 0.12 over that composites to `#ae54de`, back
 * inside the band at 4.2660:1. `texture` is passed to `conditionSurface` so the
 * COMPOSITE is what escapes, and the pick is made against the composite too.
 *
 * The photo path composites the texture as well, deliberately. Nothing there
 * can be CONDITIONED — the assumed field is a kit constant and the pixels the
 * type really sits on are the photograph's — but the texture is full bleed and
 * paints over the photo and the scrim exactly as it paints over a surface, so
 * ink-900 or cream on its own was never what the type read against either.
 * Compositing it costs nothing and makes the assumption honest as far as it
 * goes.
 *
 * Still NOT modelled: the scrim, on the path where it is painted. It is a
 * gradient, so the backdrop it lays down varies down the stack and a single
 * flat backdrop cannot express that. Over a photo that gradient is the point,
 * and resolving it is Wave 2's photo analysis.
 *
 * It is no longer a hole on the OTHER path, because there is no longer a scrim
 * there. It used to be emitted whenever a photo existed, `splitPanel` included,
 * where the type sits clear of the photo on the brand surface: on `#facc15` the
 * scrim's terminal stop composites to `#4d4017`, luminance 0.0531, where the
 * ink-900 this function picks against the surface itself (11.42:1) reads
 * 1.72:1. Nothing flat can serve both ends of that ramp — white is 1.53:1 at
 * the top of it and 10.19:1 at the bottom — so the scrim is now scoped by
 * `imageCoversText` as well, and the surface this returns is the surface the
 * glyphs actually land on.
 */
function resolveForegrounds(
  kit: CreativeKit,
  palette: ResolvedPalette,
  textOverPhoto: boolean,
  texture: SurfaceOverlay | null,
): Foregrounds {
  const surface = kit.prefersLightSurface ? SURFACE_CREAM : palette.primary;
  // Only a stack clear of the photo puts type straight onto this color, and
  // only there is the backdrop knowable, so that is the only place worth
  // conditioning.
  const background = textOverPhoto
    ? surface
    : conditionSurface(surface, PRIMARY_CANDIDATES, AA_RATIO, texture);
  const field = textOverPhoto
    ? kit.prefersLightSurface
      ? SURFACE_CREAM
      : INK_900
    : background;
  const backdrop = texture
    ? compositeOver(texture.color, field, texture.alpha)
    : field;

  const primary = pickReadableColor(PRIMARY_CANDIDATES, backdrop, AA_RATIO);
  return {
    background,
    primary,
    muted: pickReadableColor(
      [...MUTED_CANDIDATES, primary],
      backdrop,
      AA_RATIO,
    ),
  };
}

export function buildScene(input: BuildSceneInput): Scene {
  const { kit, composition, format, canvasW, canvasH, palette } = input;
  const hasPhoto = input.photoUrl.trim().length > 0;

  // Resolved once and used twice — by the contrast maths below and by the node
  // that paints it — so the layer the foregrounds are computed against and the
  // layer actually painted cannot drift apart.
  const textureMotif = TEXTURE_MOTIF[kit.texture];
  const texture: SurfaceOverlay | null = textureMotif
    ? { color: palette.accent, alpha: TEXTURE_OPACITY }
    : null;

  // `hasPhoto` decides whether a PHOTO is painted; whether the photo lands
  // under the type decides both the contrast maths and whether a scrim is
  // painted, because a scrim clear of the photo is a darkening the foreground
  // pick cannot see. See `resolveForegrounds`.
  const textOverPhoto = hasPhoto && imageCoversText(composition, format);
  const foreground = resolveForegrounds(kit, palette, textOverPhoto, texture);

  const bounds = composition.boundsFor(format);
  const bands: Band[] = composition.bandsFor(format).map((template) => ({
    ...template,
    text: (input.slots[template.key] ?? "").trim(),
    // The kit's type scale is folded in BEFORE solving so the solver measures
    // the size that will actually be painted. Applying it afterwards would let
    // any kit whose scale is not 1 lay out against a size it never renders.
    sizePct: template.sizePct * kit.typePairing.scale,
    // Same argument, same place, for the same reason: the face and the weight
    // are resolved BEFORE solving, and the text node below reads them straight
    // back off the band. Computing them here and again down there is how the
    // solver came to measure every band in one font while the walker painted
    // six. What decides them is `bandTypeFor` and nothing else — see there for
    // why the brand's own font setting is not part of that decision.
    ...bandTypeFor(kit, template.key),
  }));

  const solved = solveStack({
    bands,
    bounds,
    canvasW,
    canvasH,
    anchor: composition.anchor,
    measure: input.measure,
  });

  const nodes: SceneNode[] = [];

  if (hasPhoto) {
    const grade = gradeFor(kit.gradePreset.filter, input.analysis ?? null);
    nodes.push(
      photoNode({
        url: input.photoUrl,
        rect: composition.imageAreaFor(format),
        crop: input.crop,
        // The kit's declared grade, corrected for what this photograph actually
        // is. `whiteBalance` is spread conditionally rather than assigned
        // `undefined`: `exactOptionalPropertyTypes` is off, so an explicit
        // undefined would satisfy the type and then be a property the walker
        // has to guard against anyway.
        filter: grade.filter,
        ...(grade.whiteBalance ? { whiteBalance: grade.whiteBalance } : {}),
        z: Z.photo,
      }),
    );
  }

  /*
   * The scrim exists to hold type legible over a PHOTOGRAPH, so it is emitted
   * exactly where there is one under the type — not merely where the post has
   * one. `splitPanel` is the case that separates the two: it requires a photo,
   * paints it, and puts every band below it, so a scrim there darkens the brand
   * surface under the type by up to the kit's terminal alpha while the
   * foreground was picked against that surface undarkened.
   *
   * What a family loses by this is the fade over its own photo panel, which is
   * a composition's business rather than the text scrim's: `scrimRect` is
   * derived from the TEXT bounds and reaches the panel only incidentally, by
   * the `SCRIM_BLEED` above them. A panel-edge treatment belongs in
   * `composition.motifs` or a shape node, where its geometry is declared.
   */
  if (textOverPhoto) {
    const measured = input.scrimLuma ?? null;
    const boost =
      measured != null && kit.scrimStyle.adaptive
        ? adaptiveScrimBoost(measured, kit.scrimStyle.luminanceThreshold)
        : null;
    nodes.push(
      scrimNode({
        rect: scrimRect(bounds),
        from: kit.scrimStyle.from,
        to: kit.scrimStyle.to,
        // A measured band is a decided band: the walker has nothing left to
        // work out, so it must not pay for a raster readback to reach the same
        // answer — and a print or motion walker can now honour it at all.
        adaptive: measured == null ? kit.scrimStyle.adaptive : false,
        luminanceThreshold: kit.scrimStyle.luminanceThreshold,
        ...(boost ? { boost } : {}),
        z: Z.scrim,
      }),
    );
  }

  bands.forEach((band) => {
    // Absent for a blank slot, and for any band the solver clipped off the end
    // of an overfull stack.
    const layout = solved[band.key];
    if (!layout) return;

    const isPill = PILL_SLOTS.has(band.key);

    nodes.push(
      textNode({
        key: band.key,
        text: band.text,
        rect: layout.rect,
        z: Z.text,
        // Read off the band, not recomputed from the kit: these are the exact
        // values the solver measured this band's width with.
        font: band.font,
        weight: band.weight,
        // The solver is the single source of truth for type size: `fontPx`
        // already carries both the kit scale folded in above and whatever
        // further shrink the stack needed to fit. Recomputing it from
        // `band.sizePct` would paint text the rect was never measured for —
        // precisely when the solver worked hardest.
        sizePct: canvasW > 0 ? layout.fontPx / canvasW : 0,
        align: band.align,
        color: isPill
          ? palette.onAccent
          : MUTED_SLOTS.has(band.key)
            ? foreground.muted
            : foreground.primary,
        maxLines: band.maxLines,
        lineHeight: band.lineHeight,
        uppercase: UPPERCASE_SLOTS.has(band.key),
        tracking: kit.typePairing.tracking,
        ...(isPill
          ? { pill: { fill: palette.accent, padX: PILL_PAD_X, padY: PILL_PAD_Y } }
          : {}),
      }),
    );
  });

  // The intersection of what the kit owns and what the composition allows, so a
  // kit can never stamp a motif a layout has no room for. Each one is framed on
  // the text bounds, clamped so the ink it puts OUTSIDE that frame still lands
  // inside the platform-safe area — see `safeMotifRect`.
  kit.motifSet
    .filter((motif) => composition.motifs.includes(motif))
    .forEach((motif) => {
      nodes.push(
        motifNode({
          motif,
          rect: safeMotifRect(motif, bounds, format, canvasW, canvasH),
          color: palette.accent,
          opacity: 0.9,
          z: Z.motif,
        }),
      );
    });

  // The kit's texture, and deliberately NOT run through the composition
  // intersection above: that intersection exists so a layout can veto an accent
  // it has no room for, and a full-bleed field has no geometry to conflict
  // with. Pushed here, after the text, because this is where motif nodes are
  // built — `Z.texture` puts it back underneath them.
  if (textureMotif && texture) {
    nodes.push(
      motifNode({
        motif: textureMotif,
        rect: FULL_BLEED,
        color: texture.color,
        opacity: texture.alpha,
        z: Z.texture,
      }),
    );
  }

  const logoUrl = input.logoUrl.trim();
  if (logoUrl) {
    const anchor = composition.logoAnchorFor(format);
    nodes.push(
      logoNode({
        url: logoUrl,
        // `w` is the whole geometry: the mark is a disc of that diameter, and
        // the diameter is width-relative, so it stays circular at 1:1, 4:5 and
        // 9:16 without an aspect correction. This used to also carry an `h`
        // computed from the aspect for exactly that correction; nothing ever
        // read it, because there is no second dimension to correct. See
        // `LogoRect` in ./types.ts.
        rect: { x: anchor.x, y: anchor.y, w: anchor.size },
        z: Z.logo,
      }),
    );
  }

  return {
    width: canvasW,
    height: canvasH,
    background: foreground.background,
    nodes: sortNodes(nodes),
  };
}
