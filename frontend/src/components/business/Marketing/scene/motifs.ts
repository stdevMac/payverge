import type { MotifId } from "../artDirection/kits";
import { resolveRect, type Px } from "./geometry";
import type { MotifNode } from "./types";

/**
 * Decorative accents, drawn straight onto a 2D context. Every colour arrives on
 * the node (`buildScene` resolves it through the kit palette), so nothing here
 * names a colour and nothing here decides one.
 *
 * `px` is the node's rect resolved to device pixels. Treat it as an ANCHOR, not
 * a clip: several drawers deliberately paint outside it, because a rule and a
 * seal are meant to sit above the stack they accent rather than on its first
 * line of type. How far each one reaches is declared in `MOTIF_OUTSET` below —
 * consult that table rather than this paragraph, which cannot stay in step with
 * seven drawers by hand. Positioning is the caller's job.
 *
 * Pure and deterministic — no `Math.random`, no `Date.now`. Two renders of the
 * same node emit the same call sequence.
 */

type MotifDrawer = (
  ctx: CanvasRenderingContext2D,
  node: MotifNode,
  px: Px,
  canvasW: number,
) => void;

/*
 * ---------------------------------------------------------------------------
 * Geometry constants.
 *
 * Every value below is a fraction of canvas WIDTH unless its name ends in `_PX`
 * or the comment says otherwise. Width-relative is what keeps a motif the same
 * physical size across 1:1, 4:5 and 9:16 — the same rule `ShapeNode.radiusPct`
 * and `TextNode.sizePct` follow in ./types.ts.
 *
 * Most of these are art-direction picks with no derivation to record. Where
 * that is the case it is said plainly rather than dressed up, following
 * `LONG_DISH_NAME_CHARS` in ../composition/chooser.ts. The ones that ARE
 * derived say what from, and every one of them is pinned by an exact-coordinate
 * test in motifs.test.ts.
 * ---------------------------------------------------------------------------
 */

/**
 * Pitch floor for both dot-field drawers, in device pixels.
 *
 * DERIVED, and load-bearing: these drawers advance their loop counter by the
 * pitch, so a pitch of 0 never terminates. `canvasW = 0` is reachable —
 * `buildScene` carries explicit `canvasW > 0 ? … : 0` guards, so a zero
 * dimension is a state the pipeline already models, and `renderScene` passes
 * `scene.width` straight through. `drawHalftone` shipped without this floor and
 * hung the tab on an unmeasured canvas; `drawGrain` always had it.
 *
 * 2px rather than 1 also caps the iteration count on a small preview canvas,
 * where a proportional pitch would go sub-pixel and the loop would run to
 * millions before finishing. See the termination block in motifs.test.ts.
 */
const MIN_PITCH_PX = 2;

/** How far above the rect the rule floats. Art direction; no derivation. */
const RULE_LIFT = 0.02;
/** Rule stroke weight, floored so it never disappears on a small canvas. */
const RULE_WEIGHT = 0.004;
const RULE_MIN_WEIGHT_PX = 2;
/** Rule length, as a fraction of the RECT width (not the canvas). A short
 * accent mark, not an underline — art direction; no derivation. */
const RULE_LENGTH_OF_RECT = 0.18;

/** Length of each bracket arm. Art direction; no derivation. */
const BRACKET_ARM = 0.05;
/** Lighter than the rule: four of them frame the content, so they read as a
 * frame rather than as marks. No derivation for the exact value. */
const BRACKET_WEIGHT = 0.003;
const BRACKET_MIN_WEIGHT_PX = 2;

/** Seal disc radius. Art direction; no derivation. */
const SEAL_RADIUS = 0.035;
/**
 * How far the seal's CENTRE sits above the rect, in radii. With the radius on
 * top of it the disc clears the rect by `SEAL_LIFT_RADII + 1` radii — see
 * `MOTIF_OUTSET.seal`. Art direction; no derivation for 1.4.
 */
const SEAL_LIFT_RADII = 1.4;

/** Notch radius. Art direction; no derivation. */
const NOTCH_RADIUS = 0.028;

/** Halftone dot pitch and radius. Art direction; no derivation. */
const HALFTONE_PITCH = 0.03;
const HALFTONE_DOT_RADIUS = 0.004;

/** Grain cell pitch and dot size. Art direction; no derivation. */
const GRAIN_PITCH = 0.006;
const GRAIN_DOT = 0.0015;
const GRAIN_MIN_DOT_PX = 1;
/**
 * One cell in `GRAIN_FILL_MODULUS` is inked, giving a ~20% fill. Measured on
 * the full-bleed field: 19.79% at 1080x1080, 19.95% at 1080x1350, 19.92% at
 * 1080x1920. The density target is the designed quantity; 5 is simply the
 * modulus that lands on it.
 */
const GRAIN_FILL_MODULUS = 5;

/**
 * Numerical Recipes' "quick and dirty" LCG (`ranqd1`): a = 1664525,
 * c = 1013904223, m = 2^32.
 */
const LCG_MULTIPLIER = 1664525;
const LCG_INCREMENT = 1013904223;
const LCG_MODULUS = 4294967296;
/**
 * The grain seed.
 *
 * ARBITRARY. Any fixed value does the job, which is reproducibility — the same
 * post must render byte-identically twice. It is emphatically NOT 1 to dodge a
 * degenerate zero seed: that hazard belongs to a MULTIPLICATIVE LCG (c = 0),
 * and this is a MIXED one. Hull-Dobell is satisfied here — c is odd so
 * gcd(c, 2^32) = 1, and a - 1 = 1664524 = 4 x 416131 is divisible by 4 — so the
 * generator has full period 2^32 and every seed, 0 included, lies on the same
 * single cycle. Seed 0 yields 1013904223, 1196435762, 3519870697, ... and a
 * grain field indistinguishable from seed 1's.
 */
const GRAIN_SEED = 1;

/** Strip dimensions. Art direction; no derivation. */
const TAPE_W = 0.16;
const TAPE_H = 0.045;
/** How far in from the rect's left edge the strip starts, in strip widths. */
const TAPE_INSET_OF_W = 0.2;
/** Tilt in RADIANS, anticlockwise. -0.14 rad = -8.02 degrees. */
const TAPE_TILT = -0.14;

const ruleWeight = (canvasW: number) =>
  Math.max(RULE_MIN_WEIGHT_PX, canvasW * RULE_WEIGHT);
const bracketWeight = (canvasW: number) =>
  Math.max(BRACKET_MIN_WEIGHT_PX, canvasW * BRACKET_WEIGHT);
const grainDot = (canvasW: number) =>
  Math.max(GRAIN_MIN_DOT_PX, canvasW * GRAIN_DOT);

/**
 * A short accent rule.
 *
 * Paints ABOVE the rect — see `MOTIF_OUTSET.rule`.
 */
const drawRule: MotifDrawer = (ctx, node, px, canvasW) => {
  const y = px.y - canvasW * RULE_LIFT;
  ctx.strokeStyle = node.color;
  ctx.lineWidth = ruleWeight(canvasW);
  ctx.beginPath();
  ctx.moveTo(px.x, y);
  ctx.lineTo(px.x + px.w * RULE_LENGTH_OF_RECT, y);
  ctx.stroke();
};

/**
 * Corner brackets framing the content bounds.
 *
 * The arms point INWARD, so the only ink outside the rect is the stroke's own
 * half-width — see `MOTIF_OUTSET.cornerBrackets`.
 */
const drawCornerBrackets: MotifDrawer = (ctx, node, px, canvasW) => {
  const arm = canvasW * BRACKET_ARM;
  ctx.strokeStyle = node.color;
  ctx.lineWidth = bracketWeight(canvasW);
  const corners: Array<[number, number, number, number]> = [
    [px.x, px.y, 1, 1],
    [px.x + px.w, px.y, -1, 1],
    [px.x, px.y + px.h, 1, -1],
    [px.x + px.w, px.y + px.h, -1, -1],
  ];
  corners.forEach(([cx, cy, sx, sy]) => {
    ctx.beginPath();
    ctx.moveTo(cx + arm * sx, cy);
    ctx.lineTo(cx, cy);
    ctx.lineTo(cx, cy + arm * sy);
    ctx.stroke();
  });
};

/**
 * A small filled roundel, top-right of the bounds.
 *
 * Paints ABOVE the rect — see `MOTIF_OUTSET.seal`, which is the largest outset
 * of any motif here.
 */
const drawSeal: MotifDrawer = (ctx, node, px, canvasW) => {
  const r = canvasW * SEAL_RADIUS;
  ctx.fillStyle = node.color;
  ctx.beginPath();
  ctx.arc(px.x + px.w - r, px.y - r * SEAL_LIFT_RADII, r, 0, Math.PI * 2);
  ctx.fill();
};

/**
 * Two notches biting into the left and right edges, ticket-style.
 *
 * The discs are centred ON the edges, so half of each is outside the rect —
 * see `MOTIF_OUTSET.ticketNotch`.
 */
const drawTicketNotch: MotifDrawer = (ctx, node, px, canvasW) => {
  const r = canvasW * NOTCH_RADIUS;
  const midY = px.y + px.h / 2;
  ctx.fillStyle = node.color;
  [px.x, px.x + px.w].forEach((cx) => {
    ctx.beginPath();
    ctx.arc(cx, midY, r, 0, Math.PI * 2);
    ctx.fill();
  });
};

/**
 * A sparse dot field. Deterministic spacing, no randomness.
 *
 * Cost scales with rect area: exactly three ops per dot (beginPath, arc, fill).
 * Worst case is the 9:16 export, where a full-bleed field is 2,040 dots and
 * 6,123 ops; 1:1 is 1,156 dots and 4:5 is 1,428.
 *
 * The pitch is floored at `MIN_PITCH_PX` — without it a zero `canvasW` makes
 * the outer loop advance by 0 and never return.
 */
const drawHalftone: MotifDrawer = (ctx, node, px, canvasW) => {
  const step = Math.max(MIN_PITCH_PX, canvasW * HALFTONE_PITCH);
  const r = canvasW * HALFTONE_DOT_RADIUS;
  ctx.fillStyle = node.color;
  for (let y = px.y; y < px.y + px.h; y += step) {
    for (let x = px.x; x < px.x + px.w; x += step) {
      ctx.beginPath();
      ctx.arc(x, y, r, 0, Math.PI * 2);
      ctx.fill();
    }
  }
};

/**
 * A grain field. Uses a linear congruential sequence rather than Math.random so
 * repeated renders of the same post are byte-identical.
 *
 * The costliest drawer. Worst case is the 9:16 export: over a full-bleed
 * 1080x1920 rect the loop runs 49,599 times at a
 * `max(2, canvasW * 0.006)` pitch and fills 9,879 of those cells (19.92%).
 * For reference 1080x1350 is 34,903 / 6,962 and 1080x1080 is 27,889 / 5,518 —
 * so budget against the tall aspect, which is ~1.8x the square.
 */
const drawGrain: MotifDrawer = (ctx, node, px, canvasW) => {
  const step = Math.max(MIN_PITCH_PX, canvasW * GRAIN_PITCH);
  const dot = grainDot(canvasW);
  ctx.fillStyle = node.color;
  let seed = GRAIN_SEED;
  for (let y = px.y; y < px.y + px.h; y += step) {
    for (let x = px.x; x < px.x + px.w; x += step) {
      seed = (seed * LCG_MULTIPLIER + LCG_INCREMENT) % LCG_MODULUS;
      if (seed % GRAIN_FILL_MODULUS !== 0) continue;
      ctx.fillRect(x, y, dot, dot);
    }
  }
};

/**
 * A rotated strip above the top-left corner.
 *
 * Paints ENTIRELY above the rect — see `MOTIF_OUTSET.tape`, whose top outset is
 * the second largest here and over 3x the rule's.
 *
 * The `save`/`restore` pair is NOT redundant with `drawMotif`'s, even though
 * that wrapper would unwind the transform too: `MOTIF_DRAWERS` is exported, so
 * a drawer must leave the context as it found it on its own account rather than
 * lean on one particular caller. motifs.test.ts calls every drawer directly,
 * without the wrapper, and asserts exactly that.
 */
const drawTape: MotifDrawer = (ctx, node, px, canvasW) => {
  const w = canvasW * TAPE_W;
  const h = canvasW * TAPE_H;
  ctx.save();
  ctx.translate(px.x + w * TAPE_INSET_OF_W, px.y - h);
  ctx.rotate(TAPE_TILT);
  ctx.fillStyle = node.color;
  ctx.fillRect(0, 0, w, h);
  ctx.restore();
};

export const MOTIF_DRAWERS: Record<MotifId, MotifDrawer> = {
  rule: drawRule,
  cornerBrackets: drawCornerBrackets,
  seal: drawSeal,
  ticketNotch: drawTicketNotch,
  halftone: drawHalftone,
  grain: drawGrain,
  tape: drawTape,
};

/** Ink extent beyond each edge of the motif's rect, in device pixels. Never negative. */
export interface MotifOutset {
  top: number;
  right: number;
  bottom: number;
  left: number;
}

/**
 * How far past its rect each motif actually puts ink, per side, in device
 * pixels.
 *
 * WHY THIS LIVES HERE. Four of the seven drawers paint outside the rect they
 * are handed, by design — the rule and the seal are *meant* to sit above the
 * stack they accent, and moving them inside would put them on the first line of
 * type. So the caller cannot fix this by clipping, and it cannot fix it in
 * `boundsFor` either: that feeds the layout solver, so shrinking it would
 * change solved font sizes on every family, and it is aspect-only, so families
 * declaring `motifs: []` would pay the same tax for nothing. Only this module
 * knows that a rule reaches `canvasW * 0.02 + lineWidth / 2` above its rect and
 * a seal `radius * 2.4`, so this is where that knowledge is written down.
 *
 * WHY IT IS WIDTH-RELATIVE. Every offset in this file scales off canvas WIDTH
 * while the platform safe area is a fraction of HEIGHT, which is why the
 * overflow grows with aspect: the same rule that clears the 1:1 safe top by a
 * hair is 23.8px outside it at 9:16.
 *
 * NO CONSUMER YET. `buildScene` does not apply this when it builds the motif
 * rect; wiring it in is a follow-up. Until then the values are enforced against
 * the drawers by motifs.test.ts and nothing else reads them — a hand-kept table
 * with no such test would be worse than useless, since the caller would trust
 * it.
 *
 * PRECONDITION. These are the outsets for a rect large enough to contain the
 * mark itself, which every composition's `boundsFor` satisfies with room to
 * spare (the narrowest is `cornerCard` at 0.6 of canvas width, the shortest
 * `legacyMinimal` at 0.18 of canvas height). Specifically, each of these
 * assumes:
 *   - `seal`:           `px.w >= 0.070 * canvasW`, else the disc clears the
 *                       rect's LEFT edge as well.
 *   - `ticketNotch`:    `px.h >= 0.056 * canvasW`, else the discs clear the top
 *                       and bottom edges too.
 *   - `tape`:           `px.w >= 0.197 * canvasW`, else the strip runs off the
 *                       right.
 *   - `cornerBrackets`: `px.w` and `px.h` both `>= 0.05 * canvasW`, else the
 *                       inward arms overshoot the opposite edge.
 * A consumer placing a motif on a rect smaller than that owes its own check.
 * motifs.test.ts asserts the matrix it measures against honours every one of
 * these, so the preconditions cannot quietly stop being true of real bounds.
 *
 * The declared values are upper bounds on real ink, computed cap-agnostically:
 * a stroke contributes `lineWidth / 2` at its ends as well as along its length,
 * even though the default `butt` cap puts no ink past the endpoint. That costs
 * `rule.left` 2.16px of slack at 1080 and buys immunity to a cap style ever
 * being set.
 */
export const MOTIF_OUTSET: Record<
  MotifId,
  (canvasW: number) => MotifOutset
> = {
  /** Lifted `RULE_LIFT` above the rect, plus half the stroke. */
  rule: (canvasW) => {
    const half = ruleWeight(canvasW) / 2;
    const lift = canvasW * RULE_LIFT;
    return {
      top: lift + half,
      right: 0,
      // Zero at every real canvas size: the stroke is far thinner than the
      // lift. Only a canvas under ~50px wide could push the bottom of the
      // stroke back below the rect, and the max() keeps that sound rather than
      // negative.
      bottom: Math.max(0, half - lift),
      left: half,
    };
  },
  /** Arms point inward; only the stroke's half-width escapes, on all four sides. */
  cornerBrackets: (canvasW) => {
    const half = bracketWeight(canvasW) / 2;
    return { top: half, right: half, bottom: half, left: half };
  },
  /**
   * Centre sits `SEAL_LIFT_RADII` radii above the rect and the disc adds one
   * more, so the top of the ink is `(1.4 + 1) * radius` clear. The disc's right
   * edge lands exactly on the rect's right edge, by construction.
   */
  seal: (canvasW) => ({
    top: canvasW * SEAL_RADIUS * (SEAL_LIFT_RADII + 1),
    right: 0,
    bottom: 0,
    left: 0,
  }),
  /** Discs centred ON the left and right edges: exactly half of each is outside. */
  ticketNotch: (canvasW) => {
    const r = canvasW * NOTCH_RADIUS;
    return { top: 0, right: r, bottom: 0, left: r };
  },
  /**
   * The first dot is centred on the rect's top-left corner, so it hangs one
   * radius over both. The last dot in each axis stops short of the far edge but
   * by less than a radius in the worst case, so the same radius bounds all four.
   */
  halftone: (canvasW) => {
    const r = canvasW * HALFTONE_DOT_RADIUS;
    return { top: r, right: r, bottom: r, left: r };
  },
  /**
   * Cells are drawn from their top-left, so the field starts flush with the
   * rect and can only overhang the far edges — by at most one dot, when the
   * last cell lands just inside.
   */
  grain: (canvasW) => {
    const dot = grainDot(canvasW);
    return { top: 0, right: dot, bottom: dot, left: 0 };
  },
  /**
   * The strip is drawn from `px.y - TAPE_H` and then rotated about that point,
   * which lifts its far corner a further `TAPE_W * sin(|TAPE_TILT|)`. Both
   * terms are width-relative: 48.6px + 24.11px = 72.71px at 1080.
   */
  tape: (canvasW) => ({
    top: canvasW * (TAPE_H + TAPE_W * Math.sin(Math.abs(TAPE_TILT))),
    right: 0,
    bottom: 0,
    left: 0,
  }),
};

export function drawMotif(
  ctx: CanvasRenderingContext2D,
  node: MotifNode,
  canvasW: number,
  canvasH: number,
): void {
  const drawer = MOTIF_DRAWERS[node.motif];
  if (!drawer) return;
  const px = resolveRect(node.rect, canvasW, canvasH);
  // Alpha is deliberately NOT applied here. `BaseNode.opacity` is applied once,
  // by the walker, for every node kind — see `paintNode` in ./renderScene.ts.
  // This drawer applying it too would square it for motifs alone (0.9 -> 0.81)
  // the moment the walker grew that envelope, which is exactly what Wave 3 did.
  // The save/restore pair stays: the drawers translate and rotate.
  ctx.save();
  drawer(ctx, node, px, canvasW);
  ctx.restore();
}
