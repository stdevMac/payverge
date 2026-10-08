import type { FrameSinkConfig } from "./frameSink";
import {
  KEYFRAME_INTERVAL,
  VIDEO_CODEC,
  VIDEO_MIME,
  createWebCodecsSinkFactory,
  type WebCodecsDeps,
} from "./webcodecsSink";

interface Recorded {
  op: string;
  args: unknown[];
}

function fakeDeps() {
  const calls: Recorded[] = [];
  let onOutput: ((chunk: unknown, meta: unknown) => void) | null = null;
  let onError: ((error: Error) => void) | null = null;
  let queueSize = 0;

  class FakeVideoEncoder {
    state = "unconfigured";
    get encodeQueueSize() {
      return queueSize;
    }
    constructor(init: {
      output: (c: unknown, m: unknown) => void;
      error: (e: Error) => void;
    }) {
      onOutput = init.output;
      onError = init.error;
      calls.push({ op: "new VideoEncoder", args: [] });
    }
    configure(config: unknown) {
      this.state = "configured";
      calls.push({ op: "configure", args: [config] });
    }
    encode(frame: unknown, options: unknown) {
      calls.push({ op: "encode", args: [frame, options] });
      onOutput?.({ chunk: calls.length }, { meta: true });
    }
    async flush() {
      calls.push({ op: "flush", args: [] });
    }
    close() {
      this.state = "closed";
      calls.push({ op: "close", args: [] });
    }
  }

  class FakeVideoFrame {
    constructor(source: unknown, init: unknown) {
      calls.push({ op: "new VideoFrame", args: [source, init] });
    }
    close() {
      calls.push({ op: "frame.close", args: [] });
    }
  }

  const muxer = {
    addVideoChunk: (chunk: unknown, meta: unknown) => {
      calls.push({ op: "addVideoChunk", args: [chunk, meta] });
    },
    finalize: () => {
      calls.push({ op: "finalize", args: [] });
    },
    target: { buffer: new ArrayBuffer(8) },
  };

  const deps: WebCodecsDeps = {
    VideoEncoder: FakeVideoEncoder as unknown as WebCodecsDeps["VideoEncoder"],
    VideoFrame: FakeVideoFrame as unknown as WebCodecsDeps["VideoFrame"],
    createMuxer: async (config) => {
      calls.push({ op: "createMuxer", args: [config] });
      return muxer;
    },
  };

  return {
    deps,
    calls,
    ops: () => calls.map((c) => c.op),
    setQueueSize: (value: number) => {
      queueSize = value;
    },
    fail: (error: Error) => onError?.(error),
  };
}

const config = (over: Partial<FrameSinkConfig> = {}): FrameSinkConfig => ({
  source: {} as CanvasImageSource,
  width: 1080,
  height: 1350,
  fps: 30,
  frameCount: 180,
  ...over,
});

describe("createWebCodecsSinkFactory", () => {
  it("configures the encoder for the surface it was given", async () => {
    const harness = fakeDeps();
    await createWebCodecsSinkFactory(harness.deps)(config());
    const configured = harness.calls.find((c) => c.op === "configure");
    expect(configured?.args[0]).toMatchObject({
      codec: VIDEO_CODEC,
      width: 1080,
      height: 1350,
      framerate: 30,
      avc: { format: "avc" },
    });
  });

  it("builds a muxer sized to the same surface", async () => {
    const harness = fakeDeps();
    await createWebCodecsSinkFactory(harness.deps)(config());
    const created = harness.calls.find((c) => c.op === "createMuxer");
    expect(created?.args[0]).toMatchObject({
      width: 1080,
      height: 1350,
      fps: 30,
    });
  });

  it("encodes one frame per write and closes each VideoFrame", async () => {
    const harness = fakeDeps();
    const sink = await createWebCodecsSinkFactory(harness.deps)(config());
    await sink.write(0, 0);
    await sink.write(1, 33333);
    expect(harness.ops().filter((op) => op === "encode")).toHaveLength(2);
    expect(harness.ops().filter((op) => op === "frame.close")).toHaveLength(2);
  });

  it("stamps each frame with the timestamp the loop gave it", async () => {
    const harness = fakeDeps();
    const sink = await createWebCodecsSinkFactory(harness.deps)(config());
    await sink.write(3, 100000);
    const frame = harness.calls.find((c) => c.op === "new VideoFrame");
    expect(frame?.args[1]).toMatchObject({ timestamp: 100000 });
  });

  /**
   * Without periodic keyframes a six-second clip is one long GOP: seeking is
   * broken, and every social platform's transcoder re-encodes it badly. One per
   * second is the ordinary choice.
   */
  it("requests a keyframe on the configured interval", async () => {
    const harness = fakeDeps();
    const sink = await createWebCodecsSinkFactory(harness.deps)(config());
    for (let index = 0; index < KEYFRAME_INTERVAL * 2; index += 1) {
      await sink.write(index, index * 33333);
    }
    const keyframes = harness.calls
      .filter((c) => c.op === "encode")
      .map((c) => (c.args[1] as { keyFrame: boolean }).keyFrame);
    expect(keyframes[0]).toBe(true);
    expect(keyframes[1]).toBe(false);
    expect(keyframes[KEYFRAME_INTERVAL]).toBe(true);
  });

  it("forwards every encoded chunk to the muxer", async () => {
    const harness = fakeDeps();
    const sink = await createWebCodecsSinkFactory(harness.deps)(config());
    await sink.write(0, 0);
    await sink.write(1, 33333);
    expect(harness.ops().filter((op) => op === "addVideoChunk")).toHaveLength(
      2,
    );
  });

  it("flushes before finalizing and hands back an mp4 blob", async () => {
    const harness = fakeDeps();
    const sink = await createWebCodecsSinkFactory(harness.deps)(config());
    await sink.write(0, 0);
    const blob = await sink.finish();
    const ops = harness.ops();
    expect(ops.indexOf("flush")).toBeLessThan(ops.indexOf("finalize"));
    expect(ops).toContain("close");
    expect(blob.type).toBe(VIDEO_MIME);
  });

  it("closes the encoder on cancel without finalizing a file", async () => {
    const harness = fakeDeps();
    const sink = await createWebCodecsSinkFactory(harness.deps)(config());
    sink.cancel();
    expect(harness.ops()).toContain("close");
    expect(harness.ops()).not.toContain("finalize");
  });

  it("survives cancel being called twice", async () => {
    const harness = fakeDeps();
    const sink = await createWebCodecsSinkFactory(harness.deps)(config());
    sink.cancel();
    expect(() => sink.cancel()).not.toThrow();
    expect(harness.ops().filter((op) => op === "close")).toHaveLength(1);
  });

  it("surfaces an encoder error on the next write instead of swallowing it", async () => {
    const harness = fakeDeps();
    const sink = await createWebCodecsSinkFactory(harness.deps)(config());
    harness.fail(new Error("codec_exploded"));
    await expect(sink.write(0, 0)).rejects.toThrow("codec_exploded");
  });

  it("surfaces an encoder error on finish", async () => {
    const harness = fakeDeps();
    const sink = await createWebCodecsSinkFactory(harness.deps)(config());
    harness.fail(new Error("codec_exploded"));
    await expect(sink.finish()).rejects.toThrow("codec_exploded");
  });

  it("waits for the encoder queue to drain before piling on more frames", async () => {
    const harness = fakeDeps();
    const sink = await createWebCodecsSinkFactory(harness.deps)(config());
    harness.setQueueSize(99);
    let settled = false;
    const pending = Promise.resolve(sink.write(0, 0)).then(() => {
      settled = true;
    });
    await Promise.resolve();
    expect(settled).toBe(false);
    harness.setQueueSize(0);
    await pending;
    expect(settled).toBe(true);
  });
});
