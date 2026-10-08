/** @jest-environment jsdom */
import { act, renderHook } from "@testing-library/react";
import { useEffect } from "react";

import {
  type ImageMeasurementsResult,
  useImageMeasurements,
} from "./useImageMeasurements";

const OriginalImage = globalThis.Image;

type MockImageInstance = MockImage & { crossOrigin: string | null };

const images = new Map<string, MockImageInstance>();
const createdImages: MockImageInstance[] = [];

class MockImage {
  naturalWidth = 0;
  naturalHeight = 0;
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  crossOrigin: string | null = null;
  crossOriginWhenSrcWasSet: string | null = null;
  private value = "";

  constructor() {
    createdImages.push(this);
  }

  set src(url: string) {
    this.value = url;
    this.crossOriginWhenSrcWasSet = this.crossOrigin;
    images.set(url, this);
  }

  get src() {
    return this.value;
  }
}

const resolveMockImage = (
  url: string,
  size: { width: number; height: number },
) => {
  const image = images.get(url);
  if (!image) throw new Error(`No mock image for ${url}`);
  image.naturalWidth = size.width;
  image.naturalHeight = size.height;
  image.onload?.();
};

const rejectMockImage = (url: string) => {
  const image = images.get(url);
  if (!image) throw new Error(`No mock image for ${url}`);
  image.onerror?.();
};

beforeEach(() => {
  jest.useFakeTimers();
  globalThis.Image = MockImage as unknown as typeof Image;
});

afterEach(() => {
  globalThis.Image = OriginalImage;
  images.clear();
  createdImages.length = 0;
  jest.useRealTimers();
});

describe("useImageMeasurements", () => {
  it("deduplicates normalized urls and records ready and failed images", () => {
    const { result } = renderHook(() =>
      useImageMeasurements([" good.jpg ", "good.jpg", "", "  ", "bad.jpg"], {
        timeoutMs: 5000,
      }),
    );

    expect(createdImages).toHaveLength(2);
    expect(images.get("good.jpg")?.crossOriginWhenSrcWasSet).toBe("anonymous");
    expect(result.current).toEqual({
      status: "loading",
      measurements: {
        "bad.jpg": { status: "loading" },
        "good.jpg": { status: "loading" },
      },
    });

    act(() => {
      resolveMockImage("good.jpg", { width: 1600, height: 1200 });
      rejectMockImage("bad.jpg");
    });

    expect(result.current).toEqual({
      status: "complete",
      measurements: {
        "bad.jpg": { status: "failed" },
        "good.jpg": { status: "ready", width: 1600, height: 1200 },
      },
    });
  });

  it("records timed-out images and always leaves loading", () => {
    const { result } = renderHook(() =>
      useImageMeasurements(["slow.jpg"], { timeoutMs: 250 }),
    );

    expect(result.current.status).toBe("loading");

    act(() => {
      jest.advanceTimersByTime(250);
    });

    expect(result.current).toEqual({
      status: "complete",
      measurements: { "slow.jpg": { status: "timeout" } },
    });
    expect(images.get("slow.jpg")?.onload).toBeNull();
    expect(images.get("slow.jpg")?.onerror).toBeNull();
    expect(jest.getTimerCount()).toBe(0);
  });

  it("cleans previous handlers and every timer on dependency change and unmount", () => {
    const { result, rerender, unmount } = renderHook(
      ({ urls }) => useImageMeasurements(urls, { timeoutMs: 5000 }),
      { initialProps: { urls: ["old-a.jpg", "old-b.jpg"] } },
    );
    const oldA = images.get("old-a.jpg");
    const oldB = images.get("old-b.jpg");

    rerender({ urls: ["next.jpg"] });

    expect(oldA?.onload).toBeNull();
    expect(oldA?.onerror).toBeNull();
    expect(oldB?.onload).toBeNull();
    expect(oldB?.onerror).toBeNull();
    expect(result.current).toEqual({
      status: "loading",
      measurements: { "next.jpg": { status: "loading" } },
    });
    expect(jest.getTimerCount()).toBe(1);

    const pending = images.get("next.jpg");
    unmount();

    expect(pending?.onload).toBeNull();
    expect(pending?.onerror).toBeNull();
    expect(jest.getTimerCount()).toBe(0);

    act(() => {
      pending?.onload?.();
      jest.runOnlyPendingTimers();
    });
  });

  it("aborts pending measurements when the active url list becomes empty", () => {
    const { result, rerender } = renderHook(
      ({ urls }) => useImageMeasurements(urls, { timeoutMs: 5000 }),
      { initialProps: { urls: ["pending.jpg"] } },
    );
    const pending = images.get("pending.jpg");

    expect(result.current.status).toBe("loading");
    expect(jest.getTimerCount()).toBe(1);
    rerender({ urls: [] });

    expect(result.current).toEqual({ status: "complete", measurements: {} });
    expect(pending?.onload).toBeNull();
    expect(pending?.onerror).toBeNull();
    expect(jest.getTimerCount()).toBe(0);
  });

  it("is complete for empty urls and does not restart for reordered duplicates", () => {
    const { result, rerender } = renderHook(
      ({ urls }) => useImageMeasurements(urls, { timeoutMs: 5000 }),
      { initialProps: { urls: ["", "   "] } },
    );

    expect(result.current).toEqual({
      status: "complete",
      measurements: {},
    });
    expect(createdImages).toHaveLength(0);

    rerender({ urls: ["beta.jpg", " alpha.jpg ", "beta.jpg"] });
    expect(createdImages).toHaveLength(2);

    act(() => {
      resolveMockImage("alpha.jpg", { width: 100, height: 200 });
      resolveMockImage("beta.jpg", { width: 300, height: 400 });
    });
    const completed = result.current;

    rerender({ urls: ["beta.jpg", "alpha.jpg", "alpha.jpg"] });

    expect(createdImages).toHaveLength(2);
    expect(result.current).toBe(completed);
  });

  it("never exposes stale completion to a consumer effect when the url batch changes", () => {
    const observations: ImageMeasurementsResult[] = [];
    const { result, rerender } = renderHook(
      ({ urls }) => {
        const measurementResult = useImageMeasurements(urls, {
          timeoutMs: 5000,
        });
        useEffect(() => {
          observations.push(measurementResult);
        }, [measurementResult]);
        return measurementResult;
      },
      { initialProps: { urls: ["first.jpg"] } },
    );

    expect(observations[0]).toEqual({
      status: "loading",
      measurements: { "first.jpg": { status: "loading" } },
    });

    act(() => {
      resolveMockImage("first.jpg", { width: 640, height: 480 });
    });
    expect(result.current.status).toBe("complete");

    observations.length = 0;
    rerender({ urls: ["next.jpg"] });

    expect(observations.length).toBeGreaterThan(0);
    expect(observations).toEqual(
      observations.map(() => ({
        status: "loading",
        measurements: { "next.jpg": { status: "loading" } },
      })),
    );
  });

  it("starts a distinct loading batch when timeout changes for the same urls", () => {
    const observations: ImageMeasurementsResult[] = [];
    const { result, rerender } = renderHook(
      ({ timeoutMs }) => {
        const measurementResult = useImageMeasurements(["same.jpg"], {
          timeoutMs,
        });
        useEffect(() => {
          observations.push(measurementResult);
        }, [measurementResult]);
        return measurementResult;
      },
      { initialProps: { timeoutMs: 5000 } },
    );
    const oldImage = images.get("same.jpg");

    act(() => {
      resolveMockImage("same.jpg", { width: 640, height: 480 });
    });
    expect(result.current).toEqual({
      status: "complete",
      measurements: {
        "same.jpg": { status: "ready", width: 640, height: 480 },
      },
    });

    observations.length = 0;
    rerender({ timeoutMs: 250 });

    expect(oldImage?.onload).toBeNull();
    expect(oldImage?.onerror).toBeNull();
    expect(createdImages).toHaveLength(2);
    expect(jest.getTimerCount()).toBe(1);
    expect(observations.length).toBeGreaterThan(0);
    expect(observations).toEqual(
      observations.map(() => ({
        status: "loading",
        measurements: { "same.jpg": { status: "loading" } },
      })),
    );

    act(() => {
      resolveMockImage("same.jpg", { width: 1200, height: 800 });
    });

    expect(result.current).toEqual({
      status: "complete",
      measurements: {
        "same.jpg": { status: "ready", width: 1200, height: 800 },
      },
    });
    expect(jest.getTimerCount()).toBe(0);
  });
});
