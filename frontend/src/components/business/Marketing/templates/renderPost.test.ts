/** @jest-environment jsdom */
import React from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import {
  aspectDimensions,
  buildPostScene,
  fitCover,
  renderPost,
  renderPostScene,
  resolveArtDirection,
  sampleBottomLuminance,
  wrapText,
} from "./renderPost";
import { FORMATS } from "../formats/formats";
import { EDITORIAL, TEMPLATES, TEMPLATE_ORDER } from "./templates";
import { KITS, KIT_ORDER } from "../artDirection/kits";
import {
  COMPOSITIONS,
  COMPOSITION_ORDER,
  LEGACY_COMPOSITION_FOR_TEMPLATE,
} from "../composition/compositions";
import * as photoCache from "../photo/cache";
import { PostPreview } from "../PostPreview";
import { PlatformPreviewFrame } from "../PlatformPreviewFrame";
import { logError } from "@/utils/errorLogger";

jest.mock("@/utils/errorLogger", () => ({
  logError: jest.fn(),
}));

const mockLogError = logError as jest.MockedFunction<typeof logError>;

interface DrawTextRecord {
  text: string;
  x: number;
  y: number;
  font: string;
}

interface GradientRecord {
  args: number[];
  stops: Array<[number, string]>;
}

function stubCanvas(luminance = 128) {
  const text: DrawTextRecord[] = [];
  const gradients: GradientRecord[] = [];
  const drawImages: unknown[][] = [];
  const fillRects: Array<{
    x: number;
    y: number;
    w: number;
    h: number;
    fillStyle: unknown;
  }> = [];
  const ctx = {
    font: "",
    fillStyle: "",
    textAlign: "left",
    textBaseline: "top",
    createLinearGradient: jest.fn((...args: number[]) => {
      const record: GradientRecord = { args, stops: [] };
      gradients.push(record);
      return {
        addColorStop: (offset: number, color: string) =>
          record.stops.push([offset, color]),
      };
    }),
    fillRect: jest.fn((x: number, y: number, w: number, h: number) => {
      fillRects.push({ x, y, w, h, fillStyle: ctx.fillStyle });
    }),
    fillText: jest.fn((value: string, x: number, y: number) => {
      text.push({ text: value, x, y, font: ctx.font });
    }),
    measureText: jest.fn((value: string) => {
      const px = Number.parseFloat(ctx.font.match(/(\d+)px/)?.[1] ?? "20");
      return { width: value.length * px * 0.52 };
    }),
    drawImage: jest.fn((...args: unknown[]) => drawImages.push(args)),
    getImageData: jest.fn((_x: number, _y: number, w: number, h: number) => {
      const data = new Uint8ClampedArray(w * h * 4);
      for (let index = 0; index < data.length; index += 4) {
        data[index] = luminance;
        data[index + 1] = luminance;
        data[index + 2] = luminance;
        data[index + 3] = 255;
      }
      // width/height are required: the analysis downsample reads them off the
      // ImageData, and missing them collapses every grid to zeros which
      // silently disables the adaptive scrim boost.
      return { data, width: w, height: h } as ImageData;
    }),
    beginPath: jest.fn(),
    moveTo: jest.fn(),
    arcTo: jest.fn(),
    closePath: jest.fn(),
    fill: jest.fn(),
    arc: jest.fn(),
    clip: jest.fn(),
    save: jest.fn(),
    restore: jest.fn(),
    // The scene walker's motif drawers stroke and transform; the legacy
    // renderer never did, so these arrived with the scene switch-over.
    // Assigning strokeStyle/lineWidth/globalAlpha needs no stub — they are
    // plain properties on this object — but the calls below do.
    strokeStyle: "",
    lineWidth: 0,
    globalAlpha: 1,
    lineTo: jest.fn(),
    stroke: jest.fn(),
    translate: jest.fn(),
    rotate: jest.fn(),
  };
  jest
    .spyOn(HTMLCanvasElement.prototype, "getContext")
    .mockReturnValue(ctx as unknown as CanvasRenderingContext2D);
  return { ctx, text, gradients, drawImages, fillRects };
}

function stubLoadedImage(width = 2000, height = 1000) {
  const OriginalImage = global.Image;
  class LoadedImage {
    crossOrigin = "";
    naturalWidth = width;
    naturalHeight = height;
    onload: (() => void) | null = null;
    onerror: (() => void) | null = null;

    set src(_value: string) {
      queueMicrotask(() => this.onload?.());
    }
  }
  Object.defineProperty(global, "Image", {
    configurable: true,
    value: LoadedImage,
  });
  return () => {
    Object.defineProperty(global, "Image", {
      configurable: true,
      value: OriginalImage,
    });
  };
}

function stubDeferredImages() {
  const OriginalImage = global.Image;
  const pending = new Map<
    string,
    { resolve: () => void; reject: () => void }
  >();
  class DeferredImage {
    crossOrigin = "";
    naturalWidth = 2000;
    naturalHeight = 1000;
    onload: (() => void) | null = null;
    onerror: (() => void) | null = null;

    set src(value: string) {
      pending.set(value, {
        resolve: () => this.onload?.(),
        reject: () => this.onerror?.(),
      });
    }
  }
  Object.defineProperty(global, "Image", {
    configurable: true,
    value: DeferredImage,
  });
  return {
    pending,
    restore: () =>
      Object.defineProperty(global, "Image", {
        configurable: true,
        value: OriginalImage,
      }),
  };
}

function stubFailingImages() {
  const OriginalImage = global.Image;
  class FailingImage {
    crossOrigin = "";
    naturalWidth = 0;
    naturalHeight = 0;
    onload: (() => void) | null = null;
    onerror: (() => void) | null = null;

    set src(_value: string) {
      queueMicrotask(() => this.onerror?.());
    }
  }
  Object.defineProperty(global, "Image", {
    configurable: true,
    value: FailingImage,
  });
  return () =>
    Object.defineProperty(global, "Image", {
      configurable: true,
      value: OriginalImage,
    });
}

const brand = {
  primary: "#1a6b6a",
  secondary: "#0f3d3c",
  fontFamily: "Serif",
};

describe("shared canvas mock", () => {
  it("supports renderer measurement, gradients, transforms, state, clipping, and drawing", () => {
    const canvasModule = jest.requireActual(
      "../../../../../test/mocks/canvas.js",
    ) as {
      createCanvas: (
        width: number,
        height: number,
      ) => {
        getContext: (kind: string) => CanvasRenderingContext2D;
      };
    };
    const canvas = canvasModule.createCanvas(1080, 1350);
    const ctx = canvas.getContext("2d") as CanvasRenderingContext2D & {
      __calls?: Array<{ method: string; args: unknown[] }>;
    };

    ctx.font = "20px Inter";
    expect(ctx.measureText("Payverge").width).toBeGreaterThan(0);
    const gradient = ctx.createLinearGradient(0, 0, 100, 100);
    gradient.addColorStop(0, "#000000");
    gradient.addColorStop(1, "#ffffff");
    expect(() => {
      ctx.save();
      ctx.translate(4, 8);
      ctx.scale(2, 2);
      ctx.rotate(Math.PI / 4);
      ctx.setTransform(1, 0, 0, 1, 0, 0);
      ctx.resetTransform();
      ctx.beginPath();
      ctx.moveTo(0, 0);
      ctx.lineTo(20, 20);
      ctx.arcTo(20, 20, 40, 40, 4);
      ctx.closePath();
      ctx.clip();
      ctx.fillRect(0, 0, 10, 10);
      ctx.clearRect(0, 0, 5, 5);
      ctx.fillText("Ready", 5, 5);
      ctx.drawImage(canvas as unknown as CanvasImageSource, 0, 0);
      ctx.restore();
    }).not.toThrow();
    expect(ctx.__calls?.map(({ method }) => method)).toEqual(
      expect.arrayContaining([
        "save",
        "translate",
        "scale",
        "rotate",
        "setTransform",
        "resetTransform",
        "clip",
        "fillRect",
        "clearRect",
        "fillText",
        "drawImage",
        "restore",
      ]),
    );
  });
});

describe("renderPost pure helpers", () => {
  it("returns canvas dimensions per aspect", () => {
    expect(aspectDimensions("1:1")).toEqual({ w: 1080, h: 1080 });
    expect(aspectDimensions("4:5")).toEqual({ w: 1080, h: 1350 });
    expect(aspectDimensions("9:16")).toEqual({ w: 1080, h: 1920 });
  });

  it("scales dimensions proportionally when targetWidth is below export width", () => {
    expect(aspectDimensions("1:1", 256)).toEqual({ w: 256, h: 256 });
    expect(aspectDimensions("4:5", 256)).toEqual({ w: 256, h: 320 });
    expect(aspectDimensions("9:16", 256)).toEqual({ w: 256, h: 455 });
    // Exact native width stays native.
    expect(aspectDimensions("1:1", 1080)).toEqual({ w: 1080, h: 1080 });
  });

  it("allows modest upscale for print-shop high-dpi packs (capped 2×)", () => {
    // S3-Reach: window cling 2× square.
    expect(aspectDimensions("1:1", 2160)).toEqual({ w: 2160, h: 2160 });
    // Above 2× native is clamped (1080×2 = 2160).
    expect(aspectDimensions("1:1", 5000)).toEqual({ w: 2160, h: 2160 });
    // exportScale path via formatDimensions third arg (aspectDimensions only
    // passes targetWidth — exercise formatDimensions directly if exported).
    expect(aspectDimensions("wide", 3840)).toEqual({ w: 3840, h: 2160 });
  });

  it("fitCover crops the wider source horizontally to fill a square", () => {
    expect(fitCover(2000, 1000, 1000, 1000)).toEqual({
      sx: 500,
      sy: 0,
      sw: 1000,
      sh: 1000,
    });
  });

  it("fitCover crops the taller source vertically", () => {
    expect(fitCover(1000, 2000, 1000, 1000)).toEqual({
      sx: 0,
      sy: 500,
      sw: 1000,
      sh: 1000,
    });
  });

  it("changes the source rectangle when focal position or zoom changes", () => {
    const cropFit = fitCover as unknown as (
      srcW: number,
      srcH: number,
      dstW: number,
      dstH: number,
      crop: { x: number; y: number; zoom: number },
    ) => { sx: number; sy: number; sw: number; sh: number };
    const centered = cropFit(2000, 1000, 1000, 1000, {
      x: 0.5,
      y: 0.5,
      zoom: 1,
    });
    const focusedAndZoomed = cropFit(2000, 1000, 1000, 1000, {
      x: 0,
      y: 0.5,
      zoom: 2,
    });

    expect(focusedAndZoomed.sx).toBeLessThan(centered.sx);
    expect(focusedAndZoomed.sw).toBeLessThan(centered.sw);
  });

  it("wraps a long localized headline and truncates predictably", () => {
    const ctx = {
      measureText: (value: string) => ({ width: value.length * 10 }),
    } as unknown as CanvasRenderingContext2D;
    const lines = wrapText(
      ctx,
      "Probá nuestros ravioles artesanales rellenos de calabaza y salvia",
      190,
      2,
    );

    expect(lines).toHaveLength(2);
    expect(lines[0]).toBe("Probá nuestros");
    expect(lines[1].endsWith("…")).toBe(true);
  });

  it("wraps CJK text without spaces and keeps every line within the width budget", () => {
    const ctx = {
      measureText: (value: string) => ({
        width: Array.from(value).length * 10,
      }),
    } as unknown as CanvasRenderingContext2D;

    const lines = wrapText(ctx, "超長無空白價格文字", 40, 2);

    expect(lines).toEqual(["超長無空", "白價格…"]);
    expect(lines.every((line) => ctx.measureText(line).width <= 40)).toBe(true);
  });

  it("breaks long price-like values deterministically instead of overflowing", () => {
    const ctx = {
      measureText: (value: string) => ({
        width: Array.from(value).length * 10,
      }),
    } as unknown as CanvasRenderingContext2D;

    const lines = wrapText(ctx, "$1234567890USD", 50, 2);

    expect(lines).toEqual(["$1234", "5678…"]);
    expect(lines.every((line) => ctx.measureText(line).width <= 50)).toBe(true);
  });

  it("never splits an extended grapheme cluster while wrapping unbroken text", () => {
    const family = "👨‍👩‍👧‍👦";
    const segmenter = new Intl.Segmenter("en", { granularity: "grapheme" });
    const ctx = {
      measureText: (value: string) => ({
        width: Array.from(segmenter.segment(value)).length * 10,
      }),
    } as unknown as CanvasRenderingContext2D;

    expect(wrapText(ctx, `${family}${family}${family}`, 20, 2)).toEqual([
      `${family}${family}`,
      family,
    ]);
  });

  it.each([
    ["combining marks", "e\u0301"],
    ["regional flags", "🇦🇷"],
    ["skin tones", "👍🏽"],
    ["ZWJ families", "👨‍👩‍👧‍👦"],
  ])(
    "keeps %s intact when Intl.Segmenter is unavailable",
    (_label, cluster) => {
      const originalSegmenter = Intl.Segmenter;
      Object.defineProperty(Intl, "Segmenter", {
        configurable: true,
        value: undefined,
      });
      const ctx = {
        measureText: (value: string) => ({
          width: Array.from(value.split(cluster).join("x")).length * 10,
        }),
      } as unknown as CanvasRenderingContext2D;

      try {
        expect(wrapText(ctx, `${cluster}${cluster}${cluster}`, 20, 2)).toEqual([
          `${cluster}${cluster}`,
          cluster,
        ]);
      } finally {
        Object.defineProperty(Intl, "Segmenter", {
          configurable: true,
          value: originalSegmenter,
        });
      }
    },
  );

  // `resolveColor` and its WHITE / WHITE_MUTED / INK constants went with
  // `drawSlot`: the scene walker paints a `TextNode` whose colour was already
  // resolved upstream by `buildScene`, against the surface it will actually sit
  // on and at a 4.5:1 floor. There is no longer a paint-time colour token to
  // resolve, so the helper had no caller left but this test.

  it("sampleBottomLuminance returns mid-range when pixels cannot be read", () => {
    const ctx = {
      getImageData: () => {
        throw new Error("tainted");
      },
    } as unknown as CanvasRenderingContext2D;
    expect(sampleBottomLuminance(ctx, 1080, 1350)).toBe(128);
  });
});

describe("aspect-specific template geometry", () => {
  it("keeps every Story slot within the 12%-82% platform-safe band", () => {
    TEMPLATE_ORDER.forEach((id) => {
      Object.values(TEMPLATES[id].layouts["9:16"].slots).forEach((slot) => {
        expect(slot?.rect.y).toBeGreaterThanOrEqual(0.12);
        expect((slot?.rect.y ?? 0) + (slot?.rect.h ?? 0)).toBeLessThanOrEqual(
          0.82,
        );
      });
    });
  });

  it("uses a distinct composition for square, feed, and Story", () => {
    TEMPLATE_ORDER.forEach((id) => {
      expect(
        new Set(
          Object.values(TEMPLATES[id].layouts).map((value) =>
            JSON.stringify(value),
          ),
        ).size,
      ).toBe(3);
    });
  });

  it("uses the exact active template layout bounds for the Story safe-zone overlay", () => {
    const customBounds = { x: 0.08, y: 0.2, w: 0.84, h: 0.5 };
    render(
      React.createElement(
        PlatformPreviewFrame,
        {
          aspect: "9:16",
          platformLabel: "Story",
          contentBounds: customBounds,
        } as React.ComponentProps<typeof PlatformPreviewFrame>,
        React.createElement("div", null, "preview"),
      ),
    );
    const top = document.querySelector<HTMLElement>('[data-safe-zone="top"]');
    const bottom = document.querySelector<HTMLElement>(
      '[data-safe-zone="bottom"]',
    );
    expect(top).not.toBeNull();
    expect(bottom).not.toBeNull();
    if (!top || !bottom) return;
    expect(top.style.height).toBe(`${customBounds.y * 100}%`);
    expect(bottom.style.height).toBe(
      `${(1 - customBounds.y - customBounds.h) * 100}%`,
    );
  });
});

describe("recorded canvas renderer", () => {
  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("brand-fills the background when there is no photo", async () => {
    const { fillRects } = stubCanvas();
    const imageSpy = jest.spyOn(global, "Image" as never);
    await renderPost({
      template: EDITORIAL,
      aspect: "4:5",
      photoUrl: "",
      slots: {},
      palette: brand,
    });
    // The brand fill used to be a primary -> secondary vertical gradient painted
    // by `paintBrandFill`. The scene walker lays the surface down as one flat
    // full-canvas fill instead, so this asserts the fill rather than a gradient.
    // The colour is still the brand's: the editorial kit does not prefer a light
    // surface, so the surface IS the brand primary — #1a6b6a already clears
    // 4.5:1 against white, so `conditionSurface` hands it back untouched.
    expect(fillRects[0]).toEqual({
      x: 0,
      y: 0,
      w: 1080,
      h: 1350,
      fillStyle: brand.primary,
    });
    expect(imageSpy).not.toHaveBeenCalled();
    imageSpy.mockRestore();
  });

  it("renders a thumbnail-sized canvas when targetWidth is set", async () => {
    stubCanvas();
    const canvas = await renderPost(
      {
        template: EDITORIAL,
        aspect: "4:5",
        photoUrl: "",
        slots: { dishName: "Ravioles" },
        palette: brand,
      },
      { targetWidth: 256 },
    );
    expect(canvas.width).toBe(256);
    expect(canvas.height).toBe(320);
  });

  it("keeps full export size when only an AbortSignal is passed (legacy)", async () => {
    stubCanvas();
    const controller = new AbortController();
    const canvas = await renderPost(
      {
        template: EDITORIAL,
        aspect: "1:1",
        photoUrl: "",
        slots: {},
        palette: brand,
      },
      controller.signal,
    );
    expect(canvas.width).toBe(1080);
    expect(canvas.height).toBe(1080);
  });

  it("scales typography from canvas width instead of canvas height", async () => {
    const { text } = stubCanvas();
    const input = {
      template: EDITORIAL,
      photoUrl: "",
      slots: { dishName: "Ravioles" },
      palette: brand,
    };

    await renderPost({ ...input, aspect: "1:1" });
    const squareFont = text.find((call) => call.text === "Ravioles")?.font;
    text.length = 0;
    await renderPost({ ...input, aspect: "9:16" });
    const storyFont = text.find((call) => call.text === "Ravioles")?.font;

    expect(squareFont).toBeDefined();
    expect(storyFont).toBe(squareFont);
  });

  it("maps a Serif brand headline to DM Serif Display and utility copy to DM Sans", async () => {
    const { text } = stubCanvas();
    await renderPost({
      template: EDITORIAL,
      aspect: "4:5",
      photoUrl: "",
      slots: { dishName: "Ravioles", price: "$18" },
      palette: brand,
    });

    expect(text.find((call) => call.text === "Ravioles")?.font).toContain(
      "DM Serif Display",
    );
    expect(text.find((call) => call.text === "$18")?.font).toContain("DM Sans");
  });

  it("passes crop position and zoom through to the canvas source rectangle", async () => {
    const restoreImage = stubLoadedImage();
    const { drawImages } = stubCanvas(110);
    try {
      await renderPost({
        template: EDITORIAL,
        aspect: "1:1",
        photoUrl: "https://cdn.example/dish.jpg",
        slots: {},
        palette: brand,
        crop: { x: 0, y: 0.5, zoom: 2 },
      });
    } finally {
      // Restore in finally: a throw here would otherwise leak the stubbed
      // Image into every later test in this file.
      restoreImage();
    }

    // The analysis pre-pass draws the source into a 64×64 buffer; the scene
    // photo is the nine-arg cover-fit draw that is not that square.
    const sceneDraws = drawImages.filter(
      (args) => !(args[7] === 64 && args[8] === 64),
    );
    expect(sceneDraws).toHaveLength(1);
    expect(sceneDraws[0].slice(1, 5)).toEqual([0, 250, 500, 500]);
  });

  it("adds the layout's adaptive scrim when a bright photo has low contrast", async () => {
    const restoreImage = stubLoadedImage();
    const { gradients, fillRects } = stubCanvas(240);
    try {
      await renderPost({
        template: EDITORIAL,
        aspect: "4:5",
        photoUrl: "https://cdn.example/bright.jpg",
        slots: { dishName: "Ravioles" },
        palette: brand,
      });
    } finally {
      // Restore in finally: a throw here would otherwise leak the stubbed
      // Image into every later test in this file.
      restoreImage();
    }

    const layout = EDITORIAL.layouts["4:5"];
    const dims = aspectDimensions("4:5");
    // Still the pre-rewrite template's declared scrim band, to the pixel: the
    // legacyEditorial composition's bounds plus buildScene's scrim bleed
    // reproduce it. Compared field-by-field with a tolerance rather than by deep
    // equality because the scene derives the band by arithmetic (bounds bottom +
    // bleed, clamped) where the template stated it as a literal, so the height
    // lands on 783.0000000000001 instead of 783.
    const expectedScrim = {
      x: layout.scrim.region.x * dims.w,
      y: layout.scrim.region.y * dims.h,
      w: layout.scrim.region.w * dims.w,
      h: layout.scrim.region.h * dims.h,
    };
    const scrimFill = fillRects.find(
      (rect) =>
        Math.abs(rect.y - expectedScrim.y) < 1 &&
        Math.abs(rect.h - expectedScrim.h) < 1,
    );
    expect(scrimFill).toBeDefined();
    expect(scrimFill?.x).toBeCloseTo(expectedScrim.x, 6);
    expect(scrimFill?.y).toBeCloseTo(expectedScrim.y, 6);
    expect(scrimFill?.w).toBeCloseTo(expectedScrim.w, 6);
    expect(scrimFill?.h).toBeCloseTo(expectedScrim.h, 6);
    const scrimGradients = gradients.filter(
      (gradient) =>
        gradient.args[0] === layout.scrim.region.x * dims.w &&
        gradient.args[1] === layout.scrim.region.y * dims.h,
    );
    expect(scrimGradients).toHaveLength(2);
    expect(scrimGradients[0].stops).toEqual([
      [0, layout.scrim.from],
      [1, layout.scrim.to],
    ]);
    expect(scrimGradients[1].stops[0]).toEqual([0, "rgba(28,25,23,0)"]);
    expect(scrimGradients[1].stops[1]?.[1]).toMatch(
      /^rgba\(28,25,23,0\.(?:7|8)/,
    );
    expect(scrimGradients[1].stops).not.toEqual(scrimGradients[0].stops);
  });

  it("deduplicates shared image and font loads", async () => {
    let imageCount = 0;
    const OriginalImage = global.Image;
    class CountedImage {
      crossOrigin = "";
      naturalWidth = 2000;
      naturalHeight = 1000;
      onload: (() => void) | null = null;
      onerror: (() => void) | null = null;
      constructor() {
        imageCount += 1;
      }
      set src(_value: string) {
        queueMicrotask(() => this.onload?.());
      }
    }
    Object.defineProperty(global, "Image", {
      configurable: true,
      value: CountedImage,
    });
    const fontLoad = jest.fn(() => Promise.resolve([]));
    const originalFonts = Object.getOwnPropertyDescriptor(document, "fonts");
    Object.defineProperty(document, "fonts", {
      configurable: true,
      value: { load: fontLoad, ready: Promise.resolve() },
    });
    stubCanvas();
    const input = {
      template: EDITORIAL,
      aspect: "4:5" as const,
      photoUrl: "https://cdn.example/shared.jpg",
      slots: {},
      palette: brand,
    };

    try {
      await Promise.all([renderPost(input), renderPost(input)]);
      expect(imageCount).toBe(1);
      expect(fontLoad).toHaveBeenCalledTimes(5);
    } finally {
      Object.defineProperty(global, "Image", {
        configurable: true,
        value: OriginalImage,
      });
      if (originalFonts)
        Object.defineProperty(document, "fonts", originalFonts);
      else delete (document as unknown as { fonts?: FontFaceSet }).fonts;
    }
  });

  it("rejects with photo_load_failed when the requested photo fails to load", async () => {
    mockLogError.mockClear();
    const restoreImage = stubFailingImages();
    const { fillRects, drawImages } = stubCanvas();
    const photoUrl = "https://cdn.example.com/broken.jpg";

    try {
      // The audit case (L4-21): a paid, generated asset 503s. Resolving here
      // with the no-photo layout is what made previewState report "ready",
      // armed export, and put "✓ Foto" over a blank preview — the render must
      // fail so PostPreview's failure state and exportBlockedReason
      // ("photo_broken") actually fire for the failure they were built for.
      await expect(
        renderPost({
          template: EDITORIAL,
          aspect: "4:5",
          photoUrl,
          slots: { dishName: "Milanesa" },
          palette: brand,
        }),
      ).rejects.toThrow("photo_load_failed");
      // Nothing may be painted on the way out — the rejection happens before
      // any canvas is allocated, so a consumer can never mistake the result
      // for a finished post.
      expect(fillRects).toHaveLength(0);
      expect(drawImages).toHaveLength(0);
      // A real load failure (as opposed to an abort) must be reported so a
      // bucket-wide CORS misconfiguration is visible on-call rather than
      // silently degrading every card.
      expect(mockLogError).toHaveBeenCalledTimes(1);
      const loggedError = mockLogError.mock.calls[0][0] as Error;
      // The URL is folded into the message (not just additionalInfo) so
      // logError's debounce key — component::function::message — treats
      // each distinct failing image as its own signature; see the dedicated
      // "differentiates failing images" test below for the multi-URL case.
      expect(loggedError.message).toBe(`image_load_failed: ${photoUrl}`);
      // The original onerror failure must still be reachable so the Sentry
      // stack trace points at the real failure site, not just this catch.
      expect(loggedError.cause).toBeInstanceOf(Error);
      expect((loggedError.cause as Error).message).toBe("image_load_failed");
      expect(mockLogError.mock.calls[0][1]).toBe("renderPost");
      expect(mockLogError.mock.calls[0][3]).toMatchObject({
        asset: "photo",
        url: photoUrl,
      });
    } finally {
      restoreImage();
    }
  });

  it("differentiates failing images in telemetry by URL and strips query strings from signed URLs", async () => {
    mockLogError.mockClear();
    const restoreImage = stubFailingImages();
    stubCanvas();
    const firstUrl = "https://cdn.example.com/broken-a.jpg";
    const secondUrl =
      "https://bucket.s3.amazonaws.com/broken-b.jpg?X-Amz-Signature=super-secret-signature&X-Amz-Expires=900";

    try {
      await expect(
        renderPost({
          template: EDITORIAL,
          aspect: "4:5",
          photoUrl: firstUrl,
          slots: {},
          palette: brand,
        }),
      ).rejects.toThrow("photo_load_failed");
      await expect(
        renderPost({
          template: EDITORIAL,
          aspect: "4:5",
          photoUrl: secondUrl,
          slots: {},
          palette: brand,
        }),
      ).rejects.toThrow("photo_load_failed");

      expect(mockLogError).toHaveBeenCalledTimes(2);
      const firstError = mockLogError.mock.calls[0][0] as Error;
      const secondError = mockLogError.mock.calls[1][0] as Error;

      // This is the actual regression guard: two distinct broken URLs must
      // produce two distinct messages, or logError's per-signature debounce
      // (component::function::message) collapses every broken image in a
      // grid into a single report within the debounce window — exactly the
      // shape of a bucket-wide CORS/S3 outage on-call would otherwise miss.
      expect(firstError.message).not.toBe(secondError.message);
      expect(firstError.message).toBe(`image_load_failed: ${firstUrl}`);
      // The query string carries the S3 signature and must never reach the
      // message or additionalInfo — only the stripped base URL should.
      expect(secondError.message).toBe(
        "image_load_failed: https://bucket.s3.amazonaws.com/broken-b.jpg",
      );
      expect(secondError.message).not.toContain("X-Amz-Signature");
      expect(mockLogError.mock.calls[1][3]).toMatchObject({
        asset: "photo",
        url: "https://bucket.s3.amazonaws.com/broken-b.jpg",
      });
      expect(JSON.stringify(mockLogError.mock.calls[1])).not.toContain(
        "super-secret-signature",
      );
    } finally {
      restoreImage();
    }
  });

  it("strips userinfo credentials from a failing image URL before it reaches telemetry", async () => {
    mockLogError.mockClear();
    const restoreImage = stubFailingImages();
    stubCanvas();
    const accessKey = "AKIAIOSFODNN7EXAMPLE";
    const secretKey = "wJalrXUtnFEMIsupersecret";
    const photoUrl = `https://${accessKey}:${secretKey}@bucket.s3.amazonaws.com/dish.jpg`;

    try {
      await expect(
        renderPost({
          template: EDITORIAL,
          aspect: "4:5",
          photoUrl,
          slots: {},
          palette: brand,
        }),
      ).rejects.toThrow("photo_load_failed");

      expect(mockLogError).toHaveBeenCalledTimes(1);
      const loggedError = mockLogError.mock.calls[0][0] as Error;
      // Userinfo is credential material exactly like the SigV4 query params,
      // and the sanitizer's whole reason to exist is that credentials must
      // not leave the client — in the message or in additionalInfo.
      expect(loggedError.message).toBe(
        "image_load_failed: https://bucket.s3.amazonaws.com/dish.jpg",
      );
      expect(loggedError.message).not.toContain(accessKey);
      expect(loggedError.message).not.toContain(secretKey);
      expect(mockLogError.mock.calls[0][3]).toMatchObject({
        asset: "photo",
        url: "https://bucket.s3.amazonaws.com/dish.jpg",
      });
      const reportedInfo = JSON.stringify(mockLogError.mock.calls[0][3]);
      expect(reportedInfo).not.toContain(accessKey);
      expect(reportedInfo).not.toContain(secretKey);
    } finally {
      restoreImage();
    }
  });

  it("keeps the distinguishing tail when capping an over-long image URL", async () => {
    mockLogError.mockClear();
    const restoreImage = stubFailingImages();
    stubCanvas();
    // A shared prefix longer than the telemetry cap: truncating to a fixed
    // head alone collapses both URLs into one message, which re-breaks
    // logError's component::function::message debounce key.
    const sharedPrefix = `https://cdn.example.com/${"a".repeat(280)}/`;
    expect(sharedPrefix.length).toBeGreaterThan(300);
    const firstUrl = `${sharedPrefix}alpha-one.jpg`;
    const secondUrl = `${sharedPrefix}alpha-two.jpg`;

    try {
      await expect(
        renderPost({
          template: EDITORIAL,
          aspect: "4:5",
          photoUrl: firstUrl,
          slots: {},
          palette: brand,
        }),
      ).rejects.toThrow("photo_load_failed");
      await expect(
        renderPost({
          template: EDITORIAL,
          aspect: "4:5",
          photoUrl: secondUrl,
          slots: {},
          palette: brand,
        }),
      ).rejects.toThrow("photo_load_failed");

      expect(mockLogError).toHaveBeenCalledTimes(2);
      const firstMessage = (mockLogError.mock.calls[0][0] as Error).message;
      const secondMessage = (mockLogError.mock.calls[1][0] as Error).message;

      expect(firstMessage).not.toBe(secondMessage);
      expect(firstMessage).toContain("-one.jpg");
      expect(secondMessage).toContain("-two.jpg");
      // Still bounded: the cap exists so the debounce key stays small.
      expect(firstMessage.length).toBeLessThanOrEqual(
        "image_load_failed: ".length + 301,
      );
    } finally {
      restoreImage();
    }
  });

  it("still rejects with an AbortError when the signal aborts while the photo is loading, without logging it", async () => {
    mockLogError.mockClear();
    const images = stubDeferredImages();
    stubCanvas();
    const controller = new AbortController();
    const photoUrl = "https://cdn.example/mid-abort.jpg";

    try {
      const promise = renderPost(
        {
          template: EDITORIAL,
          aspect: "4:5",
          photoUrl,
          slots: {},
          palette: brand,
        },
        controller.signal,
      );

      // Wait until the photo load has actually started (src assigned)
      // before aborting, so this reaches loadOptionalImage's catch handler
      // mid-flight rather than one of the earlier throwIfAborted guards.
      while (!images.pending.has(photoUrl)) {
        await Promise.resolve();
      }
      controller.abort();

      // NOTE: this rejection assertion alone would still pass even if
      // loadOptionalImage's AbortError rethrow were deleted — the
      // throwIfAborted(signal) immediately after the Promise.all in
      // renderPost independently catches the aborted signal. The assertion
      // that actually pins the rethrow is the one below: without it, the
      // catch handler falls through and reports the abort to Sentry, which
      // is the real regression this test guards against. Aborts fire on
      // every re-render and scroll, so reporting them would spam Sentry.
      await expect(promise).rejects.toMatchObject({ name: "AbortError" });
      expect(mockLogError).not.toHaveBeenCalled();
    } finally {
      images.restore();
    }
  });

  describe("negative image cache", () => {
    /**
     * Root cause of findings 19/32/33: onerror deleted the cache entry, so
     * every re-render re-requested permanently-404ing URLs and re-fired
     * Sentry via loadOptionalImage. Failures must stay cached for a TTL;
     * aborts must not.
     */
    it("does not re-request a URL that failed once within the TTL", async () => {
      mockLogError.mockClear();
      let imageCount = 0;
      let srcAssignments = 0;
      const OriginalImage = global.Image;
      class CountedFailingImage {
        crossOrigin = "";
        naturalWidth = 0;
        naturalHeight = 0;
        onload: (() => void) | null = null;
        onerror: (() => void) | null = null;
        constructor() {
          imageCount += 1;
        }
        set src(_value: string) {
          srcAssignments += 1;
          queueMicrotask(() => this.onerror?.());
        }
      }
      Object.defineProperty(global, "Image", {
        configurable: true,
        value: CountedFailingImage,
      });
      stubCanvas();
      const photoUrl = "https://cdn.example.com/negative-cache-once.jpg";
      const input = {
        template: EDITORIAL,
        aspect: "4:5" as const,
        photoUrl,
        slots: {},
        palette: brand,
      };

      try {
        await expect(renderPost(input)).rejects.toThrow("photo_load_failed");
        await expect(renderPost(input)).rejects.toThrow("photo_load_failed");
        // Second render must reuse the negative cache — no new Image, no
        // second network assignment.
        expect(imageCount).toBe(1);
        expect(srcAssignments).toBe(1);
      } finally {
        Object.defineProperty(global, "Image", {
          configurable: true,
          value: OriginalImage,
        });
      }
    });

    it("fires exactly one logError per URL regardless of render count", async () => {
      mockLogError.mockClear();
      const restoreImage = stubFailingImages();
      stubCanvas();
      const photoUrl = "https://cdn.example.com/negative-cache-once-log.jpg";
      const input = {
        template: EDITORIAL,
        aspect: "4:5" as const,
        photoUrl,
        slots: {},
        palette: brand,
      };

      try {
        await expect(renderPost(input)).rejects.toThrow("photo_load_failed");
        await expect(renderPost(input)).rejects.toThrow("photo_load_failed");
        await expect(renderPost(input)).rejects.toThrow("photo_load_failed");
        expect(mockLogError).toHaveBeenCalledTimes(1);
        expect((mockLogError.mock.calls[0][0] as Error).message).toBe(
          `image_load_failed: ${photoUrl}`,
        );
      } finally {
        restoreImage();
      }
    });

    it("does not poison the cache with AbortError — a later load may succeed", async () => {
      mockLogError.mockClear();
      let imageCount = 0;
      const OriginalImage = global.Image;
      const pending = new Map<
        string,
        { resolve: () => void; reject: () => void }
      >();
      class DeferredThenLiveImage {
        crossOrigin = "";
        naturalWidth = 2000;
        naturalHeight = 1000;
        onload: (() => void) | null = null;
        onerror: (() => void) | null = null;
        constructor() {
          imageCount += 1;
        }
        set src(value: string) {
          // First construction stays deferred so we can abort mid-flight.
          // Subsequent constructions resolve successfully (abort must not
          // have written a failed cache entry).
          if (imageCount === 1) {
            pending.set(value, {
              resolve: () => this.onload?.(),
              reject: () => this.onerror?.(),
            });
            return;
          }
          queueMicrotask(() => this.onload?.());
        }
      }
      Object.defineProperty(global, "Image", {
        configurable: true,
        value: DeferredThenLiveImage,
      });
      stubCanvas();
      const photoUrl = "https://cdn.example.com/negative-cache-abort.jpg";
      const controller = new AbortController();

      try {
        const aborted = renderPost(
          {
            template: EDITORIAL,
            aspect: "4:5",
            photoUrl,
            slots: {},
            palette: brand,
          },
          controller.signal,
        );
        while (!pending.has(photoUrl)) {
          await Promise.resolve();
        }
        controller.abort();
        await expect(aborted).rejects.toMatchObject({ name: "AbortError" });
        expect(mockLogError).not.toHaveBeenCalled();

        // Same URL after abort must be free to load — not pinned as failed.
        const canvas = await renderPost({
          template: EDITORIAL,
          aspect: "4:5",
          photoUrl,
          slots: {},
          palette: brand,
        });
        expect(canvas).toBeTruthy();
        expect(imageCount).toBe(2);
        expect(mockLogError).not.toHaveBeenCalled();
      } finally {
        Object.defineProperty(global, "Image", {
          configurable: true,
          value: OriginalImage,
        });
      }
    });
  });
});

describe("PostPreview lifecycle", () => {
  afterEach(() => {
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  it("shows a visible polite status while rendering and generating", () => {
    const images = stubDeferredImages();
    stubCanvas();
    const props = {
      renderInput: {
        template: EDITORIAL,
        aspect: "4:5" as const,
        photoUrl: "https://cdn.example/pending.jpg",
        slots: {},
        palette: brand,
      },
      isGenerating: false,
      emptyLabel: "No photo",
      previewErrorLabel: "Preview unavailable",
      previewRetryLabel: "Try preview again",
      renderingLabel: "Rendering preview",
      generatingLabel: "Generating photo",
    };
    const { rerender } = render(React.createElement(PostPreview, props));

    const rendering = screen.getByRole("status");
    expect(rendering).toHaveAttribute("aria-live", "polite");
    expect(rendering).toHaveTextContent("Rendering preview");
    expect(rendering).toBeVisible();

    rerender(
      React.createElement(PostPreview, { ...props, isGenerating: true }),
    );
    expect(screen.getByRole("status")).toHaveTextContent("Generating photo");
    images.restore();
  });

  it("abandons rapid edits before canvas allocation and commits only the latest render", async () => {
    jest.useFakeTimers();
    const images = stubDeferredImages();
    const { text } = stubCanvas();
    const createElement = jest.spyOn(document, "createElement");
    const props = {
      isGenerating: false,
      emptyLabel: "No photo",
      previewErrorLabel: "Preview unavailable",
      previewRetryLabel: "Try preview again",
      renderingLabel: "Rendering preview",
      generatingLabel: "Generating photo",
    };
    const { rerender, container } = render(
      React.createElement(PostPreview, {
        ...props,
        renderInput: {
          template: EDITORIAL,
          aspect: "4:5",
          photoUrl: "https://cdn.example/first.jpg",
          slots: { dishName: "First" },
          palette: brand,
        },
      }),
    );

    await act(async () => {
      jest.advanceTimersByTime(120);
      await Promise.resolve();
    });
    rerender(
      React.createElement(PostPreview, {
        ...props,
        renderInput: {
          template: EDITORIAL,
          aspect: "4:5",
          photoUrl: "https://cdn.example/latest.jpg",
          slots: { dishName: "Latest" },
          palette: brand,
        },
      }),
    );
    await act(async () => {
      jest.advanceTimersByTime(120);
      await Promise.resolve();
    });

    const canvasAllocations = () =>
      createElement.mock.calls.filter(([tag]) => tag === "canvas").length;
    expect(canvasAllocations()).toBe(0);
    await act(async () => {
      images.pending.get("https://cdn.example/first.jpg")?.resolve();
      await Promise.resolve();
    });
    expect(canvasAllocations()).toBe(0);
    await act(async () => {
      images.pending.get("https://cdn.example/latest.jpg")?.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    // One export canvas plus the 64×64 analysis buffer for the loaded photo.
    // The abandoned "first" render must still contribute zero allocations.
    expect(canvasAllocations()).toBe(2);
    expect(text.map(({ text: value }) => value)).toEqual(["Latest"]);
    expect(container.querySelector("canvas")).not.toBeNull();
    images.restore();
  });

  it("shows an accurate failure with an accessible retry and never the empty-photo message", async () => {
    jest.useFakeTimers();
    jest.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(null);
    const createElement = jest.spyOn(document, "createElement");
    render(
      React.createElement(PostPreview, {
        renderInput: {
          template: EDITORIAL,
          aspect: "4:5",
          photoUrl: "",
          slots: {},
          palette: brand,
        },
        isGenerating: false,
        emptyLabel: "No photo",
        previewErrorLabel: "Preview unavailable",
        previewRetryLabel: "Try preview again",
        renderingLabel: "Rendering preview",
        generatingLabel: "Generating photo",
      }),
    );

    await act(async () => {
      jest.advanceTimersByTime(120);
      await Promise.resolve();
    });
    expect(screen.getByRole("alert")).toHaveTextContent("Preview unavailable");
    expect(screen.queryByText("No photo")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Try preview again" }));
    await act(async () => {
      jest.advanceTimersByTime(120);
      await Promise.resolve();
    });
    expect(
      createElement.mock.calls.filter(([tag]) => tag === "canvas"),
    ).toHaveLength(2);
  });

  it("fails honestly when the paid photo cannot load, and retry re-reads the same asset past the negative cache", async () => {
    jest.useFakeTimers();
    const OriginalImage = global.Image;
    let imageCount = 0;
    let srcAssignments = 0;
    // The audit case (L4-21): the generated (already-paid) asset 503s on the
    // first read, then becomes available. Retry must issue a NEW request for
    // the SAME URL — never replay the 5-minute negative-cache rejection, and
    // never regenerate (which would burn a second credit).
    class FailOnceThenLiveImage {
      crossOrigin = "";
      naturalWidth = 2000;
      naturalHeight = 1000;
      onload: (() => void) | null = null;
      onerror: (() => void) | null = null;
      constructor() {
        imageCount += 1;
      }
      set src(_value: string) {
        srcAssignments += 1;
        // Promise microtasks, not queueMicrotask: fake timers fake
        // queueMicrotask, and this test drains the REAL microtask queue.
        if (srcAssignments === 1) void Promise.resolve().then(() => this.onerror?.());
        else void Promise.resolve().then(() => this.onload?.());
      }
    }
    Object.defineProperty(global, "Image", {
      configurable: true,
      value: FailOnceThenLiveImage,
    });
    stubCanvas();
    const onRenderStateChange = jest.fn();

    try {
      const { container } = render(
        React.createElement(PostPreview, {
          renderInput: {
            template: EDITORIAL,
            aspect: "4:5",
            photoUrl: "https://cdn.example/paid-generated-asset.jpg",
            slots: { dishName: "Milanesa" },
            palette: brand,
          },
          isGenerating: false,
          emptyLabel: "No photo",
          previewErrorLabel: "Preview unavailable",
          previewRetryLabel: "Try preview again",
          renderingLabel: "Rendering preview",
          generatingLabel: "Generating photo",
          onRenderStateChange,
        }),
      );

      await act(async () => {
        jest.advanceTimersByTime(120);
        // The load→degrade→reject chain crosses several awaits; drain enough
        // microtask ticks for the failure to reach React state.
        for (let tick = 0; tick < 25; tick += 1) await Promise.resolve();
      });

      // A failed photo must surface as a failure — never a silent no-photo
      // render that reports "ready" and arms export over a blank preview.
      expect(screen.getByRole("alert")).toHaveTextContent("Preview unavailable");
      expect(onRenderStateChange).toHaveBeenLastCalledWith("failed");
      expect(container.querySelector("canvas")).toBeNull();

      fireEvent.click(screen.getByRole("button", { name: "Try preview again" }));
      await act(async () => {
        jest.advanceTimersByTime(120);
        // The load→degrade→reject chain crosses several awaits; drain enough
        // microtask ticks for the failure to reach React state.
        for (let tick = 0; tick < 25; tick += 1) await Promise.resolve();
      });

      // The retry made a real second request for the paid asset (the failure
      // is inside the negative-cache TTL, so this only happens if retry
      // evicts the failed entry first) and the preview now reports ready.
      expect(imageCount).toBe(2);
      expect(srcAssignments).toBe(2);
      expect(onRenderStateChange).toHaveBeenLastCalledWith("ready");
      expect(container.querySelector("canvas")).not.toBeNull();
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    } finally {
      Object.defineProperty(global, "Image", {
        configurable: true,
        value: OriginalImage,
      });
    }
  });
});

/**
 * The switch-over: `renderPost` keeps its exported signature but paints through
 * `buildScene` + `renderScene` instead of the deleted per-template draw code.
 *
 * Nothing here asserts on a kit or composition id, because `renderPost` returns
 * a canvas and never names one. Each test instead pins a mark on that canvas
 * that only one composition could have produced — an anchor x, a font size, a
 * drawImage arity — read off `COMPOSITIONS`/`KITS` rather than hard-coded, so a
 * retuned layout moves the expectation with it instead of failing.
 */
describe("renderPost on the scene system", () => {
  afterEach(() => {
    jest.restoreAllMocks();
  });

  /** Font size in px out of a recorded CSS font string. */
  const fontPx = (font?: string): number =>
    Number.parseFloat(font?.match(/(\d+)px/)?.[1] ?? "0");

  const sceneBrand = { primary: "#1a6b6a", secondary: "#0f3d3c" };

  it("renders a kit + composition when supplied", async () => {
    const { text } = stubCanvas();
    await renderPost({
      kit: "chalkboard",
      composition: "photoBottomStack",
      aspect: "4:5",
      photoUrl: "",
      slots: { dishName: "Milanesa", cta: "Order now" },
      palette: sceneBrand,
    });
    expect(text.map((entry) => entry.text)).toEqual(
      expect.arrayContaining(["Milanesa"]),
    );
  });

  it("renders a legacy template input through the legacy composition", async () => {
    const { text } = stubCanvas();
    await renderPost({
      template: EDITORIAL,
      aspect: "4:5",
      photoUrl: "",
      slots: { dishName: "Milanesa" },
      palette: sceneBrand,
    });

    expect(text.map((entry) => entry.text)).toEqual(
      expect.arrayContaining(["Milanesa"]),
    );
    // Not merely "some text was painted": a legacy TemplateDef must route to
    // ITS legacy composition, so the headline sits on that family's left edge.
    const bounds = COMPOSITIONS.legacyEditorial.boundsFor(FORMATS["4:5"]);
    expect(text.find((entry) => entry.text === "Milanesa")?.x).toBeCloseTo(
      bounds.x * 1080,
      5,
    );
  });

  it("returns a canvas at the aspect's export dimensions", async () => {
    stubCanvas();
    const canvas = await renderPost({
      kit: "editorial",
      composition: "posterStack",
      aspect: "9:16",
      photoUrl: "",
      slots: { dishName: "Milanesa" },
      palette: sceneBrand,
    });
    expect(canvas.width).toBe(1080);
    expect(canvas.height).toBe(1920);
  });

  it("falls back to the no-photo composition when a kit arrives without one", async () => {
    const { text } = stubCanvas();
    await renderPost({
      kit: "editorial",
      aspect: "1:1",
      photoUrl: "",
      slots: { dishName: "Milanesa" },
      palette: sceneBrand,
    });

    // A kit-only input must not land on a `requiresPhoto` composition. The two
    // candidates are told apart by the anchor: posterStack centres its bands,
    // photoBottomStack left-aligns them on its own bounds.
    const poster = COMPOSITIONS.posterStack.boundsFor(FORMATS["1:1"]);
    const photoFirst = COMPOSITIONS.photoBottomStack.boundsFor(FORMATS["1:1"]);
    const drawn = text.find((entry) => entry.text === "Milanesa");
    expect(drawn).toBeDefined();
    expect(drawn?.x).toBeCloseTo((poster.x + poster.w / 2) * 1080, 5);
    expect(drawn?.x).not.toBeCloseTo(photoFirst.x * 1080, 5);
  });

  it("rejects for kit-shaped input too when the requested photo fails to load", async () => {
    mockLogError.mockClear();
    const restoreImage = stubFailingImages();
    const { text } = stubCanvas();

    // The same honesty contract as the template-shaped input: a requested
    // photo that failed must fail the render, never silently re-lay the post
    // out on the no-photo path (audit L4-21). Scene-level degradation for
    // pre-loaded images stays pinned in the `buildPostScene` describe below.
    try {
      await expect(
        renderPost({
          kit: "editorial",
          aspect: "1:1",
          photoUrl: "https://cdn.example/broken-kit.jpg",
          slots: { dishName: "Milanesa" },
          palette: sceneBrand,
        }),
      ).rejects.toThrow("photo_load_failed");
    } finally {
      restoreImage();
    }
    expect(text).toHaveLength(0);
  });

  it("renderPostScene rejects the same way, so kit print formats cannot silently ship the no-photo layout", async () => {
    mockLogError.mockClear();
    const restoreImage = stubFailingImages();
    stubCanvas();

    // Campaign-kit image formats fail through renderPostToBlob → renderPost;
    // print formats go through renderPostScene. Both must fail for a broken
    // photo or one kit would mix honest failures with silent no-photo pages.
    try {
      await expect(
        renderPostScene({
          template: EDITORIAL,
          aspect: "4:5",
          photoUrl: "https://cdn.example/broken-scene.jpg",
          slots: { dishName: "Milanesa" },
          palette: sceneBrand,
        }),
      ).rejects.toThrow("photo_load_failed");
    } finally {
      restoreImage();
    }
  });

  it("builds the scene at the thumbnail canvas size, not at export size", async () => {
    const { text } = stubCanvas();
    const input = {
      kit: "editorial" as const,
      composition: "posterStack" as const,
      aspect: "4:5" as const,
      photoUrl: "",
      slots: { dishName: "Milanesa" },
      palette: sceneBrand,
    };

    await renderPost(input);
    const full = text.find((entry) => entry.text === "Milanesa");
    text.length = 0;
    await renderPost(input, { targetWidth: 256 });
    const thumb = text.find((entry) => entry.text === "Milanesa");

    // A scene solved at 1080 and painted onto a 256px canvas would keep the
    // export-size type and anchors — a 124px headline on a 256px canvas.
    expect(fontPx(full?.font)).toBeGreaterThan(0);
    expect(
      Math.abs(fontPx(thumb?.font) - (fontPx(full?.font) * 256) / 1080),
    ).toBeLessThanOrEqual(1);
    expect(thumb?.x).toBeCloseTo(((full?.x ?? 0) * 256) / 1080, 5);
  });

  it("measures the headline in the face it is painted in", async () => {
    const { ctx, text } = stubCanvas();
    // Make the two faces disagree about how wide the same string is — which is
    // what real faces do, and what the shared stub (which reads only px) cannot
    // express. The editorial kit sets its headline in DM Serif Display and its
    // body in DM Sans, so a `measure` callback that assembled its own font
    // string (hard-coding 'DM Sans', as the pre-947f18519 code did) would solve
    // the stack against the narrow face and hand the walker a rect one line
    // short of the serif glyphs it then paints.
    ctx.measureText = jest.fn((value: string) => {
      const px = Number.parseFloat(ctx.font.match(/(\d+)px/)?.[1] ?? "20");
      const perChar = ctx.font.includes("Serif") ? 0.6 : 0.25;
      return { width: value.length * px * perChar };
    });

    await renderPost({
      kit: "editorial",
      composition: "posterStack",
      aspect: "1:1",
      photoUrl: "",
      slots: { dishName: "Pollo Milanesa" },
      palette: sceneBrand,
    });

    // Solved as serif the headline needs two lines and gets a two-line rect, so
    // the walker wraps it whole. Solved as sans it needs one, and the one-line
    // rect caps `wrapText` at a single line — which ellipsizes the name.
    expect(text.map((entry) => entry.text)).toEqual(["Pollo", "Milanesa"]);
  });

  it("paints the photo, the scrim, the logo and the text through the walker", async () => {
    const restoreImage = stubLoadedImage();
    const { text, drawImages, gradients } = stubCanvas(120);

    try {
      await renderPost({
        kit: "editorial",
        composition: "photoBottomStack",
        aspect: "4:5",
        photoUrl: "https://cdn.example/dish.jpg",
        logoUrl: "https://cdn.example/logo.png",
        slots: { dishName: "Milanesa" },
        palette: sceneBrand,
        crop: { x: 0, y: 0.5, zoom: 2 },
      });
    } finally {
      restoreImage();
    }

    // The photo goes through the 9-argument drawImage (source rect +
    // destination rect); drawLogo uses the 5-argument form.
    const photoDraw = drawImages.find((args) => args.length === 9);
    const logoDraw = drawImages.find((args) => args.length === 5);
    expect(photoDraw).toBeDefined();
    expect(logoDraw).toBeDefined();

    // The crop survives the rewrite rather than being dropped on the floor.
    const crop = fitCover(2000, 1000, 1080, 1350, { x: 0, y: 0.5, zoom: 2 });
    expect(photoDraw?.slice(1, 5)).toEqual([
      crop.sx,
      crop.sy,
      crop.sw,
      crop.sh,
    ]);

    // The scrim is the kit's, not a template's.
    expect(
      gradients.some((gradient) =>
        gradient.stops.some(
          ([, color]) => color === KITS.editorial.scrimStyle.to,
        ),
      ),
    ).toBe(true);
    expect(text.map((entry) => entry.text)).toContain("Milanesa");
  });

  /**
   * A kit must never reach a legacy composition.
   *
   * The three legacy families exist to reproduce the pre-rewrite templates
   * byte-faithfully, and a kit that carries a texture paints a full-bleed
   * grain or halftone field over whatever it is paired with — destroying
   * exactly the fidelity those families exist for. `buildScene.test.ts` pins
   * that no texture reaches a legacy render today, but only as DATA: the
   * no-kit branch pins both halves together (`kitForLegacyTemplate` and
   * `LEGACY_COMPOSITION_FOR_TEMPLATE`) and those three kits happen to declare
   * `texture: "none"`.
   *
   * The kit branch had no such pinning. It read `input.composition` verbatim,
   * `isCompositionId` accepts all nine ids including the three legacy ones,
   * `seedArtDirection` resolves the stored kit and the stored composition
   * independently, and the backend's `normalizeMarketingSnapshotIdent` is
   * charset- and length-bounded only (deliberately, so the frontend-owned id
   * set can grow per wave without a Go deploy). So a stored
   * `{ kit: "ticket", composition: "legacyEditorial" }` round-tripped and
   * rendered a halftone field over a legacy layout. Unreachable from today's
   * UI — `chooseComposition` never returns a legacy id — but nothing enforced
   * it, and Waves 3 and 4 widen the same contract further.
   */
  describe("legacy compositions on the kit path", () => {
    const LEGACY_IDS = Object.values(LEGACY_COMPOSITION_FOR_TEMPLATE);

    const legacyInput = {
      kit: "ticket",
      composition: "legacyEditorial",
      aspect: "1:1",
      photoUrl: "",
      slots: { dishName: "Milanesa", price: "$12.00" },
      palette: sceneBrand,
    } as const;

    it("refuses a legacy composition on the kit path", () => {
      // The pairing under test is genuinely the dangerous one: a legacy family
      // asked for, by a kit that paints a full-bleed halftone field.
      expect(LEGACY_IDS).toContain(legacyInput.composition);
      expect(KITS[legacyInput.kit].texture).toBe("halftone");

      const resolved = resolveArtDirection(legacyInput, false);

      expect(LEGACY_IDS).not.toContain(resolved.compositionId);
      // Refused by falling back to the chooser — the one module that decides
      // what a piece of content looks like — not by naming a second default.
      // With no photo that is the designed poster.
      expect(resolved.compositionId).toBe("posterStack");
      // Only the LAYOUT is refused. The operator's kit is still theirs.
      expect(resolved.kitId).toBe("ticket");
    });

    it("refuses every legacy family for every kit, with and without a photo", () => {
      const escaped = KIT_ORDER.flatMap((kit) =>
        LEGACY_IDS.flatMap((composition) =>
          [false, true]
            .map((hasPhoto) => ({
              hasPhoto,
              ...resolveArtDirection(
                { ...legacyInput, kit, composition },
                hasPhoto,
              ),
            }))
            .filter((resolved) => LEGACY_IDS.includes(resolved.compositionId))
            .map(
              ({ hasPhoto, compositionId }) =>
                `${kit} + ${composition} (photo: ${hasPhoto}) -> ${compositionId}`,
            ),
        ),
      );

      expect(escaped).toEqual([]);
    });

    it("still honours every choosable composition verbatim", () => {
      // The guard refuses legacy ids and nothing else: an operator's explicit
      // pick has to survive, or Reshuffle silently stops working.
      const choosable = COMPOSITION_ORDER.filter(
        (id) => !LEGACY_IDS.includes(id),
      );
      expect(choosable.length).toBeGreaterThan(0);

      choosable.forEach((composition) => {
        expect(
          resolveArtDirection({ ...legacyInput, composition }, true)
            .compositionId,
        ).toBe(composition);
      });
    });

    it("paints the chosen family's geometry, not the legacy one it was handed", async () => {
      const { text } = stubCanvas();

      await renderPost(legacyInput);

      // posterStack centres its bands on its own envelope; legacyEditorial
      // left-aligns them on the template's text column. 464px apart at 1080.
      const poster = COMPOSITIONS.posterStack.boundsFor(FORMATS["1:1"]);
      const legacy = COMPOSITIONS.legacyEditorial.boundsFor(FORMATS["1:1"]);
      const drawn = text.find((entry) => entry.text === "Milanesa");
      expect(drawn).toBeDefined();
      expect(drawn?.x).toBeCloseTo((poster.x + poster.w / 2) * 1080, 5);
      expect(drawn?.x).not.toBeCloseTo(legacy.x * 1080, 5);
    });

    it("leaves the no-kit path on its own legacy family", () => {
      // The guard must not swallow the path legacy families exist FOR: a
      // pre-Wave-1 snapshot carries a template and no kit, and has to keep
      // rendering as it always did.
      const resolved = resolveArtDirection(
        {
          template: TEMPLATES.minimal,
          aspect: "1:1",
          photoUrl: "",
          slots: legacyInput.slots,
          palette: sceneBrand,
        },
        false,
      );
      expect(resolved.compositionId).toBe("legacyMinimal");
      expect(KITS[resolved.kitId].texture).toBe("none");
    });
  });
});

describe("buildPostScene", () => {
  // Structure-only: analysis needs a real 2d context and is covered elsewhere.
  // Null analysis is a legitimate Wave 2 path (tainted canvas / CORS readback).
  let analysisSpy: jest.SpyInstance;

  beforeEach(() => {
    analysisSpy = jest
      .spyOn(photoCache, "photoAnalysisFor")
      .mockReturnValue(null);
  });
  afterEach(() => {
    analysisSpy.mockRestore();
  });

  const measure = (text: string) => text.length * 10;

  const input = {
    kit: "editorial" as const,
    composition: "photoBottomStack" as const,
    aspect: "4:5" as const,
    photoUrl: "https://cdn.example.com/dish.jpg",
    logoUrl: "https://cdn.example.com/logo.png",
    slots: { dishName: "Milanesa", price: "$12.00" },
    palette: { primary: "#1a6b6a", secondary: "#0f3d3c" },
  };

  const img = (w = 800, h = 600) =>
    ({ naturalWidth: w, naturalHeight: h }) as HTMLImageElement;

  it("emits a photo node when the photo loaded", () => {
    const scene = buildPostScene(input, { photo: img(), logo: null }, {
      width: 1080,
      height: 1350,
      measure,
    });
    expect(scene.nodes.some((n) => n.kind === "photo")).toBe(true);
  });

  /**
   * The load-failure degradation, asserted at the seam rather than only through
   * `renderPost`. A CORS-less bucket rejects the load outright — `crossOrigin =
   * "anonymous"` does not merely taint the canvas — and the post must lay itself
   * out as a poster rather than around a photograph that is not there.
   */
  it("emits no photo node when the photo failed to load, even with a photoUrl", () => {
    const scene = buildPostScene(input, { photo: null, logo: img(64, 64) }, {
      width: 1080,
      height: 1350,
      measure,
    });
    expect(scene.nodes.some((n) => n.kind === "photo")).toBe(false);
    expect(scene.nodes.some((n) => n.kind === "scrim")).toBe(false);
    expect(scene.nodes.some((n) => n.kind === "logo")).toBe(true);
  });

  it("emits no logo node when the logo failed to load", () => {
    const scene = buildPostScene(input, { photo: img(), logo: null }, {
      width: 1080,
      height: 1350,
      measure,
    });
    expect(scene.nodes.some((n) => n.kind === "logo")).toBe(false);
  });

  it("returns nodes already sorted ascending by z", () => {
    const scene = buildPostScene(input, { photo: img(), logo: img(64, 64) }, {
      width: 1080,
      height: 1350,
      measure,
    });
    const zs = scene.nodes.map((n) => n.z);
    expect([...zs].sort((a, b) => a - b)).toEqual(zs);
  });

  it("solves at the size it is given, so a thumbnail is the full post scaled", () => {
    const full = buildPostScene(input, { photo: img(), logo: null }, {
      width: 1080,
      height: 1350,
      measure,
    });
    const thumb = buildPostScene(input, { photo: img(), logo: null }, {
      width: 256,
      height: 320,
      measure,
    });
    expect(thumb.width).toBe(256);
    expect(full.nodes.map((n) => n.kind)).toEqual(thumb.nodes.map((n) => n.kind));
  });
});
