import type { MotifId } from "../artDirection/kits";
import { resolveRect } from "./geometry";
import { drawMotif, MOTIF_DRAWERS, MOTIF_OUTSET } from "./motifs";

interface Call {
  op: string;
  args: unknown[];
}

/** 2D affine, in canvas's `[a, b, c, d, e, f]` order. */
type Matrix = [number, number, number, number, number, number];
const IDENTITY: Matrix = [1, 0, 0, 1, 0, 0];

function multiply(m: Matrix, n: Matrix): Matrix {
  return [
    m[0] * n[0] + m[2] * n[1],
    m[1] * n[0] + m[3] * n[1],
    m[0] * n[2] + m[2] * n[3],
    m[1] * n[2] + m[3] * n[3],
    m[0] * n[4] + m[2] * n[5] + m[4],
    m[1] * n[4] + m[3] * n[5] + m[5],
  ];
}

function applyMatrix(m: Matrix, x: number, y: number): [number, number] {
  return [m[0] * x + m[2] * y + m[4], m[1] * x + m[3] * y + m[5]];
}

/**
 * A stub context that records calls AND keeps the state a real one would.
 *
 * The state matters. `globalAlpha` needs a real getter or `*=` reads
 * `undefined` and writes NaN, and the transform needs a real save/restore stack
 * or a drawer that leaks a `translate` looks identical to one that unwinds it.
 * The plain call log cannot tell those apart — see the note on the
 * "balances save/restore" assertion below.
 */
function recordingCtx(initialAlpha = 1) {
  const calls: Call[] = [];
  let alpha = initialAlpha;
  let ctm: Matrix = IDENTITY;
  const stack: Array<{ alpha: number; ctm: Matrix }> = [];

  const record =
    (op: string) =>
    (...args: unknown[]) => {
      calls.push({ op, args });
    };

  const ctx = {
    save: (...args: unknown[]) => {
      stack.push({ alpha, ctm });
      calls.push({ op: "save", args });
    },
    restore: (...args: unknown[]) => {
      const previous = stack.pop();
      if (previous) {
        alpha = previous.alpha;
        ctm = previous.ctm;
      }
      calls.push({ op: "restore", args });
    },
    translate: (x: number, y: number) => {
      ctm = multiply(ctm, [1, 0, 0, 1, x, y]);
      calls.push({ op: "translate", args: [x, y] });
    },
    rotate: (angle: number) => {
      const cos = Math.cos(angle);
      const sin = Math.sin(angle);
      ctm = multiply(ctm, [cos, sin, -sin, cos, 0, 0]);
      calls.push({ op: "rotate", args: [angle] });
    },
    beginPath: record("beginPath"),
    closePath: record("closePath"),
    moveTo: record("moveTo"),
    lineTo: record("lineTo"),
    arc: record("arc"),
    fill: record("fill"),
    stroke: record("stroke"),
    fillRect: record("fillRect"),
    set fillStyle(value: string) {
      calls.push({ op: "fillStyle", args: [value] });
    },
    set strokeStyle(value: string) {
      calls.push({ op: "strokeStyle", args: [value] });
    },
    set lineWidth(value: number) {
      calls.push({ op: "lineWidth", args: [value] });
    },
    get globalAlpha() {
      return alpha;
    },
    set globalAlpha(value: number) {
      alpha = value;
      calls.push({ op: "globalAlpha", args: [value] });
    },
  } as unknown as CanvasRenderingContext2D;

  return {
    ctx,
    calls,
    /** Live state, for assertions after the drawer returns. */
    state: {
      get alpha() {
        return alpha;
      },
      get ctm() {
        return ctm;
      },
      get depth() {
        return stack.length;
      },
    },
  };
}

const COLOR = "#c2410c";
const RECT = { x: 0.1, y: 0.5, w: 0.8, h: 0.3 };
/** What `buildScene` hands a texture motif. */
const FULL_BLEED = { x: 0, y: 0, w: 1, h: 1 };

function draw(
  motif: MotifId,
  canvasW: number,
  canvasH: number,
  rect = RECT,
  opacity?: number,
) {
  const harness = recordingCtx();
  drawMotif(
    harness.ctx,
    {
      kind: "motif",
      motif,
      rect,
      color: COLOR,
      z: 40,
      ...(opacity == null ? {} : { opacity }),
    },
    canvasW,
    canvasH,
  );
  return harness;
}

/** Float noise (`1080 * 0.035` is `37.800000000000004`) would make exact pins unreadable. */
function round(value: unknown): unknown {
  return typeof value === "number" ? Math.round(value * 1e6) / 1e6 : value;
}

function pinnedCalls(calls: Call[]): string[] {
  return calls.map((c) => `${c.op}(${c.args.map(round).join(", ")})`);
}

/**
 * Every motif id, listed once. `satisfies` stops a typo entering the list, and
 * `NoMotifUnlisted` below fails to compile if `MotifId` gains a member that is
 * missing from it — so the list cannot silently drift from the union.
 */
const ALL_MOTIFS = [
  "rule",
  "cornerBrackets",
  "seal",
  "ticketNotch",
  "halftone",
  "grain",
  "tape",
] as const satisfies readonly MotifId[];

type AssertNever<T extends never> = T;
type NoMotifUnlisted = AssertNever<
  Exclude<MotifId, (typeof ALL_MOTIFS)[number]>
>;
// Referenced so the alias is not dead code; the assertion happens at its
// declaration, above.
export type { NoMotifUnlisted };

const MOTIF_CASES: MotifId[] = [...ALL_MOTIFS];

/**
 * Ops that put ink on the canvas. Asserting on these rather than on the raw
 * call count matters: `drawMotif` itself always emits `save`/`restore`, so a
 * drawer that only sets `fillStyle` and paints nothing would still clear a
 * "more than two calls" bar.
 */
const PAINT_OPS = new Set(["fill", "stroke", "fillRect"]);

const TAU = Math.PI * 2;

describe("motifs", () => {
  it("has a drawer for every motif id and no extras", () => {
    expect(Object.keys(MOTIF_DRAWERS).sort()).toEqual([...MOTIF_CASES].sort());
    MOTIF_CASES.forEach((motif) => {
      expect(typeof MOTIF_DRAWERS[motif]).toBe("function");
    });
  });

  /**
   * Reads exactly as it says: the counts match. It cannot distinguish a drawer
   * that manages its own state from one leaning on `drawMotif`'s wrapper,
   * because a count has no stack semantics. "leaves the context as it found it"
   * below is the assertion that can, by calling drawers WITHOUT the wrapper.
   */
  it.each(MOTIF_CASES)("%s paints and balances save/restore", (motif) => {
    const { calls } = draw(motif, 1080, 1350);
    expect(calls.filter((c) => PAINT_OPS.has(c.op)).length).toBeGreaterThan(0);
    expect(calls.filter((c) => c.op === "save")).toHaveLength(
      calls.filter((c) => c.op === "restore").length,
    );
  });

  it.each(MOTIF_CASES)("%s draws identically on a repeat render", (motif) => {
    expect(draw(motif, 1080, 1350).calls).toEqual(
      draw(motif, 1080, 1350).calls,
    );
  });

  describe("colour", () => {
    /**
     * The module's headline claim is that nothing here names or decides a
     * colour. Checking `rule` alone left six drawers free to hardcode one, so
     * this runs over all seven and, unlike a "contains the node colour" check,
     * also rejects a drawer that paints the node colour AND something else.
     */
    it.each(MOTIF_CASES)("%s paints only in the node colour", (motif) => {
      const { calls } = draw(motif, 1080, 1350);
      const styles = calls
        .filter((c) => c.op === "fillStyle" || c.op === "strokeStyle")
        .map((c) => c.args[0]);
      expect(styles.length).toBeGreaterThan(0);
      expect([...new Set(styles)]).toEqual([COLOR]);
    });

    it.each(MOTIF_CASES)("%s follows the node colour when it changes", (motif) => {
      const harness = recordingCtx();
      drawMotif(
        harness.ctx,
        { kind: "motif", motif, rect: RECT, color: "#0f766e", z: 40 },
        1080,
        1350,
      );
      const styles = harness.calls
        .filter((c) => c.op === "fillStyle" || c.op === "strokeStyle")
        .map((c) => c.args[0]);
      expect([...new Set(styles)]).toEqual(["#0f766e"]);
    });
  });

  describe("opacity", () => {
    /**
     * `drawMotif` used to own alpha. It does not any more: `BaseNode.opacity` is
     * applied once by the walker for every node kind (see `paintNode` in
     * ./renderScene.ts), and the four cases that used to live here now live in
     * renderScene.test.ts under "per-node opacity". What remains is the guard
     * that this drawer never reaches for alpha again — because if it does, every
     * motif silently squares its own opacity.
     */
    it("never touches globalAlpha, even when the node carries opacity", () => {
      const { calls } = draw("grain", 1080, 1350, RECT, 0.4);
      expect(calls.some((c) => c.op === "globalAlpha")).toBe(false);
    });

    it("leaves the caller's group alpha exactly as it found it", () => {
      const harness = recordingCtx(0.5);
      drawMotif(
        harness.ctx,
        {
          kind: "motif",
          motif: "grain",
          rect: RECT,
          color: COLOR,
          opacity: 0.4,
          z: 15,
        },
        1080,
        1350,
      );
      expect(harness.state.alpha).toBeCloseTo(0.5, 10);
    });
  });

  /**
   * Called WITHOUT `drawMotif`'s wrapper, because `MOTIF_DRAWERS` is exported
   * and a caller may reach a drawer directly. This is the assertion that would
   * catch `drawTape` dropping its own `save`/`restore` and leaking a
   * `translate`/`rotate` onto the caller's context — the plain call-count check
   * above cannot, since the wrapper's own `restore` would balance the books.
   */
  it.each(MOTIF_CASES)("%s leaves the context as it found it", (motif) => {
    const harness = recordingCtx();
    const px = resolveRect(RECT, 1080, 1350);
    MOTIF_DRAWERS[motif](
      harness.ctx,
      { kind: "motif", motif, rect: RECT, color: COLOR, z: 40 },
      px,
      1080,
    );
    expect(harness.state.depth).toBe(0);
    expect(harness.state.ctm).toEqual(IDENTITY);
  });

  it("draws nothing for an unknown motif id", () => {
    const { calls } = draw("notAMotif" as MotifId, 1080, 1350);
    expect(calls).toHaveLength(0);
  });

  /**
   * TERMINATION is the property under test, not the assertion body.
   *
   * The assertion below is deliberately trivial — reaching it at all is the
   * result. Both dot-field drawers advance their loop counter by a pitch, so a
   * pitch that resolves to 0 spins forever on any rect with a non-zero extent
   * on the other axis. `drawHalftone` did exactly that at `canvasW = 0`
   * (`step = canvasW * 0.03`) while its sibling `drawGrain` did not
   * (`step = max(2, canvasW * 0.006)`).
   *
   * A regression here does NOT fail the suite — it wedges it. Jest's per-test
   * timeout is a timer, and a synchronous loop never yields the event loop for
   * it to fire, so the run produces no output at all and has to be killed.
   * Measured against the pre-fix drawer: `timeout 45 npx jest … -t "halftone @
   * zero width"` exits 124 with an empty log, while the same probe for `grain`
   * exits 0 in 0.239s. Treat a hang on this block as this assertion failing.
   *
   * `canvasW = 0` is reachable: `buildScene` carries explicit `canvasW > 0 ? …
   * : 0` guards, so a zero dimension is a state the pipeline already models,
   * and `renderScene` passes `scene.width` straight through. The ticket kit's
   * full-bleed halftone texture means an unmeasured canvas would lock the tab.
   */
  describe("terminates at degenerate dimensions", () => {
    const DEGENERATE: Array<[string, number, number]> = [
      ["zero width", 0, 1350],
      ["zero height", 1080, 0],
      ["both zero", 0, 0],
      // Below the pitch floors, where a proportional step would go sub-pixel
      // and the loop count would explode even though it stays finite.
      ["one-pixel canvas", 1, 1],
    ];

    it.each(
      DEGENERATE.flatMap(([label, w, h]) =>
        MOTIF_CASES.map(
          (motif) => [`${motif} @ ${label}`, motif, w, h] as const,
        ),
      ),
    )("%s returns", (_label, motif, canvasW, canvasH) => {
      draw(motif, canvasW, canvasH, FULL_BLEED);
      // Reaching this line IS the assertion.
      expect(true).toBe(true);
    });

    it.each(MOTIF_CASES)("%s returns on a degenerate rect", (motif) => {
      draw(motif, 1080, 1350, { x: 0.5, y: 0.5, w: 0, h: 0 });
      expect(true).toBe(true);
    });
  });

  /**
   * `resolveRect` exists so the axis pairing is stated once instead of being
   * transposed at each call site, so both halves of it need guarding. A fixture
   * of `{ x: 0, y: 0, w: 1, h: 1 }` cannot do that: both ORIGINS multiply to
   * zero, so resolving `x` against height or `y` against width is invisible.
   * Non-zero origins and a non-square canvas make all four assertions bite.
   *
   * `cornerBrackets` carries this rather than `ticketNotch` for two reasons: it
   * is the only drawer that touches all four edges of the rect, and unlike
   * `ticketNotch` it is actually reachable from a composition today.
   */
  it("scales the rect against canvas width and height", () => {
    const { calls } = draw("cornerBrackets", 1000, 2000, {
      x: 0.1,
      y: 0.2,
      w: 0.5,
      h: 0.6,
    });
    // x = 0.1 * 1000 = 100, y = 0.2 * 2000 = 400,
    // w = 0.5 * 1000 = 500, h = 0.6 * 2000 = 1200 -> right 600, bottom 1600.
    const corners = calls
      .filter((c) => c.op === "lineTo")
      .filter((_, index) => index % 2 === 0)
      .map((c) => [round(c.args[0]), round(c.args[1])]);
    expect(corners).toEqual([
      [100, 400],
      [600, 400],
      [100, 1600],
      [600, 1600],
    ]);
  });

  it("resolves each rect field against exactly one axis", () => {
    expect(resolveRect({ x: 0.1, y: 0.2, w: 0.5, h: 0.6 }, 1000, 2000)).toEqual({
      x: 100,
      y: 400,
      w: 500,
      h: 1200,
    });
  });

  /**
   * Exact painted geometry, at a NON-SQUARE canvas so that a drawer scaling off
   * canvas height instead of width shows up. Rect { 0.1, 0.5, 0.8, 0.3 } at
   * 1080x1350 resolves to px { x: 108, y: 675, w: 864, h: 405 }.
   *
   * Pinning coordinates rather than call counts is the point: a suite that only
   * checks "something was painted" passes every one of a rule's length, a
   * seal's radius, a bracket's arm, a halftone's pitch and a tape's rotation
   * being changed out from under it.
   */
  describe("painted geometry at 1080x1350", () => {
    it("rule: lifted 21.6px above the rect, 18% of the rect wide", () => {
      expect(pinnedCalls(draw("rule", 1080, 1350).calls)).toEqual([
        "save()",
        `strokeStyle(${COLOR})`,
        // max(2, 1080 * 0.004)
        "lineWidth(4.32)",
        "beginPath()",
        // y = 675 - 1080 * 0.02
        "moveTo(108, 653.4)",
        // x = 108 + 864 * 0.18
        "lineTo(263.52, 653.4)",
        "stroke()",
        "restore()",
      ]);
    });

    it("cornerBrackets: four inward L's, arm 54px", () => {
      expect(pinnedCalls(draw("cornerBrackets", 1080, 1350).calls)).toEqual([
        "save()",
        `strokeStyle(${COLOR})`,
        // max(2, 1080 * 0.003)
        "lineWidth(3.24)",
        // arm = 1080 * 0.05 = 54; every arm points INWARD from its corner.
        "beginPath()",
        "moveTo(162, 675)",
        "lineTo(108, 675)",
        "lineTo(108, 729)",
        "stroke()",
        "beginPath()",
        "moveTo(918, 675)",
        "lineTo(972, 675)",
        "lineTo(972, 729)",
        "stroke()",
        "beginPath()",
        "moveTo(162, 1080)",
        "lineTo(108, 1080)",
        "lineTo(108, 1026)",
        "stroke()",
        "beginPath()",
        "moveTo(918, 1080)",
        "lineTo(972, 1080)",
        "lineTo(972, 1026)",
        "stroke()",
        "restore()",
      ]);
    });

    it("seal: r=37.8, centre 1.4r above the rect and flush with its right edge", () => {
      expect(pinnedCalls(draw("seal", 1080, 1350).calls)).toEqual([
        "save()",
        `fillStyle(${COLOR})`,
        "beginPath()",
        // cx = 108 + 864 - 37.8, cy = 675 - 37.8 * 1.4, r = 1080 * 0.035
        `arc(934.2, 622.08, 37.8, 0, ${round(TAU)})`,
        "fill()",
        "restore()",
      ]);
    });

    it("ticketNotch: r=30.24 discs centred ON both vertical edges", () => {
      expect(pinnedCalls(draw("ticketNotch", 1080, 1350).calls)).toEqual([
        "save()",
        `fillStyle(${COLOR})`,
        "beginPath()",
        // midY = 675 + 405 / 2
        `arc(108, 877.5, 30.24, 0, ${round(TAU)})`,
        "fill()",
        "beginPath()",
        `arc(972, 877.5, 30.24, 0, ${round(TAU)})`,
        "fill()",
        "restore()",
      ]);
    });

    it("halftone: 32.4px pitch, r=4.32, 27x13 lattice from the rect origin", () => {
      const { calls } = draw("halftone", 1080, 1350);
      const dots = calls.filter((c) => c.op === "arc");
      // ceil(864 / 32.4) = 27 columns, ceil(405 / 32.4) = 13 rows.
      expect(dots).toHaveLength(351);
      expect(calls.filter((c) => c.op === "fill")).toHaveLength(351);
      // Exactly three ops per dot, plus the wrapper's save/restore and one
      // fillStyle — the cost model the drawer's doc comment quotes.
      expect(calls).toHaveLength(351 * 3 + 3);
      expect(pinnedCalls([dots[0]])).toEqual([`arc(108, 675, 4.32, 0, ${round(TAU)})`]);
      expect(pinnedCalls([dots[1]])).toEqual([`arc(140.4, 675, 4.32, 0, ${round(TAU)})`]);
      expect(pinnedCalls([dots[dots.length - 1]])).toEqual([
        `arc(950.4, 1063.8, 4.32, 0, ${round(TAU)})`,
      ]);
    });

    it("grain: 6.48px pitch, 1.62px dots, one cell in five inked", () => {
      const { calls } = draw("grain", 1080, 1350);
      const dots = calls.filter((c) => c.op === "fillRect");
      // ceil(864 / 6.48) = 134 columns, ceil(405 / 6.48) = 63 rows = 8442 cells.
      expect(dots).toHaveLength(1730);
      expect(dots.length / (134 * 63)).toBeCloseTo(0.2, 2);
      expect(pinnedCalls([dots[0]])).toEqual(["fillRect(127.44, 675, 1.62, 1.62)"]);
      expect(pinnedCalls([dots[dots.length - 1]])).toEqual([
        "fillRect(943.92, 1076.76, 1.62, 1.62)",
      ]);
      // Every cell sits on the 6.48px lattice anchored at the rect origin.
      const px = resolveRect(RECT, 1080, 1350);
      dots.forEach((c) => {
        const col = ((c.args[0] as number) - px.x) / 6.48;
        const row = ((c.args[1] as number) - px.y) / 6.48;
        expect(Math.abs(col - Math.round(col))).toBeLessThan(1e-6);
        expect(Math.abs(row - Math.round(row))).toBeLessThan(1e-6);
      });
    });

    it("tape: 172.8x48.6 strip, lifted a full height and tilted -0.14rad", () => {
      expect(pinnedCalls(draw("tape", 1080, 1350).calls)).toEqual([
        "save()",
        // The drawer's own pair, inside drawMotif's.
        "save()",
        // x = 108 + 172.8 * 0.2, y = 675 - 48.6
        "translate(142.56, 626.4)",
        "rotate(-0.14)",
        `fillStyle(${COLOR})`,
        "fillRect(0, 0, 172.8, 48.6)",
        "restore()",
        "restore()",
      ]);
    });

    /**
     * The scaling AXIS, isolated. Every constant in this module is a fraction
     * of canvas WIDTH, so rendering the same rect at the same width but a
     * different height must leave every motif-INTRINSIC length identical. Only
     * positions may move, since `y`/`h` legitimately resolve against height —
     * hence sizes only below (radii, stroke weights, strip dimensions, tilt),
     * never coordinates, and compared as a distinct set because the taller
     * canvas legitimately emits more dots.
     */
    it.each(MOTIF_CASES)("%s scales off canvas width, not height", (motif) => {
      const sizes = (calls: Call[]) => {
        const values: number[] = [];
        calls.forEach((c) => {
          const n = c.args as number[];
          if (c.op === "lineWidth" || c.op === "rotate") values.push(n[0]);
          if (c.op === "arc") values.push(n[2]);
          if (c.op === "fillRect") values.push(n[2], n[3]);
        });
        return [...new Set(values.map((v) => round(v) as number))].sort(
          (a, b) => a - b,
        );
      };
      const square = sizes(draw(motif, 1080, 1080, FULL_BLEED).calls);
      expect(square.length).toBeGreaterThan(0);
      expect(sizes(draw(motif, 1080, 1920, FULL_BLEED).calls)).toEqual(square);
    });
  });

  /**
   * `MOTIF_OUTSET` is a hand-written table describing what seven drawers do. It
   * is only worth anything if it cannot drift from them, so it is checked two
   * ways: pinned to exact numbers here (a table of `Infinity` would satisfy
   * coverage alone), and measured against the drawers' real recorded ink below.
   */
  describe("MOTIF_OUTSET", () => {
    it("declares an entry for every motif and no extras", () => {
      expect(Object.keys(MOTIF_OUTSET).sort()).toEqual([...MOTIF_CASES].sort());
    });

    it("pins the outsets at canvasW = 1080", () => {
      const at = (motif: MotifId) => {
        const o = MOTIF_OUTSET[motif](1080);
        return {
          top: Math.round(o.top * 1e4) / 1e4,
          right: Math.round(o.right * 1e4) / 1e4,
          bottom: Math.round(o.bottom * 1e4) / 1e4,
          left: Math.round(o.left * 1e4) / 1e4,
        };
      };
      // rule:   lift 1080*0.02 = 21.6, plus half of max(2, 4.32) = 2.16.
      expect(at("rule")).toEqual({ top: 23.76, right: 0, bottom: 0, left: 2.16 });
      // cornerBrackets: half of max(2, 1080*0.003) = 1.62, all four sides.
      expect(at("cornerBrackets")).toEqual({
        top: 1.62,
        right: 1.62,
        bottom: 1.62,
        left: 1.62,
      });
      // seal:   1080*0.035 = 37.8 radius, x (1.4 + 1) = 90.72.
      expect(at("seal")).toEqual({ top: 90.72, right: 0, bottom: 0, left: 0 });
      // ticketNotch: 1080*0.028 = 30.24, half the disc outside each edge.
      expect(at("ticketNotch")).toEqual({
        top: 0,
        right: 30.24,
        bottom: 0,
        left: 30.24,
      });
      // halftone: dot radius 1080*0.004 = 4.32 on every side.
      expect(at("halftone")).toEqual({
        top: 4.32,
        right: 4.32,
        bottom: 4.32,
        left: 4.32,
      });
      // grain: dot max(1, 1080*0.0015) = 1.62, far edges only.
      expect(at("grain")).toEqual({ top: 0, right: 1.62, bottom: 1.62, left: 0 });
      // tape: 1080*0.045 = 48.6 lift + 1080*0.16*sin(0.14) = 24.1131.
      expect(at("tape")).toEqual({ top: 72.7131, right: 0, bottom: 0, left: 0 });
    });

    it("scales linearly with canvas width above the stroke floors", () => {
      MOTIF_CASES.forEach((motif) => {
        const one = MOTIF_OUTSET[motif](1080);
        const two = MOTIF_OUTSET[motif](2160);
        (["top", "right", "bottom", "left"] as const).forEach((side) => {
          expect(two[side]).toBeCloseTo(one[side] * 2, 6);
        });
      });
    });

    it("never declares a negative outset", () => {
      [0, 1, 100, 540, 1080, 2160].forEach((canvasW) => {
        MOTIF_CASES.forEach((motif) => {
          const o = MOTIF_OUTSET[motif](canvasW);
          (["top", "right", "bottom", "left"] as const).forEach((side) => {
            expect(o[side]).toBeGreaterThanOrEqual(0);
          });
        });
      });
    });

    /**
     * The envelopes every composition in ../composition/compositions.ts
     * declares, plus the full-bleed rect a kit texture uses.
     *
     * These are the DECLARED rects. `safeBounds` clamps them into the
     * platform-safe area before `buildScene` passes them on, which on the tall
     * aspects trims height (photoBottomStack's 0.44 becomes 0.32 at 9:16); the
     * clamped versions clear every precondition below just as comfortably, the
     * tightest being legacyMinimal's unclamped 0.18 x 1080 = 194px against a
     * 60px floor. `legacyEditorial` is omitted only because its envelope is
     * identical to `photoBottomStack`'s.
     */
    const REAL_RECTS = [
      { id: "photoBottomStack", x: 0.07, y: 0.5, w: 0.86, h: 0.44 },
      { id: "photoTopStack", x: 0.07, y: 0.08, w: 0.86, h: 0.4 },
      { id: "splitPanel", x: 0.08, y: 0.63, w: 0.84, h: 0.3 },
      { id: "badgeHero", x: 0.08, y: 0.46, w: 0.84, h: 0.46 },
      { id: "cornerCard", x: 0.08, y: 0.58, w: 0.6, h: 0.34 },
      { id: "posterStack", x: 0.1, y: 0.16, w: 0.8, h: 0.68 },
      { id: "legacyBold@1:1", x: 0.06, y: 0.07, w: 0.88, h: 0.875 },
      { id: "legacyBold@9:16", x: 0.06, y: 0.15, w: 0.88, h: 0.665 },
      { id: "legacyMinimal@1:1", x: 0.08, y: 0.75, w: 0.84, h: 0.18 },
      { id: "legacyMinimal@4:5", x: 0.08, y: 0.72, w: 0.84, h: 0.19 },
      { id: "legacyMinimal@9:16", x: 0.08, y: 0.59, w: 0.84, h: 0.21 },
      { id: "texture(full bleed)", ...FULL_BLEED },
    ];

    const ASPECTS: Array<[string, number, number]> = [
      ["1:1", 1080, 1080],
      ["4:5", 1080, 1350],
      ["9:16", 1080, 1920],
    ];

    /**
     * The declared outsets assume a rect at least as large as the mark itself.
     * Asserting that here means the preconditions in `MOTIF_OUTSET`'s docstring
     * cannot quietly stop being true of the bounds real compositions produce.
     */
    it("the measured rects satisfy the documented preconditions", () => {
      ASPECTS.forEach(([, canvasW, canvasH]) => {
        REAL_RECTS.forEach((rect) => {
          const px = resolveRect(rect, canvasW, canvasH);
          const label = `${rect.id} @ ${canvasW}x${canvasH}`;
          // seal: px.w >= 2r = 0.07 * canvasW
          expect(`${label} seal ${px.w >= 0.07 * canvasW}`).toBe(`${label} seal true`);
          // ticketNotch: px.h >= 2r = 0.056 * canvasW
          expect(`${label} notch ${px.h >= 0.056 * canvasW}`).toBe(`${label} notch true`);
          // tape: px.w >= 0.197 * canvasW
          expect(`${label} tape ${px.w >= 0.197 * canvasW}`).toBe(`${label} tape true`);
          // cornerBrackets: both extents >= arm = 0.05 * canvasW
          expect(
            `${label} brackets ${px.w >= 0.05 * canvasW && px.h >= 0.05 * canvasW}`,
          ).toBe(`${label} brackets true`);
        });
      });
    });

    /**
     * The declared table against the drawers' actual ink.
     *
     * The ink model inflates a stroked path by `lineWidth / 2` in BOTH axes.
     * That is an upper bound for every cap style and for the 90-degree miter
     * joins the brackets make (whose outer corner sits exactly `lineWidth / 2`
     * out on each axis); it would understate an acute miter, of which there are
     * none here. `fillRect` and `arc` are exact.
     */
    function inkBounds(calls: Call[]) {
      let minX = Infinity;
      let minY = Infinity;
      let maxX = -Infinity;
      let maxY = -Infinity;
      let ctm: Matrix = IDENTITY;
      const stack: Matrix[] = [];
      let lineWidth = 1;
      let path: Array<[number, number]> = [];
      let arcRadius = 0;

      const add = (x: number, y: number, pad: number) => {
        minX = Math.min(minX, x - pad);
        minY = Math.min(minY, y - pad);
        maxX = Math.max(maxX, x + pad);
        maxY = Math.max(maxY, y + pad);
      };

      calls.forEach((c) => {
        const n = c.args as number[];
        switch (c.op) {
          case "save":
            stack.push(ctm);
            break;
          case "restore":
            ctm = stack.pop() ?? IDENTITY;
            break;
          case "translate":
            ctm = multiply(ctm, [1, 0, 0, 1, n[0], n[1]]);
            break;
          case "rotate":
            ctm = multiply(ctm, [
              Math.cos(n[0]),
              Math.sin(n[0]),
              -Math.sin(n[0]),
              Math.cos(n[0]),
              0,
              0,
            ]);
            break;
          case "lineWidth":
            lineWidth = n[0];
            break;
          case "beginPath":
            path = [];
            arcRadius = 0;
            break;
          case "moveTo":
          case "lineTo":
            path.push(applyMatrix(ctm, n[0], n[1]));
            break;
          case "arc":
            path.push(applyMatrix(ctm, n[0], n[1]));
            arcRadius = n[2];
            break;
          case "stroke":
            path.forEach(([x, y]) => add(x, y, lineWidth / 2));
            break;
          case "fill":
            path.forEach(([x, y]) => add(x, y, arcRadius));
            break;
          case "fillRect": {
            const [x, y, w, h] = n;
            [
              [x, y],
              [x + w, y],
              [x + w, y + h],
              [x, y + h],
            ].forEach(([cx, cy]) => {
              const [ax, ay] = applyMatrix(ctm, cx, cy);
              add(ax, ay, 0);
            });
            break;
          }
          default:
            break;
        }
      });

      return { minX, minY, maxX, maxY };
    }

    const CASES = MOTIF_CASES.flatMap((motif) =>
      ASPECTS.flatMap(([aspect, canvasW, canvasH]) =>
        REAL_RECTS.map(
          (rect) =>
            [
              `${motif} on ${rect.id} @ ${aspect}`,
              motif,
              canvasW,
              canvasH,
              rect,
            ] as const,
        ),
      ),
    );

    it.each(CASES)(
      "%s: declared outset covers the ink",
      (label, motif, canvasW, canvasH, rect) => {
        const { calls } = draw(motif, canvasW, canvasH, rect);
        const ink = inkBounds(calls);
        const px = resolveRect(rect, canvasW, canvasH);
        const declared = MOTIF_OUTSET[motif](canvasW);
        const actual = {
          top: Math.max(0, px.y - ink.minY),
          right: Math.max(0, ink.maxX - (px.x + px.w)),
          bottom: Math.max(0, ink.maxY - (px.y + px.h)),
          left: Math.max(0, px.x - ink.minX),
        };
        (["top", "right", "bottom", "left"] as const).forEach((side) => {
          // Reported as a string so a failure names the motif, the rect and the
          // side rather than just "23.76 is not >= 90.72".
          expect(
            `${label} ${side}: declared ${declared[side].toFixed(2)} vs ink ${actual[side].toFixed(2)}`,
          ).toBe(
            `${label} ${side}: declared ${declared[side].toFixed(2)} vs ink ${Math.min(actual[side], declared[side]).toFixed(2)}`,
          );
        });
      },
    );

    /**
     * Coverage alone is satisfied by a table of huge numbers, and the numeric
     * pin above is satisfied by numbers invented to match it. This closes the
     * loop from the other side: every side declared non-zero must be ink the
     * drawers actually put there, reached somewhere in the matrix.
     */
    it("declares no outset the drawers do not actually need", () => {
      /**
       * `grain` and `halftone` overhang the far edges only when their lattice
       * happens to land within one dot of the rect's right or bottom edge,
       * which the composition envelopes above never do. The bound is real
       * regardless — it is a property of the rect's SIZE modulo the pitch — so
       * the sweep below walks a range of widths and heights fine enough to
       * cross every phase of both pitches (6.48px and 32.4px at canvasW 1080,
       * i.e. 0.006 and 0.03 normalized).
       */
      const PHASE_RECTS = Array.from({ length: 80 }, (_, i) => ({
        id: `phase-${i}`,
        x: 0.02,
        y: 0.02,
        w: 0.5 + i * 0.001,
        h: 0.5 + i * 0.001,
      }));
      const SWEEP = [...REAL_RECTS, ...PHASE_RECTS];

      MOTIF_CASES.forEach((motif) => {
        const reached = { top: 0, right: 0, bottom: 0, left: 0 };
        ASPECTS.forEach(([, canvasW, canvasH]) => {
          SWEEP.forEach((rect) => {
            const ink = inkBounds(draw(motif, canvasW, canvasH, rect).calls);
            if (!Number.isFinite(ink.minX)) return;
            const px = resolveRect(rect, canvasW, canvasH);
            reached.top = Math.max(reached.top, px.y - ink.minY);
            reached.right = Math.max(reached.right, ink.maxX - (px.x + px.w));
            reached.bottom = Math.max(reached.bottom, ink.maxY - (px.y + px.h));
            reached.left = Math.max(reached.left, px.x - ink.minX);
          });
        });
        const declared = MOTIF_OUTSET[motif](1080);
        (["top", "right", "bottom", "left"] as const).forEach((side) => {
          if (declared[side] === 0) return;
          expect(`${motif}.${side} reached ${reached[side] > 0}`).toBe(
            `${motif}.${side} reached true`,
          );
        });
      });
    });
  });
});
