import type { ImageDataLike } from "../artDirection/palette";

/**
 * `ImageDataLike` — `{ data, width, height }` — is re-exported here so every
 * module in this folder imports its pixel-input type from one place. It is
 * DELIBERATELY not a canvas: jest maps `^canvas$` to a mock, so a function that
 * took a context could not be tested at all. Every pixel-derived function in
 * this folder takes one of these and is fed synthetic arrays by its tests.
 */
export type { ImageDataLike };

/** A rectangle in 0..1 coordinates of whatever frame the caller names. */
export interface Rect01 {
  x: number;
  y: number;
  w: number;
  h: number;
}

/**
 * A square grid of per-cell scalars, row-major. `values.length === size * size`.
 * The cell at (row, col) is `values[row * size + col]`.
 */
export interface CellGrid {
  size: number;
  values: number[];
}

export interface ExposureStats {
  /**
   * `EXPOSURE_BUCKETS` fractions of the sampled pixels, by luma. Sums to 1 for
   * any non-empty image, so it is comparable across photo sizes.
   */
  histogram: number[];
  /** Mean perceptual luma, 0–255. */
  meanLuma: number;
  /** Fraction of pixels at or below luma 8 — detail lost to black. */
  shadowClipping: number;
  /** Fraction at or above luma 247 — detail lost to white. */
  highlightClipping: number;
  /** Per-channel means, 0–255. The input to grey-world white balance. */
  channelMeans: { r: number; g: number; b: number };
}

/**
 * Which band of the frame is quiet enough to put type on.
 *
 * `left` and `right` are in the union but `negativeSpaceFrom` never returns
 * them: no composition can exploit a side band today, and returning a value no
 * consumer reads would be dead weight dressed as a feature. Wave 4's wide and
 * strip formats are where they earn their place. A test pins the restriction.
 */
export type NegativeSpace =
  | "top"
  | "bottom"
  | "left"
  | "right"
  | "center"
  | "none";

/**
 * Everything derived from one photo, once, per URL.
 *
 * Produced by `analyzePhoto` from a single ~64×64 downsample and cached by
 * `photoAnalysisFor`. Five consumers read it: the composition chooser, the crop,
 * the grade stage, the scrim resolver and the readiness panel.
 */
export interface PhotoAnalysis {
  /** Cell-averaged perceptual luma, 0–255. */
  luma: CellGrid;
  /** Cell-averaged normalized Sobel magnitude, 0–1. */
  edges: CellGrid;
  /** Variance of the Laplacian. Higher is sharper; see `BLUR_FLOOR`. */
  blurScore: number;
  exposure: ExposureStats;
  /** The tightest cell-aligned box around the high-edge-energy region. */
  subject: Rect01;
  /** The centre of `subject`, in 0..1 — the crop's focal point. */
  focal: { x: number; y: number };
  negativeSpace: NegativeSpace;
  busy: boolean;
  /** Median-cut dominant colours, most-populous first, as `#rrggbb`. */
  dominantColors: string[];
  /**
   * Intrinsic pixel size of the decoded source (not the 64×64 analysis buffer).
   * Optional so pure `analyzePhoto(ImageDataLike)` tests stay dimension-free;
   * `photoAnalysisFor` fills these from `HTMLImageElement.naturalWidth/Height`
   * so readiness can flag extreme aspect ratios that fitCover will crop hard.
   */
  sourceWidth?: number;
  sourceHeight?: number;
}
