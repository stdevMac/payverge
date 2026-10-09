import type { FrameSink, FrameSinkConfig, FrameSinkFactory } from "./frameSink";

/**
 * WebM via `MediaRecorder` + `HTMLCanvasElement.captureStream`.
 *
 * Fallback when WebCodecs is absent. Produces WebM (VP8/VP9), not H.264/MP4 —
 * destinations that insist on MP4 may re-transcode or reject. Prefer WebCodecs
 * whenever `detectMotionSupport` reports that path.
 *
 * Architecture notes (why this is not a free plug-in):
 * - `captureStream` lives on `HTMLCanvasElement`; `OffscreenCanvas` is not
 *   uniformly supported for this path, so the export surface must be an
 *   on-document (or at least HTML) canvas.
 * - MediaRecorder records wall-clock time, not discrete frame indices. The
 *   sink paces each `write` so paints land at the intended timestamps.
 * - jsdom has neither API; constructors are injected for contract tests.
 */

export const VIDEO_MIME_WEBM = "video/webm";

const PREFERRED_MIME_TYPES = [
  "video/webm;codecs=vp9",
  "video/webm;codecs=vp8",
  "video/webm",
] as const;

export interface MediaRecorderLike {
  start(timeslice?: number): void;
  stop(): void;
  state: "inactive" | "recording" | "paused";
  ondataavailable: ((event: { data: Blob }) => void) | null;
  onerror: ((event: { error?: Error }) => void) | null;
  onstop: (() => void) | null;
}

interface MediaStreamTrackLike {
  stop(): void;
}

export interface MediaStreamLike {
  getTracks(): MediaStreamTrackLike[];
}

interface CapturableCanvas {
  captureStream(frameRate?: number): MediaStreamLike;
}

export interface MediaRecorderDeps {
  MediaRecorder: {
    new (
      stream: MediaStreamLike,
      options?: { mimeType?: string; videoBitsPerSecond?: number },
    ): MediaRecorderLike;
    isTypeSupported?: (mimeType: string) => boolean;
  };
  /** Wall-clock wait. Inject a no-op in tests that only assert the protocol. */
  waitMs?: (ms: number) => Promise<void>;
  /** Clock for pacing. Defaults to `performance.now` / `Date.now`. */
  nowMs?: () => number;
}

const BITRATE = 6_000_000;

function pickMimeType(
  MediaRecorderCtor: MediaRecorderDeps["MediaRecorder"],
): string {
  const isSupported = MediaRecorderCtor.isTypeSupported?.bind(MediaRecorderCtor);
  if (!isSupported) return VIDEO_MIME_WEBM;
  for (const mime of PREFERRED_MIME_TYPES) {
    if (isSupported(mime)) return mime;
  }
  return VIDEO_MIME_WEBM;
}

function defaultWait(ms: number): Promise<void> {
  if (ms <= 0) return Promise.resolve();
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function defaultNow(): number {
  return typeof performance !== "undefined" && typeof performance.now === "function"
    ? performance.now()
    : Date.now();
}

function isCapturableCanvas(source: CanvasImageSource): source is CapturableCanvas &
  CanvasImageSource {
  return (
    typeof source === "object" &&
    source !== null &&
    "captureStream" in source &&
    typeof (source as CapturableCanvas).captureStream === "function"
  );
}

export function createMediaRecorderSinkFactory(
  deps: MediaRecorderDeps,
): FrameSinkFactory {
  const waitMs = deps.waitMs ?? defaultWait;
  const nowMs = deps.nowMs ?? defaultNow;

  return async (config: FrameSinkConfig): Promise<FrameSink> => {
    if (!isCapturableCanvas(config.source)) {
      throw new Error("mediarecorder_requires_html_canvas_captureStream");
    }

    const mimeType = pickMimeType(deps.MediaRecorder);
    const stream = config.source.captureStream(config.fps);
    const recorder = new deps.MediaRecorder(stream, {
      mimeType,
      videoBitsPerSecond: BITRATE,
    });

    const chunks: Blob[] = [];
    let recorderError: Error | null = null;
    let started = false;
    let closed = false;
    const originMs = nowMs();
    const frameDurationMs = 1000 / config.fps;

    recorder.ondataavailable = (event) => {
      if (event.data && event.data.size > 0) chunks.push(event.data);
    };
    recorder.onerror = (event) => {
      recorderError =
        event.error instanceof Error
          ? event.error
          : new Error("mediarecorder_error");
    };

    const throwIfFailed = () => {
      if (recorderError) throw recorderError;
    };

    const stopTracks = () => {
      for (const track of stream.getTracks()) {
        try {
          track.stop();
        } catch {
          // Track may already be ended.
        }
      }
    };

    return {
      async write(frameIndex, _timestampUs) {
        throwIfFailed();
        if (!started) {
          // timeslice keeps chunks flowing so cancel mid-encode still has data.
          recorder.start(100);
          started = true;
        }
        // Wall-clock pacing: frame N should appear at origin + N * frameDuration.
        const targetMs = originMs + frameIndex * frameDurationMs;
        const delay = targetMs - nowMs();
        if (delay > 0) await waitMs(delay);
        throwIfFailed();
      },

      async finish() {
        throwIfFailed();
        if (!started) {
          // Zero-frame export — still produce an empty container rather than hang.
          return new Blob([], { type: mimeType });
        }
        const blob = await new Promise<Blob>((resolve, reject) => {
          recorder.onstop = () => {
            resolve(new Blob(chunks, { type: mimeType }));
          };
          try {
            if (recorder.state === "recording" || recorder.state === "paused") {
              recorder.stop();
            } else {
              resolve(new Blob(chunks, { type: mimeType }));
            }
          } catch (error) {
            reject(error instanceof Error ? error : new Error(String(error)));
          }
        });
        closed = true;
        stopTracks();
        throwIfFailed();
        return blob;
      },

      cancel() {
        if (closed) return;
        closed = true;
        try {
          if (recorder.state === "recording" || recorder.state === "paused") {
            recorder.stop();
          }
        } catch {
          // Ignore double-stop.
        }
        stopTracks();
      },
    };
  };
}

/** Production factory wired to real globals. */
export function mediaRecorderSinkFactory(): FrameSinkFactory {
  return createMediaRecorderSinkFactory({
    MediaRecorder: MediaRecorder as unknown as MediaRecorderDeps["MediaRecorder"],
  });
}
