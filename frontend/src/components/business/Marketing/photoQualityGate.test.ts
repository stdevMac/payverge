import {
  clearPhotoAnalysisCache,
  photoAnalysisFor,
} from "./photo/cache";
import { photoQualityGateForUrl } from "./photoQualityGate";
import type { PhotoAnalysis } from "./photo/types";

function analysis(over: {
  blurScore?: number;
  meanLuma?: number;
  sourceWidth?: number;
  sourceHeight?: number;
}): PhotoAnalysis {
  return {
    luma: { size: 1, values: [over.meanLuma ?? 128] },
    edges: { size: 1, values: [0] },
    blurScore: over.blurScore ?? 500,
    exposure: {
      histogram: new Array(16).fill(0),
      meanLuma: over.meanLuma ?? 128,
      shadowClipping: 0.01,
      highlightClipping: 0.01,
      channelMeans: { r: 40, g: 40, b: 40 },
    },
    subject: { x: 0.2, y: 0.2, w: 0.5, h: 0.5 },
    focal: { x: 0.45, y: 0.45 },
    negativeSpace: "bottom",
    busy: false,
    dominantColors: ["#1c1917"],
    sourceWidth: over.sourceWidth ?? 1200,
    sourceHeight: over.sourceHeight ?? 1600,
  };
}

function seedAnalysis(url: string, photo: PhotoAnalysis): void {
  const img = {
    naturalWidth: photo.sourceWidth || 1200,
    naturalHeight: photo.sourceHeight || 1600,
  } as HTMLImageElement;
  jest
    .spyOn(require("./photo/downsample"), "toAnalysisImageData")
    .mockReturnValue({
      data: new Uint8ClampedArray(64 * 64 * 4),
      width: 64,
      height: 64,
    } as ImageData);
  jest.spyOn(require("./photo/analyze"), "analyzePhoto").mockReturnValue(photo);
  photoAnalysisFor(url, img);
}

describe("photoQualityGateForUrl", () => {
  beforeEach(() => {
    clearPhotoAnalysisCache();
    jest.restoreAllMocks();
  });

  it("returns unknown before the preview has analysed the photo", () => {
    expect(photoQualityGateForUrl("https://cdn/steak.jpg")).toBe("unknown");
  });

  it("blocks Ready-to-post for underexposed library grades", () => {
    seedAnalysis(
      "https://cdn/tea.jpg",
      analysis({ blurScore: 80, meanLuma: 40 }),
    );
    expect(photoQualityGateForUrl("https://cdn/tea.jpg")).toBe("too_dark");
  });
});
