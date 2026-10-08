import {
  layoutTextNode,
  renderScene,
  type SceneImage,
  type SceneImages,
} from "./renderScene";
import {
  logoNode,
  motifNode,
  photoNode,
  scrimNode,
  shapeNode,
  textNode,
  type Scene,
  type SceneNode,
} from "./types";

interface Call {
  op: string;
  args: unknown[];
}

interface RecordingOptions {
  /**
   * Luma the fake `getImageData` reports for a readback at (x, y). Returning a
   * uniform RGB value makes `sampleRegionLuminance`'s
   * `0.299R + 0.587G + 0.114B` weighting collapse to that value, so the number
   * here IS the luminance the walker sees.
   */
  luma?: (x: number, y: number) => number;
}

/**
 * A context that records every call and every property write in order. Jest maps
 * `canvas` to a mock, so there is no real 2D context and no golden image to
 * diff — the recorded op sequence IS the render.
 *
 * `globalAlpha` is the one property that needs to be READ as well as written,
 * so it is a backing field with a getter, a setter and a save/restore stack
 * rather than a write-only recorder like the rest. The walker multiplies into
 * it (`ctx.globalAlpha *= node.opacity`) because `BaseNode.opacity` is
 * documented as a multiplier; against a setter-only stub that multiply reads
 * `undefined` and records `NaN`, and every assertion on the recorded value
 * passes vacuously because `NaN` is simply never the number anyone asserted.
 * Mirrors the stub in ./motifs.test.ts, which keeps the same state for the same
 * reason.
 */
function recordingCtx(options: RecordingOptions = {}) {
  const calls: Call[] = [];
  let alpha = 1;
  let filter = "none";
  let globalCompositeOperation = "source-over";
  const stack: number[] = [];
  let fillStyle: unknown = "";
  const record =
    (op: string) =>
    (...args: unknown[]) => {
      calls.push({ op, args });
    };
  const setter = (op: string) => (value: unknown) => {
    calls.push({ op, args: [value] });
  };
  const ctx = {
    save: (...args: unknown[]) => {
      stack.push(alpha);
      calls.push({ op: "save", args });
    },
    restore: (...args: unknown[]) => {
      const previous = stack.pop();
      if (previous !== undefined) alpha = previous;
      // restore also resets composite mode for white-balance assertions.
      globalCompositeOperation = "source-over";
      calls.push({ op: "restore", args });
    },
    beginPath: record("beginPath"),
    closePath: record("closePath"),
    moveTo: record("moveTo"),
    lineTo: record("lineTo"),
    arc: record("arc"),
    arcTo: record("arcTo"),
    fill: record("fill"),
    stroke: record("stroke"),
    fillRect: record("fillRect"),
    translate: record("translate"),
    rotate: record("rotate"),
    clip: record("clip"),
    drawImage: record("drawImage"),
    fillText: record("fillText"),
    measureText: (text: string) => {
      calls.push({ op: "measureText", args: [text] });
      return { width: text.length * 10 };
    },
    createLinearGradient: (...args: unknown[]) => {
      calls.push({ op: "createLinearGradient", args });
      return {
        addColorStop: (...stop: unknown[]) => {
          calls.push({ op: "addColorStop", args: stop });
        },
      };
    },
    getImageData: (x: number, y: number, w: number, h: number) => {
      calls.push({ op: "getImageData", args: [x, y, w, h] });
      const value = options.luma ? options.luma(x, y) : 200;
      // One pixel is enough: sampleRegionLuminance averages over
      // `data.data.length`, so the reported region size never has to match.
      return { data: new Uint8ClampedArray(4).fill(value), width: 1, height: 1 };
    },
    set fillStyle(value: unknown) {
      fillStyle = value;
      setter("fillStyle")(value);
    },
    set strokeStyle(value: unknown) {
      setter("strokeStyle")(value);
    },
    set lineWidth(value: unknown) {
      setter("lineWidth")(value);
    },
    get globalAlpha() {
      return alpha;
    },
    set globalAlpha(value: number) {
      alpha = value;
      calls.push({ op: "globalAlpha", args: [value] });
    },
    set font(value: unknown) {
      setter("font")(value);
    },
    set textAlign(value: unknown) {
      setter("textAlign")(value);
    },
    set textBaseline(value: unknown) {
      setter("textBaseline")(value);
    },
    get filter() {
      return filter;
    },
    set filter(value: unknown) {
      filter = String(value);
      setter("filter")(value);
    },
    get globalCompositeOperation() {
      return globalCompositeOperation;
    },
    set globalCompositeOperation(value: string) {
      globalCompositeOperation = value;
      calls.push({ op: "globalCompositeOperation", args: [value] });
    },
  } as unknown as CanvasRenderingContext2D;
  return {
    ctx,
    calls,
    /** Live context state, for assertions after the walk returns. */
    state: {
      get alpha() {
        return alpha;
      },
      get depth() {
        return stack.length;
      },
      get filter() {
        return filter;
      },
      get globalCompositeOperation() {
        return globalCompositeOperation;
      },
    },
  };
}

/**
 * The ops that actually put ink on the canvas. Ordering assertions run against
 * this projection rather than the raw call list so they read as "what was
 * painted, in what order" and do not churn on bookkeeping calls.
 */
const PAINT_OPS = new Set(["fillRect", "fill", "stroke", "drawImage", "fillText"]);

const paintOps = (calls: Call[]): string[] =>
  calls.filter((call) => PAINT_OPS.has(call.op)).map((call) => call.op);

const ops = (calls: Call[]): string[] => calls.map((call) => call.op);

const only = (calls: Call[], op: string): Call[] =>
  calls.filter((call) => call.op === op);

const texts = (calls: Call[]): string[] =>
  only(calls, "fillText").map((call) => String(call.args[0]));

/** `fillText` reduced to what it actually put where: [text, x, y] per line. */
const placedText = (calls: Call[]): unknown[][] =>
  only(calls, "fillText").map((call) => call.args.slice(0, 3));

/**
 * The canvas is deliberately NOT square, and every geometry assertion below
 * resolves x/w against W and y/h against H. An axis transposition anywhere in
 * the walker therefore changes a recorded number rather than cancelling out.
 */
const W = 1080;
const H = 1350;

const scene = (over: Partial<Scene> = {}): Scene => ({
  width: W,
  height: H,
  background: "#1a6b6a",
  nodes: [],
  ...over,
});

/**
 * Structural drawable for walker tests. `source` is the identity `drawImage`
 * receives; width/height replace naturalWidth/naturalHeight so ImageBitmap
 * (worker path) and HTMLImageElement (main path) share one fixture shape.
 */
const image = (width = 800, height = 600): SceneImage => {
  const source = { naturalWidth: width, naturalHeight: height } as HTMLImageElement;
  return { source, width, height };
};

/** Empty URL→bitmap map: no photo or logo node can resolve. */
const NO_IMAGES: SceneImages = {};

const RULE = motifNode({
  motif: "rule",
  rect: { x: 0.1, y: 0.6, w: 0.8, h: 0.1 },
  color: "#ffffff",
  z: 40,
});

const BOTTOM_SCRIM = scrimNode({
  rect: { x: 0, y: 0.5, w: 1, h: 0.5 },
  z: 10,
  from: "rgba(0,0,0,0)",
  to: "rgba(0,0,0,0.8)",
  adaptive: false,
  luminanceThreshold: 155,
});

const DISH = textNode({
  key: "dishName",
  text: "Milanesa",
  rect: { x: 0.1, y: 0.6, w: 0.8, h: 0.1 },
  z: 30,
  font: "sans",
  weight: 700,
  sizePct: 0.07,
  align: "left",
  color: "#ffffff",
  maxLines: 2,
  lineHeight: 1.2,
});

describe("renderScene", () => {
  it("paints the background before any node", () => {
    const { ctx, calls } = recordingCtx();
    renderScene(ctx, scene({ nodes: [RULE] }), NO_IMAGES);
    expect(paintOps(calls)[0]).toBe("fillRect");
    expect(only(calls, "fillRect")[0].args).toEqual([0, 0, W, H]);
    expect(calls[0]).toEqual({ op: "fillStyle", args: ["#1a6b6a"] });
  });

  it("renders an empty scene without throwing", () => {
    const { ctx, calls } = recordingCtx();
    expect(() => renderScene(ctx, scene(), NO_IMAGES)).not.toThrow();
    expect(paintOps(calls)).toEqual(["fillRect"]);
  });

  it("draws text nodes through fillText", () => {
    const { ctx, calls } = recordingCtx();
    renderScene(ctx, scene({ nodes: [DISH] }), NO_IMAGES);
    expect(texts(calls)).toContain("Milanesa");
  });

  it("skips the photo node when no image was supplied", () => {
    const { ctx, calls } = recordingCtx();
    renderScene(
      ctx,
      scene({
        nodes: [
          photoNode({ url: "a.jpg", rect: { x: 0, y: 0, w: 1, h: 1 }, z: 0 }),
        ],
      }),
      NO_IMAGES,
    );
    expect(ops(calls)).not.toContain("drawImage");
  });

  it("draws the photo when an image was supplied", () => {
    const { ctx, calls } = recordingCtx();
    const photo = image();
    renderScene(
      ctx,
      scene({
        nodes: [
          photoNode({ url: "a.jpg", rect: { x: 0, y: 0, w: 1, h: 1 }, z: 0 }),
        ],
      }),
      { "a.jpg": photo },
    );
    const drawn = only(calls, "drawImage");
    expect(drawn).toHaveLength(1);
    expect(drawn[0].args[0]).toBe(photo.source);
    // Destination is the node rect resolved against the canvas.
    expect(drawn[0].args.slice(5)).toEqual([0, 0, W, H]);
  });

  /**
   * SceneImages is keyed by source URL so a scene can carry two PhotoNodes
   * that each resolve a different bitmap. The old `{ photo, logo }` pair could
   * not name both: whichever was assigned to `photo` would paint every photo
   * node, and the other bitmap was unreachable.
   */
  it("resolves each PhotoNode against its own URL-keyed bitmap", () => {
    const { ctx, calls } = recordingCtx();
    const left = image(400, 300);
    const right = image(800, 600);
    renderScene(
      ctx,
      scene({
        nodes: [
          photoNode({
            url: "left.jpg",
            rect: { x: 0, y: 0, w: 0.5, h: 1 },
            z: 0,
          }),
          photoNode({
            url: "right.jpg",
            rect: { x: 0.5, y: 0, w: 0.5, h: 1 },
            z: 1,
          }),
        ],
      }),
      { "left.jpg": left, "right.jpg": right },
    );
    const drawn = only(calls, "drawImage");
    expect(drawn).toHaveLength(2);
    expect(drawn[0].args[0]).toBe(left.source);
    expect(drawn[1].args[0]).toBe(right.source);
    // Left panel destination, then right — array order, not a single shared bitmap.
    expect(drawn[0].args.slice(5)).toEqual([0, 0, 0.5 * W, H]);
    expect(drawn[1].args.slice(5)).toEqual([0.5 * W, 0, 0.5 * W, H]);
  });

  /**
   * `PhotoNode.crop` is the operator's focal point and zoom, carried from the
   * editor through `buildScene` to here. Asserting only the destination rect
   * leaves the whole source-crop plumbing free: dropping `node.crop`, passing a
   * fixed crop, or transposing naturalWidth/naturalHeight all still fill the
   * same destination with a different — and visibly wrong — piece of the photo.
   */
  it("passes the node crop and the image's own dimensions to fitCover", () => {
    const { ctx, calls } = recordingCtx();
    const photo = image(800, 600);
    renderScene(
      ctx,
      scene({
        nodes: [
          photoNode({
            url: "a.jpg",
            rect: { x: 0.1, y: 0.2, w: 0.6, h: 0.3 },
            crop: { x: 0.7, y: 0.3, zoom: 2 },
            z: 0,
          }),
        ],
      }),
      { "a.jpg": photo },
    );
    const drawn = only(calls, "drawImage");
    expect(drawn).toHaveLength(1);
    // Source rect: 800x600 cover-fitted to a 648x405 box, zoomed 2x about
    // (0.7, 0.3). The default crop would read [0, 50, 800, 500] and a
    // naturalWidth/naturalHeight swap [270, 146, 300, 188].
    expect(drawn[0].args.slice(1, 5)).toEqual([360, 55, 400, 250]);
    // Destination: x/w against WIDTH, y/h against HEIGHT.
    expect(drawn[0].args.slice(5)).toEqual([0.1 * W, 0.2 * H, 0.6 * W, 0.3 * H]);
  });

  // Correction to the plan: asserting `indexOf("stroke") > indexOf("fillRect")`
  // is vacuous, because the background fillRect is always index 0. These pin the
  // whole paint sequence instead, so painting the motif before the scrim fails.
  it("paints nodes in the order given", () => {
    const { ctx, calls } = recordingCtx();
    renderScene(ctx, scene({ nodes: [BOTTOM_SCRIM, RULE] }), NO_IMAGES);
    expect(paintOps(calls)).toEqual(["fillRect", "fillRect", "stroke"]);
    // Provenance: the second fillRect is the scrim's own band, not the
    // background's, so the stroke is being compared against the scrim.
    expect(only(calls, "fillRect")[1].args).toEqual([0, H * 0.5, W, H * 0.5]);
  });

  /**
   * The walker's contract is that `buildScene` has already sorted by z, so it
   * paints in array order and never re-sorts. Handing it deliberately
   * z-inverted nodes is the only way that contract is visible.
   */
  it("does not re-sort: out-of-order nodes paint in array order", () => {
    const { ctx, calls } = recordingCtx();
    const highZRule = motifNode({ ...RULE, z: 90 });
    const lowZScrim = scrimNode({ ...BOTTOM_SCRIM, z: 5 });
    renderScene(ctx, scene({ nodes: [highZRule, lowZScrim] }), NO_IMAGES);
    // Array order (rule, scrim), NOT z order (scrim z=5 then rule z=90).
    expect(paintOps(calls)).toEqual(["fillRect", "stroke", "fillRect"]);
  });

  it("fills a shape node with square corners when radiusPct is omitted", () => {
    const { ctx, calls } = recordingCtx();
    renderScene(
      ctx,
      scene({
        nodes: [
          shapeNode({
            rect: { x: 0.1, y: 0.2, w: 0.5, h: 0.25 },
            fill: "#c2410c",
            z: 20,
          }),
        ],
      }),
      NO_IMAGES,
    );
    expect(paintOps(calls)).toEqual(["fillRect", "fillRect"]);
    expect(ops(calls)).not.toContain("arcTo");
    expect(only(calls, "fillRect")[1].args).toEqual([
      0.1 * W,
      0.2 * H,
      0.5 * W,
      0.25 * H,
    ]);
    expect(calls.some((c) => c.op === "fillStyle" && c.args[0] === "#c2410c")).toBe(
      true,
    );
  });

  it("rounds a shape node through roundRect when radiusPct is set", () => {
    const { ctx, calls } = recordingCtx();
    renderScene(
      ctx,
      scene({
        nodes: [
          shapeNode({
            rect: { x: 0.1, y: 0.2, w: 0.5, h: 0.25 },
            fill: "#c2410c",
            radiusPct: 0.04,
            z: 20,
          }),
        ],
      }),
      NO_IMAGES,
    );
    // Background fillRect, then a filled rounded path — no second fillRect.
    expect(paintOps(calls)).toEqual(["fillRect", "fill"]);
    expect(only(calls, "arcTo")).toHaveLength(4);
    // The path itself, not just its shape: radiusPct is a fraction of canvas
    // WIDTH (0.04 * 1080 = 43.2), so a height-relative radius reads 54 and
    // moves the first vertex.
    expect(only(calls, "moveTo")[0].args).toEqual([0.1 * W + 0.04 * W, 0.2 * H]);
    expect(only(calls, "arcTo")[0].args).toEqual([
      0.1 * W + 0.5 * W,
      0.2 * H,
      0.1 * W + 0.5 * W,
      0.2 * H + 0.25 * H,
      0.04 * W,
    ]);
  });

  it("draws a logo node through drawLogo", () => {
    const { ctx, calls } = recordingCtx();
    const logo = image(200, 200);
    renderScene(
      ctx,
      scene({
        nodes: [
          logoNode({
            url: "logo.png",
            // Every field distinct on purpose, so a transposed x/y or a size
            // taken off the wrong one changes a recorded number. `LogoRect`
            // carries no `h` since 68835bb5d — `w` is the whole geometry on
            // both axes, which is what keeps the mark square across aspects.
            rect: { x: 0.05, y: 0.8, w: 0.12 },
            z: 60,
          }),
        ],
      }),
      { "logo.png": logo },
    );
    const drawn = only(calls, "drawImage");
    expect(drawn).toHaveLength(1);
    expect(drawn[0].args[0]).toBe(logo.source);
    // Square, sized off canvas WIDTH from rect.w, positioned at the node rect
    // origin — x against WIDTH and y against HEIGHT.
    expect(drawn[0].args.slice(1)).toEqual([
      0.05 * W,
      0.8 * H,
      Math.round(0.12 * W),
      Math.round(0.12 * W),
    ]);
    expect(ops(calls)).toContain("clip");
  });

  it("skips a logo node when no logo image was supplied", () => {
    const { ctx, calls } = recordingCtx();
    renderScene(
      ctx,
      scene({
        nodes: [
          logoNode({
            url: "logo.png",
            rect: { x: 0.06, y: 0.06, w: 0.12 },
            z: 60,
          }),
        ],
      }),
      NO_IMAGES,
    );
    expect(ops(calls)).not.toContain("drawImage");
  });

  it("uppercases text when the node asks for it", () => {
    const { ctx, calls } = recordingCtx();
    renderScene(
      ctx,
      scene({ nodes: [textNode({ ...DISH, text: "milanesa", uppercase: true })] }),
      NO_IMAGES,
    );
    expect(texts(calls)).toEqual(["MILANESA"]);
  });

  it("leaves text as authored when uppercase is omitted", () => {
    const { ctx, calls } = recordingCtx();
    renderScene(
      ctx,
      scene({ nodes: [textNode({ ...DISH, text: "milanesa" })] }),
      NO_IMAGES,
    );
    expect(texts(calls)).toEqual(["milanesa"]);
  });

  it("paints the pill behind the text, not over it", () => {
    const { ctx, calls } = recordingCtx();
    renderScene(
      ctx,
      scene({
        nodes: [
          textNode({
            ...DISH,
            pill: { fill: "#ffffff", padX: 0.02, padY: 0.012 },
            color: "#1c1917",
          }),
        ],
      }),
      NO_IMAGES,
    );
    const sequence = ops(calls);
    expect(sequence.indexOf("fill")).toBeGreaterThan(-1);
    expect(sequence.indexOf("fill")).toBeLessThan(sequence.indexOf("fillText"));
    // The pill fill colour is set before the plate is filled, and the text
    // colour after it.
    const pillFill = calls.findIndex(
      (c) => c.op === "fillStyle" && c.args[0] === "#ffffff",
    );
    const textFill = calls.findIndex(
      (c) => c.op === "fillStyle" && c.args[0] === "#1c1917",
    );
    expect(pillFill).toBeLessThan(sequence.indexOf("fill"));
    expect(textFill).toBeGreaterThan(sequence.indexOf("fill"));
    expect(only(calls, "arcTo")).toHaveLength(4);
  });

  /**
   * Alignment is applied by TWO mechanisms that have to agree: `ctx.textAlign`
   * names WHICH edge of the glyph run `anchorX` is, and `anchorX` is the point
   * on the box that edge is pinned to. They are asserted together on purpose —
   * pinning only the anchor lets `textAlign` freeze at "left", and pinning only
   * `textAlign` lets the anchor collapse to `startX`, which is a centred
   * headline centred on the box's left edge with half of it off-canvas.
   */
  it.each([
    ["left" as const, DISH.rect.x * W],
    ["center" as const, DISH.rect.x * W + (DISH.rect.w * W) / 2],
    ["right" as const, DISH.rect.x * W + DISH.rect.w * W],
  ])("anchors %s-aligned text against the matching box edge", (align, anchorX) => {
    const { ctx, calls } = recordingCtx();
    renderScene(
      ctx,
      scene({ nodes: [textNode({ ...DISH, align })] }),
      NO_IMAGES,
    );
    expect({
      textAlign: only(calls, "textAlign").map((call) => call.args[0]),
      anchorX: only(calls, "fillText").map((call) => call.args[1]),
    }).toEqual({ textAlign: [align], anchorX: [anchorX] });
  });

  /**
   * A box wide enough for one line only and tall enough for four, so the wrap
   * is forced and the second line has somewhere to go. Fixture text wraps
   * deterministically under the stub's `10px per character` metric: 216px of
   * usable width fits 21 characters, so the break lands on the space at 19.
   */
  const WRAPPING = textNode({
    ...DISH,
    text: "Milanesa napolitana completa",
    rect: { x: 0.1, y: 0.2, w: 0.2, h: 0.3 },
  });

  it("advances each wrapped line by one line height", () => {
    const { ctx, calls } = recordingCtx();
    renderScene(ctx, scene({ nodes: [WRAPPING] }), NO_IMAGES);
    // lineH is sizePct * WIDTH * lineHeight; the second line sits exactly one
    // of those below the first, not at the same y and not a bare font height.
    const lineH = DISH.sizePct * W * DISH.lineHeight;
    expect(placedText(calls)).toEqual([
      ["Milanesa napolitana", 0.1 * W, 0.2 * H],
      ["completa", 0.1 * W, 0.2 * H + lineH],
    ]);
  });

  it("clamps to maxLines and ellipsizes even when the box would fit more", () => {
    const { ctx, calls } = recordingCtx();
    renderScene(
      ctx,
      // The box holds four lines; the node asks for one. Honouring the box
      // instead would paint "Milanesa napolitana" plus "completa", unelided.
      scene({ nodes: [textNode({ ...WRAPPING, maxLines: 1 })] }),
      NO_IMAGES,
    );
    expect(texts(calls)).toEqual(["Milanesa napolitana…"]);
  });

  /**
   * The pill is the only slot geometry the walker computes itself rather than
   * delegating, and `badge` — a centred band — is the slot that gets one in
   * production. Pinning the plate's own rect covers the padding axis (padX and
   * padY are BOTH fractions of WIDTH), the shrink-to-fit sizing, and the pill's
   * separate copy of the alignment branches.
   */
  it("sizes and centres the pill around the text, padding on the width axis", () => {
    const { ctx, calls } = recordingCtx();
    renderScene(
      ctx,
      scene({
        nodes: [
          textNode({
            ...DISH,
            align: "center",
            pill: { fill: "#ffffff", padX: 0.02, padY: 0.012 },
            color: "#1c1917",
          }),
        ],
      }),
      NO_IMAGES,
    );
    const padX = 0.02 * W;
    const padY = 0.012 * W;
    const lineH = DISH.sizePct * W * DISH.lineHeight;
    const boxX = DISH.rect.x * W;
    const boxW = DISH.rect.w * W;
    // Shrink-to-fit: "Milanesa" measures 8 * 10px under the stub.
    const pillW = 8 * 10 + 2 * padX;
    const pillH = lineH + 2 * padY;
    const pillX = boxX + (boxW - pillW) / 2;
    const radius = pillH / 2;
    expect(only(calls, "moveTo")[0].args).toEqual([pillX + radius, 0.6 * H]);
    expect(only(calls, "arcTo")[0].args).toEqual([
      pillX + pillW,
      0.6 * H,
      pillX + pillW,
      0.6 * H + pillH,
      radius,
    ]);
    // And the text sits inside it: centred on the box, dropped by padY.
    expect(placedText(calls)).toEqual([
      ["Milanesa", boxX + boxW / 2, 0.6 * H + padY],
    ]);
  });

  /**
   * `drawMotif` takes canvas width and height positionally, so a transposed
   * call site is a plain argument swap with nothing to catch it. The rule motif
   * derives its y from HEIGHT and its length from WIDTH, so both show up here.
   */
  it("hands the motif drawer width and height in that order", () => {
    const { ctx, calls } = recordingCtx();
    renderScene(ctx, scene({ nodes: [RULE] }), NO_IMAGES);
    const ruleY = RULE.rect.y * H - W * 0.02;
    expect(only(calls, "moveTo")[0].args).toEqual([RULE.rect.x * W, ruleY]);
    expect(only(calls, "lineTo")[0].args).toEqual([
      RULE.rect.x * W + RULE.rect.w * W * 0.18,
      ruleY,
    ]);
  });

  /**
   * `BaseNode.opacity` is a MULTIPLIER applied by the walker (`paintNode`), not
   * by `drawMotif`. Read through a stub that only had a setter that expression
   * evaluated `undefined * 0.4` and recorded `NaN`, and every comparison against
   * `NaN` is false — so the value could be asserted and still prove nothing.
   * `Number.isFinite` is asserted alongside the value for exactly that reason:
   * it is the half a setter-only stub cannot satisfy.
   */
  it("multiplies a motif's opacity into a real, finite globalAlpha", () => {
    const { ctx, calls, state } = recordingCtx();
    renderScene(
      ctx,
      scene({ nodes: [motifNode({ ...RULE, opacity: 0.4 })] }),
      NO_IMAGES,
    );
    const alphas = only(calls, "globalAlpha").map((call) => call.args[0]);
    expect(alphas).toEqual([0.4]);
    expect(alphas.every((value) => Number.isFinite(value))).toBe(true);
    // Unwound by the walker's save/restore envelope, so the next node starts
    // from an opaque context rather than inheriting this one's alpha.
    expect(state.alpha).toBe(1);
    expect(state.depth).toBe(0);
  });

  /**
   * The multiply is only safe because it is bracketed by save/restore. Two
   * motifs in one scene is the arrangement that shows it: without the restore
   * the second would paint at 0.16 and the walk would fade to nothing down the
   * node list.
   */
  it("does not compound opacity across motif nodes", () => {
    const { ctx, calls, state } = recordingCtx();
    const faded = motifNode({ ...RULE, opacity: 0.4 });
    renderScene(
      ctx,
      scene({ nodes: [faded, motifNode({ ...faded })] }),
      NO_IMAGES,
    );
    expect(only(calls, "globalAlpha").map((call) => call.args[0])).toEqual([
      0.4, 0.4,
    ]);
    expect(state.alpha).toBe(1);
  });

  it("leaves globalAlpha untouched for a motif with no opacity", () => {
    const { ctx, calls, state } = recordingCtx();
    renderScene(ctx, scene({ nodes: [RULE] }), NO_IMAGES);
    expect(only(calls, "globalAlpha")).toEqual([]);
    expect(state.alpha).toBe(1);
  });

  it("ignores an unknown node kind without throwing", () => {
    const { ctx, calls } = recordingCtx();
    const alien = { kind: "sparkle", rect: { x: 0, y: 0, w: 1, h: 1 }, z: 5 };
    expect(() =>
      renderScene(
        ctx,
        scene({ nodes: [alien as unknown as SceneNode] }),
        NO_IMAGES,
      ),
    ).not.toThrow();
    expect(paintOps(calls)).toEqual(["fillRect"]);
  });

  it("forces weight 400 for serif text, and honours the node weight for sans", () => {
    const serif = recordingCtx();
    renderScene(
      serif.ctx,
      scene({ nodes: [textNode({ ...DISH, font: "serif", weight: 700 })] }),
      NO_IMAGES,
    );
    const serifFont = String(only(serif.calls, "font")[0].args[0]);
    // DM Serif Display ships a single weight; 400 avoids synthetic bold.
    expect(serifFont).toBe(
      `400 ${Math.round(0.07 * W)}px 'DM Serif Display', Georgia, serif`,
    );

    const sans = recordingCtx();
    renderScene(
      sans.ctx,
      scene({ nodes: [textNode({ ...DISH, font: "sans", weight: 700 })] }),
      NO_IMAGES,
    );
    expect(String(only(sans.calls, "font")[0].args[0])).toBe(
      `700 ${Math.round(0.07 * W)}px 'DM Sans', sans-serif`,
    );
  });

  it("still renders one line when the box is too short for a full line", () => {
    const { ctx, calls } = recordingCtx();
    renderScene(
      ctx,
      // 0.001 * 1350 = 1.35px tall, against a 90.7px line box.
      scene({ nodes: [textNode({ ...DISH, rect: { ...DISH.rect, h: 0.001 } })] }),
      NO_IMAGES,
    );
    expect(texts(calls)).toEqual(["Milanesa"]);
  });

  it("does not sample luminance when there is no photo underneath", () => {
    const { ctx, calls } = recordingCtx();
    renderScene(
      ctx,
      scene({ nodes: [scrimNode({ ...BOTTOM_SCRIM, adaptive: true })] }),
      NO_IMAGES,
    );
    expect(ops(calls)).not.toContain("getImageData");
  });

  /** A full-bleed photo, so a scrim anywhere on the canvas sits over it. */
  const FULL_PHOTO = photoNode({
    url: "a.jpg",
    rect: { x: 0, y: 0, w: 1, h: 1 },
    z: 0,
  });

  /** The colour stops of every gradient painted, gradient by gradient. */
  const gradientStops = (calls: Call[]): unknown[][] => {
    const stops: unknown[][] = [];
    calls.forEach((call) => {
      if (call.op === "createLinearGradient") stops.push([]);
      if (call.op === "addColorStop")
        stops[stops.length - 1]?.push(call.args[1]);
    });
    return stops;
  };

  /**
   * `splitPanel` in miniature. A photo EXISTS but it is somewhere else on the
   * canvas: the panel is the top half and every band sits below it, over the
   * flat brand surface. Gating the adaptive boost on "any photo URL resolved"
   * asks the wrong question there — the sample reads the bright brand panel
   * (luma 196.8 against a 155 threshold) and stacks a second ink gradient at
   * alpha 0.65 on a surface the foreground was already picked against, taking
   * ink-900 from 1.72:1 to roughly 1.16:1. Worse than no boost at all.
   *
   * The right question is the one `imageCoversText` asks for the scrim's own
   * emission: is a photograph actually under this rect. Answered here off the
   * IR, because the photo node and the scrim node both carry their rects.
   */
  it("does not boost a scrim the photo does not sit under", () => {
    const { ctx, calls } = recordingCtx({ luma: () => 240 });
    renderScene(
      ctx,
      scene({
        nodes: [
          photoNode({ ...FULL_PHOTO, rect: { x: 0, y: 0, w: 1, h: 0.5 } }),
          scrimNode({
            ...BOTTOM_SCRIM,
            rect: { x: 0, y: 0.58, w: 1, h: 0.42 },
            adaptive: true,
          }),
        ],
      }),
      { [FULL_PHOTO.url]: image() },
    );
    // Not merely "no second gradient": no readback either. Sampling a region
    // with no photograph in it is a `getImageData` spent on the brand fill.
    expect(ops(calls)).not.toContain("getImageData");
    expect(gradientStops(calls)).toEqual([
      ["rgba(0,0,0,0)", "rgba(0,0,0,0.8)"],
    ]);
    // Background + the authored scrim, with nothing stacked on top.
    expect(only(calls, "fillRect")).toHaveLength(2);
  });

  /**
   * The other side of the same gate, so scoping it cannot quietly become
   * disabling it: where the photo IS under the band, a bright sample still
   * earns the second pass.
   */
  it("still boosts a scrim the photo does sit under", () => {
    const { ctx, calls } = recordingCtx({ luma: () => 240 });
    renderScene(
      ctx,
      scene({
        nodes: [FULL_PHOTO, scrimNode({ ...BOTTOM_SCRIM, adaptive: true })],
      }),
      { [FULL_PHOTO.url]: image() },
    );
    expect(only(calls, "getImageData")).toHaveLength(1);
    expect(gradientStops(calls)).toEqual([
      ["rgba(0,0,0,0)", "rgba(0,0,0,0.8)"],
      // 0.55 + (240 - 155) / 400 = 0.7625 → rounded to three decimals by
      // adaptiveScrimBoost so the string is stable across IEEE-754 noise.
      ["rgba(28,25,23,0)", "rgba(28,25,23,0.763)"],
    ]);
    expect(only(calls, "fillRect")).toHaveLength(3);
  });

  /**
   * A band that straddles the edge of the photo panel counts as covered, the
   * same conservative call `imageCoversText` makes: part of the type genuinely
   * does sit on unknowable pixels, and skipping the boost there would leave it
   * on whatever the photograph happens to be.
   */
  it("boosts a scrim that only partly overlaps the photo", () => {
    const { ctx, calls } = recordingCtx({ luma: () => 240 });
    renderScene(
      ctx,
      scene({
        nodes: [
          photoNode({ ...FULL_PHOTO, rect: { x: 0, y: 0, w: 1, h: 0.6 } }),
          scrimNode({
            ...BOTTOM_SCRIM,
            rect: { x: 0, y: 0.5, w: 1, h: 0.5 },
            adaptive: true,
          }),
        ],
      }),
      { [FULL_PHOTO.url]: image() },
    );
    expect(only(calls, "getImageData")).toHaveLength(1);
    expect(only(calls, "fillRect")).toHaveLength(3);
  });

  /**
   * Touching edges share a zero-area boundary and no pixel, which is the same
   * line `imageCoversText` draws. Pinned because `>= 0` instead of `> 0` is
   * the easiest way to write this predicate wrong.
   */
  it("treats a photo that ends exactly where the scrim begins as a miss", () => {
    const { ctx, calls } = recordingCtx({ luma: () => 240 });
    renderScene(
      ctx,
      scene({
        nodes: [
          photoNode({ ...FULL_PHOTO, rect: { x: 0, y: 0, w: 1, h: 0.5 } }),
          scrimNode({ ...BOTTOM_SCRIM, adaptive: true }),
        ],
      }),
      { [FULL_PHOTO.url]: image() },
    );
    expect(ops(calls)).not.toContain("getImageData");
    expect(only(calls, "fillRect")).toHaveLength(2);
  });

  /**
   * The scene can carry a photo node the caller could not load. Only a URL that
   * actually resolved contributes a painted rect; the geometric cover test
   * narrows that set, it does not replace the load gate.
   */
  it("does not boost when the photo node never painted", () => {
    const { ctx, calls } = recordingCtx({ luma: () => 240 });
    renderScene(
      ctx,
      scene({
        nodes: [FULL_PHOTO, scrimNode({ ...BOTTOM_SCRIM, adaptive: true })],
      }),
      NO_IMAGES,
    );
    expect(ops(calls)).not.toContain("getImageData");
    expect(only(calls, "fillRect")).toHaveLength(2);
  });

  /**
   * URL-keyed map precision: a *different* photo loaded under another URL must
   * not grant scrim boost under a node whose own bitmap is missing. Global
   * "any photo present" would falsely cover the unloaded node's rect.
   */
  it("does not boost under a photo node whose URL is missing from the map", () => {
    const { ctx, calls } = recordingCtx({ luma: () => 240 });
    renderScene(
      ctx,
      scene({
        nodes: [
          photoNode({
            url: "missing.jpg",
            rect: { x: 0, y: 0, w: 1, h: 1 },
            z: 0,
          }),
          scrimNode({ ...BOTTOM_SCRIM, adaptive: true }),
        ],
      }),
      // A loaded photo exists — just not for this node's URL.
      { "other.jpg": image() },
    );
    expect(ops(calls)).not.toContain("getImageData");
    expect(only(calls, "fillRect")).toHaveLength(2);
  });

  /**
   * Luminance is a property of a REGION. The sample is cached because
   * `getImageData` is the most expensive call in the walk, but the cache is
   * keyed by rect: a second scrim over a different band must read that band
   * back, not inherit the first band's answer.
   */
  it("samples each scrim region separately", () => {
    const { ctx, calls } = recordingCtx({
      // Top band dark (below threshold), bottom band bright (above it).
      luma: (_x, y) => (y === 0 ? 30 : 240),
    });
    renderScene(
      ctx,
      scene({
        nodes: [
          // Full-bleed, so both bands genuinely have a photograph under them
          // and the only thing under test is the cache key.
          FULL_PHOTO,
          scrimNode({
            ...BOTTOM_SCRIM,
            rect: { x: 0, y: 0, w: 1, h: 0.4 },
            adaptive: true,
          }),
          scrimNode({
            ...BOTTOM_SCRIM,
            rect: { x: 0, y: 0.6, w: 1, h: 0.4 },
            adaptive: true,
          }),
        ],
      }),
      { [FULL_PHOTO.url]: image() },
    );
    const reads = only(calls, "getImageData");
    expect(reads).toHaveLength(2);
    expect(reads.map((call) => call.args)).toEqual([
      [0, 0, W, Math.floor(0.4 * H)],
      [0, Math.floor(0.6 * H), W, Math.floor(0.4 * H)],
    ]);
    // Background + dark band (authored scrim only) + bright band (authored
    // scrim plus the adaptive boost pass) = 4 fills. A shared, un-keyed cache
    // would hand the dark band's 30 to the bright band and drop the boost.
    expect(only(calls, "fillRect")).toHaveLength(4);
  });

  it("samples a repeated scrim region only once", () => {
    const { ctx, calls } = recordingCtx({ luma: () => 240 });
    const band = { x: 0, y: 0.6, w: 1, h: 0.4 };
    renderScene(
      ctx,
      scene({
        nodes: [
          FULL_PHOTO,
          scrimNode({ ...BOTTOM_SCRIM, rect: band, adaptive: true }),
          scrimNode({ ...BOTTOM_SCRIM, rect: { ...band }, adaptive: true }),
        ],
      }),
      { [FULL_PHOTO.url]: image() },
    );
    expect(only(calls, "getImageData")).toHaveLength(1);
    // Both scrims still paint, boost included: 1 background + 2 * 2.
    expect(only(calls, "fillRect")).toHaveLength(5);
  });

  describe("per-node opacity", () => {
    const FADED_TEXT = textNode({
      key: "dishName",
      text: "Milanesa",
      rect: { x: 0.1, y: 0.6, w: 0.8, h: 0.1 },
      z: 30,
      font: "sans",
      weight: 700,
      sizePct: 0.07,
      align: "left",
      color: "#ffffff",
      maxLines: 2,
      lineHeight: 1.2,
      opacity: 0.4,
    });

    it("applies a text node's opacity", () => {
      const harness = recordingCtx();
      renderScene(harness.ctx, scene({ nodes: [FADED_TEXT] }), NO_IMAGES);
      expect(
        harness.calls.some((c) => c.op === "globalAlpha" && c.args[0] === 0.4),
      ).toBe(true);
    });

    it("multiplies rather than replaces, so a group alpha would survive", () => {
      const harness = recordingCtx();
      harness.ctx.globalAlpha = 0.5;
      renderScene(harness.ctx, scene({ nodes: [FADED_TEXT] }), NO_IMAGES);
      const written = harness.calls
        .filter((c) => c.op === "globalAlpha")
        .map((c) => c.args[0] as number);
      expect(written).toContainEqual(expect.closeTo(0.2, 10));
    });

    it("restores alpha between nodes, so it cannot compound", () => {
      const harness = recordingCtx();
      renderScene(
        harness.ctx,
        scene({
          nodes: [FADED_TEXT, { ...FADED_TEXT, key: "cta", text: "Order" }],
        }),
        NO_IMAGES,
      );
      const written = harness.calls
        .filter((c) => c.op === "globalAlpha")
        .map((c) => c.args[0] as number);
      expect(written).toHaveLength(2);
      written.forEach((value) => expect(value).toBeCloseTo(0.4, 10));
      expect(harness.state.alpha).toBeCloseTo(1, 10);
    });

    it("does not touch alpha when no node sets it", () => {
      const harness = recordingCtx();
      renderScene(harness.ctx, scene({ nodes: [DISH, RULE] }), NO_IMAGES);
      expect(harness.calls.some((c) => c.op === "globalAlpha")).toBe(false);
    });

    it("applies a motif's opacity exactly once", () => {
      const harness = recordingCtx();
      renderScene(
        harness.ctx,
        scene({ nodes: [{ ...RULE, opacity: 0.9 }] }),
        NO_IMAGES,
      );
      const written = harness.calls
        .filter((c) => c.op === "globalAlpha")
        .map((c) => c.args[0] as number);
      expect(written).toHaveLength(1);
      expect(written[0]).toBeCloseTo(0.9, 10);
    });
  });

  describe("per-node clip", () => {
    const CLIPPED = {
      ...DISH,
      clip: { x: 0.1, y: 0.6, w: 0.4, h: 0.05 },
    };

    it("installs a clip path before painting the node", () => {
      const { ctx, calls } = recordingCtx();
      renderScene(ctx, scene({ nodes: [CLIPPED] }), NO_IMAGES);
      const sequence = ops(calls);
      const clipAt = sequence.indexOf("clip");
      const textAt = sequence.indexOf("fillText");
      expect(clipAt).toBeGreaterThan(-1);
      expect(textAt).toBeGreaterThan(clipAt);
    });

    it("resolves the clip rect on the same axes as every other rect", () => {
      const { ctx, calls } = recordingCtx();
      renderScene(ctx, scene({ nodes: [CLIPPED] }), NO_IMAGES);
      // roundRect at radius 0 opens with moveTo(x + 0, y).
      expect(only(calls, "moveTo")[0].args).toEqual([0.1 * W, 0.6 * H]);
      // ...and its first arcTo corner is (x + w, y).
      expect(only(calls, "arcTo")[0].args.slice(0, 4)).toEqual([
        0.1 * W + 0.4 * W,
        0.6 * H,
        0.1 * W + 0.4 * W,
        0.6 * H + 0.05 * H,
      ]);
    });

    it("wraps the clip in save/restore so it cannot leak to the next node", () => {
      const { ctx, calls } = recordingCtx();
      renderScene(ctx, scene({ nodes: [CLIPPED, RULE] }), NO_IMAGES);
      const sequence = ops(calls);
      const clipAt = sequence.indexOf("clip");
      const restoreAfterClip = sequence.indexOf("restore", clipAt);
      // RULE paints with stroke (not fillRect); the point is that the next node
      // paints after the clip has been restored, not that a particular ink op.
      const motifStroke = sequence.indexOf("stroke", restoreAfterClip);
      expect(restoreAfterClip).toBeGreaterThan(clipAt);
      expect(motifStroke).toBeGreaterThan(restoreAfterClip);
    });

    it("paints a zero-height clip as nothing visible, not as everything", () => {
      const { ctx, calls } = recordingCtx();
      renderScene(
        ctx,
        scene({ nodes: [{ ...DISH, clip: { x: 0.1, y: 0.6, w: 0.4, h: 0 } }] }),
        NO_IMAGES,
      );
      // The walker still issues the draw; the empty clip path is what makes it
      // invisible. Asserting the path is what a stub can see.
      expect(only(calls, "moveTo")[0].args).toEqual([0.1 * W, 0.6 * H]);
      expect(ops(calls)).toContain("clip");
    });

    it("issues no clip ops when no node sets one", () => {
      const { ctx, calls } = recordingCtx();
      renderScene(ctx, scene({ nodes: [DISH] }), NO_IMAGES);
      expect(ops(calls)).not.toContain("clip");
      expect(ops(calls)).not.toContain("arcTo");
    });
  });

  /**
   * The Wave 3 envelope (opacity + clip) is additive, and this is the assertion
   * that it stays additive: a scene with neither field set must produce a call
   * stream byte-for-byte identical to a build that predates the envelope. The
   * fixture below is deliberately a full post — photo, scrim, text, motif, logo
   * — so any painter that gains an unconditional save/restore or an alpha write
   * shows up here rather than in a screenshot review nobody runs.
   */
  it("emits no extra ops for a scene that sets neither opacity nor clip", () => {
    const full: SceneNode[] = [
      photoNode({ url: "a.jpg", rect: { x: 0, y: 0, w: 1, h: 1 }, z: 0 }),
      BOTTOM_SCRIM,
      DISH,
      RULE,
      logoNode({
        url: "l.png",
        // LogoRect is origin + diameter — no height (see LogoNode docstring).
        rect: { x: 0.07, y: 0.07, w: 0.11 },
        z: 50,
      }),
    ];
    const { ctx, calls } = recordingCtx();
    // SceneImages is URL-keyed; the photo/logo URLs above are the map keys.
    renderScene(ctx, scene({ nodes: full }), {
      "a.jpg": image(),
      "l.png": image(64, 64),
    });

    // No node carries opacity or clip, so the walker must have installed no
    // envelope at all: every save/restore in the stream belongs to `drawLogo`
    // or a motif drawer, and nothing wrote alpha.
    expect(calls.some((c) => c.op === "globalAlpha")).toBe(false);
    expect(ops(calls).filter((op) => op === "save")).toHaveLength(
      ops(calls).filter((op) => op === "restore").length,
    );
    expect(paintOps(calls)).toEqual([
      "fillRect", // background
      "drawImage", // photo
      "fillRect", // scrim gradient
      "fillText", // dishName
      "stroke", // rule motif (line, not fillRect)
      "fill", // logo disc
      "drawImage", // logo bitmap
    ]);
  });
});

describe("photo grading", () => {
  const PHOTO_URL = "grade.jpg";
  const photoScene = (over: { filter?: string; whiteBalance?: string }) =>
    scene({
      width: 100,
      height: 100,
      background: "#ffffff",
      nodes: [
        photoNode({
          url: PHOTO_URL,
          rect: { x: 0, y: 0, w: 1, h: 1 },
          z: 0,
          ...over,
        }),
      ],
    });

  it("applies the node's filter for the drawImage and then releases it", () => {
    const { ctx, calls, state } = recordingCtx();
    const seen: string[] = [];
    const originalDraw = ctx.drawImage.bind(ctx);
    ctx.drawImage = ((...args: unknown[]) => {
      seen.push(state.filter);
      return (originalDraw as (...a: unknown[]) => void)(...args);
    }) as typeof ctx.drawImage;
    renderScene(
      ctx,
      photoScene({ filter: "saturate(1.1) brightness(1.2)" }),
      { [PHOTO_URL]: image() },
    );
    expect(seen).toEqual(["saturate(1.1) brightness(1.2)"]);
    expect(state.filter).toBe("none");
    expect(ops(calls)).toContain("drawImage");
  });

  it("leaves the filter alone when the node carries none", () => {
    const { ctx, state } = recordingCtx();
    renderScene(ctx, photoScene({}), { [PHOTO_URL]: image() });
    expect(state.filter).toBe("none");
  });

  it("multiplies the white balance over the photo rect only", () => {
    const { ctx, calls, state } = recordingCtx();
    renderScene(
      ctx,
      photoScene({ whiteBalance: "#f0f7ff" }),
      { [PHOTO_URL]: image() },
    );
    expect(
      calls.some(
        (call) => call.op === "fillStyle" && call.args[0] === "#f0f7ff",
      ),
    ).toBe(true);
    const multiplyFill = only(calls, "fillRect").find(
      (call) =>
        call.args[0] === 0 &&
        call.args[1] === 0 &&
        call.args[2] === 100 &&
        call.args[3] === 100,
    );
    // Background also fills 0,0,100,100; white-balance is the second full rect
    // after the photo (background + multiply).
    expect(only(calls, "fillRect").length).toBeGreaterThanOrEqual(2);
    expect(multiplyFill).toBeDefined();
    expect(ops(calls)).toContain("clip");
    expect(
      calls.some(
        (call) =>
          call.op === "globalCompositeOperation" &&
          call.args[0] === "multiply",
      ),
    ).toBe(true);
    // The composite mode is restored, or every later node would multiply too.
    expect(state.globalCompositeOperation).toBe("source-over");
  });

  it("paints no multiply when the photo needs no balancing", () => {
    const { ctx, calls, state } = recordingCtx();
    renderScene(ctx, photoScene({}), { [PHOTO_URL]: image() });
    expect(
      calls.some(
        (call) => call.op === "fillStyle" && call.args[0] === "#f0f7ff",
      ),
    ).toBe(false);
    expect(state.globalCompositeOperation).toBe("source-over");
  });

  // A context without `filter` (an older embedded browser, or a stub) must not
  // throw — it renders ungraded, which is the same degradation as no analysis.
  it("renders without a filter-capable context", () => {
    const { ctx, calls } = recordingCtx();
    delete (ctx as unknown as Record<string, unknown>).filter;
    expect(() =>
      renderScene(
        ctx,
        photoScene({ filter: "saturate(1.1)" }),
        { [PHOTO_URL]: image() },
      ),
    ).not.toThrow();
    expect(ops(calls)).toContain("drawImage");
  });
});

describe("pre-resolved scrims", () => {
  const PHOTO_URL = "scrim-photo.jpg";
  const FULL = photoNode({
    url: PHOTO_URL,
    rect: { x: 0, y: 0, w: 1, h: 1 },
    z: 0,
  });
  const scrimScene = (over: {
    adaptive: boolean;
    boost?: { from: string; to: string };
  }) =>
    scene({
      width: 100,
      height: 100,
      background: "#ffffff",
      nodes: [
        FULL,
        scrimNode({
          z: 10,
          rect: { x: 0, y: 0.5, w: 1, h: 0.5 },
          from: "rgba(28,25,23,0)",
          to: "rgba(28,25,23,0.78)",
          luminanceThreshold: 155,
          ...over,
        }),
      ],
    });

  const gradientStops = (calls: Call[]): unknown[][] => {
    const stops: unknown[][] = [];
    calls.forEach((call) => {
      if (call.op === "createLinearGradient") stops.push([]);
      if (call.op === "addColorStop")
        stops[stops.length - 1]?.push(call.args[1]);
    });
    return stops;
  };

  it("paints the boost without reading the canvas back", () => {
    const { ctx, calls } = recordingCtx();
    renderScene(
      ctx,
      scrimScene({
        adaptive: false,
        boost: { from: "rgba(28,25,23,0)", to: "rgba(28,25,23,0.7)" },
      }),
      { [PHOTO_URL]: image() },
    );
    expect(ops(calls)).not.toContain("getImageData");
    expect(gradientStops(calls)).toEqual([
      ["rgba(28,25,23,0)", "rgba(28,25,23,0.78)"],
      ["rgba(28,25,23,0)", "rgba(28,25,23,0.7)"],
    ]);
  });

  it("paints one gradient when the band was measured and needs no boost", () => {
    const { ctx, calls } = recordingCtx();
    renderScene(
      ctx,
      scrimScene({ adaptive: false }),
      { [PHOTO_URL]: image() },
    );
    expect(ops(calls)).not.toContain("getImageData");
    expect(gradientStops(calls)).toHaveLength(1);
  });

  it("still reads back when nothing decided upstream", () => {
    const { ctx, calls } = recordingCtx();
    renderScene(
      ctx,
      scrimScene({ adaptive: true }),
      { [PHOTO_URL]: image() },
    );
    expect(ops(calls)).toContain("getImageData");
  });
});

/**
 * The layout half of the text drawer, split out so a walker that cannot paint
 * can still ask where the glyphs go: Wave 3's motion walker evaluates a scene
 * at time `t` without a context, and Wave 4's print walker needs the resolved
 * line boxes to emit HTML. Both need the measuring without the drawing.
 *
 * These assertions therefore care about two things the painted-output tests
 * above cannot see: that the function is reachable WITHOUT a context, and that
 * what it returns is the same geometry the walker goes on to paint. The last
 * test in this block pins the second directly — layout and paint agreeing is
 * the whole point of the split, and a drifted copy would still pass every
 * assertion taken on only one of them.
 */
describe("layoutTextNode", () => {
  /** Matches the recording context's `text.length * 10` metric. */
  const measure = (text: string) => text.length * 10;

  const WRAPPED = textNode({
    ...DISH,
    text: "Milanesa napolitana completa",
    rect: { x: 0.1, y: 0.2, w: 0.2, h: 0.3 },
  });

  it("measures without painting", () => {
    const { ctx, calls } = recordingCtx();
    layoutTextNode(WRAPPED, W, H, (text) => ctx.measureText(text).width);
    // Not merely "no ink": no context STATE either. A layout pass that set
    // `ctx.font` would be reading a context it is not allowed to have.
    expect([...new Set(ops(calls))]).toEqual(["measureText"]);
    expect(paintOps(calls)).toEqual([]);
  });

  it("resolves one line box per wrapped line, advanced by the line height", () => {
    const layout = layoutTextNode(WRAPPED, W, H, measure);
    const lineH = DISH.sizePct * W * DISH.lineHeight;
    expect(layout.lineHeight).toBe(lineH);
    expect(layout.lines).toEqual([
      { text: "Milanesa napolitana", x: 0.1 * W, y: 0.2 * H },
      { text: "completa", x: 0.1 * W, y: 0.2 * H + lineH },
    ]);
  });

  /**
   * The font string is part of the layout, not of the paint: a print walker
   * emitting HTML has to set the same face and size the widths were measured
   * at, and it has no context to read it back off.
   */
  it("reports the font the measurements were taken in", () => {
    const layout = layoutTextNode(
      textNode({ ...DISH, font: "serif", weight: 700 }),
      W,
      H,
      measure,
    );
    expect(layout.font).toBe(
      `400 ${Math.round(0.07 * W)}px 'DM Serif Display', Georgia, serif`,
    );
  });

  it("resolves the pill plate, padded on the width axis", () => {
    const layout = layoutTextNode(
      textNode({
        ...DISH,
        align: "center",
        pill: { fill: "#ffffff", padX: 0.02, padY: 0.012 },
        color: "#1c1917",
      }),
      W,
      H,
      measure,
    );
    const padX = 0.02 * W;
    const padY = 0.012 * W;
    const lineH = DISH.sizePct * W * DISH.lineHeight;
    const pillW = 8 * 10 + 2 * padX;
    const pillH = lineH + 2 * padY;
    expect(layout.pill).toEqual({
      fill: "#ffffff",
      x: DISH.rect.x * W + (DISH.rect.w * W - pillW) / 2,
      y: DISH.rect.y * H,
      w: pillW,
      h: pillH,
      radius: pillH / 2,
    });
    expect(layout.lines).toEqual([
      {
        text: "Milanesa",
        x: DISH.rect.x * W + (DISH.rect.w * W) / 2,
        y: DISH.rect.y * H + padY,
      },
    ]);
  });

  it("returns no lines and no plate for empty text", () => {
    const layout = layoutTextNode(
      textNode({
        ...DISH,
        text: "   ",
        pill: { fill: "#fff", padX: 0.02, padY: 0.012 },
      }),
      W,
      H,
      measure,
    );
    expect(layout.lines).toEqual([]);
    expect(layout.pill).toBeNull();
  });

  it("carries the alignment that names which edge the anchor is", () => {
    expect(
      layoutTextNode(textNode({ ...DISH, align: "right" }), W, H, measure),
    ).toMatchObject({ align: "right" });
  });

  /**
   * The split's actual contract: what layout resolves is what paint puts down.
   * Asserted across all three alignments and with a pill, because the anchor
   * and the plate are the two places a fork would diverge first.
   */
  it.each(["left", "center", "right"] as const)(
    "resolves exactly what the walker paints, %s-aligned",
    (align) => {
      const node = textNode({
        ...WRAPPED,
        align,
        pill: { fill: "#ffffff", padX: 0.02, padY: 0.012 },
      });
      const { ctx, calls } = recordingCtx();
      renderScene(ctx, scene({ nodes: [node] }), NO_IMAGES);
      const layout = layoutTextNode(node, W, H, measure);
      expect(placedText(calls)).toEqual(
        layout.lines.map((line) => [line.text, line.x, line.y]),
      );
      expect(only(calls, "textAlign")[0].args[0]).toBe(layout.align);
      expect(only(calls, "font")[0].args[0]).toBe(layout.font);
      // The plate the walker actually drew, read back off the path roundRect
      // laid down: moveTo starts at (x + radius, y).
      const radius = Math.min(
        layout.pill!.radius,
        layout.pill!.w / 2,
        layout.pill!.h / 2,
      );
      expect(only(calls, "moveTo")[0].args).toEqual([
        layout.pill!.x + radius,
        layout.pill!.y,
      ]);
      expect(only(calls, "arcTo")[0].args).toEqual([
        layout.pill!.x + layout.pill!.w,
        layout.pill!.y,
        layout.pill!.x + layout.pill!.w,
        layout.pill!.y + layout.pill!.h,
        radius,
      ]);
    },
  );
});
