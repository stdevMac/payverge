/** @jest-environment jsdom */
import {
  clearThumbnailCache,
  getCachedThumbnailUrl,
  getThumbnailRenderStats,
  putCachedThumbnail,
  releaseCachedThumbnail,
  resetThumbnailRenderQueue,
  retainCachedThumbnail,
  setThumbnailCacheLimit,
  setThumbnailRenderConcurrency,
  thumbnailCacheSize,
  withThumbnailRenderSlot,
} from "./thumbnailCache";

beforeEach(() => {
  clearThumbnailCache();
  resetThumbnailRenderQueue();
  setThumbnailCacheLimit(120);
  setThumbnailRenderConcurrency(3);
});

describe("thumbnail blob cache", () => {
  it("stores and retrieves blob URLs by activity key", () => {
    putCachedThumbnail("activity-1", "blob:thumb-1");
    expect(getCachedThumbnailUrl("activity-1")).toBe("blob:thumb-1");
    expect(thumbnailCacheSize()).toBe(1);
  });

  it("tracks retain/release consumers and trims unretained entries under the limit", () => {
    setThumbnailCacheLimit(2);
    putCachedThumbnail("a", "blob:a");
    putCachedThumbnail("b", "blob:b");
    retainCachedThumbnail("a");
    putCachedThumbnail("c", "blob:c");
    // b is unretained and oldest → evicted; a retained, c new.
    expect(getCachedThumbnailUrl("a")).toBe("blob:a");
    expect(getCachedThumbnailUrl("c")).toBe("blob:c");
    expect(getCachedThumbnailUrl("b")).toBeNull();
    releaseCachedThumbnail("a");
  });
});

describe("thumbnail render concurrency", () => {
  it("caps concurrent thumbnail renders", async () => {
    setThumbnailRenderConcurrency(2);
    let release1!: () => void;
    let release2!: () => void;
    let release3!: () => void;
    const p1 = withThumbnailRenderSlot(
      () =>
        new Promise<string>((resolve) => {
          release1 = () => resolve("1");
        }),
    );
    const p2 = withThumbnailRenderSlot(
      () =>
        new Promise<string>((resolve) => {
          release2 = () => resolve("2");
        }),
    );
    let p3Started = false;
    const p3 = withThumbnailRenderSlot(async () => {
      p3Started = true;
      return new Promise<string>((resolve) => {
        release3 = () => resolve("3");
      });
    });

    await Promise.resolve();
    expect(getThumbnailRenderStats().active).toBe(2);
    expect(getThumbnailRenderStats().waiting).toBe(1);
    expect(p3Started).toBe(false);

    release1();
    await p1;
    await Promise.resolve();
    expect(p3Started).toBe(true);

    release2();
    release3();
    await Promise.all([p2, p3]);
    expect(getThumbnailRenderStats().active).toBe(0);
    expect(getThumbnailRenderStats().waiting).toBe(0);
  });
});
