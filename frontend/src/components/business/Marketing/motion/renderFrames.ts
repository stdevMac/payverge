import type { Scene } from "../scene/types";
import { evaluateSceneAt } from "./evaluate";
import type { FrameSink } from "./frameSink";
import { MOTION_FPS } from "./presets";
import type { AnimTrack } from "./tracks";

/**
 * The frame loop.
 *
 * TERMINATION IS THE LOAD-BEARING PROPERTY HERE, not throughput. A synchronous
 * infinite loop does NOT surface as a jest timeout: the per-test deadline is a
 * timer, and a sync loop never yields the event loop for it to fire, so the run
 * produces no output at all and has to be killed. `frameCountFor` therefore
 * resolves an INTEGER count up front, clamped into [1, MAX_FRAMES], and the loop
 * iterates that integer. Nothing here accumulates a float and compares it to a
 * bound.
 *
 * Everything effectful is injected — the sink, the paint step, the yield — so
 * this is fully testable in a harness with no canvas and no codec.
 */

/**
 * Thirty seconds at 30fps. Five times the longest clip the product ships, and a
 * hard stop on any arithmetic that produced a nonsense duration.
 */
export const MAX_FRAMES = 900;

/** Frames rendered between yields to the host, on the main-thread path. */
const DEFAULT_YIELD_EVERY = 1;

export interface RenderFramesInput {
  /** Already built and already prepared. Never rebuilt inside the loop. */
  scene: Scene;
  tracks: readonly AnimTrack[];
  durationMs: number;
  fps: number;
  sink: FrameSink;
  /** Paints the evaluated scene onto whatever surface the sink reads. */
  paint: (scene: Scene, frameIndex: number) => void;
  onProgress?: (done: number, total: number) => void;
  signal?: AbortSignal;
  yieldEvery?: number;
  /**
   * How the loop gives the host a turn. Defaults to a macrotask, which is what
   * lets the browser paint and keeps the tab responsive during a main-thread
   * export. Tests inject a no-op so the suite does not spend 180 timer ticks.
   */
  yieldTo?: () => Promise<void>;
}

function abortError(): Error {
  if (typeof DOMException === "function") {
    return new DOMException("motion_export_aborted", "AbortError");
  }
  const error = new Error("motion_export_aborted");
  error.name = "AbortError";
  return error;
}

/**
 * How many frames a clip of `durationMs` at `fps` is.
 *
 * Total, integral, and clamped at both ends. A malformed duration resolves to a
 * single frame — the smallest honest answer — while an INFINITE one is capped at
 * `MAX_FRAMES` instead, because infinity is the shape a runaway computation
 * takes and the cap is what makes the loop provably terminate.
 */
export function frameCountFor(durationMs: number, fps: number): number {
  const safeFps = Number.isFinite(fps) && fps > 0 ? fps : MOTION_FPS;
  if (!Number.isFinite(durationMs) || durationMs <= 0) {
    return durationMs === Number.POSITIVE_INFINITY ? MAX_FRAMES : 1;
  }
  return Math.max(
    1,
    Math.min(MAX_FRAMES, Math.round((durationMs / 1000) * safeFps)),
  );
}

const macrotask = (): Promise<void> =>
  new Promise((resolve) => {
    setTimeout(resolve, 0);
  });

export async function renderFrames(input: RenderFramesInput): Promise<Blob> {
  const {
    scene,
    tracks,
    durationMs,
    fps,
    sink,
    paint,
    onProgress,
    signal,
    yieldEvery = DEFAULT_YIELD_EVERY,
    yieldTo = macrotask,
  } = input;

  const total = frameCountFor(durationMs, fps);
  const safeFps = Number.isFinite(fps) && fps > 0 ? fps : MOTION_FPS;
  const cadence = Number.isFinite(yieldEvery) && yieldEvery > 0 ? yieldEvery : 1;

  try {
    for (let frameIndex = 0; frameIndex < total; frameIndex += 1) {
      if (signal?.aborted) throw abortError();
      const timeMs = (frameIndex * 1000) / safeFps;
      paint(evaluateSceneAt(scene, tracks, timeMs), frameIndex);
      await sink.write(frameIndex, Math.round(frameIndex * (1_000_000 / safeFps)));
      onProgress?.(frameIndex + 1, total);
      if ((frameIndex + 1) % cadence === 0) await yieldTo();
    }
    if (signal?.aborted) throw abortError();
    return await sink.finish();
  } catch (error) {
    // One cancel for every failure mode — abort, a throwing paint step, an
    // encoder error — so no path can leave a codec open holding GPU memory.
    // `FrameSink.cancel` is documented as safe to call twice for this reason.
    sink.cancel();
    throw error;
  }
}
