import { FORMATS } from "../formats/formats";
import { KITS, KIT_ORDER } from "../artDirection/kits";
import { COMPOSITIONS } from "../composition/compositions";
import type { MeasureFn } from "../composition/solver";
import { buildScene, type BuildSceneInput } from "./buildScene";
import { renderScene } from "./renderScene";
import type { Scene, TextNode } from "./types";

/**
 * The solver measures text; the walker paints it. If the two use different
 * fonts, every width the solver computed is wrong for the glyphs that land on
 * the canvas — and because both historical divergences (a serif display face
 * measured as DM Sans, a 700 headline measured at 400) UNDER-measure,
 * `wrapText` fits too many characters per line and the headline overruns the
 * very bounds the solver exists to enforce. `fillText` does not clip, so that
 * overrun is visible on the exported post.
 *
 * These tests pin the property that makes the divergence impossible rather than
 * merely absent: every CSS font string the walker sets on the context must be
 * one the solver already measured with.
 *
 * Note what this file deliberately does NOT prove. Both sides run the same
 * `cssFontString`, so a bug INSIDE that builder moves both sides together and
 * parity still holds. Pinning the builder's own output is
 * `artDirection/typeface.test.ts`'s job, and pinning the walker's exact string
 * is `renderScene.test.ts`'s.
 */

/** Hex literals are fine in tests; `eslint.config.mjs` exempts them. */
const BRAND_PRIMARY = "#1a6b6a";

/** `${weight} ${px}px ${family}` — the px the string was built for. */
const pxOf = (cssFont: string): number => {
  const match = cssFont.match(/ (\d+)px /);
  if (!match) throw new Error(`not a CSS font string: ${cssFont}`);
  return Number(match[1]);
};

/** The shared stand-in metric: a 0.5em advance per glyph, on both sides. */
const widthOf = (text: string, fontPx: number) => text.length * fontPx * 0.5;

/**
 * A context that records nothing but `ctx.font` writes, and measures text the
 * same way the injected `measure` does so the walker's wrap and the solver's
 * fit agree. Everything else is a no-op: this file asserts on fonts, and
 * geometry is `renderScene.test.ts`'s subject.
 */
function fontRecordingCtx() {
  const fonts: string[] = [];
  let current = "";
  const stubs: Record<string, unknown> = {
    measureText: (text: string) => ({ width: widthOf(text, pxOf(current)) }),
    createLinearGradient: () => ({ addColorStop: () => undefined }),
    getImageData: () => ({ data: new Uint8ClampedArray(4).fill(200) }),
  };
  const ctx = new Proxy(
    {},
    {
      get: (_target, prop) => stubs[prop as string] ?? (() => undefined),
      set: (_target, prop, value) => {
        if (prop === "font") {
          current = String(value);
          fonts.push(current);
        }
        return true;
      },
    },
  ) as unknown as CanvasRenderingContext2D;
  return { ctx, fonts };
}

const textNodes = (scene: Scene): TextNode[] =>
  scene.nodes.filter((n): n is TextNode => n.kind === "text");

interface Run {
  scene: Scene;
  /** Every font string the solver measured with, at every scale it tried. */
  measured: Set<string>;
  /** Every font string the walker set on the context, in paint order. */
  painted: string[];
}

/**
 * Build a scene and paint it, capturing both sides of the contract. Every slot
 * is filled and the dish name is long enough to wrap, so the solver runs its
 * scale descent instead of settling on the first try.
 */
function run(over: Partial<BuildSceneInput> = {}): Run {
  const measured = new Set<string>();
  const measure: MeasureFn = (text, cssFont) => {
    measured.add(cssFont);
    return widthOf(text, pxOf(cssFont));
  };

  const scene = buildScene({
    kit: KITS.editorial,
    composition: COMPOSITIONS.photoBottomStack,
    format: FORMATS["4:5"],
    canvasW: 1080,
    canvasH: 1350,
    photoUrl: "https://cdn.example.com/dish.jpg",
    logoUrl: "",
    slots: {
      badge: "Chef's pick",
      dishName: "Milanesa napolitana con papas fritas y ensalada mixta",
      price: "$12",
      cta: "Order now",
      handle: "@casa",
    },
    palette: {
      primary: BRAND_PRIMARY,
      secondary: "#0f3d3c",
      accent: "#c2410c",
      onAccent: "#ffffff",
    },
    measure,
    ...over,
  });

  const { ctx, fonts } = fontRecordingCtx();
  // SceneImages is URL-keyed; an empty map means no photo/logo assets loaded.
  renderScene(ctx, scene, {});
  return { scene, measured, painted: fonts };
}

describe("measured font === painted font", () => {
  it.each(KIT_ORDER)(
    "paints no font the solver did not measure with (%s)",
    (kitId) => {
      const { scene, measured, painted } = run({ kit: KITS[kitId] });

      // Not vacuous: the fixture fills five slots, so there is a real stack to
      // measure and a real stack to paint.
      expect(textNodes(scene).length).toBeGreaterThan(1);
      expect(painted).toHaveLength(textNodes(scene).length);
      expect(measured.size).toBeGreaterThan(0);

      // Reported as the offending list rather than one at a time, so a run
      // names every band that diverged.
      expect(painted.filter((font) => !measured.has(font))).toEqual([]);
    },
  );

  /**
   * Guards the suite above against passing because every kit happens to want
   * one font. The defect this file exists for was invisible precisely while the
   * measurer had a single hard-coded font, so a parity suite that only ever saw
   * one font would have been green throughout.
   */
  it("exercises both faces and more than one weight across the kit set", () => {
    const all = KIT_ORDER.flatMap((kitId) => run({ kit: KITS[kitId] }).painted);

    expect(all.some((font) => font.includes("DM Serif Display"))).toBe(true);
    expect(all.some((font) => font.includes("DM Sans"))).toBe(true);
    expect(new Set(all.map((font) => font.split(" ")[0])).size).toBeGreaterThan(
      1,
    );
  });
});

describe("the kit decides the face and the weight, once", () => {
  it("gives the headline the kit's display face and the display weight", () => {
    const kit = KITS.editorial;
    expect(kit.typePairing.display).toBe("serif");
    expect(kit.typePairing.body).toBe("sans");

    const { scene } = run({ kit });
    const byKey = new Map(textNodes(scene).map((n) => [n.key, n]));

    expect(byKey.get("dishName")!.font).toBe("serif");
    expect(byKey.get("dishName")!.weight).toBe(700);
    expect(byKey.get("price")!.font).toBe("sans");
    expect(byKey.get("price")!.weight).toBe(500);
  });

  /**
   * The assertion above is about the node; this one is about the pixels. A
   * sans-display kit is the case where the headline weight actually reaches the
   * canvas — the serif branch forces 400 regardless — so this is what a
   * constant band weight would have to survive.
   */
  it("measures and paints a sans headline at the display weight", () => {
    const kit = KITS.bold;
    expect(kit.typePairing.display).toBe("sans");

    const { scene, measured, painted } = run({ kit });
    const dish = textNodes(scene).find((n) => n.key === "dishName")!;
    const px = Math.round(dish.sizePct * scene.width);
    const expected = `700 ${px}px 'DM Sans', sans-serif`;

    expect(painted).toContain(expected);
    expect(measured.has(expected)).toBe(true);
  });
});
