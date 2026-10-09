import {
  compositeOver,
  contrastRatio,
  parseHex,
  relativeLuminance,
  unreadableBand,
} from "../artDirection/color";
import {
  KITS,
  KIT_ORDER,
  kitForLegacyTemplate,
  type CreativeKit,
  type MotifId,
} from "../artDirection/kits";
import {
  COMPOSITIONS,
  COMPOSITION_ORDER,
  LEGACY_COMPOSITION_FOR_TEMPLATE,
  type CompositionDef,
} from "../composition/compositions";
import { solveStack, type Band } from "../composition/solver";
import {
  ASPECT_DIMS,
  PLATFORM_CONTENT_BOUNDS,
  type AspectRatio,
  type NormalizedRect,
  type TemplateStyle,
} from "../templates/types";
import { FORMATS } from "../formats/formats";
import type { PhotoAnalysis } from "../photo/types";
import { buildScene, type BuildSceneInput } from "./buildScene";
import { MOTIF_OUTSET } from "./motifs";
import type {
  LogoNode,
  MotifNode,
  Scene,
  SceneNode,
  ScrimNode,
  TextNode,
} from "./types";

// Hex literals are fine here: `eslint.config.mjs` exempts test files from the
// inline-hex ban, and these have to be the same literal values the module
// paints with for the contrast assertions to mean anything.
const SURFACE_CREAM = "#faf9f6";
const INK_900 = "#1c1917";
const BRAND_PRIMARY = "#1a6b6a";

/** The px a CSS font string was built for; `measure` receives the whole string. */
const pxOf = (cssFont: string): number => {
  const match = cssFont.match(/ (\d+)px /);
  if (!match) throw new Error(`not a CSS font string: ${cssFont}`);
  return Number(match[1]);
};

/**
 * Deterministic stand-in for a canvas text metric: every glyph is half an em
 * wide. Jest maps `^canvas$` to a mock, so nothing under test may measure text
 * for real — this is the injection point that keeps the assembler pure.
 */
const measure = (text: string, cssFont: string) =>
  text.length * pxOf(cssFont) * 0.5;

const input = (over: Partial<BuildSceneInput> = {}): BuildSceneInput => ({
  kit: KITS.editorial,
  composition: COMPOSITIONS.photoBottomStack,
  format: FORMATS["4:5"],
  canvasW: 1080,
  canvasH: 1350,
  photoUrl: "https://cdn.example.com/dish.jpg",
  logoUrl: "",
  crop: { x: 0.5, y: 0.5, zoom: 1 },
  slots: {
    dishName: "Milanesa",
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

const textNodes = (scene: Scene): TextNode[] =>
  scene.nodes.filter((n): n is TextNode => n.kind === "text");

const motifNodes = (scene: Scene): MotifNode[] =>
  scene.nodes.filter((n): n is MotifNode => n.kind === "motif");

const scrimNodes = (scene: Scene): ScrimNode[] =>
  scene.nodes.filter((n): n is ScrimNode => n.kind === "scrim");

const logoNodes = (scene: Scene): LogoNode[] =>
  scene.nodes.filter((n): n is LogoNode => n.kind === "logo");

/** Throws rather than returning a sentinel, so a non-hex paint color fails loudly. */
const ratio = (foreground: string, backdrop: string): number => {
  const fg = parseHex(foreground);
  const bg = parseHex(backdrop);
  if (!fg || !bg) {
    throw new Error(`unparseable color pair: ${foreground} on ${backdrop}`);
  }
  return contrastRatio(fg, bg);
};

/**
 * A crowded fixture: five filled bands and a dish name long enough to reach
 * `splitPanel`'s three-line cap, in the 378px envelope that family declares at
 * 1:1. The stack wants 461.7px there, so it overruns by 83.7px — 22.1% — and
 * the solver has to descend. Everything in the shrink tests depends on that, so
 * each one asserts the descent happened before asserting anything else.
 *
 * THIS USED TO BE `cornerCard` AT 9:16 and it no longer descends at all. That
 * family's four gaps cost 138.24px of its 460.8px envelope while `gapPct` was a
 * fraction of canvas HEIGHT; 46ccb1298 made it a fraction of WIDTH, which is
 * 77.76px at every aspect, and handed 60.48px back. The stack now fits at full
 * size with 19.3px to spare and the fixture went quietly vacuous — which is
 * the defect that change fixed, not a regression: the headline had been painted
 * 15% under its declared size purely because gaps were measured on the wrong
 * axis.
 *
 * LONGER COPY CANNOT REVIVE IT, which is worth knowing before reaching for that
 * lever. `dishName` declares `maxLines: 3` and the fixture's 53-character name
 * already needs 2.98 lines at full size, so it is pinned at the cap: 53, 91 and
 * 113 characters all produce the identical 441.5px stack against a 460.8px
 * envelope. The fixture had to move families, not grow words.
 *
 * `splitPanel` at 1:1 is the deepest overflow left in the whole matrix, which
 * is what buys margin against this happening again — `photoTopStack` at 1:1 is
 * next at 11.5% and `cornerCard` at 1:1 manages 7.6%. Measured settling: the
 * editorial kit lands at scale 0.85 and the bold kit (type scale 1.15, used by
 * the sibling test below) at 0.75, so both descend several steps and both stay
 * well clear of the solver's 0.55 floor with all five bands kept.
 */
const CROWDED = {
  composition: COMPOSITIONS.splitPanel,
  format: FORMATS["1:1"],
  canvasW: 1080,
  canvasH: 1080,
  slots: {
    badge: "Chef's pick",
    dishName: "Milanesa napolitana con papas fritas y ensalada mixta",
    price: "$12",
    cta: "Order now",
    handle: "@casa",
  },
};

/**
 * Solve the crowded fixture's stack exactly as the assembler must: kit scale folded
 * in first, and each band carrying the face and weight the kit gives it. The
 * scale is separately overridable so a test can solve the SAME kit's type at a
 * different scale; the face and weight always come from the kit, because a
 * stack measured in a different font is not the same stack.
 */
const solveCrowded = (kit: CreativeKit, scale = kit.typePairing.scale) => {
  const bands: Band[] = CROWDED.composition
    .bandsFor(CROWDED.format)
    .map((t) => ({
      ...t,
      text: (CROWDED.slots[t.key as keyof typeof CROWDED.slots] ?? "").trim(),
      sizePct: t.sizePct * scale,
      font:
        t.key === "dishName" ? kit.typePairing.display : kit.typePairing.body,
      weight: t.key === "dishName" ? 700 : 500,
    }));
  return solveStack({
    bands,
    bounds: CROWDED.composition.boundsFor(CROWDED.format),
    canvasW: CROWDED.canvasW,
    canvasH: CROWDED.canvasH,
    anchor: CROWDED.composition.anchor,
    measure,
  });
};

const declaredCrowdedDishSizePct = (scale: number) =>
  CROWDED.composition
    .bandsFor(CROWDED.format)
    .find((b) => b.key === "dishName")!.sizePct * scale;

describe("buildScene", () => {
  it("emits a photo node carrying the url, crop and the kit's grade filter", () => {
    const scene = buildScene(input());

    const photos = scene.nodes.filter((n) => n.kind === "photo");
    expect(photos).toHaveLength(1);
    expect(photos[0]).toMatchObject({
      url: "https://cdn.example.com/dish.jpg",
      crop: { x: 0.5, y: 0.5, zoom: 1 },
      filter: KITS.editorial.gradePreset.filter,
    });
  });

  it("emits no photo or scrim for a poster, and paints the brand primary as the surface", () => {
    // Depends on real kit data: `editorial` does not prefer a light surface, so
    // the brand primary — not the cream fallback — is the literal background.
    expect(KITS.editorial.prefersLightSurface).toBe(false);

    const scene = buildScene(
      input({ composition: COMPOSITIONS.posterStack, photoUrl: "" }),
    );

    expect(scene.nodes.some((n) => n.kind === "photo")).toBe(false);
    expect(scrimNodes(scene)).toHaveLength(0);
    expect(scene.background).toBe(BRAND_PRIMARY);
  });

  it("emits one text node per filled slot and none for the empty ones", () => {
    const scene = buildScene(input());

    // `photoBottomStack` declares badge -> dishName -> price -> cta -> handle;
    // the fixture leaves `badge` unset, so it must not reach the scene at all.
    expect(
      COMPOSITIONS.photoBottomStack.bandsFor(FORMATS["4:5"]).map((b) => b.key),
    ).toEqual(["badge", "dishName", "price", "cta", "handle"]);
    expect(textNodes(scene).map((n) => n.key)).toEqual([
      "dishName",
      "price",
      "cta",
      "handle",
    ]);
    expect(textNodes(scene).every((n) => n.text.length > 0)).toBe(true);
  });

  it("emits a scrim beneath the text when a photo is present", () => {
    const scene = buildScene(input());

    const scrim = scrimNodes(scene)[0];
    expect(scrim).toBeDefined();
    expect(scrim.z).toBeLessThan(textNodes(scene)[0].z);
    expect(scrim.from).toBe(KITS.editorial.scrimStyle.from);
    expect(scrim.to).toBe(KITS.editorial.scrimStyle.to);
    expect(scrim.adaptive).toBe(KITS.editorial.scrimStyle.adaptive);
  });

  it("keeps the scrim inside the canvas when the bleed would overrun the bottom", () => {
    // The fixture matters: `photoBottomStack` at 4:5 sits low enough that the
    // bleed pushes past the bottom edge, which is the only case where clamping
    // y and h independently produces an off-canvas gradient.
    const bounds = COMPOSITIONS.photoBottomStack.boundsFor(FORMATS["4:5"]);
    expect(bounds.y + bounds.h).toBeGreaterThan(0.9);

    const scrim = scrimNodes(buildScene(input()))[0];

    expect(scrim.rect.y).toBeGreaterThanOrEqual(0);
    expect(scrim.rect.y + scrim.rect.h).toBeLessThanOrEqual(1);
    // Still bleeding past the text bounds — clamped, not collapsed.
    expect(scrim.rect.h).toBeGreaterThan(bounds.h);
  });

  it("emits only the motifs the kit and the composition agree on", () => {
    // Both halves of the intersection are real: the kit's notch and tape have
    // no home in this layout, and the composition's seal is not in the kit.
    expect(KITS.ticket.motifSet).toEqual(["ticketNotch", "rule", "tape"]);
    expect(COMPOSITIONS.badgeHero.motifs).toEqual(["rule", "seal"]);
    // This kit also carries a texture, which is not an accent and does not go
    // through the intersection at all — see the texture tests below.
    expect(KITS.ticket.texture).toBe("halftone");

    const scene = buildScene(
      input({ kit: KITS.ticket, composition: COMPOSITIONS.badgeHero }),
    );

    // Sorted, so the texture (z 15) precedes the accent (z 40).
    expect(motifNodes(scene).map((n) => n.motif)).toEqual([
      "halftone",
      "rule",
    ]);
  });

  describe("kit texture", () => {
    const textureOf = (scene: Scene) =>
      motifNodes(scene).find((n) => n.rect.w === 1 && n.rect.h === 1);

    it("paints the kit's texture full bleed, beneath the text, at low alpha", () => {
      expect(KITS.chalkboard.texture).toBe("grain");
      // Not reachable through the motif intersection: no kit lists a texture in
      // its motifSet and no composition lists one in its motifs, which is why
      // this layer was dead before it was fed from `kit.texture`.
      expect(KITS.chalkboard.motifSet).not.toContain("grain");
      expect(COMPOSITIONS.photoBottomStack.motifs).not.toContain("grain");

      const scene = buildScene(input({ kit: KITS.chalkboard }));

      const texture = textureOf(scene)!;
      expect(texture).toBeDefined();
      expect(texture.motif).toBe("grain");
      expect(texture.rect).toEqual({ x: 0, y: 0, w: 1, h: 1 });
      expect(texture.z).toBeLessThan(textNodes(scene)[0].z);
      expect(texture.z).toBeGreaterThan(scrimNodes(scene)[0].z);
      // Both pinned exactly, not bounded. The colour is what the contrast
      // maths composites with, so swapping it for white silently cuts white
      // type on `#6264ee` from 4.5883:1 to 3.7146:1; and "under 0.2" left 0.19
      // — a wash, not a texture — passing while the commit claimed 0.12.
      expect(texture.color).toBe("#c2410c");
      expect(texture.color).toBe(input().palette.accent);
      expect(texture.opacity).toBe(0.12);
    });

    it("emits nothing for a kit with no texture, or one with no drawer for it", () => {
      expect(KITS.editorial.texture).toBe("none");
      expect(textureOf(buildScene(input({ kit: KITS.editorial })))).toBeUndefined();

      // `paper` is a real texture with no MotifId and no drawer behind it, so
      // linen paints no texture rather than borrowing grain's.
      expect(KITS.linen.texture).toBe("paper");
      expect(textureOf(buildScene(input({ kit: KITS.linen })))).toBeUndefined();
    });
  });

  it("emits a logo node only when a logo url is present", () => {
    expect(logoNodes(buildScene(input()))).toHaveLength(0);
    expect(logoNodes(buildScene(input({ logoUrl: "   " })))).toHaveLength(0);

    const scene = buildScene(
      input({ logoUrl: "  https://cdn.example.com/logo.png  " }),
    );

    const logo = logoNodes(scene)[0];
    expect(logo.url).toBe("https://cdn.example.com/logo.png");
    // The mark is a disc of `w` diameter, and `w` is width-relative, so it is
    // circular at every aspect with no height to correct. This used to assert
    // that a carried `h` had been aspect-corrected; `drawLogo` never read it.
    expect(logo.rect).toEqual({
      x: COMPOSITIONS.photoBottomStack.logoAnchorFor(FORMATS["4:5"]).x,
      y: COMPOSITIONS.photoBottomStack.logoAnchorFor(FORMATS["4:5"]).y,
      w: COMPOSITIONS.photoBottomStack.logoAnchorFor(FORMATS["4:5"]).size,
    });
  });

  /*
   * The sort is only worth anything if the assembler can push a node out of
   * paint order, and until the texture layer was fed from `kit.texture` it
   * could not: photo -> scrim -> text -> motif -> logo is already ascending, so
   * deleting `sortNodes` from buildScene left every one of these tests green.
   *
   * The chalkboard fixture is what makes this bite. Its grain texture is pushed
   * with the motifs, after the text, and paints beneath them, so push order and
   * z order genuinely disagree and only the sort reconciles them.
   */
  it("returns nodes sorted by z even where they are not pushed in that order", () => {
    const scene = buildScene(
      input({
        kit: KITS.chalkboard,
        logoUrl: "https://cdn.example.com/logo.png",
      }),
    );

    const zs = scene.nodes.map((n) => n.z);
    expect(zs.length).toBeGreaterThan(3);
    expect(zs).toEqual([...zs].sort((a, b) => a - b));

    // Named explicitly so the failure says which node moved, not just that the
    // list is unsorted: the texture is pushed last-but-one and paints third.
    const texture = motifNodes(scene).find((n) => n.motif === "grain")!;
    expect(texture).toBeDefined();
    const firstText = scene.nodes.findIndex((n) => n.kind === "text");
    expect(scene.nodes.indexOf(texture)).toBeLessThan(firstText);
    expect(texture.z).toBeLessThan(scene.nodes[firstText].z);
  });

  /*
   * The whole ladder, not just the rung the texture needed. Asserting only
   * `texture < text` and `scrim < texture` leaves the accent motifs free to
   * slide anywhere above 15: dropping `Z.motif` from 40 to 20 keeps both of
   * those true and silently paints every rule, seal and bracket UNDERNEATH the
   * type it is supposed to stamp over.
   */
  it("pins the full paint order from photo to logo", () => {
    const scene = buildScene(
      input({
        kit: KITS.ticket,
        composition: COMPOSITIONS.badgeHero,
        logoUrl: "https://cdn.example.com/logo.png",
        slots: {
          badge: "Chef's pick",
          dishName: "Milanesa",
          cta: "Order now",
          handle: "@casa",
        },
      }),
    );

    const zOf = (kind: SceneNode["kind"]): number => {
      const node = scene.nodes.find((n) => n.kind === kind);
      if (!node) throw new Error(`no ${kind} node in the scene`);
      return node.z;
    };

    // The kit carries a texture AND the composition shares a motif with it, so
    // both motif layers are present and genuinely distinguishable.
    const texture = motifNodes(scene).find((n) => n.rect.w === 1)!;
    const accent = motifNodes(scene).find((n) => n !== texture)!;
    expect(texture.motif).toBe("halftone");
    expect(accent.motif).toBe("rule");

    expect(zOf("photo")).toBeLessThan(zOf("scrim"));
    expect(zOf("scrim")).toBeLessThan(texture.z);
    expect(texture.z).toBeLessThan(zOf("text"));
    // An accent is a stamp, not a wash: it belongs over the type, and the
    // texture belongs under it. They are not interchangeable motif layers.
    expect(zOf("text")).toBeLessThan(accent.z);
    expect(accent.z).toBeLessThan(zOf("logo"));
  });

  describe("type size follows the solver", () => {
    it("paints the size the solver settled on, not the size the composition declared", () => {
      const kit = KITS.editorial;
      // Isolates the shrink: this kit's type scale is 1, so any difference
      // between declared and painted size is the solver's doing alone.
      expect(kit.typePairing.scale).toBe(1);

      const solved = solveCrowded(kit);
      const declared = declaredCrowdedDishSizePct(kit.typePairing.scale);
      // Guard against a vacuous assertion: if the fixture ever stops
      // overflowing, the shrink below is not being tested at all.
      expect(solved.dishName.fontPx / CROWDED.canvasW).toBeLessThan(declared);

      const scene = buildScene(input({ kit, ...CROWDED }));
      const dish = textNodes(scene).find((n) => n.key === "dishName")!;

      expect(dish.sizePct).toBeLessThan(declared);
      expect(dish.sizePct).toBeCloseTo(solved.dishName.fontPx / CROWDED.canvasW, 12);
      // Rect and size come from the same solve, so there is one source of truth.
      expect(dish.rect).toEqual(solved.dishName.rect);
    });

    it("folds the kit type scale in before solving, so the solver measures what gets painted", () => {
      const kit = KITS.bold;
      expect(kit.typePairing.scale).not.toBe(1);

      const scaled = solveCrowded(kit);
      const unscaled = solveCrowded(kit, 1);

      const scene = buildScene(input({ kit, ...CROWDED }));
      const dish = textNodes(scene).find((n) => n.key === "dishName")!;

      expect(dish.sizePct).toBeCloseTo(scaled.dishName.fontPx / CROWDED.canvasW, 12);

      // Scaling after the solve — measuring one size and painting another —
      // yields a strictly larger size here, because the un-scaled stack does not
      // overflow as hard and so is not shrunk as far.
      const scaledAfterSolving =
        (unscaled.dishName.fontPx * kit.typePairing.scale) / CROWDED.canvasW;
      expect(scaledAfterSolving).toBeGreaterThan(dish.sizePct);
    });
  });

  describe("WCAG AA contrast", () => {
    /**
     * The full-bleed texture node, if this kit emits one. It is the layer
     * between the surface and the type (z 15 vs z 30), so it is part of every
     * backdrop measurement below — see `paintedBackdrop`.
     */
    const textureNode = (scene: Scene) =>
      motifNodes(scene).find((n) => n.rect.w === 1 && n.rect.h === 1);

    /**
     * Lay the kit texture over `field` exactly as the walker does, so the
     * backdrop under test is the bitmap the glyphs sit on rather than the
     * colour underneath it.
     *
     * Area-averaging the texture away would be the wrong model: `drawHalftone`
     * paints 8.6px discs on a 32.4px pitch at 1080px wide and `drawGrain`
     * paints a dense dot field, both wide enough to sit whole behind a glyph
     * stem, so a stroke can land entirely on a texture mark.
     */
    const overTexture = (scene: Scene, field: string): string => {
      const texture = textureNode(scene);
      if (!texture) return field;
      return compositeOver(texture.color, field, texture.opacity!);
    };

    /**
     * The backdrop the assembler ASSUMES text will sit on: the flat dark or
     * light field a kit's scrim is meant to lay down over a photo, or the brand
     * surface when there is no photo — in both cases with the kit texture
     * composited on top, because the texture is full bleed and paints over the
     * photo, the scrim and the surface alike.
     *
     * Over a photo these assertions therefore verify the assumed backdrop only.
     * Whether a scrim actually achieves that field over a particular photograph
     * is a different question — it depends on the pixels underneath — and it is
     * Wave 2's photo-analysis problem, not something this pure module can
     * answer. Nothing here should be read as proof that real posts clear AA.
     */
    const assumedBackdrop = (
      kit: CreativeKit,
      scene: Scene,
      hasPhoto: boolean,
    ): string => {
      const field = hasPhoto
        ? kit.prefersLightSurface
          ? SURFACE_CREAM
          : INK_900
        : scene.background;
      return overTexture(scene, field);
    };

    const expectAllTextAA = (scene: Scene, backdrop: string, pills = 1) => {
      const all = textNodes(scene);
      const plain = all.filter((n) => !n.pill);
      // A pill node carries its own fill, so it is excluded here on purpose —
      // and the count is asserted so the exclusion is never silently vacuous.
      // `pills` is 0 only for a composition that declares no badge band at all.
      expect(all.length - plain.length).toBe(pills);
      expect(plain.length).toBeGreaterThan(0);
      plain.forEach((node) => {
        expect(ratio(node.color, backdrop)).toBeGreaterThanOrEqual(4.5);
      });
    };

    const withBadge = { badge: "Chef's pick", dishName: "Milanesa", price: "$12", cta: "Order now", handle: "@casa" };

    it.each(KIT_ORDER)(
      "gives every non-pill text node at least 4.5:1 over the scrim field (%s)",
      (kitId) => {
        const kit = KITS[kitId];
        const scene = buildScene(input({ kit, slots: withBadge }));

        expect(scene.nodes.some((n) => n.kind === "photo")).toBe(true);
        expectAllTextAA(scene, assumedBackdrop(kit, scene, true));
      },
    );

    it.each(KIT_ORDER)(
      "gives every non-pill text node at least 4.5:1 on the poster surface (%s)",
      (kitId) => {
        const kit = KITS[kitId];
        const scene = buildScene(
          input({
            kit,
            composition: COMPOSITIONS.posterStack,
            photoUrl: "",
            slots: withBadge,
          }),
        );

        // No photo, so the backdrop is not assumed at all — it is literally the
        // surface this scene paints plus the texture it paints over it, which
        // makes this the one contrast case that is fully checkable here.
        expectAllTextAA(scene, assumedBackdrop(kit, scene, false));
      },
    );

    /*
     * Real brand primaries whose luminance falls in the band where BOTH white
     * and ink-900 miss 4.5:1 against them. Before the surface was conditioned,
     * the poster path painted every one of these with silently failing text.
     *
     * The first three are the Tailwind indigo, violet and purple 500 shades.
     * None of them can tell whether the foreground was picked against the
     * CONDITIONED surface or the raw one, because none of them straddles the
     * crossover: white and ink-900 tie at luminance 0.201082 while the
     * darken/lighten midpoint of the band is 0.201757, so only backdrops inside
     * that 0.000675-wide window get a different answer from the two. `#6366f1`
     * and `#8b5cf6` are below it (white either way) and `#a855f7` above it (ink
     * either way).
     *
     * `#0076fe` sits inside that window — 23,197 sRGB colours do — and it is
     * where picking against the unconditioned surface yields ink-900 at 3.82:1
     * on a background white clears at 4.58:1. `#0284c7` is outside the window
     * but inside the band, and it is the second colour whose texture composite
     * lands back in the band (4.5570:1 on the surface, 4.0080:1 on the
     * composite) — see the texture-composite cases below.
     */
    const DEAD_BAND_PRIMARIES = [
      "#6366f1",
      "#8b5cf6",
      "#a855f7",
      "#0076fe",
      "#0284c7",
    ];

    const poster = (primary: string, kit: CreativeKit = KITS.editorial) =>
      buildScene(
        input({
          kit,
          composition: COMPOSITIONS.posterStack,
          photoUrl: "",
          palette: {
            primary,
            secondary: "#0f3d3c",
            accent: "#c2410c",
            onAccent: "#ffffff",
          },
          slots: withBadge,
        }),
      );

    it.each(DEAD_BAND_PRIMARIES)(
      "conditions a poster surface no foreground could have read on (%s)",
      (primary) => {
        // Not vacuous: unconditioned, the best available foreground is under
        // the floor, which is the whole reason the surface has to move.
        const best = Math.max(
          ratio("#ffffff", primary),
          ratio("#1c1917", primary),
        );
        expect(best).toBeLessThan(4.5);

        const scene = poster(primary);

        expect(scene.background).not.toBe(primary);
        // No texture on this kit, so the painted backdrop IS the background.
        expect(textureNode(scene)).toBeUndefined();
        expectAllTextAA(scene, scene.background);
      },
    );

    /*
     * The texture kits, on the same dead-band surfaces.
     *
     * `Z.texture` (15) sits above the surface and below the type (30), so the
     * pixels a glyph reads against are the surface with the accent laid over it
     * at TEXTURE_OPACITY — not the surface. Conditioning the surface alone puts
     * the SURFACE just outside the band and then the texture drags the
     * composite straight back into it: `#a855f7` conditions to `#ab57fb` at
     * 4.5702:1, and `#c2410c` at 0.12 over that is `#ae54de`, where the same
     * ink-900 reads 4.2660:1.
     */
    const TEXTURE_KITS: Array<[keyof typeof KITS, string]> = [
      ["chalkboard", "grain"],
      ["ticket", "halftone"],
    ];

    it.each(
      TEXTURE_KITS.flatMap(([kitId, motif]) =>
        DEAD_BAND_PRIMARIES.map(
          (primary) => [kitId, motif, primary] as const,
        ),
      ),
    )(
      "reads the type against the texture composite, not the bare surface (%s / %s / %s)",
      (kitId, motif, primary) => {
        const kit = KITS[kitId];
        expect(kit.texture).toBe(motif);

        const scene = poster(primary, kit);

        const texture = textureNode(scene)!;
        expect(texture).toBeDefined();
        expect(texture.motif).toBe(motif);

        const composite = compositeOver(
          texture.color,
          scene.background,
          texture.opacity!,
        );
        // Not vacuous: the texture genuinely moves the backdrop, so measuring
        // against `scene.background` would be measuring a bitmap nobody paints.
        expect(composite).not.toBe(scene.background);

        // The band is the whole point: the composite has to be outside it, not
        // merely the surface underneath it.
        const band = unreadableBand(["#ffffff", "#1c1917"], 4.5);
        const composed = relativeLuminance(parseHex(composite)!);
        expect(composed <= band.low || composed >= band.high).toBe(true);

        expectAllTextAA(scene, composite);
      },
    );

    /*
     * Whether the surface is conditioned is a question about GEOMETRY, not
     * about whether a photo url is non-empty. `splitPanel` carries a photo and
     * puts the whole text stack underneath it, so its type sits on the brand
     * surface exactly as a poster's does; scoping the conditioning by
     * `photoUrl` left it picking a foreground against an ink-900 field that is
     * nowhere near the glyphs.
     */
    describe("photo coverage, not photo presence, decides the backdrop", () => {
      const paletteOf = (primary: string) => ({
        primary,
        secondary: "#0f3d3c",
        accent: "#c2410c",
        onAccent: "#ffffff",
      });

      /**
       * 4:5 on purpose. `splitPanel` is the only choosable family that carves
       * the photo out of the canvas instead of bleeding it, and its 4:5
       * envelope starts below the panel, so the two rects genuinely do not meet.
       */
      const SPLIT_ASPECT = "4:5" as const;

      it("conditions the surface when the image area never reaches the type (splitPanel)", () => {
        const image = COMPOSITIONS.splitPanel.imageAreaFor(FORMATS[SPLIT_ASPECT]);
        const text = COMPOSITIONS.splitPanel.boundsFor(FORMATS[SPLIT_ASPECT]);
        // Real composition data: the panel ends above the first band.
        expect(image.y + image.h).toBeLessThanOrEqual(text.y);

        // A near-white brand. Assuming an ink-900 scrim field here picks white,
        // and white on this surface is 1.53:1 — white type on yellow.
        const primary = "#facc15";
        expect(ratio("#ffffff", primary)).toBeLessThan(2);

        const scene = buildScene(
          input({
            composition: COMPOSITIONS.splitPanel,
            format: FORMATS[SPLIT_ASPECT],
            palette: paletteOf(primary),
            slots: withBadge,
          }),
        );

        // A photo IS painted — just not under the type.
        expect(scene.nodes.some((n) => n.kind === "photo")).toBe(true);
        expect(scene.background).toBe(primary);
        expectAllTextAA(scene, scene.background);
      });

      it("conditions a dead-band surface for the same layout", () => {
        // The other half of the same predicate: with no photo pixels under the
        // type the surface is knowable, so it is also conditionable.
        const scene = buildScene(
          input({
            composition: COMPOSITIONS.splitPanel,
            format: FORMATS[SPLIT_ASPECT],
            palette: paletteOf("#a855f7"),
            slots: withBadge,
          }),
        );

        expect(scene.background).not.toBe("#a855f7");
        expectAllTextAA(scene, scene.background);
      });

      /**
       * `legacyMinimal` is the one family whose image area PARTLY covers its
       * text stack — the photo ends 0.01 below the top of the first band on
       * 1:1 — which is the case that must stay on the photo path, because the
       * pixels under that overlap genuinely are unknowable.
       */
      const PARTIAL = {
        composition: COMPOSITIONS.legacyMinimal,
        format: FORMATS["1:1"],
        canvasW: 1080,
        canvasH: 1080,
      };

      it("keeps the photo path when the image area only partly covers the type", () => {
        const image = PARTIAL.composition.imageAreaFor(PARTIAL.format);
        const text = PARTIAL.composition.boundsFor(PARTIAL.format);
        // Partial, not total: it starts inside the stack and ends inside it.
        expect(image.y + image.h).toBeGreaterThan(text.y);
        expect(image.y + image.h).toBeLessThan(text.y + text.h);

        const scene = buildScene(
          input({ ...PARTIAL, palette: paletteOf("#a855f7"), slots: withBadge }),
        );

        // Untouched: conditioning a surface the type only partly sits on would
        // restyle the brand to fix a backdrop it cannot see anyway.
        expect(scene.background).toBe("#a855f7");
      });

      it.each(KIT_ORDER)(
        "gives every non-pill text node 4.5:1 over a partly-covering image area (%s)",
        (kitId) => {
          const kit = KITS[kitId];
          const scene = buildScene(
            input({
              ...PARTIAL,
              kit,
              palette: paletteOf("#a855f7"),
              slots: withBadge,
            }),
          );

          expect(scene.nodes.some((n) => n.kind === "photo")).toBe(true);
          // `legacyMinimal` declares dishName/price/handle and no badge band,
          // so there is no pill to exclude here.
          expectAllTextAA(scene, assumedBackdrop(kit, scene, true), 0);
        },
      );
    });

    /*
     * Conditioning is allowed to move the surface, but only just far enough.
     *
     * `conditionSurface(surface, PRIMARY_CANDIDATES, ...)` swapped for
     * MUTED_CANDIDATES is a silent, visible regression — `#a855f7` goes to
     * `#8240c1` instead of `#ab57fb`, a 33% luminance drop — and it is exactly
     * the "drag mid-tone brands halfway to black to satisfy a deliberately
     * low-contrast token" behaviour the change rejected. Nothing pinned the
     * rejection, so this bounds the move.
     *
     * The ceiling is derived, not chosen. A conditioned surface leaves from
     * inside the band and lands `CONTRAST_MARGIN` past one of its edges, so the
     * furthest it can travel is the band's own width plus that overshoot, plus
     * whatever the final integer-RGB rounding adds — 0.001843, the worst shift
     * measured over all 1,267,369 in-band colours (see CONTRAST_MARGIN in
     * color.ts). Sweeping the same colours puts the real worst case at 0.041810
     * (`#c836fd` -> `#b630e7`), inside the 0.042690 ceiling below. The muted
     * swap moves these five between 0.0620 and 0.0927.
     */
    /** CONTRAST_MARGIN (0.004) plus the worst rounding shift (0.001843). */
    const CONDITIONING_OVERSHOOT = 0.005843;

    it.each(DEAD_BAND_PRIMARIES)(
      "moves the surface no further than the band it has to leave (%s)",
      (primary) => {
        // The default kit carries no texture, so the surface is conditioned
        // against itself and the ceiling above is a closed form. With a texture
        // it is the COMPOSITE that must clear the band and the composite's
        // luminance is not an affine function of the surface's, so no closed
        // form exists there; the texture kits are covered by the composite
        // assertions above instead.
        expect(KITS.editorial.texture).toBe("none");

        const scene = poster(primary);
        const band = unreadableBand(["#ffffff", "#1c1917"], 4.5);
        const moved = Math.abs(
          relativeLuminance(parseHex(scene.background)!) -
            relativeLuminance(parseHex(primary)!),
        );

        expect(moved).toBeGreaterThan(0);
        expect(moved).toBeLessThanOrEqual(
          band.high - band.low + CONDITIONING_OVERSHOOT,
        );
      },
    );

    it("paints a brand surface outside the dead band exactly as authored", () => {
      // The brand teal clears the band on its own, so conditioning must be a
      // no-op for it — an AA fix that quietly restyles every brand is a
      // different, worse bug.
      expect(
        Math.max(ratio("#ffffff", BRAND_PRIMARY), ratio("#1c1917", BRAND_PRIMARY)),
      ).toBeGreaterThanOrEqual(4.5);

      expect(poster(BRAND_PRIMARY).background).toBe(BRAND_PRIMARY);
    });

    it("routes the badge through the accent pill instead of the text foreground", () => {
      const scene = buildScene(input({ slots: withBadge }));

      const badge = textNodes(scene).find((n) => n.key === "badge")!;
      expect(badge.pill).toEqual({
        fill: "#c2410c",
        padX: expect.any(Number),
        padY: expect.any(Number),
      });
      expect(badge.color).toBe("#ffffff");
    });
  });

  /*
   * ---------------------------------------------------------------------------
   * Shared sweep fixtures. Every assertion below runs over the whole matrix —
   * six kits x nine families x three aspects — because both properties they
   * check (where motif ink lands, where a scrim is painted) are functions of
   * kit AND composition AND aspect, and a spot check on the default fixture
   * misses the combinations that actually breach.
   * ---------------------------------------------------------------------------
   */

  const ASPECTS = Object.keys(ASPECT_DIMS) as AspectRatio[];

  const ALL_SLOTS = {
    badge: "Chef's pick",
    dishName: "Milanesa",
    price: "$12",
    cta: "Order now",
    handle: "@casa",
  };

  /** The scene every family paints at a given aspect, with a photo present. */
  const sweptScene = (
    kit: CreativeKit,
    composition: CompositionDef,
    aspect: AspectRatio,
    over: Partial<BuildSceneInput> = {},
  ) =>
    buildScene(
      input({
        kit,
        composition,
        format: FORMATS[aspect],
        canvasW: ASPECT_DIMS[aspect].w,
        canvasH: ASPECT_DIMS[aspect].h,
        slots: ALL_SLOTS,
        ...over,
      }),
    );

  /** A texture is a full-bleed surface, not a mark. See the sweep below. */
  const isFullBleed = (rect: NormalizedRect) =>
    rect.x === 0 && rect.y === 0 && rect.w === 1 && rect.h === 1;

  describe("motif ink inside the platform-safe area", () => {
    /**
     * The box a motif's INK occupies, in device pixels: its rect grown by
     * `MOTIF_OUTSET`, which is the only place that knows a rule floats above
     * the rect it is handed and a seal further still.
     *
     * The outsets are all width-relative device pixels, so the horizontal pair
     * is measured against the rect resolved on WIDTH and the vertical pair
     * against the rect resolved on HEIGHT. Dividing both by the same dimension
     * is the transposition this whole check exists to catch.
     */
    const inkBoxPx = (node: MotifNode, canvasW: number, canvasH: number) => {
      const outset = MOTIF_OUTSET[node.motif](canvasW);
      return {
        left: node.rect.x * canvasW - outset.left,
        right: (node.rect.x + node.rect.w) * canvasW + outset.right,
        top: node.rect.y * canvasH - outset.top,
        bottom: (node.rect.y + node.rect.h) * canvasH + outset.bottom,
      };
    };

    /**
     * The accent motifs a scene stamps: every motif node except the kit's
     * full-bleed texture.
     *
     * The texture is excluded on purpose and the exclusion is asserted rather
     * than assumed. `PLATFORM_CONTENT_BOUNDS` is where meaningful CONTENT must
     * live — type, marks, accents — and a texture is a surface treatment that
     * covers the canvas by definition (`FULL_BLEED` in buildScene.ts). Insetting
     * a grain field to the safe area would leave four bare margins, which is not
     * a safer render, it is a broken one.
     */
    const accents = (scene: Scene, kit: CreativeKit): MotifNode[] => {
      const all = motifNodes(scene);
      const full = all.filter((n) => isFullBleed(n.rect));
      // The kit either paints a texture or it does not, and the count says
      // which — so the filter can never silently swallow an accent.
      expect(full).toHaveLength(
        kit.texture === "grain" || kit.texture === "halftone" ? 1 : 0,
      );
      return all.filter((n) => !isFullBleed(n.rect));
    };

    it("keeps every emitted motif's ink inside the safe area, for every kit, family and aspect", () => {
      const breaches: string[] = [];
      const seen = new Set<MotifId>();

      KIT_ORDER.forEach((kitId) => {
        const kit = KITS[kitId];
        COMPOSITION_ORDER.forEach((cid) => {
          ASPECTS.forEach((aspect) => {
            const { w: canvasW, h: canvasH } = ASPECT_DIMS[aspect];
            const safe = PLATFORM_CONTENT_BOUNDS[aspect];
            const scene = sweptScene(kit, COMPOSITIONS[cid], aspect);

            accents(scene, kit).forEach((node) => {
              seen.add(node.motif);
              const ink = inkBoxPx(node, canvasW, canvasH);
              const past: Array<[string, number]> = [
                ["left", safe.x * canvasW - ink.left],
                ["right", ink.right - (safe.x + safe.w) * canvasW],
                ["top", safe.y * canvasH - ink.top],
                ["bottom", ink.bottom - (safe.y + safe.h) * canvasH],
              ];
              const out = past.filter(([, px]) => px > 1e-9);
              if (out.length) {
                breaches.push(
                  `${kitId}/${cid} @ ${aspect} ${node.motif}: ${out
                    .map(([edge, px]) => `${px.toFixed(2)}px past ${edge}`)
                    .join(", ")}`,
                );
              }
            });
          });
        });
      });

      // Asserted as one object so a run reports both halves: the breach list,
      // and the guard against it being empty because nothing was checked. The
      // sweep has to have exercised every motif any kit can stamp.
      expect({ breaches, exercised: [...seen].sort() }).toEqual({
        breaches: [],
        exercised: ["cornerBrackets", "rule", "seal", "tape", "ticketNotch"],
      });
    });

    it("leaves a motif whose ink already fits exactly where the composition put it", () => {
      // `photoBottomStack` sits at y 0.5 on a square canvas, so the rule's
      // 23.76px lift clears the safe top by a mile. Insetting every motif by its
      // outset unconditionally would pass the sweep above and move this one.
      const scene = sweptScene(
        KITS.editorial,
        COMPOSITIONS.photoBottomStack,
        "1:1",
      );

      const rule = motifNodes(scene).find((n) => n.motif === "rule")!;
      expect(rule).toBeDefined();
      expect(rule.rect).toEqual(COMPOSITIONS.photoBottomStack.boundsFor(FORMATS["1:1"]));
    });

    it("corrects a breaching motif exactly to the safe edge and no further", () => {
      // `photoTopStack` at 9:16 is the worst breach in the matrix: its envelope
      // starts on the safe top, so the whole 23.76px lift is outside it.
      const aspect = "9:16" as const;
      const { w: canvasW, h: canvasH } = ASPECT_DIMS[aspect];
      const safe = PLATFORM_CONTENT_BOUNDS[aspect];
      const bounds = COMPOSITIONS.photoTopStack.boundsFor(FORMATS[aspect]);
      const outset = MOTIF_OUTSET.rule(canvasW);
      expect(bounds.y * canvasH - outset.top).toBeLessThan(safe.y * canvasH);

      const scene = sweptScene(
        KITS.editorial,
        COMPOSITIONS.photoTopStack,
        aspect,
      );
      const rule = motifNodes(scene).find((n) => n.motif === "rule")!;

      // Flush with the safe top: a correction that overshoots would push the
      // rule down into the type it accents.
      expect(rule.rect.y * canvasH - outset.top).toBeCloseTo(
        safe.y * canvasH,
        9,
      );
      // The bottom edge is untouched, so the rect the drawer measures its
      // length against is not silently shortened by a top-edge fix.
      expect(rule.rect.y + rule.rect.h).toBeCloseTo(bounds.y + bounds.h, 12);
    });
  });

  describe("the scrim follows the photo, not the photo url", () => {
    /** The intersection test `buildScene` scopes its contrast maths by. */
    const coversText = (composition: CompositionDef, aspect: AspectRatio) => {
      const image = composition.imageAreaFor(FORMATS[aspect]);
      const text = composition.boundsFor(FORMATS[aspect]);
      return (
        Math.min(image.x + image.w, text.x + text.w) -
          Math.max(image.x, text.x) >
          0 &&
        Math.min(image.y + image.h, text.y + text.h) -
          Math.max(image.y, text.y) >
          0
      );
    };

    it("paints a scrim only where the photo actually reaches the type", () => {
      const wrong: string[] = [];
      // A Set: coverage is a property of the family and the aspect alone, so
      // the six kits each contribute the same entry.
      const clear = new Set<string>();

      KIT_ORDER.forEach((kitId) => {
        COMPOSITION_ORDER.forEach((cid) => {
          ASPECTS.forEach((aspect) => {
            const covers = coversText(COMPOSITIONS[cid], aspect);
            if (!covers) clear.add(`${cid} @ ${aspect}`);
            const scrims = scrimNodes(
              sweptScene(KITS[kitId], COMPOSITIONS[cid], aspect),
            );
            if (scrims.length !== (covers ? 1 : 0)) {
              wrong.push(
                `${kitId}/${cid} @ ${aspect}: ${scrims.length} scrim(s), photo ${covers ? "covers" : "misses"} the type`,
              );
            }
          });
        });
      });

      // Not vacuous: there really are families whose photo misses their type.
      // `splitPanel` is the only one, at all three aspects — see the knife-edge
      // case below for the family that is nearly the second.
      expect([...clear].sort()).toEqual([
        "splitPanel @ 1:1",
        "splitPanel @ 4:5",
        "splitPanel @ 9:16",
      ]);
      expect(wrong).toEqual([]);
    });

    /**
     * `legacyMinimal` at 4:5 is decided by binary rounding, and it is pinned
     * here so it cannot flip in silence.
     *
     * In decimals its panel ends exactly where its stack begins — 0.04 + 0.68
     * against 0.72 — which is the case `imageCoversText` documents as NOT
     * covering ("touching edges do not intersect"). In IEEE-754 doubles
     * 0.04 + 0.68 is 0.7200000000000001, so the overlap is 1.1e-16 rather than
     * 0 and the family stays on the photo path by that margin.
     *
     * Which is the render we want to keep: the family exists to reproduce a
     * pre-rewrite snapshot, and its kit (`minimal`, via `kitForLegacyTemplate`)
     * lays a cream scrim over a cream surface, so the only thing that scrim
     * paints visibly is the wash over the bottom of the photo panel — which the
     * pre-rewrite MINIMAL template also painted. Widening the predicate by an
     * epsilon would be correct by its own docstring and would delete that wash,
     * so the epsilon is deliberately NOT added; this test is the guard that
     * makes the trade-off visible if those constants are ever retuned.
     */
    it("keeps the legacy minimal scrim that a zero-width overlap hangs on", () => {
      const image = COMPOSITIONS.legacyMinimal.imageAreaFor(FORMATS["4:5"]);
      const text = COMPOSITIONS.legacyMinimal.boundsFor(FORMATS["4:5"]);
      const overlap = image.y + image.h - text.y;

      expect(image.y + image.h).toBeCloseTo(text.y, 12);
      expect(overlap).toBeGreaterThan(0);
      expect(overlap).toBeLessThan(1e-12);

      expect(
        scrimNodes(sweptScene(KITS.minimal, COMPOSITIONS.legacyMinimal, "4:5")),
      ).toHaveLength(1);
    });

    it("stops laying an unreadable field under a light brand's split panel", () => {
      // The measured hazard, reproduced. `splitPanel` puts its whole stack
      // below the photo panel, so the type sits on the brand surface — and a
      // scrim emitted there darkens that surface to near-black under the last
      // band while the foreground was picked against the surface itself.
      const primary = "#facc15";
      const kit = KITS.editorial;
      expect(kit.scrimStyle.to).toBe("rgba(28,25,23,0.78)");

      const scene = buildScene(
        input({
          kit,
          composition: COMPOSITIONS.splitPanel,
          format: FORMATS["4:5"],
          palette: {
            primary,
            secondary: "#0f3d3c",
            accent: "#c2410c",
            onAccent: "#ffffff",
          },
          slots: ALL_SLOTS,
        }),
      );

      // The photo is still painted; only the scrim is scoped.
      expect(scene.nodes.some((n) => n.kind === "photo")).toBe(true);
      expect(scene.background).toBe(primary);

      // What the scrim's terminal stop would have laid down, and what the
      // foreground the assembler actually picked reads on it.
      const scrimmed = compositeOver(INK_900, primary, 0.78);
      const picked = textNodes(scene).find((n) => !n.pill)!.color;
      expect(picked).toBe(INK_900);
      expect(ratio(picked, scrimmed)).toBeLessThan(2);
      expect(ratio(picked, scene.background)).toBeGreaterThanOrEqual(4.5);

      expect(scrimNodes(scene)).toHaveLength(0);
    });
  });

  /*
   * The invariant the "texture breaks legacy byte-fidelity" reading turns on.
   *
   * `TEXTURE_MOTIF` in buildScene.ts maps only grain and halftone, the three
   * legacy families declare `motifs: []`, and `resolveArtDirection` in
   * renderPost.ts pairs a legacy composition with `kitForLegacyTemplate(...)`
   * on the branch that selects one. So no texture and no accent reaches a
   * legacy render — but only because those three kits happen to declare
   * `texture: "none"` today. That is data, not construction, which is what
   * this pins.
   */
  it("paints neither texture nor accent on any legacy pairing", () => {
    const styles: TemplateStyle[] = ["editorial", "bold", "minimal"];
    const painted: string[] = [];

    styles.forEach((style) => {
      const kit = KITS[kitForLegacyTemplate(style)];
      const composition = COMPOSITIONS[LEGACY_COMPOSITION_FOR_TEMPLATE[style]];
      // Named separately so a kit that grows a texture fails HERE, saying which
      // kit and which texture, rather than as an unexplained extra node below.
      expect(`${style}: ${kit.texture}`).toBe(`${style}: none`);
      expect(composition.motifs).toEqual([]);

      ASPECTS.forEach((aspect) => {
        const nodes = motifNodes(sweptScene(kit, composition, aspect));
        if (nodes.length) {
          painted.push(
            `${style} @ ${aspect}: ${nodes.map((n) => n.motif).join(", ")}`,
          );
        }
      });
    });

    expect(painted).toEqual([]);
  });

  /*
   * ---------------------------------------------------------------------------
   * The typeface belongs to the kit, and to nothing else.
   *
   * The pre-rewrite renderer had a second voice in this decision: a brand
   * `design_settings.font_family` of anything but "serif" forced a serif slot to
   * sans (`slotFontString`, deleted in 1c79f6b89). Wave 1 dropped that rule, and
   * this block is the deliberate decision NOT to bring it back, pinned so the
   * decision cannot be reversed by accident. `bandTypeFor` in buildScene.ts
   * carries the reasoning; the short version is that every caller defaults the
   * field to "Inter", "Inter" is not "serif", so the rule was a global serif
   * kill-switch rather than a brand preference — reinstating it would make three
   * of the six kits' display faces unreachable for every business on the
   * default.
   *
   * Two halves, because either alone is weak. The sweep proves no brand input
   * moves the face at runtime; the type assertion proves the brand input has no
   * `fontFamily` to be moved BY, so honouring one would take a type change
   * first, and that type change is what breaks `npm run typecheck` here.
   * ---------------------------------------------------------------------------
   */
  describe("the kit decides the typeface, and the brand font setting does not", () => {
    /**
     * A brand blob of the shape callers really assemble — `usePostComposer`
     * builds `{ primary, secondary, fontFamily }` straight off
     * `design_settings` — smuggled past the palette type with a cast. The cast
     * is the point: it is the only way to get the field this far, which is
     * itself the guarantee being demonstrated.
     */
    const withFontFamily = (fontFamily: string) =>
      ({
        primary: BRAND_PRIMARY,
        secondary: "#0f3d3c",
        accent: "#c2410c",
        onAccent: "#ffffff",
        fontFamily,
      }) as unknown as BuildSceneInput["palette"];

    /** Every value `business_design_validation.go` will accept, plus a blank. */
    const FONT_FAMILIES = ["Inter", "Sans", "Serif", "serif", ""];

    it("gives every band the kit's face, for every kit, family and aspect", () => {
      const wrong: string[] = [];

      KIT_ORDER.forEach((kitId) => {
        const kit = KITS[kitId];
        COMPOSITION_ORDER.forEach((cid) => {
          ASPECTS.forEach((aspect) => {
            textNodes(sweptScene(kit, COMPOSITIONS[cid], aspect)).forEach(
              (node) => {
                const want =
                  node.key === "dishName"
                    ? kit.typePairing.display
                    : kit.typePairing.body;
                if (node.font !== want) {
                  wrong.push(
                    `${kitId}/${cid} @ ${aspect} ${node.key}: ${node.font} != ${want}`,
                  );
                }
              },
            );
          });
        });
      });

      // Not vacuous: the matrix has to have painted both faces for the
      // assertion above to have discriminated anything.
      const faces = new Set(
        KIT_ORDER.flatMap((kitId) =>
          textNodes(
            sweptScene(KITS[kitId], COMPOSITIONS.photoBottomStack, "1:1"),
          ).map((n) => n.font),
        ),
      );
      expect([...faces].sort()).toEqual(["sans", "serif"]);

      expect(wrong).toEqual([]);
    });

    it.each(FONT_FAMILIES)(
      "paints the identical faces whatever font_family the brand carries (%p)",
      (fontFamily) => {
        const facesFor = (palette?: BuildSceneInput["palette"]) =>
          KIT_ORDER.flatMap((kitId) =>
            ASPECTS.flatMap((aspect) =>
              textNodes(
                sweptScene(
                  KITS[kitId],
                  COMPOSITIONS.photoBottomStack,
                  aspect,
                  palette ? { palette } : {},
                ),
              ).map((n) => `${kitId} @ ${aspect} ${n.key}: ${n.font}`),
            ),
          );

        expect(facesFor(withFontFamily(fontFamily))).toEqual(facesFor());
      },
    );

    /**
     * A serif-display kit is where the deleted rule would have bitten hardest:
     * "Inter" — the value every caller defaults to and the value every business
     * in the measured population carries — is what used to force this headline
     * to DM Sans. Spelled out separately from the sweep above so the specific
     * regression the rule would reintroduce is named in the test source.
     */
    it("keeps a serif-display kit's headline serif under the default font_family", () => {
      const scene = sweptScene(
        KITS.editorial,
        COMPOSITIONS.photoBottomStack,
        "1:1",
        { palette: withFontFamily("Inter") },
      );

      expect(KITS.editorial.typePairing.display).toBe("serif");
      expect(textNodes(scene).find((n) => n.key === "dishName")!.font).toBe(
        "serif",
      );
    });

    /**
     * The structural half. `BuildSceneInput.palette` is a `ResolvedPalette`,
     * which declares primary / secondary / accent / onAccent and nothing else,
     * so there is no `fontFamily` for `buildScene` to read even if someone
     * wanted to — the honouring branch is unreachable rather than merely
     * unwritten.
     *
     * The annotation resolves to `never` the moment that stops being true, so
     * widening the palette to carry the field again fails `npm run typecheck`
     * on this line rather than silently re-opening the decision. Asserted at
     * runtime too so the test reports as a test rather than only as a build
     * break.
     */
    it("has no fontFamily on its brand input at all", () => {
      const brandInputCarriesNoFontFamily: "fontFamily" extends keyof BuildSceneInput["palette"]
        ? never
        : true = true;

      expect(brandInputCarriesNoFontFamily).toBe(true);
      expect(Object.keys(input().palette).sort()).toEqual([
        "accent",
        "onAccent",
        "primary",
        "secondary",
      ]);
    });

    /**
     * What the legacy path actually paints, recorded rather than assumed.
     *
     * A stored pre-rewrite `creative_snapshot` renders through
     * `kitForLegacyTemplate(template)` plus `LEGACY_COMPOSITION_FOR_TEMPLATE`,
     * so the template picks the kit and the kit picks the face. Against what
     * the pre-rewrite renderer put on the canvas for a business on the default
     * `font_family: "Inter"`:
     *
     *  - editorial: template slot said serif, `slotFontString` forced sans,
     *    canvas got DM Sans. The kit says serif, so the headline is now DM
     *    Serif Display. THIS IS THE ONE BEHAVIOURAL DELTA and it is accepted
     *    deliberately — see `bandTypeFor`.
     *  - bold: every slot sans then, every band sans now. Unchanged.
     *  - minimal: template slot said serif, the kill-switch forced sans, and
     *    the minimal kit's display face is sans. Unchanged on the default.
     *
     * A business explicitly on `font_family: "Serif"` had the kill-switch off,
     * so it saw serif for editorial (unchanged now) and serif for minimal (sans
     * now). Nothing in the measured population is on that value.
     */
    it("pins the face each legacy template now renders through its kit", () => {
      const faces: Record<string, string> = {};

      (["editorial", "bold", "minimal"] as TemplateStyle[]).forEach((style) => {
        const kit = KITS[kitForLegacyTemplate(style)];
        const scene = sweptScene(
          kit,
          COMPOSITIONS[LEGACY_COMPOSITION_FOR_TEMPLATE[style]],
          "1:1",
          { palette: withFontFamily("Inter") },
        );
        const dish = textNodes(scene).find((n) => n.key === "dishName")!;
        faces[style] = `${kit.id}/${dish.font}`;
      });

      expect(faces).toEqual({
        editorial: "editorial/serif",
        bold: "bold/sans",
        minimal: "minimal/sans",
      });
    });
  });
});

describe("photo grading in the scene", () => {
  const analysis = (channelMeans: {
    r: number;
    g: number;
    b: number;
  }): PhotoAnalysis => ({
    luma: { size: 1, values: [128] },
    edges: { size: 1, values: [0] },
    blurScore: 500,
    exposure: {
      histogram: new Array(16).fill(0),
      meanLuma: 128,
      shadowClipping: 0,
      highlightClipping: 0,
      channelMeans,
    },
    subject: { x: 0, y: 0, w: 1, h: 1 },
    focal: { x: 0.5, y: 0.5 },
    negativeSpace: "none",
    busy: false,
    dominantColors: [],
  });

  it("grades the photo node from the kit preset and the analysis", () => {
    const scene = buildScene({
      ...input(),
      analysis: analysis({ r: 132, g: 128, b: 124 }),
    });
    const photo = scene.nodes.find((node) => node.kind === "photo");
    expect(photo).toMatchObject({
      filter: "saturate(1.04) contrast(1.03) brightness(1.032)",
      whiteBalance: "#f0f7ff",
    });
  });

  it("falls back to the bare kit preset with no analysis", () => {
    const scene = buildScene({ ...input(), analysis: null });
    const photo = scene.nodes.find((node) => node.kind === "photo");
    expect(photo).toMatchObject({ filter: "saturate(1.04) contrast(1.03)" });
    expect(photo).not.toHaveProperty("whiteBalance");
  });
});
