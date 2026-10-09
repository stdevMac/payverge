/**
 * The seam between "paint 180 frames" and "turn them into a file".
 *
 * Types only, and a LEAF: nothing here imports a codec, a muxer or a canvas. That
 * is what lets `renderFrames` be tested against a fake in a harness where
 * `VideoEncoder` does not exist, and it is where a second encoder — a
 * `MediaRecorder` fallback, say — would plug in without touching the loop.
 */

export interface FrameSinkConfig {
  /**
   * The surface the frame loop paints onto. The sink reads it after each paint;
   * it never draws to it.
   */
  source: CanvasImageSource;
  width: number;
  height: number;
  fps: number;
  /** Known up front, so a sink can size buffers and report progress. */
  frameCount: number;
}

export interface FrameSink {
  /**
   * Capture whatever the loop just painted onto `config.source`.
   *
   * May return a promise: a real encoder applies backpressure when its queue is
   * deep, and awaiting that is how the loop avoids allocating 180 frames of
   * uncompressed video at once.
   */
  write(frameIndex: number, timestampUs: number): void | Promise<void>;
  /** Drain, mux, and hand back the finished file. */
  finish(): Promise<Blob>;
  /** Tear down without producing a file. Must be safe to call twice. */
  cancel(): void;
}

export type FrameSinkFactory = (config: FrameSinkConfig) => Promise<FrameSink>;
