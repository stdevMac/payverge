/**
 * Feature detection for the motion export path.
 *
 * Takes the global scope as an ORDINARY OBJECT ARGUMENT rather than reading
 * `globalThis` directly, for the same reason the pixel functions in Wave 2 take
 * an `ImageData` shape rather than a canvas: it is the only way to test the
 * detection in a harness that has none of the APIs being detected. jsdom has no
 * `VideoEncoder`, no `VideoFrame` and no `OffscreenCanvas`, so the real-world
 * "supported" branch is unreachable in jest unless it can be handed a scope.
 *
 * Two encode paths, preferred in order:
 * 1. WebCodecs + OffscreenCanvas → H.264/MP4 (preferred)
 * 2. MediaRecorder + HTMLCanvasElement.captureStream → WebM (fallback)
 */

/** Constructors the WebCodecs sink cannot run without. */
const WEBCODECS_REQUIRED = [
  "VideoEncoder",
  "VideoFrame",
  "OffscreenCanvas",
] as const;

/** APIs the MediaRecorder fallback cannot run without. */
const MEDIA_RECORDER_REQUIRED = ["MediaRecorder"] as const;

export type WebCodecsRequirement = (typeof WEBCODECS_REQUIRED)[number];
type MediaRecorderRequirement = (typeof MEDIA_RECORDER_REQUIRED)[number];
/** Union kept for MotionUnsupportedError / operator-facing copy. */
export type MotionRequirement =
  | WebCodecsRequirement
  | MediaRecorderRequirement
  | "captureStream";

export type MotionEncodePath = "webcodecs" | "mediarecorder";

export interface MotionSupport {
  supported: boolean;
  /** Encode path when supported; null when neither path works. */
  path: MotionEncodePath | null;
  /** Every missing constructor for the preferred path that failed, not just the first. */
  missing: MotionRequirement[];
}

function missingFrom(
  scope: Record<string, unknown>,
  names: readonly string[],
): string[] {
  return names.filter((name) => typeof scope[name] !== "function");
}

/**
 * True when an HTMLCanvasElement prototype (or a stand-in on the scope) exposes
 * `captureStream`. Tests can pass `{ HTMLCanvasElement: { prototype: { captureStream: fn } } }`.
 */
function hasCaptureStream(scope: Record<string, unknown>): boolean {
  const Ctor = scope.HTMLCanvasElement as
    | { prototype?: { captureStream?: unknown } }
    | undefined;
  if (Ctor && typeof Ctor.prototype?.captureStream === "function") return true;
  // Fall back to the real global when the test scope did not stub it.
  if (
    typeof HTMLCanvasElement !== "undefined" &&
    typeof HTMLCanvasElement.prototype?.captureStream === "function"
  ) {
    return true;
  }
  return typeof scope.captureStream === "function";
}

export function detectWebCodecsSupport(
  scope: Record<string, unknown> = globalThis as unknown as Record<
    string,
    unknown
  >,
): { supported: boolean; missing: WebCodecsRequirement[] } {
  const missing = missingFrom(scope, WEBCODECS_REQUIRED) as WebCodecsRequirement[];
  return { supported: missing.length === 0, missing: [...missing] };
}

export function detectMediaRecorderSupport(
  scope: Record<string, unknown> = globalThis as unknown as Record<
    string,
    unknown
  >,
): { supported: boolean; missing: MotionRequirement[] } {
  const missing: MotionRequirement[] = missingFrom(
    scope,
    MEDIA_RECORDER_REQUIRED,
  ) as MediaRecorderRequirement[];
  if (!hasCaptureStream(scope)) missing.push("captureStream");
  return { supported: missing.length === 0, missing: [...missing] };
}

/**
 * Prefer WebCodecs; fall back to MediaRecorder when WebCodecs is incomplete.
 * `missing` lists what the preferred path still needs when neither works, so
 * operator copy can name real constructors.
 */
export function detectMotionSupport(
  scope: Record<string, unknown> = globalThis as unknown as Record<
    string,
    unknown
  >,
): MotionSupport {
  const webcodecs = detectWebCodecsSupport(scope);
  if (webcodecs.supported) {
    return { supported: true, path: "webcodecs", missing: [] };
  }
  const mediaRecorder = detectMediaRecorderSupport(scope);
  if (mediaRecorder.supported) {
    return { supported: true, path: "mediarecorder", missing: [] };
  }
  // Prefer naming WebCodecs gaps — that is the primary ship path.
  return {
    supported: false,
    path: null,
    missing: [...webcodecs.missing],
  };
}
