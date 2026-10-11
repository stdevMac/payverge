import type { MarketingCrop } from "@/api/marketing";
import { logError } from "@/utils/errorLogger";
import { KITS, kitForLegacyTemplate, type KitId } from "../artDirection/kits";
import { harmonizePalette } from "../artDirection/palette";
import {
  chooseComposition,
  contentSignalsFrom,
  type PhotoSignals,
} from "../composition/chooser";
import {
  COMPOSITIONS,
  LEGACY_COMPOSITION_FOR_TEMPLATE,
  type CompositionId,
} from "../composition/compositions";
import {
  DEFAULT_FORMAT_ID,
  FORMATS,
  isFormatId,
  type FormatDef,
  type FormatId,
} from "../formats/formats";
import { photoSignalsFrom } from "../photo/analyze";
import { photoAnalysisFor } from "../photo/cache";
import { meanOfCells, mapCanvasRectToSource } from "../photo/regions";
import type { PhotoAnalysis } from "../photo/types";
import { buildScene, scrimRectFor } from "../scene/buildScene";
import {
  DEFAULT_CROP,
  fitCover,
  sampleBottomLuminance,
  sampleRegionLuminance,
  wrapText,
} from "../scene/canvasPrimitives";
import { resolveRect } from "../scene/geometry";
import type { MeasureFn } from "../composition/solver";
import {
  renderScene,
  toSceneImage,
  type SceneImages,
} from "../scene/renderScene";
import type { Scene } from "../scene/types";
import type { SlotKey, TemplateDef } from "./types";

// Re-export canvas primitives from the scene leaf so existing callers keep a
// stable import path. The implementations live in `../scene/canvasPrimitives`
// (documented leaf) so this module and `renderScene` do not form a cycle.
export {
  DEFAULT_CROP,
  fitCover,
  sampleBottomLuminance,
  sampleRegionLuminance,
  wrapText,
};

interface Palette {
  primary: string;
  secondary: string;
  /**
   * Wire snapshot font honesty only — NOT a face override for the renderer.
   *
   * Type faces are the art-direction kit's decision (`CreativeKit.typePairing`),
   * resolved once in `buildScene` so the composition solver measures the face
   * the walker paints. Callers derive this via `fontFamilyFromKit` /
   * `resolveBrandPalette` (see `brandLock.ts`). `design_settings.font_family`
   * is retired for this pipeline (still used on the public site + mood hints).
   */
  fontFamily?: string;
}

export interface RenderPostInput {
  /** Legacy path: a TemplateDef. Ignored when `kit` is set. */
  template?: TemplateDef;
  /** Art-direction kit. Absent means legacy. */
  kit?: KitId;
  /**
   * Composition family. Read only on the kit path — a legacy `template` pins
   * its own legacy composition, which is the whole point of those families:
   * a stored snapshot from before the rewrite renders as it always did.
   * Absent with a kit means the chooser decides; see `resolveArtDirection`.
   */
  composition?: CompositionId;
  /**
   * Which canvas. Widened from the old three-value `AspectRatio` to the format
   * registry; the three original strings are still valid ids, so every existing
   * caller and every stored snapshot keeps working unchanged.
   */
  aspect: FormatId;
  photoUrl: string;
  slots: Partial<Record<SlotKey, string>>;
  palette: Palette;
  logoUrl?: string;
  crop?: MarketingCrop;
}

/**
 * Full export dimensions for a format, or a proportionally scaled thumbnail size
 * when `targetWidth` is below the format's own export width.
 *
 * Every composition token is a fraction of the canvas, so a scaled render is the
 * full-size one proportionally — no second definition is needed. Note the width
 * is the FORMAT's, not a global 1080: a wide hero scales from 1920.
 */
/**
 * @param targetWidth — when set below export width, scale down (thumbnails).
 *   When set above native width (print-shop high-dpi), scale up, capped at 2×
 *   native and 4096 on the long edge to bound canvas memory.
 * @param exportScale — alternative upscale when targetWidth is omitted; clamped
 *   to [1, 2]. Prefer targetWidth for explicit pixel control.
 */
function formatDimensions(
  format: FormatDef,
  targetWidth?: number,
  exportScale?: number,
): { w: number; h: number } {
  const full = format.px;
  if (targetWidth != null && Number.isFinite(targetWidth) && targetWidth > 0) {
    if (targetWidth === full.w) {
      return { w: full.w, h: full.h };
    }
    // Allow modest upscale for print-shop packs (S3-Reach); still bound memory.
    const maxW = Math.min(full.w * 2, 4096);
    const clampedW = Math.min(maxW, Math.max(1, Math.round(targetWidth)));
    const scale = clampedW / full.w;
    return {
      w: Math.max(1, Math.round(full.w * scale)),
      h: Math.max(1, Math.round(full.h * scale)),
    };
  }
  const scaleRaw =
    exportScale != null && Number.isFinite(exportScale) ? exportScale : 1;
  const scale = Math.min(2, Math.max(1, scaleRaw));
  if (scale === 1) {
    return { w: full.w, h: full.h };
  }
  return {
    w: Math.max(1, Math.round(full.w * scale)),
    h: Math.max(1, Math.round(full.h * scale)),
  };
}

/**
 * The format for a snapshot's stored `aspect`, falling back to the default for a
 * value this build does not know. A backend newer than this frontend can
 * legitimately hand back a format id that has no definition here, and an
 * undefined registry lookup is a zero-sized canvas.
 */
function formatForInput(aspect: string): FormatDef {
  return FORMATS[isFormatId(aspect) ? aspect : DEFAULT_FORMAT_ID];
}

/** @deprecated Use `formatDimensions`. Kept for callers still passing an id. */
export function aspectDimensions(
  aspect: string,
  targetWidth?: number,
): { w: number; h: number } {
  return formatDimensions(formatForInput(aspect), targetWidth);
}

/** Default library-row thumbnail width (7rem host ≈ 112px CSS; 256px covers 2x). */
export const LIBRARY_THUMBNAIL_WIDTH = 256;

export type RenderPostOptions = {
  signal?: AbortSignal;
  /**
   * When set below export width, render a proportionally scaled canvas
   * (thumbnails). When set above native width (capped 2× / 4096), upscale for
   * print-shop high-dpi PNG packs.
   */
  targetWidth?: number;
  /**
   * Print-shop raster scale when targetWidth is omitted. Clamped to [1, 2].
   * Native export is scale 1 (default).
   */
  exportScale?: number;
};

export type RenderPostSignalOrOptions = AbortSignal | RenderPostOptions;

function resolveRenderOptions(
  signalOrOptions?: RenderPostSignalOrOptions,
): RenderPostOptions {
  if (!signalOrOptions) return {};
  if (
    typeof AbortSignal !== "undefined" &&
    signalOrOptions instanceof AbortSignal
  ) {
    return { signal: signalOrOptions };
  }
  // jsdom / polyfilled signals may not pass instanceof.
  if (
    typeof signalOrOptions === "object" &&
    "aborted" in signalOrOptions &&
    typeof (signalOrOptions as AbortSignal).addEventListener === "function" &&
    !("targetWidth" in signalOrOptions)
  ) {
    return { signal: signalOrOptions as AbortSignal };
  }
  return signalOrOptions as RenderPostOptions;
}

interface FontLoadCache {
  fontSet: FontFaceSet;
  promise: Promise<void>;
}

interface ImageLoadCacheEntry {
  image: HTMLImageElement;
  promise: Promise<HTMLImageElement>;
  consumers: number;
  settled: boolean;
  cancel: () => void;
  /** Wall-clock ms when this entry failed. Present only for negative-cache hits. */
  failedAt?: number;
}

const IMAGE_CACHE_LIMIT = 8;
/** How long a failed URL stays negative-cached before a retry is allowed. */
const NEGATIVE_IMAGE_CACHE_TTL_MS = 5 * 60 * 1000;
let fontLoadCache: FontLoadCache | null = null;
const imageLoadCache = new Map<string, ImageLoadCacheEntry>();
/** Session-level: at most one logError per failing image URL. */
const reportedImageLoadFailures = new Set<string>();

function abortError(): Error {
  if (typeof DOMException === "function") {
    return new DOMException("render_aborted", "AbortError");
  }
  const error = new Error("render_aborted");
  error.name = "AbortError";
  return error;
}

function throwIfAborted(signal?: AbortSignal): void {
  if (signal?.aborted) throw abortError();
}

function waitForPromise<T>(
  promise: Promise<T>,
  signal?: AbortSignal,
): Promise<T> {
  if (!signal) return promise;
  throwIfAborted(signal);
  return new Promise<T>((resolve, reject) => {
    const onAbort = () => reject(abortError());
    signal.addEventListener("abort", onAbort, { once: true });
    promise.then(resolve, reject).finally(() => {
      signal.removeEventListener("abort", onAbort);
    });
  });
}

async function ensureFonts(signal?: AbortSignal): Promise<void> {
  if (typeof document === "undefined" || !("fonts" in document)) return;
  const fontSet = document.fonts;
  if (!fontLoadCache || fontLoadCache.fontSet !== fontSet) {
    const promise = Promise.all([
      fontSet.load("700 80px 'DM Sans'"),
      fontSet.load("600 80px 'DM Sans'"),
      fontSet.load("500 80px 'DM Sans'"),
      fontSet.load("400 80px 'DM Sans'"),
      fontSet.load("400 80px 'DM Serif Display'"),
      fontSet.ready,
    ]).then(() => undefined);
    fontLoadCache = { fontSet, promise };
    promise.catch(() => {
      if (fontLoadCache?.promise === promise) fontLoadCache = null;
    });
  }

  try {
    await waitForPromise(fontLoadCache.promise, signal);
  } catch (error) {
    if ((error as Error).name === "AbortError") throw error;
    // Font loading may reject in embedded browsers; canvas uses its fallbacks.
  }
}

function trimImageCache(): void {
  while (imageLoadCache.size > IMAGE_CACHE_LIMIT) {
    const evictable = Array.from(imageLoadCache.entries()).find(
      ([, entry]) => entry.settled && entry.consumers === 0,
    );
    if (!evictable) return;
    imageLoadCache.delete(evictable[0]);
  }
}

function createImageCacheEntry(url: string): ImageLoadCacheEntry {
  const image = new Image();
  image.crossOrigin = "anonymous";
  let resolveImage!: (image: HTMLImageElement) => void;
  let rejectImage!: (error: Error) => void;
  const entry: ImageLoadCacheEntry = {
    image,
    consumers: 0,
    settled: false,
    promise: new Promise<HTMLImageElement>((resolve, reject) => {
      resolveImage = resolve;
      rejectImage = reject;
    }),
    cancel: () => {},
  };

  const clearHandlers = () => {
    image.onload = null;
    image.onerror = null;
  };
  entry.cancel = () => {
    if (entry.settled) return;
    entry.settled = true;
    clearHandlers();
    if (imageLoadCache.get(url) === entry) imageLoadCache.delete(url);
    image.src = "";
    rejectImage(abortError());
  };
  image.onload = () => {
    if (entry.settled) return;
    entry.settled = true;
    clearHandlers();
    resolveImage(image);
    trimImageCache();
  };
  image.onerror = () => {
    if (entry.settled) return;
    entry.settled = true;
    entry.failedAt = Date.now();
    clearHandlers();
    // Keep the entry as a negative cache hit for NEGATIVE_IMAGE_CACHE_TTL_MS.
    // Deleting here re-requested permanently-404ing URLs on every re-render
    // and re-fired Sentry through loadOptionalImage (findings 19 / 32 / 33).
    rejectImage(new Error("image_load_failed"));
    trimImageCache();
  };
  return entry;
}

async function loadImage(
  url: string,
  signal?: AbortSignal,
): Promise<HTMLImageElement> {
  throwIfAborted(signal);
  let entry = imageLoadCache.get(url);
  // Honour negative-cache TTL: within the window, re-reject without a new
  // network request; past the window, drop the entry and try again.
  if (
    entry?.failedAt != null &&
    Date.now() - entry.failedAt >= NEGATIVE_IMAGE_CACHE_TTL_MS
  ) {
    imageLoadCache.delete(url);
    entry = undefined;
  }
  if (!entry) {
    entry = createImageCacheEntry(url);
    imageLoadCache.set(url, entry);
    entry.image.src = url;
  } else {
    imageLoadCache.delete(url);
    imageLoadCache.set(url, entry);
  }
  entry.consumers += 1;
  try {
    return await waitForPromise(entry.promise, signal);
  } finally {
    entry.consumers -= 1;
    if (!entry.settled && entry.consumers === 0 && signal?.aborted) {
      entry.cancel();
    }
    trimImageCache();
  }
}

/**
 * Drops a negative-cache entry so the next load re-requests the URL instead of
 * replaying the cached rejection for the rest of NEGATIVE_IMAGE_CACHE_TTL_MS.
 * Only failed entries are touched — an in-flight or successfully loaded entry
 * must never be evicted out from under its consumers.
 *
 * This exists for the preview's retry affordance (audit L4-21): a generated
 * photo is already paid for, so retrying must re-read that exact asset — and
 * without eviction a click inside the TTL is a silent no-op.
 */
export function evictFailedImage(url: string): void {
  const entry = imageLoadCache.get(url);
  if (entry?.failedAt != null) imageLoadCache.delete(url);
}

/**
 * A post that ASKS for a photo must not render without one. `loadOptionalImage`
 * degrades a failed load to `null` so shared loading stays abort-safe, but at
 * the still/scene entry points that degradation told consumers a broken post
 * was "ready" — a 503'd generated (paid) asset produced a blank-photo layout
 * that reported success, armed export, and never tripped the broken-photo UI
 * built for exactly this failure (audit L4-21). The logo stays optional.
 */
function assertRequestedPhotoLoaded(
  input: Pick<RenderPostInput, "photoUrl">,
  loaded: LoadedPostImages,
): void {
  if (input.photoUrl && !loaded.photo) throw new Error("photo_load_failed");
}

const IMAGE_TELEMETRY_URL_MAX_LENGTH = 300;
/** Trailing characters preserved when a URL exceeds the cap. */
const IMAGE_TELEMETRY_URL_TAIL_LENGTH = 8;
/** Leading characters kept; the 12-char reserve covers the tail + ellipsis. */
const IMAGE_TELEMETRY_URL_HEAD_LENGTH = IMAGE_TELEMETRY_URL_MAX_LENGTH - 12;

/**
 * Strips credentials from an image URL, then caps its length, before the URL
 * is used anywhere in telemetry. Two kinds of credential can ride along:
 * signed bucket URLs (S3, R2, GCS) carry SigV4 signature / expiry params in
 * the query string, and any URL may carry `user:password@` userinfo. Both are
 * effectively bearer credentials for the object and must not leave the
 * client, whether in a Sentry payload or the backend error-log POST (which is
 * not scrubbed the way the Sentry mirror is).
 *
 * Userinfo is removed before the cap, not after, so an over-long userinfo
 * cannot survive by having its terminating `@` truncated away.
 *
 * The cap keeps the value bounded because it becomes part of logError's
 * debounce key — but a head-only truncation would defeat that key: two URLs
 * sharing a prefix longer than the cap would truncate to the same string and
 * collapse back into one report, which is the exact bug that folding the URL
 * into the message was added to fix. Keeping a tail preserves the
 * distinguishing part (typically the filename).
 */
function sanitizeImageUrlForTelemetry(url: string): string {
  const cutIndex = [url.indexOf("?"), url.indexOf("#")]
    .filter((index) => index >= 0)
    .sort((left, right) => left - right)[0];
  const stripped = (cutIndex === undefined ? url : url.slice(0, cutIndex))
    // scheme://user:pass@host → scheme://host
    .replace(/^([a-z][a-z0-9+.-]*:\/\/)[^/@]*@/i, "$1");
  return stripped.length > IMAGE_TELEMETRY_URL_MAX_LENGTH
    ? `${stripped.slice(0, IMAGE_TELEMETRY_URL_HEAD_LENGTH)}…${stripped.slice(
        -IMAGE_TELEMETRY_URL_TAIL_LENGTH,
      )}`
    : stripped;
}

/**
 * Loads an optional (non-blocking) image for the post — the dish photo or
 * the business logo — and degrades to `null` on any failure other than an
 * abort. A missing CORS header fails the load outright (crossOrigin =
 * "anonymous" doesn't merely taint the canvas — it rejects), so callers
 * treat a failed optional image as absent rather than failing the whole
 * render. AbortError must still propagate: aborts fire on every re-render
 * and scroll, so they are expected and not logged.
 *
 * Reports the failure to Sentry (fire-and-forget, already per-signature
 * debounced by logError) so a bucket-wide CORS misconfiguration is visible
 * on-call instead of silently degrading every card in production.
 *
 * Exported so the composer can reuse the same cache entry the renderer will
 * use. Calling it twice for one URL costs one network request: `loadImage`
 * dedupes by URL and both callers await the same promise.
 */
export function loadOptionalImage(
  url: string,
  asset: "photo" | "logo",
  signal?: AbortSignal,
): Promise<HTMLImageElement | null> {
  return loadImage(url, signal).catch((error) => {
    if ((error as Error).name === "AbortError") throw error;
    const cause = error instanceof Error ? error : new Error(String(error));
    const safeUrl = sanitizeImageUrlForTelemetry(url);
    // One report per URL per session. Negative-cache rejections re-enter this
    // catch on every render; without the set, a single permanently-404ing
    // asset re-spams Sentry at the re-render rate (findings 19 / 33).
    // logError's debounce key is component::function::message. image.onerror
    // always throws the same static "image_load_failed" message, so without
    // the URL folded into the message here, every distinct broken image
    // within the debounce window collapses into ONE report — exactly the
    // shape of a bucket-wide CORS/S3 outage, where on-call would see a
    // single event with one example URL instead of the real blast radius.
    // `cause` keeps the original onerror failure (and its stack) linked so
    // the Sentry trace still points at the real failure site.
    if (!reportedImageLoadFailures.has(url)) {
      reportedImageLoadFailures.add(url);
      void logError(
        new Error(`image_load_failed: ${safeUrl}`, { cause }),
        "renderPost",
        "loadOptionalImage",
        { asset, url: safeUrl },
      );
    }
    return null;
  });
}

/**
 * The composition families that reproduce a pre-rewrite template, and are
 * therefore reachable ONLY from the branch that pins one to its template.
 *
 * Derived from `LEGACY_COMPOSITION_FOR_TEMPLATE` rather than spelled out, so a
 * fourth legacy family added for a fourth retired template is covered the
 * moment it is mapped, without a second list to remember to update.
 */
const LEGACY_COMPOSITIONS: ReadonlySet<CompositionId> = new Set(
  Object.values(LEGACY_COMPOSITION_FOR_TEMPLATE),
);

/**
 * Resolve which art-direction kit and which composition this render paints
 * with.
 *
 * `hasPhoto` is the truth the SCENE will see, not `input.photoUrl` — the photo
 * may have failed to load, and `renderPost` degrades that to the no-photo path.
 * Deciding from the URL instead would lay a post out around a photograph that
 * is not there whenever a CDN 404s or a CORS header goes missing. That is why
 * this is called after the image loads rather than before.
 *
 * `photo` is the analysis of the image that actually loaded, or null when there
 * is no photo or it could not be read back (a tainted canvas, an environment
 * with no 2D context). Null is passed through rather than defaulted: the
 * chooser's photo branches are written to be skipped when nothing is known, and
 * a fabricated "calm, no negative space" would read identically to a measured
 * one.
 *
 * The kit-only fallback goes through `chooseComposition` rather than naming a
 * second default, so there is exactly one module that decides what a given
 * piece of content should look like. Two of that function's signals are not
 * knowable here: `play` and `hasDiscount` belong to the composer, which passes
 * an explicit `composition` once the operator has one. Their absence only ever
 * costs a discount post its `badgeHero` treatment on this fallback path — it
 * cannot produce a photo layout without a photo, which is the failure this
 * fallback exists to prevent.
 *
 * A legacy composition is REFUSED on the kit path, and falls through to that
 * same chooser. The three legacy families exist to reproduce the pre-rewrite
 * templates byte-faithfully, so pairing one with a kit that carries a texture
 * paints a full-bleed grain or halftone field over exactly the fidelity they
 * exist for. That pairing was believed unreachable because the no-kit branch
 * below pins both halves together and all three legacy-mapped kits declare
 * `texture: "none"` — but that holds by DATA, not by construction, and the
 * kit branch used `input.composition` verbatim. Nothing upstream closed it
 * either: `isCompositionId` accepts all nine ids, `seedArtDirection` resolves
 * the stored kit and the stored composition independently, and the backend's
 * ident normalizer is charset- and length-bounded only — deliberately, so the
 * frontend-owned id set can grow each wave without a Go deploy. So a stored
 * `{ kit: "ticket", composition: "legacyEditorial" }` round-tripped and
 * rendered. This is the construction half, and it lives here rather than on
 * the read-back path because `renderPost` is also reached by the preview, the
 * feed card, the thumbnails and the download/share pack, none of which pass
 * through the composer's state at all.
 *
 * Only the LAYOUT is refused, never the kit: a kit is a legitimate operator
 * choice on any post, and it is the legacy FAMILY that has a fidelity
 * contract to keep.
 *
 * EXPORTED because this precedence has exactly one home. `PostEditorDrawer`
 * has to answer the same question to decide which slot editors to offer — the
 * band set is a property of the resolved composition — and a second copy of
 * the branch there is a rule that can drift from the one the render obeys,
 * leaving the editor collecting text no post will paint. Callers that only
 * need the band set still map `compositionId` themselves; what they must not
 * do is re-derive it.
 */
export function resolveArtDirection(
  input: RenderPostInput,
  hasPhoto: boolean,
  photo: PhotoSignals | null = null,
): { kitId: KitId; compositionId: CompositionId } {
  if (input.kit) {
    const chosen =
      input.composition && !LEGACY_COMPOSITIONS.has(input.composition)
        ? input.composition
        : null;
    return {
      kitId: input.kit,
      compositionId:
        chosen ??
        chooseComposition(
          contentSignalsFrom({
            play: "",
            slots: input.slots,
            hasPhoto,
            hasDiscount: false,
          }),
          photo,
          input.kit,
          formatForInput(input.aspect),
        ),
    };
  }
  const legacyStyle = input.template?.id ?? "editorial";
  return {
    kitId: kitForLegacyTemplate(legacyStyle),
    compositionId: LEGACY_COMPOSITION_FOR_TEMPLATE[legacyStyle],
  };
}

/**
 * The crop this render uses: the caller's if they gave one, otherwise the
 * subject's centre, otherwise dead centre.
 *
 * An ABSENT crop means "decide for me" — it is not the same as an explicit
 * `DEFAULT_CROP`, which means "the operator looked at this and centred it".
 * The two must not be conflated: comparing against `DEFAULT_CROP` here would
 * silently override every deliberately-centred post the moment a subject box
 * landed off-centre.
 */
function resolveCrop(
  crop: MarketingCrop | undefined,
  analysis: PhotoAnalysis | null,
): MarketingCrop {
  if (crop) return crop;
  if (!analysis) return DEFAULT_CROP;
  return { x: analysis.focal.x, y: analysis.focal.y, zoom: 1 };
}

/**
 * Loaded bitmaps for one post. A failed load is `null`, not thrown: the scene
 * degrades to the no-photo / no-logo path (see `loadOptionalImage`).
 *
 * Distinct from `SceneImages` (URL-keyed for the walker) because the loader
 * does not yet know which URLs will appear on nodes — that is decided by
 * `buildPostScene` after it filters failed loads out of `photoUrl`/`logoUrl`.
 */
export interface LoadedPostImages {
  photo: HTMLImageElement | null;
  logo: HTMLImageElement | null;
}

/**
 * Fonts and bitmaps, in the order the renderer has always loaded them.
 *
 * Extracted so the motion pipeline reuses this rather than forking it. Two
 * behaviours in particular must not be forked: `ensureFonts` swallows a font
 * rejection (embedded browsers reject it and canvas falls back), and
 * `loadOptionalImage` degrades a failed photo or logo to `null` while still
 * propagating an abort. A second loader that got either wrong would make video
 * export fail on exactly the buckets the still export survives.
 */
export async function loadPostImages(
  input: Pick<RenderPostInput, "photoUrl" | "logoUrl">,
  signal?: AbortSignal,
): Promise<LoadedPostImages> {
  await ensureFonts(signal);
  throwIfAborted(signal);
  const [photo, logo] = await Promise.all([
    input.photoUrl
      ? loadOptionalImage(input.photoUrl, "photo", signal)
      : Promise.resolve(null),
    input.logoUrl?.trim()
      ? loadOptionalImage(input.logoUrl.trim(), "logo", signal)
      : Promise.resolve(null),
  ]);
  throwIfAborted(signal);
  return { photo, logo };
}

/**
 * The one measurer. Sets and restores `ctx.font` around a single `measureText`,
 * and takes the font as a finished CSS string so there is no second place that
 * could assemble one differently from the one the walker paints with.
 */
export function measureWith(ctx: CanvasRenderingContext2D): MeasureFn {
  return (text, cssFont) => {
    const previous = ctx.font;
    ctx.font = cssFont;
    const measured = ctx.measureText(text).width;
    ctx.font = previous;
    return measured;
  };
}

/**
 * URL-keyed structural drawables for `renderScene`, matching PhotoNode.url /
 * LogoNode.url. Only entries that both loaded and survived the degradation
 * filter are keyed. Each bitmap is wrapped as a `SceneImage` so the walker
 * never reads `naturalWidth` off the source directly (worker-safe bitmaps
 * only expose `width` / `height`).
 */
export function sceneImagesFrom(
  input: Pick<RenderPostInput, "photoUrl" | "logoUrl">,
  images: LoadedPostImages,
): SceneImages {
  const photoUrl = images.photo ? input.photoUrl : "";
  const logoUrl = images.logo ? (input.logoUrl ?? "") : "";
  const out: SceneImages = {};
  if (images.photo && photoUrl) out[photoUrl] = toSceneImage(images.photo);
  if (images.logo && logoUrl) out[logoUrl] = toSceneImage(images.logo);
  return out;
}

/**
 * Turn a creative plus its loaded bitmaps into a flat, z-ordered `Scene`.
 *
 * `images` — not `input.photoUrl` — is the truth about what the scene can show.
 * A photo that failed to load degrades the whole post to the no-photo path, and
 * both the scene and the art-direction resolution read the same pair of strings
 * so neither can believe in an image the other does not have.
 *
 * Canvas-free: text width arrives through `frame.measure`. That is what lets the
 * motion pipeline build a scene against an `OffscreenCanvas` context, and what
 * lets this be tested under a harness with no real canvas.
 *
 * Wave 2 decisions (analysis, subject crop, pre-resolved scrim luma, palette
 * harmonization) stay here so still and motion share one path — not a fork.
 */
export function buildPostScene(
  input: RenderPostInput,
  images: LoadedPostImages,
  frame: { width: number; height: number; measure: MeasureFn },
): Scene {
  // A failed photo or logo load degrades to the no-photo / no-logo path. Both
  // the scene and the art-direction resolution below read this same pair of
  // strings, so neither can believe in an image the other does not have.
  const photoUrl = images.photo ? input.photoUrl : "";
  const logoUrl = images.logo ? (input.logoUrl ?? "") : "";
  const format = formatForInput(input.aspect);

  // Once per photo URL, memoized beside the image cache. Everything downstream
  // — the chooser, the palette, the crop, the grade, the scrim — reads this one
  // object, so a photo is never measured twice in a render and never measured
  // differently by two consumers.
  const analysis: PhotoAnalysis | null = photoUrl
    ? photoAnalysisFor(photoUrl, images.photo)
    : null;

  const crop = resolveCrop(input.crop, analysis);

  const { kitId, compositionId } = resolveArtDirection(
    input,
    photoUrl.trim().length > 0,
    photoSignalsFrom(analysis),
  );
  const composition = COMPOSITIONS[compositionId];

  // Which source pixels end up under the scrim band. Three frames meet here —
  // canvas, cover-fit window, source — and `mapCanvasRectToSource` is what keeps
  // them straight; measuring the canvas band against the source grid directly
  // is right only for an uncropped full-bleed photo whose aspect matches the
  // canvas. Null means no photo lands under the band at all (splitPanel), and
  // the scrim stays adaptive so nothing is decided from a measurement that does
  // not exist.
  let scrimLuma: number | null = null;
  if (analysis && images.photo) {
    const photo = images.photo;
    const imageArea = composition.imageAreaFor(format);
    const dst = resolveRect(imageArea, frame.width, frame.height);
    const band = mapCanvasRectToSource(
      scrimRectFor(composition, format),
      imageArea,
      fitCover(photo.naturalWidth, photo.naturalHeight, dst.w, dst.h, crop),
      { w: photo.naturalWidth, h: photo.naturalHeight },
    );
    if (band) scrimLuma = meanOfCells(analysis.luma, band);
  }

  return buildScene({
    kit: KITS[kitId],
    composition,
    format,
    // Thumbnails solve at their own size rather than being solved at 1080 and
    // squeezed: every composition token is a fraction of the canvas, so the
    // scaled scene is the full-size one, proportionally.
    canvasW: frame.width,
    canvasH: frame.height,
    photoUrl,
    logoUrl,
    crop,
    analysis,
    scrimLuma,
    slots: input.slots,
    palette: harmonizePalette(
      { primary: input.palette.primary, secondary: input.palette.secondary },
      analysis?.dominantColors ?? [],
    ),
    measure: frame.measure,
  });
}

/**
 * The scene a render would paint, without painting it.
 *
 * The print walker consumes a `Scene`, not a canvas, so it needs everything
 * `renderPost` does up to `buildPostScene` and nothing after it. Factoring this
 * out rather than exporting `buildScene`'s inputs from the composer keeps ONE
 * place that knows how a `RenderPostInput` becomes a scene — fonts, image loads,
 * art-direction resolution, photo analysis and the measurer are all shared, so a
 * tent and a feed post cannot disagree about which composition the content chose.
 *
 * A 2D context is still needed, purely as a text measurer: `solveStack` wraps
 * against real glyph widths and there is no other way to get them. The canvas is
 * 1x1 and is released before returning.
 */
export async function renderPostScene(
  input: RenderPostInput,
  signal?: AbortSignal,
): Promise<Scene> {
  const format = formatForInput(input.aspect);
  const loaded = await loadPostImages(input, signal);
  assertRequestedPhotoLoaded(input, loaded);

  const measureCanvas = document.createElement("canvas");
  measureCanvas.width = 1;
  measureCanvas.height = 1;
  const ctx = measureCanvas.getContext("2d");
  if (!ctx) throw new Error("canvas_2d_unavailable");

  const scene = buildPostScene(input, loaded, {
    width: format.px.w,
    height: format.px.h,
    measure: measureWith(ctx),
  });

  measureCanvas.width = 0;
  measureCanvas.height = 0;
  return scene;
}

/**
 * Preview and export both render from this single, exact input contract.
 * Pass `targetWidth` (e.g. LIBRARY_THUMBNAIL_WIDTH) for library thumbnails;
 * omit it (or pass only AbortSignal) for full-resolution export/editor previews.
 *
 * Composed from the three functions above, in the order it has always run them:
 * fonts and bitmaps, then the canvas, then the scene, then the paint. The motion
 * pipeline composes the same three differently — its own surface, its own frame
 * loop — and shares every decision that reaches a pixel.
 */
export async function renderPost(
  input: RenderPostInput,
  signalOrOptions?: RenderPostSignalOrOptions,
): Promise<HTMLCanvasElement> {
  const { signal, targetWidth, exportScale } =
    resolveRenderOptions(signalOrOptions);
  const format = formatForInput(input.aspect);
  const { w: width, h: height } = formatDimensions(
    format,
    targetWidth,
    exportScale,
  );
  const loaded = await loadPostImages(input, signal);
  assertRequestedPhotoLoaded(input, loaded);

  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("canvas_2d_unavailable");

  const scene = buildPostScene(input, loaded, {
    width,
    height,
    measure: measureWith(ctx),
  });
  renderScene(ctx, scene, sceneImagesFrom(input, loaded));
  return canvas;
}

export async function renderPostToBlob(
  input: RenderPostInput,
  options?: RenderPostOptions,
): Promise<Blob> {
  const canvas = await renderPost(input, options);
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      // Release the backing store once we have the blob (thumbnail path).
      canvas.width = 0;
      canvas.height = 0;
      if (blob) resolve(blob);
      else reject(new Error("png_export_failed"));
    }, "image/png");
  });
}
