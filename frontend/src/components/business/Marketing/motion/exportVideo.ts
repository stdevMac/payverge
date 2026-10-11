import { renderScene, type SceneImages } from "../scene/renderScene";
import type { Scene } from "../scene/types";
import type { NormalizedRect } from "../templates/types";
import {
  aspectDimensions,
  buildPostScene,
  loadPostImages,
  measureWith,
  renderPostToBlob,
  sampleRegionLuminance,
  sceneImagesFrom,
  type LoadedPostImages,
  type RenderPostInput,
} from "../templates/renderPost";
import {
  detectMotionSupport,
  type MotionEncodePath,
  type MotionRequirement,
} from "./capabilities";
import type { FrameSinkFactory } from "./frameSink";
import { mediaRecorderSinkFactory } from "./mediaRecorderSink";
import { prepareSceneForMotion } from "./prepareScene";
import {
  MOTION_DURATION_MS,
  MOTION_FPS,
  tracksForScene,
  type MotionPresetId,
} from "./presets";
import { frameCountFor, renderFrames } from "./renderFrames";
import { webCodecsSinkFactory } from "./webcodecsSink";

/**
 * Turn the post an operator is looking at into a six-second silent video.
 *
 * Preferred path: WebCodecs `VideoEncoder` + `mp4-muxer` → H.264/MP4 on an
 * `OffscreenCanvas`. Fallback: `MediaRecorder` + `captureStream` → WebM on an
 * `HTMLCanvasElement` when WebCodecs is absent.
 *
 * The pipeline, and the reason for each step's position:
 *
 *   loadPostImages  — the SAME loader the still export uses, so a CORS-less
 *                     bucket degrades identically in both.
 *   buildPostScene  — ONCE. Never inside the loop: it re-measures and re-wraps
 *                     every band, so per-frame it would cost 180x and re-break
 *                     lines mid-clip.
 *   photo-only paint— the bitmap the walker samples when it reaches the scrim,
 *                     and nothing more. Z is photo 0 < scrim 10 < texture 15, so
 *                     background + photo IS that bitmap. Painting the whole
 *                     scene first would read the scrim's own output back.
 *   prepareScene    — freezes the adaptive scrim, so no frame calls getImageData
 *                     and the scrim does not breathe as the photo pushes in.
 *   tracksForScene  — the preset reads the built scene and emits its tracks.
 *   renderFrames    — the bounded loop.
 *
 * Every collaborator is injectable. That is not ceremony: jsdom has no
 * `VideoEncoder`, no `OffscreenCanvas` and no real 2D context, so injection is
 * the only way any of this is testable at all.
 */

export class MotionUnsupportedError extends Error {
  readonly missing: MotionRequirement[];
  constructor(missing: MotionRequirement[]) {
    super("motion_unsupported");
    this.name = "MotionUnsupportedError";
    this.missing = missing;
  }
}

interface ExportSurface {
  /** What the sink reads each frame. */
  source: CanvasImageSource;
  ctx: CanvasRenderingContext2D;
  width: number;
  height: number;
  /** Drops the backing store. Called exactly once, on every exit path. */
  release: () => void;
}

interface MotionSupportSnapshot {
  supported: boolean;
  missing: MotionRequirement[];
  /** Encode path when known; production defaults pick surface + sink from this. */
  path?: MotionEncodePath | null;
}

export interface ExportVideoDeps {
  detectSupport: () => MotionSupportSnapshot;
  loadImages: (
    input: Pick<RenderPostInput, "photoUrl" | "logoUrl">,
    signal?: AbortSignal,
  ) => Promise<LoadedPostImages>;
  buildScene: typeof buildPostScene;
  paintScene: (
    ctx: CanvasRenderingContext2D,
    scene: Scene,
    images: SceneImages,
  ) => void;
  sampleLuminance: (
    ctx: CanvasRenderingContext2D,
    width: number,
    height: number,
    rect: NormalizedRect,
  ) => number;
  createSurface: (width: number, height: number) => ExportSurface;
  createSink: FrameSinkFactory;
  renderPoster: (input: RenderPostInput, signal?: AbortSignal) => Promise<Blob>;
}

export interface ExportVideoInput {
  renderInput: RenderPostInput;
  preset: MotionPresetId;
  durationMs?: number;
  fps?: number;
  /** 0..1. Fires once per frame. */
  onProgress?: (fraction: number) => void;
  signal?: AbortSignal;
  /** Tests inject fakes; production omits this entirely. */
  deps?: Partial<ExportVideoDeps>;
  /** Forwarded to the frame loop; tests inject a no-op. */
  yieldTo?: () => Promise<void>;
}

export interface ExportVideoResult {
  video: Blob;
  /**
   * The RESTING still, rendered by the ordinary static path — not the last video
   * frame. The Library already re-renders every row's snapshot through
   * `PostPreview`, so this is the same artefact those rows show, and it is what
   * keeps a video row's thumbnail identical to the still it was made from.
   */
  poster: Blob;
}

function offscreenSurface(width: number, height: number): ExportSurface {
  const canvas = new OffscreenCanvas(width, height);
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("offscreen_2d_unavailable");
  return {
    source: canvas as unknown as CanvasImageSource,
    ctx: ctx as unknown as CanvasRenderingContext2D,
    width,
    height,
    release: () => {
      canvas.width = 0;
      canvas.height = 0;
    },
  };
}

/**
 * MediaRecorder path needs `HTMLCanvasElement.captureStream`. OffscreenCanvas
 * is not uniformly capturable, so this surface is a real (off-DOM) canvas.
 */
function htmlCanvasSurface(width: number, height: number): ExportSurface {
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("canvas_2d_unavailable");
  return {
    source: canvas,
    ctx,
    width,
    height,
    release: () => {
      canvas.width = 0;
      canvas.height = 0;
    },
  };
}

/** Surface + sink pair for the chosen encode path. */
function productionDepsForPath(
  path: MotionEncodePath | null | undefined,
): Pick<ExportVideoDeps, "createSurface" | "createSink"> {
  if (path === "mediarecorder") {
    return {
      createSurface: htmlCanvasSurface,
      // Lazy factory construction so jsdom module load never touches MediaRecorder.
      createSink: (config) => mediaRecorderSinkFactory()(config),
    };
  }
  return {
    createSurface: offscreenSurface,
    createSink: (config) => webCodecsSinkFactory()(config),
  };
}

const DEFAULT_DEPS: ExportVideoDeps = {
  detectSupport: () => detectMotionSupport(),
  loadImages: loadPostImages,
  buildScene: buildPostScene,
  paintScene: renderScene,
  sampleLuminance: sampleRegionLuminance,
  createSurface: offscreenSurface,
  // Lazy: constructing the factory at module load would touch `VideoEncoder`
  // and crash under jsdom before any test can inject fakes.
  createSink: (config) => webCodecsSinkFactory()(config),
  renderPoster: (input, signal) =>
    renderPostToBlob(input, signal ? { signal } : undefined),
};

export async function exportVideo(
  input: ExportVideoInput,
): Promise<ExportVideoResult> {
  const detect = input.deps?.detectSupport ?? DEFAULT_DEPS.detectSupport;
  const support = detect();
  if (!support.supported) throw new MotionUnsupportedError(support.missing);

  const path = support.path ?? "webcodecs";
  // Path defaults first so a MediaRecorder browser gets HTML canvas + MR sink;
  // test-injected deps still win last.
  const deps: ExportVideoDeps = {
    ...DEFAULT_DEPS,
    ...productionDepsForPath(path),
    ...input.deps,
  };

  const durationMs = input.durationMs ?? MOTION_DURATION_MS;
  const fps = input.fps ?? MOTION_FPS;
  const { w: width, h: height } = aspectDimensions(input.renderInput.aspect);

  const loaded = await deps.loadImages(input.renderInput, input.signal);
  // renderScene looks up bitmaps by node URL; buildPostScene wants the
  // {photo, logo} pair. Convert once so paint and build never disagree.
  const sceneImages = sceneImagesFrom(input.renderInput, loaded);
  const surface = deps.createSurface(width, height);
  try {
    const built = deps.buildScene(input.renderInput, loaded, {
      width,
      height,
      measure: measureWith(surface.ctx),
    });

    // Paint the photo alone, so the scrim is measured against the pixels the
    // walker would have measured it against — see the header note on Z order.
    deps.paintScene(
      surface.ctx,
      { ...built, nodes: built.nodes.filter((node) => node.kind === "photo") },
      sceneImages,
    );
    const scene = prepareSceneForMotion(
      built,
      (rect) => deps.sampleLuminance(surface.ctx, width, height, rect),
      { hasPhoto: loaded.photo != null },
    );

    const tracks = tracksForScene(scene, input.preset, durationMs);
    const sink = await deps.createSink({
      source: surface.source,
      width,
      height,
      fps,
      frameCount: frameCountFor(durationMs, fps),
    });

    const video = await renderFrames({
      scene,
      tracks,
      durationMs,
      fps,
      sink,
      paint: (evaluated) =>
        deps.paintScene(surface.ctx, evaluated, sceneImages),
      ...(input.onProgress
        ? {
            onProgress: (done: number, total: number) =>
              input.onProgress?.(done / total),
          }
        : {}),
      ...(input.signal ? { signal: input.signal } : {}),
      ...(input.yieldTo ? { yieldTo: input.yieldTo } : {}),
    });

    const poster = await deps.renderPoster(input.renderInput, input.signal);
    return { video, poster };
  } finally {
    surface.release();
  }
}
