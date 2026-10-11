import { motifNode, logoNode, photoNode, scrimNode, textNode, type Scene } from "../scene/types";
import { evaluateSceneAt } from "./evaluate";
import { MOTION_DURATION_MS, tracksForScene } from "./presets";

/**
 * A CPU guard on the ONE part of the motion pipeline jest can measure.
 *
 * `evaluateSceneAt` runs once per frame — 180 times for a six-second clip at 30
 * fps — so an accidental O(nodes x tracks x something) or a stray allocation
 * here multiplies by 180. Rasterization and encoding are NOT measured and cannot
 * be: jest maps `^canvas$` to a stub and jsdom has no `VideoEncoder`. This is the
 * pure evaluator's own JavaScript, nothing else.
 *
 * Budget rationale. A full scene (11 nodes) under the busiest preset produces
 * ~24 tracks. The evaluator is O(nodes x tracks) with one shallow clone per
 * touched node; on an Apple-silicon dev box that measures ~0.02-0.04 ms.
 *
 * `BUDGET_MS` used to be 0.5 (~15× local). That was fine on a quiet laptop and
 * flaky on shared GitHub Actions VMs: AWS Lite runs 30207001023 / 30207630385
 * failed at medians 0.51 ms and 0.77 ms while suites were otherwise green —
 * sub-millisecond wall-clock noise, not a real regression. The budget is now
 * 2.0 ms (~50× quiet local, ~3× the noisiest observed CI sample): still
 * invisible for a whole clip (180 × 2 ms = 360 ms of pure eval) and still
 * catches a true 100× regression; it will not red-build a healthy release on
 * a busy runner.
 *
 * This budget is NOT a substitute for `../templates/renderPost.perf.test.ts`,
 * whose `BUDGET_MS = 6` guards the per-thumbnail static path and which this wave
 * must leave untouched.
 */
const BUDGET_MS = 2.0;
const WARMUP_RUNS = 15;
const SAMPLE_RUNS = 40;
/** Best-of-N medians: CI GC spikes one trial without reflecting steady cost. */
const TRIALS = 3;

const W = 1080;
const H = 1350;

const text = (key: string, y: number) =>
  textNode({
    key,
    text: "Milanesa napolitana con papas",
    rect: { x: 0.07, y, w: 0.86, h: 0.06 },
    z: 30,
    font: "sans",
    weight: 500,
    sizePct: 0.03,
    align: "left",
    color: "#ffffff",
    maxLines: 1,
    lineHeight: 1.15,
  });

const scene: Scene = {
  width: W,
  height: H,
  background: "#1a6b6a",
  nodes: [
    photoNode({ url: "a.jpg", rect: { x: 0, y: 0, w: 1, h: 1 }, z: 0 }),
    scrimNode({
      rect: { x: 0, y: 0.42, w: 1, h: 0.58 },
      z: 10,
      from: "rgba(28,25,23,0)",
      to: "rgba(28,25,23,0.78)",
      adaptive: false,
      luminanceThreshold: 155,
    }),
    motifNode({ motif: "halftone", rect: { x: 0, y: 0, w: 1, h: 1 }, color: "#fff", z: 15, opacity: 0.12 }),
    text("badge", 0.55),
    text("dishName", 0.63),
    text("price", 0.75),
    text("cta", 0.82),
    text("handle", 0.88),
    motifNode({ motif: "rule", rect: { x: 0.07, y: 0.5, w: 0.86, h: 0.4 }, color: "#fff", z: 40, opacity: 0.9 }),
    // LogoRect is origin + diameter only — no `h` (see scene/types.ts LogoRect).
    logoNode({ url: "l.png", rect: { x: 0.07, y: 0.07, w: 0.11 }, z: 50 }),
  ],
};

const tracks = tracksForScene(scene, "ticketSlide", MOTION_DURATION_MS);

interface Timings {
  minMs: number;
  medianMs: number;
  p90Ms: number;
  maxMs: number;
}

function timeEvaluation(): Timings {
  for (let run = 0; run < WARMUP_RUNS; run += 1) {
    evaluateSceneAt(scene, tracks, (run * MOTION_DURATION_MS) / WARMUP_RUNS);
  }
  const samples: number[] = [];
  for (let run = 0; run < SAMPLE_RUNS; run += 1) {
    const at = (run * MOTION_DURATION_MS) / SAMPLE_RUNS;
    const started = performance.now();
    evaluateSceneAt(scene, tracks, at);
    samples.push(performance.now() - started);
  }
  samples.sort((a, b) => a - b);
  const quantile = (q: number) => samples[Math.floor((samples.length - 1) * q)];
  return {
    minMs: quantile(0),
    medianMs: quantile(0.5),
    p90Ms: quantile(0.9),
    maxMs: quantile(1),
  };
}

const WITHIN_BUDGET = `median under ${BUDGET_MS}ms`;

function verdict(timings: Timings): string {
  if (timings.medianMs < BUDGET_MS) return WITHIN_BUDGET;
  return (
    `median ${timings.medianMs.toFixed(4)}ms over the ${BUDGET_MS}ms budget ` +
    `(min ${timings.minMs.toFixed(4)}ms, p90 ${timings.p90Ms.toFixed(4)}ms, ` +
    `max ${timings.maxMs.toFixed(4)}ms, n=${SAMPLE_RUNS})`
  );
}

describe("evaluateSceneAt per-frame budget", () => {
  it("evaluates a full scene under the busiest preset within budget", () => {
    // Take the best median across trials so a single GC pause on a shared CI
    // runner cannot fail a healthy evaluator (budget still bounds each trial).
    let best: Timings | null = null;
    for (let trial = 0; trial < TRIALS; trial += 1) {
      const timings = timeEvaluation();
      if (!best || timings.medianMs < best.medianMs) best = timings;
      if (timings.medianMs < BUDGET_MS) {
        expect(verdict(timings)).toBe(WITHIN_BUDGET);
        return;
      }
    }
    expect(verdict(best!)).toBe(WITHIN_BUDGET);
  });

  it("emits enough tracks for the measurement to mean something", () => {
    expect(tracks.length).toBeGreaterThanOrEqual(15);
  });
});
