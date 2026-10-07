import type { FrameSink, FrameSinkConfig, FrameSinkFactory } from "./frameSink";

/**
 * H.264 in MP4, via WebCodecs plus mp4-muxer.
 *
 * The constructors are INJECTED rather than read off `globalThis`, because jsdom
 * has no `VideoEncoder` and no `VideoFrame`: the only thing this file can be
 * tested for is its call protocol — configure, encode with the right keyframe
 * cadence, flush, finalize, close — and that is exactly what the fake deps in
 * webcodecsSink.test.ts assert. No real codec is exercised anywhere in the
 * suite, and no attempt is made to pretend otherwise.
 *
 * The muxer arrives through an async factory so `mp4-muxer` can be loaded with a
 * dynamic `import()` at the edge and stays out of the main bundle for the
 * operators who never export a video.
 */

/**
 * H.264 High profile, level 4.0. Level 4.0 covers 1920x1080 at 30fps, which is
 * the macroblock budget the tallest export (1080x1920) needs; Baseline 3.1,
 * the usual default, does not and would be rejected by `configure`.
 */
export const VIDEO_CODEC = "avc1.640028";

export const VIDEO_MIME = "video/mp4";

/**
 * One keyframe per second at 30fps. Without periodic keyframes the whole clip is
 * a single GOP: scrubbing is broken in every preview player, and platform
 * transcoders handle it badly.
 */
export const KEYFRAME_INTERVAL = 30;

/** 6 Mbps. Generous for 1080-wide graphic content, and well inside upload caps. */
const BITRATE = 6_000_000;

/**
 * How long to wait between polls while the encoder queue drains. Long enough
 * not to spin, short enough that the loop is not the bottleneck.
 */
const DRAIN_POLL_MS = 4;

/** The depth at which the loop stops feeding the encoder and lets it catch up. */
const MAX_QUEUE_DEPTH = 8;

interface MuxerLike {
  addVideoChunk(chunk: unknown, meta: unknown): void;
  finalize(): void;
  readonly target: { buffer: ArrayBuffer | null };
}

interface MuxerConfig {
  width: number;
  height: number;
  fps: number;
}

export interface WebCodecsDeps {
  VideoEncoder: typeof VideoEncoder;
  VideoFrame: typeof VideoFrame;
  createMuxer: (config: MuxerConfig) => Promise<MuxerLike>;
}

/**
 * The production muxer factory. `mp4-muxer` is imported dynamically so it lands
 * in its own chunk: an operator who never exports a video never downloads it.
 */
async function createMp4Muxer(config: MuxerConfig): Promise<MuxerLike> {
  const { Muxer, ArrayBufferTarget } = await import("mp4-muxer");
  const target = new ArrayBufferTarget();
  const muxer = new Muxer({
    target,
    video: {
      codec: "avc",
      width: config.width,
      height: config.height,
      frameRate: config.fps,
    },
    // The whole file is assembled in memory and handed over as one Blob, so the
    // moov box can live at the front where every player expects it.
    fastStart: "in-memory",
  });
  return {
    addVideoChunk: (chunk, meta) =>
      muxer.addVideoChunk(chunk as EncodedVideoChunk, meta as never),
    finalize: () => muxer.finalize(),
    target,
  };
}

export function createWebCodecsSinkFactory(deps: WebCodecsDeps): FrameSinkFactory {
  return async (config: FrameSinkConfig): Promise<FrameSink> => {
    const muxer = await deps.createMuxer({
      width: config.width,
      height: config.height,
      fps: config.fps,
    });

    // The encoder reports failures through its `error` callback, asynchronously
    // and out of band. Latching it here and rethrowing on the next `write` or on
    // `finish` is what turns a silent half-encoded file into a failed export the
    // operator is actually told about.
    let encoderError: Error | null = null;
    let closed = false;

    const encoder = new deps.VideoEncoder({
      output: (chunk, meta) => muxer.addVideoChunk(chunk, meta),
      error: (error) => {
        encoderError = error instanceof Error ? error : new Error(String(error));
      },
    });

    encoder.configure({
      codec: VIDEO_CODEC,
      width: config.width,
      height: config.height,
      framerate: config.fps,
      bitrate: BITRATE,
      // mp4-muxer expects length-prefixed AVCC, not Annex B start codes.
      avc: { format: "avc" },
    });

    const throwIfFailed = () => {
      if (encoderError) throw encoderError;
    };

    const drain = async () => {
      while (!encoderError && encoder.encodeQueueSize > MAX_QUEUE_DEPTH) {
        await new Promise((resolve) => setTimeout(resolve, DRAIN_POLL_MS));
      }
    };

    const close = () => {
      if (closed) return;
      closed = true;
      encoder.close();
    };

    return {
      async write(frameIndex, timestampUs) {
        throwIfFailed();
        await drain();
        throwIfFailed();
        const frame = new deps.VideoFrame(config.source, {
          timestamp: timestampUs,
          duration: Math.round(1_000_000 / config.fps),
        });
        try {
          encoder.encode(frame, {
            keyFrame: frameIndex % KEYFRAME_INTERVAL === 0,
          });
        } finally {
          // A VideoFrame holds a GPU-backed buffer until closed. Dropping 180 of
          // them without closing exhausts the codec's frame pool and stalls the
          // encoder mid-clip.
          frame.close();
        }
      },

      async finish() {
        throwIfFailed();
        await encoder.flush();
        throwIfFailed();
        close();
        muxer.finalize();
        const buffer = muxer.target.buffer;
        if (!buffer) throw new Error("mp4_finalize_produced_no_buffer");
        return new Blob([buffer], { type: VIDEO_MIME });
      },

      cancel() {
        close();
      },
    };
  };
}

/** The production factory, wired to the real globals. */
export function webCodecsSinkFactory(): FrameSinkFactory {
  return createWebCodecsSinkFactory({
    VideoEncoder,
    VideoFrame,
    createMuxer: createMp4Muxer,
  });
}
