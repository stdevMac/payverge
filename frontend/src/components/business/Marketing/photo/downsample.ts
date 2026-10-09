import type { ImageDataLike } from "./types";

/**
 * Side of the square analysis buffer.
 *
 * 64×64 is 4096 pixels: enough for an 8×8 grid of 8×8 blocks, a meaningful
 * 16-bucket histogram and a stable median cut, and small enough that the whole
 * analysis is a once-per-URL cost measured in a couple of milliseconds rather
 * than a per-render one.
 *
 * `BLUR_FLOOR` in `./edges.ts` is calibrated against THIS size — Laplacian
 * variance scales with spatial frequency, so halving this roughly quarters the
 * blur score. Changing it means recalibrating that constant.
 */
export const ANALYSIS_SIZE = 64;

/**
 * Decode a loaded photo into an `ImageDataLike` for the pure analysers.
 *
 * The ONLY function in `photo/` that touches a canvas, deliberately: it is the
 * one thing here that cannot be unit-tested against synthetic arrays, so it is
 * kept to a dozen lines with no arithmetic in it.
 *
 * The photo is squashed to a square rather than letterboxed. Every consumer
 * works in 0..1 of the frame — bands, subject box, focal point — so a square
 * buffer IS the normalized frame, and letterboxing would add padding that reads
 * as flat black negative space. The cost is a slight anisotropy in the Sobel
 * response on very wide or very tall sources; edge ENERGY is what the consumers
 * read, not edge orientation, so it does not change an answer.
 *
 * Returns null — never throws — for a photo that never decoded, an environment
 * with no 2D context, and a tainted canvas. Every caller treats null as "this
 * photo was not analysed", which is a state the whole pipeline already handles
 * because it is what a CORS-blocked bucket produces.
 */
export function toAnalysisImageData(
  image: HTMLImageElement,
  size = ANALYSIS_SIZE,
): ImageDataLike | null {
  const sourceW = image.naturalWidth;
  const sourceH = image.naturalHeight;
  if (!sourceW || !sourceH || size <= 0) return null;

  const canvas = document.createElement("canvas");
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext("2d");
  if (!ctx) return null;

  try {
    ctx.drawImage(image, 0, 0, sourceW, sourceH, 0, 0, size, size);
    const raster = ctx.getImageData(0, 0, size, size);
    return { data: raster.data, width: raster.width, height: raster.height };
  } catch {
    // SecurityError on a tainted canvas, or a provider that refuses readback.
    return null;
  } finally {
    // The pixels are already copied into the ImageData above, so releasing the
    // backing store here costs nothing and keeps a 64×64 canvas per photo from
    // lingering behind the analysis cache.
    canvas.width = 0;
    canvas.height = 0;
  }
}
