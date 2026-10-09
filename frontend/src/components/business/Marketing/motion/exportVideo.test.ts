import type { Scene } from "../scene/types";
import type { RenderPostInput } from "../templates/renderPost";
import {
  MotionUnsupportedError,
  exportVideo,
  type ExportVideoDeps,
} from "./exportVideo";

const renderInput: RenderPostInput = {
  kit: "editorial",
  composition: "photoBottomStack",
  aspect: "4:5",
  photoUrl: "https://cdn.example.com/dish.jpg",
  slots: { dishName: "Milanesa", badge: "CHEF'S PICK" },
  palette: { primary: "#1a6b6a", secondary: "#0f3d3c" },
};

function fakeDeps(over: Partial<ExportVideoDeps> = {}) {
  const painted: Scene[] = [];
  const sampled: unknown[] = [];
  const released: number[] = [];
  const scene: Scene = {
    width: 1080,
    height: 1350,
    background: "#1a6b6a",
    nodes: [
      {
        kind: "photo",
        url: "https://cdn.example.com/dish.jpg",
        rect: { x: 0, y: 0, w: 1, h: 1 },
        z: 0,
      },
      {
        kind: "scrim",
        rect: { x: 0, y: 0.42, w: 1, h: 0.58 },
        z: 10,
        from: "rgba(28,25,23,0)",
        to: "rgba(28,25,23,0.78)",
        adaptive: true,
        luminanceThreshold: 155,
      },
      {
        kind: "text",
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
      },
    ],
  };

  const deps: ExportVideoDeps = {
    detectSupport: () => ({ supported: true, missing: [] }),
    loadImages: async () => ({
      photo: { naturalWidth: 800, naturalHeight: 600 } as HTMLImageElement,
      logo: null,
    }),
    buildScene: () => scene,
    paintScene: (_ctx, s) => {
      painted.push(s);
    },
    sampleLuminance: (_ctx, _w, _h, rect) => {
      sampled.push(rect);
      return 220;
    },
    createSurface: (width, height) => ({
      source: {} as CanvasImageSource,
      ctx: {} as CanvasRenderingContext2D,
      width,
      height,
      release: () => released.push(width),
    }),
    createSink: async () => ({
      write: () => {},
      finish: async () => new Blob(["mp4"], { type: "video/mp4" }),
      cancel: () => {},
    }),
    renderPoster: async () => new Blob(["png"], { type: "image/png" }),
    ...over,
  };

  return { deps, painted, sampled, released, scene };
}

const fast = { yieldTo: async () => {} };

describe("exportVideo", () => {
  it("refuses on a browser without WebCodecs, naming what is missing", async () => {
    const { deps } = fakeDeps({
      detectSupport: () => ({ supported: false, missing: ["VideoEncoder"] }),
    });
    await expect(
      exportVideo({ renderInput, preset: "pushIn", deps, ...fast }),
    ).rejects.toBeInstanceOf(MotionUnsupportedError);
    await expect(
      exportVideo({ renderInput, preset: "pushIn", deps, ...fast }),
    ).rejects.toMatchObject({ missing: ["VideoEncoder"] });
  });

  it("returns the encoded video and a poster", async () => {
    const { deps } = fakeDeps();
    const result = await exportVideo({
      renderInput,
      preset: "pushIn",
      durationMs: 200,
      fps: 30,
      deps,
      ...fast,
    });
    expect(result.video.type).toBe("video/mp4");
    expect(result.poster.type).toBe("image/png");
  });

  it("selects MediaRecorder path deps when support reports that path", async () => {
    const surfaces: string[] = [];
    const { deps } = fakeDeps({
      detectSupport: () => ({
        supported: true,
        missing: [],
        path: "mediarecorder",
      }),
      createSurface: (width, height) => {
        surfaces.push("html");
        return {
          source: { captureStream: () => ({ getTracks: () => [] }) } as unknown as CanvasImageSource,
          ctx: {} as CanvasRenderingContext2D,
          width,
          height,
          release: () => {},
        };
      },
    });
    await exportVideo({
      renderInput,
      preset: "pushIn",
      durationMs: 100,
      fps: 30,
      deps,
      ...fast,
    });
    // Injected createSurface wins over path defaults; proves path does not break export.
    expect(surfaces).toEqual(["html"]);
  });

  /**
   * The scrim's luminance must be measured against the PHOTO alone. The Z ladder
   * in buildScene is photo 0 < scrim 10 < texture 15 < text 30, so the bitmap the
   * walker samples when it reaches the scrim is exactly background-plus-photo.
   * Painting the whole scene first and then sampling would read the scrim's own
   * output back and under-boost every clip.
   */
  it("samples the scrim against a photo-only pre-pass", async () => {
    const { deps, painted } = fakeDeps();
    await exportVideo({
      renderInput,
      preset: "pushIn",
      durationMs: 100,
      fps: 30,
      deps,
      ...fast,
    });
    expect(painted[0].nodes.map((n) => n.kind)).toEqual(["photo"]);
  });

  it("samples each scrim band exactly once for the whole clip", async () => {
    const { deps, sampled } = fakeDeps();
    await exportVideo({
      renderInput,
      preset: "pushIn",
      durationMs: 1000,
      fps: 30,
      deps,
      ...fast,
    });
    expect(sampled).toHaveLength(1);
  });

  it("hands the frame loop a scene with no adaptive scrim left in it", async () => {
    const { deps, painted } = fakeDeps();
    await exportVideo({
      renderInput,
      preset: "pushIn",
      durationMs: 100,
      fps: 30,
      deps,
      ...fast,
    });
    painted.slice(1).forEach((frame) => {
      frame.nodes.forEach((node) => {
        if (node.kind === "scrim") expect(node.adaptive).toBe(false);
      });
    });
  });

  it("paints one frame per frame plus the sampling pre-pass", async () => {
    const { deps, painted } = fakeDeps();
    await exportVideo({
      renderInput,
      preset: "pushIn",
      durationMs: 1000,
      fps: 30,
      deps,
      ...fast,
    });
    expect(painted).toHaveLength(1 + 30);
  });

  it("reports progress as a fraction that reaches 1", async () => {
    const { deps } = fakeDeps();
    const fractions: number[] = [];
    await exportVideo({
      renderInput,
      preset: "pushIn",
      durationMs: 200,
      fps: 30,
      onProgress: (fraction) => fractions.push(fraction),
      deps,
      ...fast,
    });
    expect(fractions[0]).toBeCloseTo(1 / 6, 6);
    expect(fractions[fractions.length - 1]).toBe(1);
  });

  it("releases the surface even when the loop throws", async () => {
    const { deps, released } = fakeDeps({
      createSink: async () => ({
        write: () => {
          throw new Error("encode_failed");
        },
        finish: async () => new Blob([]),
        cancel: () => {},
      }),
    });
    await expect(
      exportVideo({ renderInput, preset: "pushIn", durationMs: 100, deps, ...fast }),
    ).rejects.toThrow("encode_failed");
    expect(released).toHaveLength(1);
  });

  it("propagates an abort as an AbortError and still releases the surface", async () => {
    const { deps, released } = fakeDeps();
    const controller = new AbortController();
    controller.abort();
    await expect(
      exportVideo({
        renderInput,
        preset: "pushIn",
        durationMs: 6000,
        signal: controller.signal,
        deps,
        ...fast,
      }),
    ).rejects.toMatchObject({ name: "AbortError" });
    expect(released).toHaveLength(1);
  });

  it("sizes the surface from the aspect, at full export resolution", async () => {
    const sizes: Array<[number, number]> = [];
    const { deps } = fakeDeps({
      createSurface: (width, height) => {
        sizes.push([width, height]);
        return {
          source: {} as CanvasImageSource,
          ctx: {} as CanvasRenderingContext2D,
          width,
          height,
          release: () => {},
        };
      },
    });
    await exportVideo({
      renderInput: { ...renderInput, aspect: "9:16" },
      preset: "pushIn",
      durationMs: 100,
      deps,
      ...fast,
    });
    expect(sizes).toEqual([[1080, 1920]]);
  });
});
