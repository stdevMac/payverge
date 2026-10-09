import { textNode, type Scene } from "../scene/types";
import type { FrameSink } from "./frameSink";
import { MAX_FRAMES, frameCountFor, renderFrames } from "./renderFrames";
import type { AnimTrack } from "./tracks";

const scene: Scene = {
  width: 1080,
  height: 1350,
  background: "#1a6b6a",
  nodes: [
    textNode({
      key: "dishName",
      text: "Milanesa",
      rect: { x: 0.07, y: 0.6, w: 0.86, h: 0.08 },
      z: 30,
      font: "sans",
      weight: 700,
      sizePct: 0.07,
      align: "left",
      color: "#ffffff",
      maxLines: 2,
      lineHeight: 1.2,
    }),
  ],
};

const fade: AnimTrack = {
  target: { kind: "text", key: "dishName" },
  property: "opacity",
  from: 0,
  to: 1,
  easing: "linear",
  delayMs: 0,
  durationMs: 6000,
};

function fakeSink() {
  const written: Array<{ frameIndex: number; timestampUs: number }> = [];
  let cancelled = 0;
  let finished = 0;
  const sink: FrameSink = {
    write: (frameIndex, timestampUs) => {
      written.push({ frameIndex, timestampUs });
    },
    finish: async () => {
      finished += 1;
      return new Blob(["mp4"], { type: "video/mp4" });
    },
    cancel: () => {
      cancelled += 1;
    },
  };
  return {
    sink,
    written,
    get cancelled() {
      return cancelled;
    },
    get finished() {
      return finished;
    },
  };
}

describe("frameCountFor", () => {
  it("gives 180 frames for six seconds at 30fps", () => {
    expect(frameCountFor(6000, 30)).toBe(180);
  });

  it("falls back to the default rate for a zero or negative fps", () => {
    expect(frameCountFor(6000, 0)).toBe(180);
    expect(frameCountFor(6000, -30)).toBe(180);
  });

  it("caps an infinite duration instead of looping forever", () => {
    expect(frameCountFor(Number.POSITIVE_INFINITY, 30)).toBe(MAX_FRAMES);
  });

  it("caps an absurd but finite duration", () => {
    expect(frameCountFor(60 * 60 * 1000, 30)).toBe(MAX_FRAMES);
  });

  it("gives one frame for malformed durations rather than zero or NaN", () => {
    expect(frameCountFor(Number.NaN, 30)).toBe(1);
    expect(frameCountFor(-1, 30)).toBe(1);
    expect(frameCountFor(0, 30)).toBe(1);
  });

  it("gives one frame for a non-finite fps", () => {
    expect(frameCountFor(6000, Number.NaN)).toBe(180);
    expect(frameCountFor(6000, Number.POSITIVE_INFINITY)).toBe(180);
  });

  it("never returns a non-integer", () => {
    [1, 33, 1000, 6000, 12345].forEach((ms) => {
      [1, 24, 25, 30, 60].forEach((fps) => {
        expect(Number.isInteger(frameCountFor(ms, fps))).toBe(true);
      });
    });
  });
});

describe("renderFrames", () => {
  it("paints and writes exactly frameCount frames", async () => {
    const harness = fakeSink();
    const painted: number[] = [];
    await renderFrames({
      scene,
      tracks: [fade],
      durationMs: 1000,
      fps: 30,
      sink: harness.sink,
      paint: (_evaluated, frameIndex) => painted.push(frameIndex),
      yieldTo: async () => {},
    });
    expect(painted).toHaveLength(30);
    expect(harness.written).toHaveLength(30);
    expect(harness.finished).toBe(1);
  });

  it("terminates on an infinite duration at the cap", async () => {
    const harness = fakeSink();
    let painted = 0;
    await renderFrames({
      scene,
      tracks: [],
      durationMs: Number.POSITIVE_INFINITY,
      fps: 30,
      sink: harness.sink,
      paint: () => {
        painted += 1;
      },
      yieldTo: async () => {},
    });
    expect(painted).toBe(MAX_FRAMES);
  });

  it("hands the paint step the scene evaluated at that frame's time", async () => {
    const harness = fakeSink();
    const alphas: Array<number | undefined> = [];
    await renderFrames({
      scene,
      tracks: [fade],
      durationMs: 1000,
      fps: 10,
      sink: harness.sink,
      paint: (evaluated) => alphas.push(evaluated.nodes[0].opacity),
      yieldTo: async () => {},
    });
    expect(alphas[0]).toBe(0);
    expect(alphas[5]).toBeCloseTo(5 / 60, 6);
    expect(alphas).toHaveLength(10);
  });

  it("emits monotonically increasing microsecond timestamps from zero", async () => {
    const harness = fakeSink();
    await renderFrames({
      scene,
      tracks: [],
      durationMs: 1000,
      fps: 30,
      sink: harness.sink,
      paint: () => {},
      yieldTo: async () => {},
    });
    const stamps = harness.written.map((w) => w.timestampUs);
    expect(stamps[0]).toBe(0);
    stamps.forEach((value, index) => {
      if (index === 0) return;
      expect(value).toBeGreaterThan(stamps[index - 1]);
      expect(Number.isInteger(value)).toBe(true);
    });
  });

  it("reports progress once per frame, ending at total", async () => {
    const harness = fakeSink();
    const progress: Array<[number, number]> = [];
    await renderFrames({
      scene,
      tracks: [],
      durationMs: 200,
      fps: 30,
      sink: harness.sink,
      paint: () => {},
      onProgress: (done, total) => progress.push([done, total]),
      yieldTo: async () => {},
    });
    expect(progress).toHaveLength(6);
    expect(progress[0]).toEqual([1, 6]);
    expect(progress[5]).toEqual([6, 6]);
  });

  it("awaits a sink that applies backpressure", async () => {
    let inFlight = 0;
    let maxInFlight = 0;
    const sink: FrameSink = {
      write: async () => {
        inFlight += 1;
        maxInFlight = Math.max(maxInFlight, inFlight);
        await Promise.resolve();
        inFlight -= 1;
      },
      finish: async () => new Blob([], { type: "video/mp4" }),
      cancel: () => {},
    };
    await renderFrames({
      scene,
      tracks: [],
      durationMs: 500,
      fps: 30,
      sink,
      paint: () => {},
      yieldTo: async () => {},
    });
    expect(maxInFlight).toBe(1);
  });

  it("cancels the sink and rejects when the signal aborts mid-clip", async () => {
    const harness = fakeSink();
    const controller = new AbortController();
    let painted = 0;
    const promise = renderFrames({
      scene,
      tracks: [],
      durationMs: 6000,
      fps: 30,
      sink: harness.sink,
      paint: () => {
        painted += 1;
        if (painted === 4) controller.abort();
      },
      signal: controller.signal,
      yieldTo: async () => {},
    });
    await expect(promise).rejects.toMatchObject({ name: "AbortError" });
    expect(harness.cancelled).toBe(1);
    expect(harness.finished).toBe(0);
    expect(painted).toBe(4);
  });

  it("paints nothing when the signal is already aborted", async () => {
    const harness = fakeSink();
    const controller = new AbortController();
    controller.abort();
    let painted = 0;
    await expect(
      renderFrames({
        scene,
        tracks: [],
        durationMs: 6000,
        fps: 30,
        sink: harness.sink,
        paint: () => {
          painted += 1;
        },
        signal: controller.signal,
        yieldTo: async () => {},
      }),
    ).rejects.toMatchObject({ name: "AbortError" });
    expect(painted).toBe(0);
    expect(harness.cancelled).toBe(1);
  });

  it("cancels the sink when the paint step throws", async () => {
    const harness = fakeSink();
    await expect(
      renderFrames({
        scene,
        tracks: [],
        durationMs: 200,
        fps: 30,
        sink: harness.sink,
        paint: () => {
          throw new Error("paint_failed");
        },
        yieldTo: async () => {},
      }),
    ).rejects.toThrow("paint_failed");
    expect(harness.cancelled).toBe(1);
    expect(harness.finished).toBe(0);
  });

  it("yields to the host on the configured cadence", async () => {
    const harness = fakeSink();
    let yields = 0;
    await renderFrames({
      scene,
      tracks: [],
      durationMs: 400,
      fps: 30,
      sink: harness.sink,
      paint: () => {},
      yieldEvery: 4,
      yieldTo: async () => {
        yields += 1;
      },
    });
    // 12 frames, a yield after every 4th.
    expect(yields).toBe(3);
  });
});
