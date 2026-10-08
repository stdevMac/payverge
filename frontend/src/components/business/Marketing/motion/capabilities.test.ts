import {
  detectMediaRecorderSupport,
  detectMotionSupport,
  detectWebCodecsSupport,
} from "./capabilities";

const webcodecsScope = {
  VideoEncoder: function VideoEncoder() {},
  VideoFrame: function VideoFrame() {},
  OffscreenCanvas: function OffscreenCanvas() {},
};

const mediaRecorderScope = {
  MediaRecorder: function MediaRecorder() {},
  HTMLCanvasElement: {
    prototype: { captureStream: function captureStream() {} },
  },
};

describe("detectWebCodecsSupport", () => {
  it("reports supported when every constructor is present", () => {
    expect(detectWebCodecsSupport(webcodecsScope)).toEqual({
      supported: true,
      missing: [],
    });
  });

  it("names every missing constructor", () => {
    expect(detectWebCodecsSupport({})).toEqual({
      supported: false,
      missing: ["VideoEncoder", "VideoFrame", "OffscreenCanvas"],
    });
  });
});

describe("detectMediaRecorderSupport", () => {
  it("reports supported when MediaRecorder and captureStream exist", () => {
    expect(detectMediaRecorderSupport(mediaRecorderScope)).toEqual({
      supported: true,
      missing: [],
    });
  });

  it("names MediaRecorder and captureStream gaps", () => {
    expect(detectMediaRecorderSupport({})).toEqual({
      supported: false,
      missing: ["MediaRecorder", "captureStream"],
    });
  });
});

describe("detectMotionSupport", () => {
  it("prefers the WebCodecs path when complete", () => {
    expect(detectMotionSupport(webcodecsScope)).toEqual({
      supported: true,
      path: "webcodecs",
      missing: [],
    });
  });

  it("falls back to MediaRecorder when WebCodecs is incomplete", () => {
    expect(detectMotionSupport(mediaRecorderScope)).toEqual({
      supported: true,
      path: "mediarecorder",
      missing: [],
    });
  });

  it("reports unsupported with WebCodecs gaps when neither path works", () => {
    expect(detectMotionSupport({})).toEqual({
      supported: false,
      path: null,
      missing: ["VideoEncoder", "VideoFrame", "OffscreenCanvas"],
    });
  });

  it("rejects a non-callable stand-in for VideoEncoder", () => {
    expect(
      detectMotionSupport({
        VideoEncoder: {},
        VideoFrame: function VideoFrame() {},
        OffscreenCanvas: function OffscreenCanvas() {},
      }),
    ).toMatchObject({
      supported: false,
      path: null,
      missing: ["VideoEncoder"],
    });
  });

  /**
   * jsdom has none of these, so the default argument path must resolve to
   * unsupported rather than throwing on an undefined `globalThis` member. This
   * is the case that runs in CI on every developer machine.
   */
  it("reports unsupported under the test environment's own globals", () => {
    expect(detectMotionSupport().supported).toBe(false);
  });
});
