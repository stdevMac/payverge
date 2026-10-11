import { analyzePhoto } from "./analyze";
import { toAnalysisImageData } from "./downsample";
import type { PhotoAnalysis } from "./types";

/**
 * Entries retained, matching `IMAGE_CACHE_LIMIT` in
 * `../templates/renderPost.ts`. The two caches hold the same photos for the
 * same reason — a Library grid painting many cards at once — so an analysis
 * outliving its image, or vice versa, would only mean one of them recomputing
 * against a cache that could have served it.
 */
export const ANALYSIS_CACHE_LIMIT = 8;

/**
 * `null` is a CACHED VALUE, not an absence: a photo that could not be analysed
 * (tainted canvas, no 2D context, never decoded) must not be retried on every
 * card and every scroll. `Map.has` distinguishes "known unanalysable" from
 * "not yet seen"; `Map.get` alone cannot.
 */
const cache = new Map<string, PhotoAnalysis | null>();

function touch(url: string, value: PhotoAnalysis | null): void {
  // Re-insertion moves the key to the end of Map iteration order, which is what
  // makes the eviction below least-recently-USED rather than
  // least-recently-inserted.
  cache.delete(url);
  cache.set(url, value);
  while (cache.size > ANALYSIS_CACHE_LIMIT) {
    const oldest = cache.keys().next();
    if (oldest.done) break;
    cache.delete(oldest.value);
  }
}

/**
 * The analysis for `url`, computing it from `image` on a miss.
 *
 * Synchronous by design. The expensive, asynchronous half — fetching and
 * decoding — has already happened by the time a caller holds an
 * `HTMLImageElement`, and both call sites (the renderer and the composer)
 * already await that load. Making this async as well would add a microtask to
 * the render path for a value that is usually already in a Map.
 *
 * A blank URL or a missing image returns null WITHOUT caching: neither names a
 * photo, so there is nothing to remember, and caching under "" would make the
 * first real photo at that key invisible.
 */
export function photoAnalysisFor(
  url: string,
  image: HTMLImageElement | null,
): PhotoAnalysis | null {
  const key = url.trim();
  if (!key || !image) return null;

  if (cache.has(key)) {
    const hit = cache.get(key) ?? null;
    touch(key, hit);
    return hit;
  }

  const raster = toAnalysisImageData(image);
  const analysis = raster
    ? {
        ...analyzePhoto(raster),
        // Intrinsic size for extreme-aspect readiness — analysis buffer is
        // always square 64×64 and would hide panorama / ultra-tall sources.
        sourceWidth: image.naturalWidth,
        sourceHeight: image.naturalHeight,
      }
    : null;
  touch(key, analysis);
  return analysis;
}

/**
 * A cache READ. Never downsamples and never analyses, so a caller that has no
 * decoded image — a component rendering before its photo has loaded — can ask
 * without paying for a miss.
 *
 * Returns `undefined` on a cache miss (not yet analysed). Returns `null` when
 * the photo was analysed and found unreadable — those are distinct so Ready
 * gates do not treat "still rendering" as "reshoot".
 */
export function cachedPhotoAnalysis(
  url: string,
): PhotoAnalysis | null | undefined {
  const key = url.trim();
  if (!key) return undefined;
  if (!cache.has(key)) return undefined;
  return cache.get(key) ?? null;
}

/** Test helper, and the reset used between Library sessions. */
export function clearPhotoAnalysisCache(): void {
  cache.clear();
}

/** Test helper. */
export function photoAnalysisCacheSize(): number {
  return cache.size;
}
