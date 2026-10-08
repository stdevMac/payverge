/** @jest-environment jsdom */
import {
  ANALYSIS_CACHE_LIMIT,
  cachedPhotoAnalysis,
  clearPhotoAnalysisCache,
  photoAnalysisCacheSize,
  photoAnalysisFor,
} from "./cache";
import * as downsample from "./downsample";

function image() {
  return { naturalWidth: 128, naturalHeight: 128 } as unknown as HTMLImageElement;
}

function buffer(value: number) {
  return {
    data: new Uint8ClampedArray(16 * 16 * 4).fill(value),
    width: 16,
    height: 16,
  };
}

let spy: jest.SpyInstance;

beforeEach(() => {
  clearPhotoAnalysisCache();
  spy = jest
    .spyOn(downsample, "toAnalysisImageData")
    .mockImplementation(() => buffer(200));
});

afterEach(() => {
  jest.restoreAllMocks();
  clearPhotoAnalysisCache();
});

describe("photoAnalysisFor", () => {
  it("downsamples a URL exactly once", () => {
    const first = photoAnalysisFor("https://cdn/a.png", image());
    const second = photoAnalysisFor("https://cdn/a.png", image());
    expect(spy).toHaveBeenCalledTimes(1);
    expect(second).toBe(first);
  });

  it("records intrinsic source dimensions for extreme-aspect readiness", () => {
    const tall = {
      naturalWidth: 1080,
      naturalHeight: 3000,
    } as unknown as HTMLImageElement;
    const analysis = photoAnalysisFor("https://cdn/tall.png", tall);
    expect(analysis?.sourceWidth).toBe(1080);
    expect(analysis?.sourceHeight).toBe(3000);
  });

  it("keys by URL, not by image identity", () => {
    photoAnalysisFor("https://cdn/a.png", image());
    photoAnalysisFor("https://cdn/b.png", image());
    expect(spy).toHaveBeenCalledTimes(2);
    expect(photoAnalysisCacheSize()).toBe(2);
  });

  // Without this, a CORS-blocked or tainted photo pays a full downsample
  // attempt on every card, every scroll.
  it("caches a failure so it is not retried on every render", () => {
    spy.mockReturnValue(null);
    expect(photoAnalysisFor("https://cdn/bad.png", image())).toBeNull();
    expect(photoAnalysisFor("https://cdn/bad.png", image())).toBeNull();
    expect(spy).toHaveBeenCalledTimes(1);
  });

  it("returns null without caching for a missing image or blank URL", () => {
    expect(photoAnalysisFor("https://cdn/a.png", null)).toBeNull();
    expect(photoAnalysisFor("   ", image())).toBeNull();
    expect(spy).not.toHaveBeenCalled();
    expect(photoAnalysisCacheSize()).toBe(0);
  });

  it("evicts least-recently-used entries past the limit", () => {
    for (let index = 0; index < ANALYSIS_CACHE_LIMIT + 2; index += 1) {
      photoAnalysisFor(`https://cdn/${index}.png`, image());
    }
    expect(photoAnalysisCacheSize()).toBe(ANALYSIS_CACHE_LIMIT);
    expect(cachedPhotoAnalysis("https://cdn/0.png")).toBeUndefined();
    expect(
      cachedPhotoAnalysis(`https://cdn/${ANALYSIS_CACHE_LIMIT + 1}.png`),
    ).not.toBeNull();
  });

  it("keeps a re-read entry alive across evictions", () => {
    photoAnalysisFor("https://cdn/keep.png", image());
    for (let index = 0; index < ANALYSIS_CACHE_LIMIT; index += 1) {
      photoAnalysisFor(`https://cdn/${index}.png`, image());
      photoAnalysisFor("https://cdn/keep.png", image());
    }
    expect(cachedPhotoAnalysis("https://cdn/keep.png")).not.toBeNull();
  });
});

describe("cachedPhotoAnalysis", () => {
  it("never downsamples — it is a read, not a compute", () => {
    expect(cachedPhotoAnalysis("https://cdn/cold.png")).toBeUndefined();
    expect(spy).not.toHaveBeenCalled();
  });

  it("distinguishes cache miss from cached unreadable", () => {
    expect(cachedPhotoAnalysis("https://cdn/miss.png")).toBeUndefined();
    spy.mockReturnValue(null);
    expect(photoAnalysisFor("https://cdn/bad.png", image())).toBeNull();
    expect(cachedPhotoAnalysis("https://cdn/bad.png")).toBeNull();
  });
});
