import { contrastRatio, parseHex, pickReadableColor, toHex, type Rgb } from "./color";

/**
 * Structural stand-in for `ImageData`. Taking this instead of a canvas keeps
 * every pixel-derived function pure and testable with synthetic buffers.
 */
export interface ImageDataLike {
  data: Uint8ClampedArray | number[];
  width: number;
  height: number;
}

export interface BrandColors {
  primary: string;
  secondary: string;
}

export interface ResolvedPalette extends BrandColors {
  /** A color lifted from the photo, used for pills, rules, and motifs. */
  accent: string;
  /** A readable foreground for text sitting on `accent`. */
  onAccent: string;
}

const ALPHA_FLOOR = 128;

function collectPixels(img: ImageDataLike): Rgb[] {
  const out: Rgb[] = [];
  const { data, width, height } = img;
  const total = width * height;
  for (let i = 0; i < total; i += 1) {
    const o = i * 4;
    if (data[o + 3] < ALPHA_FLOOR) continue;
    out.push({ r: data[o], g: data[o + 1], b: data[o + 2] });
  }
  return out;
}

function averageOf(pixels: Rgb[]): Rgb {
  const sum = pixels.reduce(
    (acc, p) => ({ r: acc.r + p.r, g: acc.g + p.g, b: acc.b + p.b }),
    { r: 0, g: 0, b: 0 },
  );
  return {
    r: sum.r / pixels.length,
    g: sum.g / pixels.length,
    b: sum.b / pixels.length,
  };
}

function widestChannel(pixels: Rgb[]): "r" | "g" | "b" {
  const channels: Array<"r" | "g" | "b"> = ["r", "g", "b"];
  let best: "r" | "g" | "b" = "r";
  let bestRange = -1;
  channels.forEach((channel) => {
    let min = Infinity;
    let max = -Infinity;
    pixels.forEach((p) => {
      if (p[channel] < min) min = p[channel];
      if (p[channel] > max) max = p[channel];
    });
    const range = max - min;
    if (range > bestRange) {
      bestRange = range;
      best = channel;
    }
  });
  return best;
}

/**
 * Median-cut quantization. Repeatedly splits the bucket with the most pixels
 * along its widest channel until `count` buckets exist, then averages each.
 * Deterministic, allocation-light, and adequate for palette work at thumbnail
 * resolution.
 *
 * The split loop can end before reaching `count` buckets: once every bucket
 * is down to a single pixel, there is nothing left to divide. `count` is an
 * upper bound on the length of the result, not a guarantee of that many
 * *distinct* colors — a flat or near-flat region keeps splitting on pixel
 * position until buckets bottom out at one pixel each, so it can still
 * return `count` entries that are the same averaged hex value repeated.
 * Callers that need visually distinct swatches should dedupe downstream.
 */
export function extractDominantColors(
  img: ImageDataLike,
  count: number,
): string[] {
  const pixels = collectPixels(img);
  if (!pixels.length || count <= 0) return [];

  let buckets: Rgb[][] = [pixels];
  while (buckets.length < count) {
    let targetIndex = -1;
    let targetSize = 1;
    buckets.forEach((bucket, index) => {
      if (bucket.length > targetSize) {
        targetSize = bucket.length;
        targetIndex = index;
      }
    });
    if (targetIndex === -1) break;

    const bucket = buckets[targetIndex];
    const channel = widestChannel(bucket);
    const sorted = [...bucket].sort((a, b) => a[channel] - b[channel]);
    const mid = Math.floor(sorted.length / 2);
    const left = sorted.slice(0, mid);
    const right = sorted.slice(mid);
    if (!left.length || !right.length) break;
    buckets = [
      ...buckets.slice(0, targetIndex),
      left,
      right,
      ...buckets.slice(targetIndex + 1),
    ];
  }

  return buckets
    .filter((bucket) => bucket.length > 0)
    .sort((a, b) => b.length - a.length)
    .map((bucket) => toHex(averageOf(bucket)));
}

/**
 * Combine brand colors with photo-derived colors. Brand primary and secondary
 * are never overwritten — the accent is the only photo-driven slot, which is
 * what keeps every kit on-brand.
 */
export function harmonizePalette(
  brand: BrandColors,
  photoColors: string[],
): ResolvedPalette {
  const primaryRgb = parseHex(brand.primary) ?? { r: 26, g: 107, b: 106 };

  // Prefer the photo color that is most distinct from the brand primary, so the
  // accent adds information instead of restating it. `contrastRatio` is reused
  // here purely as a cheap perceptual-distance proxy between two arbitrary
  // colors — this is NOT an accessibility check. The accessibility check is
  // the separate `pickReadableColor` call below, which picks `onAccent`.
  let accent = brand.primary;
  let bestDistance = -1;
  photoColors.forEach((hex) => {
    const rgb = parseHex(hex);
    if (!rgb) return;
    const distance = contrastRatio(rgb, primaryRgb);
    if (distance > bestDistance) {
      bestDistance = distance;
      accent = hex;
    }
  });

  return {
    primary: brand.primary,
    secondary: brand.secondary,
    accent,
    // eslint-disable-next-line no-restricted-syntax -- fallback foregrounds for canvas paint, no Tailwind class equivalent
    onAccent: pickReadableColor(["#ffffff", "#1c1917"], accent, 4.5),
  };
}
