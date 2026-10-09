import {
  createMediaRecorderSinkFactory,
  VIDEO_MIME_WEBM,
  type MediaRecorderDeps,
  type MediaRecorderLike,
  type MediaStreamLike,
} from "./mediaRecorderSink";

function fakeRecorder(): MediaRecorderLike & {
  starts: number[];
  stops: number;
} {
  const rec: MediaRecorderLike & { starts: number[]; stops: number } = {
    state: "inactive",
    starts: [],
    stops: 0,
    ondataavailable: null,
    onerror: null,
    onstop: null,
    start(timeslice?: number) {
      this.starts.push(timeslice ?? 0);
      this.state = "recording";
    },
    stop() {
      this.stops += 1;
      this.state = "inactive";
      this.ondataavailable?.({
        data: new Blob(["chunk"], { type: VIDEO_MIME_WEBM }),
      });
      this.onstop?.();
    },
  };
  return rec;
}

function deps(over: Partial<MediaRecorderDeps> = {}): MediaRecorderDeps {
  const recorder = fakeRecorder();
  return {
    MediaRecorder: Object.assign(
      function MediaRecorderMock() {
        return recorder;
      },
      { isTypeSupported: () => true },
    ) as unknown as MediaRecorderDeps["MediaRecorder"],
    waitMs: async () => {},
    nowMs: (() => {
      let t = 0;
      return () => {
        t += 0;
        return t;
      };
    })(),
    ...over,
  };
}

function capturableSource(): {
  source: CanvasImageSource;
  streams: MediaStreamLike[];
} {
  const streams: MediaStreamLike[] = [];
  const source = {
    captureStream(fps?: number) {
      const stream: MediaStreamLike = {
        getTracks: () => [{ stop: jest.fn() }],
      };
      streams.push(stream);
      void fps;
      return stream;
    },
  } as unknown as CanvasImageSource;
  return { source, streams };
}

describe("createMediaRecorderSinkFactory", () => {
  it("requires a canvas that implements captureStream", async () => {
    const factory = createMediaRecorderSinkFactory(deps());
    await expect(
      factory({
        source: {} as CanvasImageSource,
        width: 1080,
        height: 1350,
        fps: 30,
        frameCount: 6,
      }),
    ).rejects.toThrow("mediarecorder_requires_html_canvas_captureStream");
  });

  it("starts on first write, paces frames, and finishes a WebM blob", async () => {
    const { source } = capturableSource();
    const waits: number[] = [];
    let clock = 0;
    const factory = createMediaRecorderSinkFactory(
      deps({
        waitMs: async (ms) => {
          waits.push(ms);
          clock += ms;
        },
        nowMs: () => clock,
      }),
    );
    const sink = await factory({
      source,
      width: 1080,
      height: 1350,
      fps: 30,
      frameCount: 3,
    });

    await sink.write(0, 0);
    await sink.write(1, 33_333);
    await sink.write(2, 66_666);
    const blob = await sink.finish();

    expect(blob.type).toMatch(/^video\/webm/);
    expect(blob.size).toBeGreaterThan(0);
    // Frame 0 has no wait; frames 1 and 2 wait ~33.3ms each from origin.
    expect(waits.length).toBeGreaterThanOrEqual(1);
  });

  it("cancel stops the recorder without throwing", async () => {
    const { source } = capturableSource();
    const factory = createMediaRecorderSinkFactory(deps());
    const sink = await factory({
      source,
      width: 100,
      height: 100,
      fps: 30,
      frameCount: 2,
    });
    await sink.write(0, 0);
    expect(() => sink.cancel()).not.toThrow();
    expect(() => sink.cancel()).not.toThrow();
  });
});
