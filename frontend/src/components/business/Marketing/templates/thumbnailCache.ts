/**
 * In-memory cache of library thumbnail blob URLs, keyed by activity id (or
 * any stable string). Converts full canvases to compact image bitmaps so the
 * list does not retain 1080px canvases after render.
 *
 * Offscreen entries keep the blob URL (cheap) but callers must release live
 * canvas nodes themselves after caching.
 */

const DEFAULT_LIMIT = 120;

type CacheEntry = {
  url: string;
  lastAccess: number;
  consumers: number;
};

const cache = new Map<string, CacheEntry>();
let limit = DEFAULT_LIMIT;

export function setThumbnailCacheLimit(next: number): void {
  limit = Math.max(1, next);
  trimCache();
}

export function getCachedThumbnailUrl(key: string): string | null {
  const entry = cache.get(key);
  if (!entry) return null;
  entry.lastAccess = Date.now();
  // LRU: re-insert at end
  cache.delete(key);
  cache.set(key, entry);
  return entry.url;
}

export function retainCachedThumbnail(key: string): string | null {
  const entry = cache.get(key);
  if (!entry) return null;
  entry.consumers += 1;
  entry.lastAccess = Date.now();
  cache.delete(key);
  cache.set(key, entry);
  return entry.url;
}

export function releaseCachedThumbnail(key: string): void {
  const entry = cache.get(key);
  if (!entry) return;
  entry.consumers = Math.max(0, entry.consumers - 1);
  trimCache();
}

export function putCachedThumbnail(key: string, url: string): string {
  const existing = cache.get(key);
  if (existing) {
    if (existing.url !== url) {
      if (existing.consumers === 0) {
        try {
          URL.revokeObjectURL(existing.url);
        } catch {
          /* ignore */
        }
      }
    } else {
      existing.lastAccess = Date.now();
      cache.delete(key);
      cache.set(key, existing);
      return existing.url;
    }
  }
  cache.set(key, { url, lastAccess: Date.now(), consumers: 0 });
  trimCache();
  return url;
}

export function clearThumbnailCache(): void {
  for (const entry of cache.values()) {
    try {
      URL.revokeObjectURL(entry.url);
    } catch {
      /* ignore */
    }
  }
  cache.clear();
}

export function thumbnailCacheSize(): number {
  return cache.size;
}

function trimCache(): void {
  if (cache.size <= limit) return;
  const entries = Array.from(cache.entries()).sort(
    (a, b) => a[1].lastAccess - b[1].lastAccess,
  );
  for (const [key, entry] of entries) {
    if (cache.size <= limit) break;
    if (entry.consumers > 0) continue;
    try {
      URL.revokeObjectURL(entry.url);
    } catch {
      /* ignore */
    }
    cache.delete(key);
  }
}

// --- concurrency gate for thumbnail renders ---------------------------------

const DEFAULT_CONCURRENCY = 3;
let maxConcurrent = DEFAULT_CONCURRENCY;
let active = 0;
const waitQueue: Array<() => void> = [];

export function setThumbnailRenderConcurrency(n: number): void {
  maxConcurrent = Math.max(1, n);
  drainQueue();
}

export function getThumbnailRenderStats(): {
  active: number;
  waiting: number;
  maxConcurrent: number;
} {
  return { active, waiting: waitQueue.length, maxConcurrent };
}

export async function withThumbnailRenderSlot<T>(
  fn: () => Promise<T>,
  signal?: AbortSignal,
): Promise<T> {
  if (signal?.aborted) {
    throw abortError();
  }
  await acquireSlot(signal);
  try {
    if (signal?.aborted) throw abortError();
    return await fn();
  } finally {
    releaseSlot();
  }
}

function acquireSlot(signal?: AbortSignal): Promise<void> {
  if (active < maxConcurrent) {
    active += 1;
    return Promise.resolve();
  }
  return new Promise<void>((resolve, reject) => {
    const onAbort = () => {
      const idx = waitQueue.indexOf(resume);
      if (idx >= 0) waitQueue.splice(idx, 1);
      reject(abortError());
    };
    const resume = () => {
      signal?.removeEventListener("abort", onAbort);
      active += 1;
      resolve();
    };
    waitQueue.push(resume);
    if (signal) {
      if (signal.aborted) {
        waitQueue.pop();
        reject(abortError());
        return;
      }
      signal.addEventListener("abort", onAbort, { once: true });
    }
  });
}

function releaseSlot(): void {
  active = Math.max(0, active - 1);
  drainQueue();
}

function drainQueue(): void {
  while (active < maxConcurrent && waitQueue.length > 0) {
    const next = waitQueue.shift();
    next?.();
  }
}

function abortError(): Error {
  if (typeof DOMException === "function") {
    return new DOMException("thumbnail_render_aborted", "AbortError");
  }
  const error = new Error("thumbnail_render_aborted");
  error.name = "AbortError";
  return error;
}

/** Test helper: reset concurrency state between tests. */
export function resetThumbnailRenderQueue(): void {
  active = 0;
  waitQueue.length = 0;
  maxConcurrent = DEFAULT_CONCURRENCY;
}
