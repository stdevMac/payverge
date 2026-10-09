/** @jest-environment jsdom */
import { renderPost } from "./renderPost";
import * as paletteModule from "../artDirection/palette";
import * as cacheModule from "../photo/cache";
import type { PhotoAnalysis } from "../photo/types";

jest.mock("@/utils/errorLogger", () => ({ logError: jest.fn() }));

interface TextRecord {
  text: string;
  y: number;
}

function stubCanvas() {
  const text: TextRecord[] = [];
  const ctx = {
    font: "",
    fillStyle: "",
    strokeStyle: "",
    lineWidth: 0,
    globalAlpha: 1,
    filter: "none",
    globalCompositeOperation: "source-over",
    textAlign: "left",
    textBaseline: "top",
    createLinearGradient: () => ({ addColorStop: () => {} }),
    fillRect: jest.fn(),
    fillText: jest.fn((value: string, _x: number, y: number) => {
      text.push({ text: value, y });
    }),
    measureText: (value: string) => {
      const px = Number.parseFloat(ctx.font.match(/([\d.]+)px/)?.[1] ?? "20");
      return { width: value.length * px * 0.52 };
    },
    drawImage: jest.fn(),
    getImageData: (_x: number, _y: number, w: number, h: number) => ({
      data: new Uint8ClampedArray(w * h * 4).fill(128),
    }),
    beginPath: jest.fn(),
    moveTo: jest.fn(),
    lineTo: jest.fn(),
    arcTo: jest.fn(),
    arc: jest.fn(),
    closePath: jest.fn(),
    fill: jest.fn(),
    stroke: jest.fn(),
    clip: jest.fn(),
    save: jest.fn(),
    restore: jest.fn(),
    translate: jest.fn(),
    rotate: jest.fn(),
  };
  jest
    .spyOn(HTMLCanvasElement.prototype, "getContext")
    .mockReturnValue(ctx as unknown as CanvasRenderingContext2D);
  return { ctx, text };
}

function stubLoadedImage(width = 2000, height = 1000) {
  class LoadedImage {
    crossOrigin = "";
    naturalWidth = width;
    naturalHeight = height;
    onload: (() => void) | null = null;
    onerror: (() => void) | null = null;
    set src(_value: string) {
      setTimeout(() => this.onload?.(), 0);
    }
  }
  (global as unknown as { Image: unknown }).Image = LoadedImage;
}

/** A minimal analysis with only the fields the wiring reads. */
function analysisWith(over: Partial<PhotoAnalysis>): PhotoAnalysis {
  return {
    luma: { size: 1, values: [128] },
    edges: { size: 1, values: [0] },
    blurScore: 500,
    exposure: {
      histogram: new Array(16).fill(0),
      meanLuma: 128,
      shadowClipping: 0,
      highlightClipping: 0,
      channelMeans: { r: 128, g: 128, b: 128 },
    },
    subject: { x: 0, y: 0, w: 1, h: 1 },
    focal: { x: 0.5, y: 0.5 },
    negativeSpace: "none",
    busy: false,
    dominantColors: [],
    ...over,
  };
}

const INPUT = {
  kit: "editorial" as const,
  aspect: "4:5" as const,
  photoUrl: "https://cdn/dish.png",
  slots: { dishName: "Milanesa", price: "$12.00", handle: "@casasur" },
  palette: { primary: "#1a6b6a", secondary: "#0f3d3c" },
};

let analysisSpy: jest.SpyInstance;

beforeEach(() => {
  stubCanvas();
  stubLoadedImage();
  analysisSpy = jest.spyOn(cacheModule, "photoAnalysisFor");
});

afterEach(() => {
  jest.restoreAllMocks();
});

describe("renderPost photo analysis wiring", () => {
  it("lays the stack in the top half when the photo's top is quiet", async () => {
    analysisSpy.mockReturnValue(analysisWith({ negativeSpace: "top" }));
    const { text } = stubCanvas();
    await renderPost(INPUT);
    expect(text.length).toBeGreaterThan(0);
    // 4:5 exports at 1080×1350. photoTopStack solves inside y 0.07–0.48.
    text.forEach((line) => expect(line.y).toBeLessThan(1350 * 0.5));
  });

  it("lays the stack in the bottom half when the photo's bottom is quiet", async () => {
    analysisSpy.mockReturnValue(analysisWith({ negativeSpace: "bottom" }));
    const { text } = stubCanvas();
    await renderPost(INPUT);
    expect(text.length).toBeGreaterThan(0);
    text.forEach((line) => expect(line.y).toBeGreaterThan(1350 * 0.4));
  });

  it("feeds the photo's dominant colours to the palette harmonizer", async () => {
    const harmonize = jest.spyOn(paletteModule, "harmonizePalette");
    analysisSpy.mockReturnValue(
      analysisWith({ dominantColors: ["#c2410c", "#1c1917"] }),
    );
    await renderPost(INPUT);
    expect(harmonize).toHaveBeenCalledWith(
      { primary: "#1a6b6a", secondary: "#0f3d3c" },
      ["#c2410c", "#1c1917"],
    );
  });

  it("harmonizes against nothing when the photo could not be analysed", async () => {
    const harmonize = jest.spyOn(paletteModule, "harmonizePalette");
    analysisSpy.mockReturnValue(null);
    await renderPost(INPUT);
    expect(harmonize).toHaveBeenCalledWith(
      { primary: "#1a6b6a", secondary: "#0f3d3c" },
      [],
    );
  });

  it("never analyses a post that has no photo", async () => {
    await renderPost({ ...INPUT, photoUrl: "" });
    expect(analysisSpy).not.toHaveBeenCalled();
  });

  it("honours an explicit composition over anything the photo suggests", async () => {
    analysisSpy.mockReturnValue(analysisWith({ negativeSpace: "top" }));
    const { text } = stubCanvas();
    await renderPost({ ...INPUT, composition: "photoBottomStack" });
    text.forEach((line) => expect(line.y).toBeGreaterThan(1350 * 0.4));
  });
});

describe("renderPost auto focal point", () => {
  // A 2000×1000 source into a 1080×1350 frame: cover-fit takes a 800×1000
  // window, so the focal x decides which 800px column of the photo survives.
  it("crops around the subject when the caller supplies no crop", async () => {
    analysisSpy.mockReturnValue(analysisWith({ focal: { x: 0.8, y: 0.5 } }));
    const { ctx } = stubCanvas();
    await renderPost(INPUT);
    const [, sx] = (ctx.drawImage as jest.Mock).mock.calls[0];
    // focal 0.8 → 0.8 * 2000 − 400 = 1200, clamped to at most 2000 − 800.
    expect(sx).toBe(1200);
  });

  it("leaves an explicit crop alone", async () => {
    analysisSpy.mockReturnValue(analysisWith({ focal: { x: 0.8, y: 0.5 } }));
    const { ctx } = stubCanvas();
    await renderPost({ ...INPUT, crop: { x: 0.5, y: 0.5, zoom: 1 } });
    const [, sx] = (ctx.drawImage as jest.Mock).mock.calls[0];
    expect(sx).toBe(600);
  });

  it("centres when the photo could not be analysed", async () => {
    analysisSpy.mockReturnValue(null);
    const { ctx } = stubCanvas();
    await renderPost(INPUT);
    const [, sx] = (ctx.drawImage as jest.Mock).mock.calls[0];
    expect(sx).toBe(600);
  });
});
