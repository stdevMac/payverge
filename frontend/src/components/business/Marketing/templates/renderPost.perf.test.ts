/** @jest-environment jsdom */
import { LIBRARY_THUMBNAIL_WIDTH, renderPost } from "./renderPost";
import type { CompositionId } from "../composition/compositions";
import type { SlotKey } from "./types";
import { clearPhotoAnalysisCache } from "../photo/cache";

/**
 * A CPU guard on the per-thumbnail cost of the render pipeline.
 *
 * The Library paints a whole grid of these at once, so the pipeline the scene
 * rewrite introduced — chooser, intrinsic layout solver, scene assembly, walker
 * dispatch — has to stay cheap per card. This suite fails if that cost grows by
 * an order of magnitude. It is not a rasterization benchmark: jest maps
 * `^canvas$` to a stub and the context below is a plain object, so nothing here
 * measures pixel work. Every millisecond it reports is the pipeline's own
 * JavaScript, which is exactly the part a bad solver or chooser change inflates.
 *
 * Photo decode is still excluded from the warm budget: the warm case primes the
 * analysis cache so only the walker's layout math is measured. The cold case
 * measures the analysis pre-pass arithmetic on a mid-grey field (via the stub
 * context's `getImageData`), not network or codec work.
 *
 * Measured cost, pre-Wave-1 vs. now (Apple-silicon dev box, warmed, per card):
 *   pre-Wave-1  (814e4f3ff, legacy direct-to-canvas)  median ~0.13ms
 *   post-Wave-1 (scene pipeline)                      median ~0.32ms
 * The baseline came from running this same harness against the pre-Wave-1
 * renderer in a throwaway worktree. Wave 1 costs roughly 2.5x per card, the
 * expected price of the indirection and nowhere near the order of magnitude
 * this budget guards against.
 *
 * Wave 2 photo baselines (this suite, Apple-silicon, mid-grey stub pixels):
 *   warm analysis hit   median ~0.31ms
 *   cold analysis miss  median ~0.37ms
 * Cold is only slightly above warm on a flat field because the arithmetic over
 * 4096 mid-grey pixels is cheap; the budget is there for a rewrite that
 * allocates per-pixel objects or raises ANALYSIS_SIZE without recalibrating.
 *
 * Demonstrated sensitivity, by slowing `settleScale`'s descent on purpose:
 * `SCALE_STEP` at 1/100th of its real value (~100x the iterations) lands the
 * crowded case around 4ms and still passes; at 1/100000th it reports a 606ms
 * median and fails loudly. The guard is a cliff-edge alarm, not a stopwatch.
 *
 * Why 6ms. Twenty consecutive runs of the three cases below — 60 measurement
 * windows, 1500 renders — put every window's median between 0.25ms and 0.65ms
 * (pinned ~0.31, chooser ~0.27, crowded ~0.58). So 6ms is about 9x the worst
 * median observed and ~19x the usual one: comfortably clear of noise, and low
 * enough that the tenfold regression this exists to catch cannot hide under it.
 * The worst single SAMPLE in those same windows was 5.35ms — just under the
 * budget, and higher still when the box was loaded — which is exactly why the
 * assertion reads a median and not one timing.
 */
const BUDGET_MS = 6;

/**
 * Budget for a card whose photo has NOT been analysed yet.
 *
 * The cold path pays a 64×64 downsample, a Sobel pass, a Laplacian pass, a
 * 16-bucket histogram and a five-way median cut over 4096 pixels — once per
 * photo URL, never per card. 25ms is roughly 4x the warm budget, which is the
 * right shape for a one-off: high enough that a loaded CI box does not flake,
 * low enough that a rewrite of the analysis into per-pixel object allocation
 * would blow it.
 *
 * If this fails on a machine that is otherwise idle, DO NOT raise the number.
 * Lower `ANALYSIS_SIZE` in `../photo/downsample.ts` to 48 (2304 pixels, 56% of
 * the work) and recalibrate `BLUR_FLOOR` in `../photo/edges.ts` against the
 * bracket its own test pins — the constant is scale-dependent and documented as
 * such. Raising this number instead removes the only guard on the analysis.
 */
const COLD_BUDGET_MS = 25;

/** Discarded runs that pay for module init, the font cache, and JIT warm-up. */
const WARMUP_RUNS = 5;

/**
 * Sample count. The assertion reads the MEDIAN, not a single timing: one
 * un-warmed sample against a fixed threshold is a coin flip on a loaded CI box,
 * and a median over this many runs shrugs off the GC pause that lands in any
 * given run. Individual maxima here routinely run 10x the median; the median
 * itself moves by hundredths of a millisecond.
 */
const SAMPLE_RUNS = 25;

type Slots = Partial<Record<SlotKey, string>>;

/** A card an operator would actually publish: every slot filled, none overlong. */
const TYPICAL_SLOTS: Slots = {
  dishName: "Milanesa napolitana con papas",
  price: "$12.00",
  badge: "CHEF'S PICK",
  cta: "Order tonight",
  handle: "@casasur",
};

/**
 * Copy long enough to overflow its bounds, which is the only way to reach the
 * solver's shrink loop at all. `photoBottomStack` with `TYPICAL_SLOTS` — the
 * obvious fixture, and the one the two cases above use — settles at scale 1 on
 * the first check and never iterates, so on its own it would leave the descent
 * in `settleScale` completely unmeasured. `splitPanel` narrows the text column
 * to under half the canvas; this copy then wraps past the envelope and forces
 * the descent to actually walk down the type scale.
 */
const CROWDED_SLOTS: Slots = {
  dishName: "Milanesa napolitana con papas fritas y ensalada mixta de la casa",
  price: "$12.00",
  badge: "CHEF'S PICK OF THE EVENING",
  cta: "Order tonight before the kitchen closes at midnight",
  handle: "@casasurparrillaybodega",
};

/**
 * The leanest 2D context the pipeline will accept. Deliberately not the
 * recording stub `renderPost.test.ts` uses — pushing every `fillText` and
 * `fillRect` into an array would put the harness's own bookkeeping inside the
 * window being timed. `measureText` still has to return a plausible width or
 * the solver never wraps, never shrinks the type scale, and the timing stops
 * describing real work.
 */
function stubCanvas(): void {
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
    fillRect: () => {},
    fillText: () => {},
    measureText: (value: string) => {
      const px = Number.parseFloat(ctx.font.match(/([\d.]+)px/)?.[1] ?? "20");
      return { width: value.length * px * 0.52 };
    },
    drawImage: () => {},
    getImageData: (_x: number, _y: number, w: number, h: number) => ({
      data: new Uint8ClampedArray(w * h * 4).fill(128),
      // Width/height are required: `toAnalysisImageData` forwards them into the
      // pure analysers, and without them the pre-pass is a no-op that would
      // make the cold budget guard measure nothing.
      width: w,
      height: h,
    }),
    beginPath: () => {},
    moveTo: () => {},
    lineTo: () => {},
    arcTo: () => {},
    arc: () => {},
    closePath: () => {},
    fill: () => {},
    stroke: () => {},
    clip: () => {},
    save: () => {},
    restore: () => {},
    translate: () => {},
    rotate: () => {},
  };
  jest
    .spyOn(HTMLCanvasElement.prototype, "getContext")
    .mockReturnValue(ctx as unknown as CanvasRenderingContext2D);
}

/**
 * A photo that resolves immediately. The analysis reads its pixels through the
 * stubbed 2D context above, whose `getImageData` returns a flat mid-grey field
 * — so this measures the ANALYSIS ARITHMETIC at full size, which is the part
 * that scales, and not image decode, which is codec- and network-bound.
 */
function stubLoadedImage(): void {
  class LoadedImage {
    crossOrigin = "";
    naturalWidth = 2000;
    naturalHeight = 1500;
    onload: (() => void) | null = null;
    onerror: (() => void) | null = null;
    set src(_value: string) {
      setTimeout(() => this.onload?.(), 0);
    }
  }
  (global as unknown as { Image: unknown }).Image = LoadedImage;
}

interface Card {
  /** Omitted puts `chooseComposition` in the measured path. */
  composition?: CompositionId;
  slots: Slots;
  /** Omitted skips the photo pipeline entirely. */
  photoUrl?: string;
}

async function renderThumbnail(card: Card): Promise<void> {
  await renderPost(
    {
      kit: "editorial",
      composition: card.composition,
      aspect: "4:5",
      photoUrl: card.photoUrl ?? "",
      slots: card.slots,
      palette: { primary: "#1a6b6a", secondary: "#0f3d3c" },
    },
    { targetWidth: LIBRARY_THUMBNAIL_WIDTH },
  );
}

interface Timings {
  minMs: number;
  medianMs: number;
  p90Ms: number;
  maxMs: number;
}

async function timeThumbnails(card: Card): Promise<Timings> {
  for (let run = 0; run < WARMUP_RUNS; run += 1) {
    await renderThumbnail(card);
  }
  const samples: number[] = [];
  for (let run = 0; run < SAMPLE_RUNS; run += 1) {
    const started = performance.now();
    await renderThumbnail(card);
    samples.push(performance.now() - started);
  }
  samples.sort((a, b) => a - b);
  const at = (quantile: number) =>
    samples[Math.floor((samples.length - 1) * quantile)];
  return { minMs: at(0), medianMs: at(0.5), p90Ms: at(0.9), maxMs: at(1) };
}

/**
 * Like `timeThumbnails`, but every sample starts with an empty analysis cache,
 * so each one pays the full pre-pass. Warm-ups still run — they pay for module
 * init and JIT, which is not what this case is measuring.
 */
async function timeColdThumbnails(card: Card): Promise<Timings> {
  for (let run = 0; run < WARMUP_RUNS; run += 1) {
    clearPhotoAnalysisCache();
    await renderThumbnail(card);
  }
  const samples: number[] = [];
  for (let run = 0; run < SAMPLE_RUNS; run += 1) {
    clearPhotoAnalysisCache();
    const started = performance.now();
    await renderThumbnail(card);
    samples.push(performance.now() - started);
  }
  samples.sort((a, b) => a - b);
  const at = (quantile: number) =>
    samples[Math.floor((samples.length - 1) * quantile)];
  return { minMs: at(0), medianMs: at(0.5), p90Ms: at(0.9), maxMs: at(1) };
}

/**
 * Jest matchers carry no custom message, and "expected 4.1 to be less than 6"
 * says nothing about how the run was shaped. Comparing verdict strings puts the
 * whole distribution in the diff of a failing run, where it is actionable.
 */
const withinBudget = (budgetMs: number) => `median under ${budgetMs}ms`;

function verdict(timings: Timings, budgetMs = BUDGET_MS): string {
  if (timings.medianMs < budgetMs) return withinBudget(budgetMs);
  return (
    `median ${timings.medianMs.toFixed(3)}ms over the ${budgetMs}ms budget ` +
    `(min ${timings.minMs.toFixed(3)}ms, p90 ${timings.p90Ms.toFixed(3)}ms, ` +
    `max ${timings.maxMs.toFixed(3)}ms, n=${SAMPLE_RUNS})`
  );
}

describe("renderPost per-thumbnail budget", () => {
  beforeEach(() => {
    stubCanvas();
    stubLoadedImage();
    clearPhotoAnalysisCache();
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  // A pinned composition skips the chooser, so this run is the scene build and
  // the walker with nothing else in front of them.
  it("renders a pinned-composition thumbnail within the per-card budget", async () => {
    const timings = await timeThumbnails({
      composition: "photoBottomStack",
      slots: TYPICAL_SLOTS,
    });
    expect(verdict(timings)).toBe(withinBudget(BUDGET_MS));
  });

  // Omitting the composition puts `chooseComposition` in the loop as well.
  it("renders a chooser-driven thumbnail within the per-card budget", async () => {
    const timings = await timeThumbnails({ slots: TYPICAL_SLOTS });
    expect(verdict(timings)).toBe(withinBudget(BUDGET_MS));
  });

  // The solver's linear descent is the pipeline's only unbounded-looking loop
  // and the first suspect in any regression here, so one case has to reach it.
  it("renders a thumbnail whose stack must shrink within the per-card budget", async () => {
    const timings = await timeThumbnails({
      composition: "splitPanel",
      slots: CROWDED_SLOTS,
    });
    expect(verdict(timings)).toBe(withinBudget(BUDGET_MS));
  });

  // The warm path is what the Library actually pays per card: the analysis is
  // memoized per photo URL, so only the first card of a given photo is cold.
  it("renders a photo thumbnail from a warm analysis within the per-card budget", async () => {
    const card: Card = {
      composition: "photoBottomStack",
      slots: TYPICAL_SLOTS,
      photoUrl: "https://cdn.test/dish.png",
    };
    await renderThumbnail(card); // prime the cache
    expect(verdict(await timeThumbnails(card))).toBe(withinBudget(BUDGET_MS));
  });

  // And the miss, which the warm case above would otherwise never measure —
  // leaving the guard watching only the cheap path.
  it("renders a photo thumbnail from a cold analysis within the cold budget", async () => {
    const timings = await timeColdThumbnails({
      composition: "photoBottomStack",
      slots: TYPICAL_SLOTS,
      photoUrl: "https://cdn.test/dish.png",
    });
    expect(verdict(timings, COLD_BUDGET_MS)).toBe(withinBudget(COLD_BUDGET_MS));
  });
});
