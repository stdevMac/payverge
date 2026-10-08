/** @jest-environment jsdom */
import { renderPost } from "../templates/renderPost";
import { kitRenderPlan } from "./campaignKit";
import { FORMAT_ORDER, SCREEN_FORMATS } from "./formats";
import type { RenderPostInput } from "../templates/renderPost";

/**
 * A CPU guard on the fan-out, sibling to templates/renderPost.perf.test.ts.
 *
 * That suite guards ONE card at 6ms because the Library paints a whole grid of
 * them on scroll. This one guards the kit, which is a different shape of cost:
 * it runs once, on an explicit button press, over five screen formats — but two
 * of those canvases are larger than anything the feed renders (wide is
 * 1920x1080 against 4:5's 1080x1350), so the per-format cost is not uniform and
 * a naive "6 x 6ms" budget would be wrong in both directions.
 *
 * The budget below is deliberately generous in absolute terms and tight in
 * RELATIVE ones: what it actually asserts is that the kit costs no more than the
 * sum of its parts plus a small constant. A regression that made the fan-out
 * re-derive art direction per format, or re-measure the same text six times,
 * would break the ratio long before it broke a wall-clock number.
 *
 * Absolute budget was 40 ms; relative used 2.5× + 5 ms. On shared CI VMs a
 * single-card median can read unrealistically low (sub-ms) while the kit sample
 * absorbs one GC pause — producing false "superlinear" failures
 * (e.g. kit 8.29 ms vs ceiling 8.26 ms). Floor the per-card baseline, widen the
 * overhead multiplier slightly, and take the best kit median across trials so
 * the gate still fails on true O(n²) fan-out without red-building a healthy tree.
 *
 * Same harness conventions as the per-card suite: median of a warmed sample,
 * verdict compared as a string so a failure shows the whole distribution, and a
 * plain stub context so nothing measures rasterization (jest maps `^canvas$` to
 * a mock and there is no real canvas here).
 */
const BUDGET_MS = 80;
const WARMUP_RUNS = 5;
const SAMPLE_RUNS = 15;
/** Ignore single-card medians below this so noise-fast baselines cannot tighten the ratio. */
const ONE_CARD_FLOOR_MS = 0.75;
const RELATIVE_OVERHEAD = 3.5;
const RELATIVE_CONSTANT_MS = 12;
const TRIALS = 3;

const BASE: RenderPostInput = {
  kit: "editorial",
  composition: "photoBottomStack",
  aspect: "4:5",
  photoUrl: "",
  slots: {
    dishName: "Milanesa napolitana con papas",
    price: "$12.00",
    badge: "CHEF'S PICK",
    cta: "Order tonight",
    handle: "@casasur",
  },
  palette: { primary: "#1a6b6a", secondary: "#0f3d3c" },
};

function stubCanvas(): void {
  const ctx = {
    font: "",
    fillStyle: "",
    strokeStyle: "",
    lineWidth: 0,
    globalAlpha: 1,
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

async function renderScreenKit(): Promise<void> {
  const plan = kitRenderPlan(BASE, SCREEN_FORMATS, "Milanesa");
  for (const entry of plan) {
    await renderPost(entry.input);
  }
}

async function renderOne(): Promise<void> {
  await renderPost(BASE);
}

async function median(run: () => Promise<void>): Promise<number> {
  for (let index = 0; index < WARMUP_RUNS; index += 1) await run();
  const samples: number[] = [];
  for (let index = 0; index < SAMPLE_RUNS; index += 1) {
    const started = performance.now();
    await run();
    samples.push(performance.now() - started);
  }
  samples.sort((a, b) => a - b);
  return samples[Math.floor((samples.length - 1) * 0.5)];
}

const WITHIN_BUDGET = `kit median under ${BUDGET_MS}ms`;

describe("campaign kit fan-out budget", () => {
  beforeEach(() => {
    stubCanvas();
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("renders the whole screen kit within the export budget", async () => {
    let kitMs = Number.POSITIVE_INFINITY;
    for (let trial = 0; trial < TRIALS; trial += 1) {
      kitMs = Math.min(kitMs, await median(renderScreenKit));
      if (kitMs < BUDGET_MS) break;
    }
    const verdict =
      kitMs < BUDGET_MS
        ? WITHIN_BUDGET
        : `kit median ${kitMs.toFixed(3)}ms over the ${BUDGET_MS}ms budget (n=${SAMPLE_RUNS}, ${SCREEN_FORMATS.length} formats)`;
    expect(verdict).toBe(WITHIN_BUDGET);
  });

  // The real assertion. Anything that made the fan-out superlinear in the number
  // of formats — re-resolving art direction per format, re-measuring shared text,
  // rebuilding the palette — shows up here first.
  it("costs no more than its parts plus a small constant", async () => {
    const oneRaw = await median(renderOne);
    const oneMs = Math.max(oneRaw, ONE_CARD_FLOOR_MS);
    let kitMs = Number.POSITIVE_INFINITY;
    for (let trial = 0; trial < TRIALS; trial += 1) {
      kitMs = Math.min(kitMs, await median(renderScreenKit));
    }
    const ceiling =
      oneMs * SCREEN_FORMATS.length * RELATIVE_OVERHEAD + RELATIVE_CONSTANT_MS;
    const verdict =
      kitMs <= ceiling
        ? "linear in the number of formats"
        : `kit ${kitMs.toFixed(3)}ms exceeds ${ceiling.toFixed(3)}ms (one card ${oneMs.toFixed(3)}ms x ${SCREEN_FORMATS.length} formats x ${RELATIVE_OVERHEAD} + ${RELATIVE_CONSTANT_MS})`;
    expect(verdict).toBe("linear in the number of formats");
  });

  it("does not send the print format through the canvas path", () => {
    const plan = kitRenderPlan(BASE, FORMAT_ORDER, "Milanesa");
    expect(plan.filter((entry) => entry.format.medium === "screen")).toHaveLength(
      SCREEN_FORMATS.length,
    );
  });
});
