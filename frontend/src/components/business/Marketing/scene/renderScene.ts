import { cssFontString } from "../artDirection/typeface";
import type { NormalizedRect, SlotAlign } from "../templates/types";
import {
  applyScrim,
  drawLogo,
  fitCover,
  roundRect,
  sampleRegionLuminance,
  wrapText,
} from "./canvasPrimitives";
import { resolveRect } from "./geometry";
import { drawMotif } from "./motifs";
import type { Scene, SceneNode, SceneNodeKind, TextNode } from "./types";

/**
 * The canvas walker: the one place in the scene pipeline that touches a 2D
 * context. Everything upstream (`buildScene`, the art-direction kits, the
 * composition solver) is pure.
 *
 * Six drawing primitives — `applyScrim`, `drawLogo`, `fitCover`, `roundRect`,
 * `sampleRegionLuminance` and `wrapText` — live in `./canvasPrimitives`, the
 * documented leaf shared with `../templates/renderPost` (which re-exports them
 * for stable caller paths). They are not reimplemented here, so scrim, logo,
 * source-crop, corner-radius and line-breaking behaviour cannot drift between
 * preview, export and this walker. Importing those helpers from `renderPost`
 * would rebuild the `renderPost ↔ renderScene` cycle the leaf exists to break.
 *
 * The text drawer is NOT one of them. It began as a fork of `renderPost`'s
 * `drawSlot`, which took a `SlotDef` plus a `Palette` and resolved colour
 * tokens at paint time. That function is gone — 1c79f6b89 deleted it when
 * `renderPost` was rewired onto this pipeline — so the fork has healed: there
 * is no second text painter left to mirror a wrap, padding or alignment fix
 * into. `paintTextNode` is now the only one, and a `TextNode` reaches it with
 * every colour and font decision already made upstream by `buildScene`.
 *
 * That drawer is split in two: `layoutTextNode` measures, wraps and places,
 * and is exported because it is canvas-free — the planned motion and print
 * walkers need the placement without the ink. `paintTextNode` is its only
 * consumer here and does nothing the layout has not already decided.
 *
 * The font string is explicitly NOT forked. It comes from
 * `../artDirection/typeface`, the same builder the composition solver measures
 * through, because a width is only a width in a given font.
 */

/**
 * One drawable the walker can paint: a canvas-compatible source plus the pixel
 * dimensions the cover-fit crop is computed against.
 *
 * Structural (not `HTMLImageElement`) so a worker can hand over an `ImageBitmap`
 * — workers have no `new Image()`, and bitmaps expose `width`/`height` rather
 * than `naturalWidth`/`naturalHeight`. Main-thread loads wrap each decoded
 * `HTMLImageElement` through `toSceneImage`.
 */
export interface SceneImage {
  source: CanvasImageSource;
  width: number;
  height: number;
}

/**
 * Bitmaps keyed by the source URL each `PhotoNode` / `LogoNode` carries.
 *
 * A single `{ photo, logo }` pair cannot name two distinct photographs in one
 * scene; the URL is already on the node, so lookup is by that string. A missing
 * or null entry means that node paints nothing — and a photo node whose image
 * never loaded does not contribute a rect to the adaptive-scrim gate either.
 */
export type SceneImages = Record<string, SceneImage | null | undefined>;

/**
 * Wrap a decoded main-thread image (or any source that reports natural size)
 * as a structural `SceneImage` the walker can paint.
 */
export function toSceneImage(
  image: HTMLImageElement | ImageBitmap | SceneImage,
): SceneImage {
  if (
    typeof image === "object" &&
    image !== null &&
    "source" in image &&
    "width" in image &&
    "height" in image &&
    typeof (image as SceneImage).width === "number"
  ) {
    return image as SceneImage;
  }
  if (typeof ImageBitmap !== "undefined" && image instanceof ImageBitmap) {
    return { source: image, width: image.width, height: image.height };
  }
  const el = image as HTMLImageElement;
  return {
    source: el,
    width: el.naturalWidth,
    height: el.naturalHeight,
  };
}

/**
 * Delegated, not reimplemented. `cssFontString` is the single builder the
 * composition solver also measures through, so the family, the weight (serif is
 * forced to 400 there — DM Serif Display ships one weight and synthetic bold
 * looks wrong) and the px rounding cannot differ between the width the solver
 * wrapped against and the glyphs painted below. Spelling the string out here
 * again is what let a serif headline be measured as DM Sans and overrun its
 * bounds.
 */
function fontString(node: TextNode, canvasW: number): string {
  return cssFontString({
    font: node.font,
    weight: node.weight,
    px: node.sizePct * canvasW,
  });
}

/**
 * Text width in device pixels for `text` set in `cssFont`, injected so
 * `layoutTextNode` stays canvas-free. Deliberately the same shape as
 * `MeasureFn` in ../composition/solver.ts — the solver and the walker have to
 * agree on a width to the character, and one shape for "measure this" is how
 * that stays true.
 *
 * The `cssFont` argument is the font `layoutTextNode` resolved for the node,
 * handed over rather than assumed, because a measurer with no context of its
 * own (print, motion) cannot read it back from anywhere.
 */
export type MeasureText = (text: string, cssFont: string) => number;

/** One resolved line of a `TextNode`, in device pixels. */
interface TextLineBox {
  text: string;
  /**
   * The alignment anchor. WHICH edge of the glyph run it names is
   * `TextLayout.align`, exactly as `ctx.textAlign` decides for canvas — the
   * two are one decision and a consumer that honours the x without the align
   * paints a centred line starting at its own centre.
   */
  x: number;
  /** Top of the line box; the canvas painter sets `textBaseline` to "top". */
  y: number;
}

/** The rounded plate painted behind the text, already sized and placed. */
interface TextPillBox {
  fill: string;
  x: number;
  y: number;
  w: number;
  h: number;
  /**
   * Requested corner radius. `roundRect` clamps it to half the shorter side,
   * so a consumer that does its own rounding must clamp it too.
   */
  radius: number;
}

/** Everything a walker needs to put a `TextNode` down, with nothing painted. */
export interface TextLayout {
  /** The CSS font string every width below was measured in. */
  font: string;
  align: SlotAlign;
  /** Line box height in device pixels. */
  lineHeight: number;
  /** Empty when the node's text is blank or the box admits no line. */
  lines: TextLineBox[];
  pill: TextPillBox | null;
}

/**
 * `wrapText` is typed against a full 2D context but reaches for exactly one
 * member of it. This adapter is what lets the layout half stay canvas-free
 * without forking the line-breaking maths — a second wrap implementation is
 * precisely the divergence `947f18519` exists to prevent, so the cast is the
 * cheaper of the two evils and is confined to this function.
 */
function measurerFor(
  measure: MeasureText,
  cssFont: string,
): CanvasRenderingContext2D {
  return {
    measureText: (text: string) => ({ width: measure(text, cssFont) }),
  } as unknown as CanvasRenderingContext2D;
}

/**
 * The layout half of the text drawer: measure, wrap and place, without
 * painting.
 *
 * Split out because two planned walkers need it without the canvas — Wave 3's
 * motion walker evaluates a scene at time `t` with nothing to draw on, and
 * Wave 4's print walker turns line boxes into HTML. Keeping the maths here and
 * the ink in `paintTextNode` means those walkers consume the same placement
 * the canvas paints rather than re-deriving it, which is the same argument
 * that keeps `wrapText` and `cssFontString` shared rather than forked.
 *
 * `node.tracking` is NOT applied, here or in the painter. It is carried and
 * deliberately unapplied: the composition solver measures through
 * `cssFontString` and this walker paints through it, and the two must agree to
 * the character. Applying letter-spacing on only one side reopens exactly the
 * measure/paint divergence `947f18519` closed. If tracking is wanted,
 * `cssFontString` has to own it so both sides move together — see the docstring
 * on ../artDirection/typeface.ts.
 */
export function layoutTextNode(
  node: TextNode,
  canvasW: number,
  canvasH: number,
  measure: MeasureText,
): TextLayout {
  const font = fontString(node, canvasW);
  const text = node.uppercase ? node.text.toUpperCase() : node.text;

  const box = resolveRect(node.rect, canvasW, canvasH);
  // Both pill paddings are fractions of canvas WIDTH, `padY` included — see the
  // note on `TextNode.pill` in ./types.ts. Keeping padY width-relative holds the
  // plate's optical padding constant across 1:1, 4:5 and 9:16.
  const padX = node.pill ? node.pill.padX * canvasW : 0;
  const padY = node.pill ? node.pill.padY * canvasW : 0;
  const fontPx = node.sizePct * canvasW;
  const lineHeight = fontPx * node.lineHeight;
  // The `max(1, …)` floor is deliberate, not a rounding slip: a box too short
  // for even one line still renders one and overflows. Showing a clipped line
  // beats showing nothing, and the composition solver — not this walker — is
  // what keeps boxes tall enough.
  const linesByHeight = Math.max(
    1,
    Math.floor((box.h - 2 * padY) / Math.max(1, lineHeight)),
  );
  const lines = wrapText(
    measurerFor(measure, font),
    text,
    Math.max(1, box.w - 2 * padX),
    Math.min(node.maxLines, linesByHeight),
  );
  if (!lines.length) {
    return { font, align: node.align, lineHeight, lines: [], pill: null };
  }

  let pill: TextPillBox | null = null;
  if (node.pill) {
    const textW = Math.max(...lines.map((line) => measure(line, font)));
    const pillW = Math.min(box.w, textW + 2 * padX);
    const pillH = Math.min(box.h, lines.length * lineHeight + 2 * padY);
    let pillX = box.x;
    if (node.align === "center") pillX = box.x + (box.w - pillW) / 2;
    if (node.align === "right") pillX = box.x + box.w - pillW;
    pill = {
      fill: node.pill.fill,
      x: pillX,
      y: box.y,
      w: pillW,
      h: pillH,
      radius: pillH / 2,
    };
  }

  // Alignment is resolved twice and the two must agree: `align` decides which
  // edge of the glyph run `x` names, and `x` is the point on the box that edge
  // is pinned to. Change one without the other and the run slides off the box.
  let anchorX = box.x + padX;
  if (node.align === "center") anchorX = box.x + box.w / 2;
  if (node.align === "right") anchorX = box.x + box.w - padX;

  return {
    font,
    align: node.align,
    lineHeight,
    lines: lines.map((line, index) => ({
      text: line,
      x: anchorX,
      y: box.y + padY + index * lineHeight,
    })),
    pill,
  };
}

/**
 * The paint half: a consumer of `layoutTextNode` and nothing more.
 *
 * The font is written to the context BEFORE the layout runs and the measurer
 * handed over ignores the `cssFont` it is passed, because on canvas a width is
 * only a width once `ctx.font` is set — re-applying the same string per
 * measurement would be redundant state churn. The two cannot disagree: both
 * come from `fontString`, a pure function of the same node and canvas width.
 */
function paintTextNode(
  ctx: CanvasRenderingContext2D,
  node: TextNode,
  canvasW: number,
  canvasH: number,
): void {
  ctx.font = fontString(node, canvasW);
  ctx.textBaseline = "top";

  const layout = layoutTextNode(
    node,
    canvasW,
    canvasH,
    (text) => ctx.measureText(text).width,
  );
  if (!layout.lines.length) return;

  if (layout.pill) {
    ctx.fillStyle = layout.pill.fill;
    roundRect(
      ctx,
      layout.pill.x,
      layout.pill.y,
      layout.pill.w,
      layout.pill.h,
      layout.pill.radius,
    );
    ctx.fill();
  }

  ctx.fillStyle = node.color;
  ctx.textAlign = layout.align;
  layout.lines.forEach((line) => {
    ctx.fillText(line.text, line.x, line.y);
  });
}

/** Cache key for a scrim's readback region. */
function rectKey(rect: NormalizedRect): string {
  return `${rect.x},${rect.y},${rect.w},${rect.h}`;
}

/**
 * Is a photograph actually under `rect`?
 *
 * The IR-level form of `imageCoversText` in ./buildScene.ts, which decides
 * whether a scrim is emitted at all by intersecting the composition's image
 * area with its text bounds. This asks the same question of the same geometry
 * — the resolved rects the nodes carry — because the walker has the node list
 * and not the composition that produced it.
 *
 * It has to be asked. "The post has a photo" is a different question from
 * "there is a photo here", and `splitPanel` is where they part company: it
 * requires a photograph, paints it across the top, and puts every band below
 * it on the flat brand surface. Asking the first question there samples the
 * bright brand panel and stacks a second ink gradient over a surface whose
 * foreground was already chosen against it undarkened — ink-900 falls from
 * 1.72:1 to about 1.16:1, so the "adaptive" pass makes the type LESS legible.
 *
 * Any intersection at all counts, matching `imageCoversText`'s deliberately
 * conservative call: a band straddling the edge of a panel has genuinely
 * unknowable pixels under part of it, and that part is what the boost is for.
 * Touching edges share a zero-area boundary and no pixel, hence `> 0`.
 */
function photoCovers(
  rect: NormalizedRect,
  photoRects: readonly NormalizedRect[],
): boolean {
  return photoRects.some((photo) => {
    const width =
      Math.min(photo.x + photo.w, rect.x + rect.w) - Math.max(photo.x, rect.x);
    const height =
      Math.min(photo.y + photo.h, rect.y + rect.h) - Math.max(photo.y, rect.y);
    return width > 0 && height > 0;
  });
}

/** Everything a painter is allowed to reach for. */
interface PaintEnv {
  ctx: CanvasRenderingContext2D;
  width: number;
  height: number;
  images: SceneImages;
  /**
   * The rects of photo nodes this walk actually PAINTS — only nodes whose URL
   * resolved in `images`. A photo node with a missing bitmap does not appear
   * here, so adaptive scrim boost stays off over unpainted regions. See
   * `photoCovers`.
   */
  photoRects: readonly NormalizedRect[];
  /** Per-render `sampleRegionLuminance` memo; see `renderScene`. */
  luminanceByRect: Map<string, number>;
}

/**
 * One painter per node kind, narrowed to that kind. The mapped type is what
 * makes this table worth having over a `switch`: adding a member to `SceneNode`
 * without adding its painter is a compile error here (TS2741), where a `switch`
 * with a `default:` would have silently stopped painting the new kind.
 */
type Painters = {
  [K in SceneNodeKind]: (
    node: Extract<SceneNode, { kind: K }>,
    env: PaintEnv,
  ) => void;
};

const PAINTERS: Painters = {
  photo: (node, env) => {
    const { ctx, width, height, images } = env;
    const photo = images[node.url];
    if (!photo) return;
    const dst = resolveRect(node.rect, width, height);
    const crop = fitCover(
      photo.width,
      photo.height,
      dst.w,
      dst.h,
      node.crop,
    );

    // `filter` is set for the drawImage and released immediately: it is a
    // context-wide property, so leaving it set would grade the scrim, the type
    // and the logo as well. A context without the property (older embedded
    // browsers, and the leanest test stubs) renders ungraded rather than
    // throwing — the same degradation an unanalysable photo already takes.
    const supportsFilter = "filter" in ctx;
    const previousFilter = supportsFilter ? ctx.filter : "";
    if (supportsFilter && node.filter) ctx.filter = node.filter;
    ctx.drawImage(
      photo.source,
      crop.sx,
      crop.sy,
      crop.sw,
      crop.sh,
      dst.x,
      dst.y,
      dst.w,
      dst.h,
    );
    if (supportsFilter && node.filter) ctx.filter = previousFilter || "none";

    if (!node.whiteBalance) return;
    // A source-over multiply IS a per-channel scale, which is what white
    // balance is; `gradeFor` already gave back the light it costs as a
    // brightness term. Clipped to the photo rect via `roundRect` at radius 0 so
    // a split-panel layout does not tint the text panel beside the photo.
    ctx.save();
    roundRect(ctx, dst.x, dst.y, dst.w, dst.h, 0);
    ctx.clip();
    ctx.globalCompositeOperation = "multiply";
    ctx.fillStyle = node.whiteBalance;
    ctx.fillRect(dst.x, dst.y, dst.w, dst.h);
    ctx.restore();
  },

  scrim: (node, env) => {
    const { ctx, width, height, photoRects, luminanceByRect } = env;
    const treatment = {
      region: node.rect,
      from: node.from,
      to: node.to,
      adaptive: node.adaptive,
      luminanceThreshold: node.luminanceThreshold,
    };

    if (node.boost) {
      // Decided upstream from the source pixels. Two flat passes, no readback.
      applyScrim(ctx, width, height, { ...treatment, adaptive: false }, null);
      applyScrim(
        ctx,
        width,
        height,
        {
          ...treatment,
          from: node.boost.from,
          to: node.boost.to,
          adaptive: false,
        },
        null,
      );
      return;
    }

    let luminance: number | null = null;
    // `node.adaptive` gates the readback, not just the boost. Sampling for a
    // non-adaptive scrim — every light-surface kit — was a `getImageData` whose
    // result `applyScrim` then discarded. Nothing to measure without a photo
    // under THIS BAND either: the surface there is a flat brand fill.
    if (node.adaptive && photoCovers(node.rect, photoRects)) {
      const key = rectKey(node.rect);
      let sampled = luminanceByRect.get(key);
      if (sampled === undefined) {
        sampled = sampleRegionLuminance(ctx, width, height, node.rect);
        luminanceByRect.set(key, sampled);
      }
      luminance = sampled;
    }
    applyScrim(ctx, width, height, treatment, luminance);
  },

  shape: (node, env) => {
    const { ctx, width, height } = env;
    ctx.fillStyle = node.fill;
    const px = resolveRect(node.rect, width, height);
    if (node.radiusPct) {
      // Radius is a fraction of canvas WIDTH on both axes, matching
      // `ShapeNode.radiusPct` in ./types.ts — a height-relative radius would
      // make the same node round differently at 1:1 and 9:16.
      roundRect(ctx, px.x, px.y, px.w, px.h, node.radiusPct * width);
      ctx.fill();
    } else {
      ctx.fillRect(px.x, px.y, px.w, px.h);
    }
  },

  text: (node, env) => {
    paintTextNode(env.ctx, node, env.width, env.height);
  },

  motif: (node, env) => {
    drawMotif(env.ctx, node, env.width, env.height);
  },

  logo: (node, env) => {
    const logo = env.images[node.url];
    drawLogo(env.ctx, logo?.source ?? null, env.width, env.height, {
      x: node.rect.x,
      y: node.rect.y,
      // Square, sized off canvas WIDTH — `rect.h` is unused, matching
      // `LogoAnchor.size` in ../templates/types.ts.
      size: node.rect.w,
    });
  },
};

/**
 * Dispatch one node. The generic is what lets each painter keep its narrow
 * parameter type without a cast: `node.kind` is `K`, so `PAINTERS[node.kind]`
 * is the painter for exactly `K`.
 *
 * `opacity` is applied HERE and nowhere else. It used to live in `drawMotif`,
 * which was fine while motifs were the only node that could carry it; once
 * `BaseNode` carries it, a per-painter implementation would be six copies of
 * one rule and a guaranteed double-apply the day the walker grew an envelope of
 * its own. The `save`/`restore` pair is what stops the multiply compounding
 * across the flat node list.
 */
function paintNode<K extends SceneNodeKind>(
  node: Extract<SceneNode, { kind: K }> & { kind: K },
  env: PaintEnv,
): void {
  const painter = PAINTERS[node.kind];
  // A node kind this walker does not know is skipped, not thrown on: the IR is
  // shared with the planned print and motion walkers, and one of them gaining a
  // kind must not break canvas rendering. The table types every known kind as
  // present, so this guard exists purely for values that were never in the
  // union — hence the runtime check the types say is redundant.
  if (!painter) return;
  if (node.opacity == null && node.clip == null) {
    painter(node, env);
    return;
  }
  const { ctx } = env;
  ctx.save();
  if (node.clip != null) {
    const box = resolveRect(node.clip, env.width, env.height);
    // Radius 0, so `roundRect`'s four arcTo calls degenerate to straight lines.
    // `ctx.rect` would be the obvious primitive and is deliberately not used:
    // it is absent from both test stubs, while moveTo/arcTo/clip are in both.
    roundRect(ctx, box.x, box.y, box.w, box.h, 0);
    ctx.clip();
  }
  if (node.opacity != null) ctx.globalAlpha *= node.opacity;
  painter(node, env);
  ctx.restore();
}

/**
 * Paint a scene. Nodes are assumed pre-sorted by z (buildScene guarantees it,
 * via `sortNodes`), so this walks them in array order and never re-sorts —
 * re-sorting here would silently override an author who ordered nodes by hand.
 */
export function renderScene(
  ctx: CanvasRenderingContext2D,
  scene: Scene,
  images: SceneImages,
): void {
  const { width, height } = scene;

  ctx.fillStyle = scene.background;
  ctx.fillRect(0, 0, width, height);

  // `sampleRegionLuminance` calls `getImageData`, a raster readback and the most
  // expensive thing in the walk, so a region sampled once is not sampled again.
  // Luminance is a property of a REGION, though: the cache is keyed by rect so a
  // second scrim over a different band reads that band back rather than
  // inheriting the first band's answer and making the wrong adaptive decision.
  //
  // This is memoization of a NON-pure read: `sampleRegionLuminance` measures the
  // canvas and the walk paints on it, so a hit is only correct while nothing has
  // repainted the sampled region since the miss that filled it. Today that holds
  // because a scene carries a single scrim per band; a second scrim over an
  // already-scrimmed band would read the pre-scrim value and over-boost.
  const luminanceByRect = new Map<string, number>();

  // Collected once, up front, because the scrim painter needs to know what is
  // under its band and a painter only sees its own node. Only photo nodes whose
  // URL resolved to a loaded bitmap contribute a rect: a scene can carry a
  // photo node whose image never loaded (or a second photo whose URL is absent
  // from the map), and nothing was painted there either.
  const photoRects: NormalizedRect[] = [];
  for (const node of scene.nodes) {
    if (node.kind === "photo" && images[node.url]) {
      photoRects.push(node.rect);
    }
  }

  const env: PaintEnv = {
    ctx,
    width,
    height,
    images,
    photoRects,
    luminanceByRect,
  };
  scene.nodes.forEach((node) => {
    paintNode(node, env);
  });
}
