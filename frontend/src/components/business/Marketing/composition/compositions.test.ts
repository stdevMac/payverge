import {
  KITS,
  KIT_ORDER,
  kitForLegacyTemplate,
} from "../artDirection/kits";
import { BOLD, EDITORIAL, MINIMAL } from "../templates/templates";
import { FORMATS, FORMAT_ORDER } from "../formats/formats";
import {
  ASPECT_DIMS,
  PLATFORM_CONTENT_BOUNDS,
  type AspectRatio,
  type NormalizedRect,
  type SlotDef,
  type SlotFont,
  type SlotKey,
  type TemplateDef,
  type TemplateStyle,
} from "../templates/types";
import {
  CHOOSABLE_COMPOSITIONS,
  COMPOSITIONS,
  COMPOSITION_ORDER,
  LEGACY_COMPOSITION_FOR_TEMPLATE,
  deriveLogoAnchorForTests,
  type CompositionDef,
  type CompositionId,
} from "./compositions";
import { solveStack, type Band, type MeasureFn } from "./solver";

const ASPECTS = Object.keys(ASPECT_DIMS) as AspectRatio[];

describe("compositions", () => {
  it("defines a poster composition for the no-photo case", () => {
    expect(COMPOSITION_ORDER).toContain("posterStack");
    expect(COMPOSITIONS.posterStack.requiresPhoto).toBe(false);
  });

  it("every ordered composition has a definition whose id matches its key", () => {
    COMPOSITION_ORDER.forEach((id) => {
      expect(COMPOSITIONS[id]).toBeDefined();
      expect(COMPOSITIONS[id].id).toBe(id);
    });
  });

  // `CompositionId` gaining a member makes the `COMPOSITIONS` Record fail to
  // compile, but nothing forces the two parallel arrays to keep up: a fully
  // defined tenth family that is simply missing from COMPOSITION_ORDER and
  // CHOOSABLE_COMPOSITIONS typechecks clean, and is then invisible to the
  // chooser, to Reshuffle's rotation, and to every test that walks the order.
  it("keeps the parallel registries in sync with COMPOSITIONS", () => {
    expect(new Set(COMPOSITION_ORDER)).toEqual(new Set(Object.keys(COMPOSITIONS)));
    expect(COMPOSITION_ORDER).toHaveLength(new Set(COMPOSITION_ORDER).size);
    CHOOSABLE_COMPOSITIONS.forEach((id) =>
      expect(COMPOSITION_ORDER).toContain(id),
    );
  });

  it("every composition declares bands for all three aspects", () => {
    const aspects = Object.keys(ASPECT_DIMS) as Array<keyof typeof ASPECT_DIMS>;
    COMPOSITION_ORDER.forEach((id) => {
      aspects.forEach((aspect) => {
        expect(COMPOSITIONS[id].bandsFor(FORMATS[aspect]).length).toBeGreaterThan(0);
      });
    });
  });

  it("every composition keeps its content bounds inside the platform-safe area", () => {
    const aspects = Object.keys(PLATFORM_CONTENT_BOUNDS) as Array<
      keyof typeof PLATFORM_CONTENT_BOUNDS
    >;
    COMPOSITION_ORDER.forEach((id) => {
      aspects.forEach((aspect) => {
        const safe = PLATFORM_CONTENT_BOUNDS[aspect];
        const bounds = COMPOSITIONS[id].boundsFor(FORMATS[aspect]);
        expect(bounds.x).toBeGreaterThanOrEqual(safe.x - 1e-9);
        expect(bounds.y).toBeGreaterThanOrEqual(safe.y - 1e-9);
        expect(bounds.x + bounds.w).toBeLessThanOrEqual(safe.x + safe.w + 1e-9);
        expect(bounds.y + bounds.h).toBeLessThanOrEqual(safe.y + safe.h + 1e-9);
      });
    });
  });

  // The assertions above can pass vacuously: safeBounds() clamps with
  // Math.max(0, right - x), so a rect declared entirely outside the safe
  // area collapses to { w: 0, h: 0 } and still satisfies every inequality
  // above. A composition with nowhere to put text would "pass" silently.
  // This test requires real, usable room instead of a technically-legal
  // point. 0.1 normalized units is the floor: on the narrowest canvas
  // dimension (1080px, shared by all three aspects per ASPECT_DIMS) that's
  // ~108px, enough for a couple of short lines of body text at the sizes
  // `standardBands` declares (badge ~0.026, price ~0.042, cta ~0.028) —
  // below it a band stack has no legible room regardless of type-scale
  // shrinking in the solver.
  it("keeps every composition's resolved bounds comfortably non-degenerate", () => {
    const MIN_DIMENSION = 0.1;
    const aspects = Object.keys(PLATFORM_CONTENT_BOUNDS) as Array<
      keyof typeof PLATFORM_CONTENT_BOUNDS
    >;
    COMPOSITION_ORDER.forEach((id) => {
      aspects.forEach((aspect) => {
        const bounds = COMPOSITIONS[id].boundsFor(FORMATS[aspect]);
        expect(bounds.w).toBeGreaterThanOrEqual(MIN_DIMENSION);
        expect(bounds.h).toBeGreaterThanOrEqual(MIN_DIMENSION);
      });
    });
  });

  it("maps each legacy template to a defined legacy composition", () => {
    const ids: CompositionId[] = [
      LEGACY_COMPOSITION_FOR_TEMPLATE.editorial,
      LEGACY_COMPOSITION_FOR_TEMPLATE.bold,
      LEGACY_COMPOSITION_FOR_TEMPLATE.minimal,
    ];
    ids.forEach((id) => expect(COMPOSITIONS[id]).toBeDefined());
    // Each pre-rewrite template must reach *its own* legacy family: a mapping
    // that merely points at some defined composition still renders every old
    // snapshot in the wrong layout.
    expect(LEGACY_COMPOSITION_FOR_TEMPLATE).toEqual({
      editorial: "legacyEditorial",
      bold: "legacyBold",
      minimal: "legacyMinimal",
    });
  });

  /**
   * `buildScene` stamps the INTERSECTION of `kit.motifSet` and
   * `composition.motifs`, so a motif that appears on only one side of that
   * intersection is dead code with a drawer behind it. Both directions are
   * checked because both have already happened:
   *
   *  - `ticketNotch` and `tape` sat in the ticket kit's `motifSet` while no
   *    composition listed either, so the kit collapsed to `rule` everywhere and
   *    rendered NOTHING at all on `cornerCard`. Two fully implemented, fully
   *    tested drawers painted no pixel in any (kit, family, aspect) combination.
   *  - the reverse would be a family declaring a motif no kit carries, which is
   *    just as inert and reads, wrongly, as a design decision.
   *
   * Legacy families are excluded from the supply side deliberately: they
   * declare `motifs: []` because a rule or a seal the pre-rewrite template
   * never drew is new decoration on an old post. Reachability must therefore be
   * satisfied by a choosable family, never by quietly decorating a legacy one —
   * which is what the second assertion holds.
   */
  it("leaves no motif stranded on one side of the kit/composition intersection", () => {
    const declaredByKits = new Set(
      KIT_ORDER.flatMap((id) => KITS[id].motifSet),
    );
    const allowedByCompositions = new Set(
      COMPOSITION_ORDER.flatMap((id) => COMPOSITIONS[id].motifs),
    );

    expect(declaredByKits.size).toBeGreaterThan(0);
    expect({
      neverAllowed: [...declaredByKits]
        .filter((m) => !allowedByCompositions.has(m))
        .sort(),
      neverCarried: [...allowedByCompositions]
        .filter((m) => !declaredByKits.has(m))
        .sort(),
    }).toEqual({ neverAllowed: [], neverCarried: [] });

    COMPOSITION_ORDER.filter((id) => id.startsWith("legacy")).forEach((id) =>
      expect(COMPOSITIONS[id].motifs).toEqual([]),
    );
  });

  // Task 9's chooser must never surface a legacy* family — those exist only
  // so pre-rewrite creative_snapshot rows keep rendering as they did, and
  // are reached solely via LEGACY_COMPOSITION_FOR_TEMPLATE.
  it("never lets the chooser pick a legacy composition", () => {
    expect(CHOOSABLE_COMPOSITIONS.length).toBeGreaterThan(0);
    CHOOSABLE_COMPOSITIONS.forEach((id) => {
      expect(COMPOSITIONS[id]).toBeDefined();
      expect(id.startsWith("legacy")).toBe(false);
    });
  });
});

describe("composition layouts", () => {
  /**
   * The band list each family paints. Kept as data rather than derived from
   * the definitions so that *shortening* a list is a failure: clipping is the
   * wrong detector for a truncated band list, since a family cut down to one
   * band packs into its bounds more easily, not less.
   */
  const EXPECTED_BAND_KEYS: Record<CompositionId, string[]> = {
    photoBottomStack: ["badge", "dishName", "price", "cta", "handle"],
    photoTopStack: ["badge", "dishName", "price", "cta", "handle"],
    splitPanel: ["badge", "dishName", "price", "cta", "handle"],
    badgeHero: ["badge", "dishName", "cta", "handle"],
    cornerCard: ["badge", "dishName", "price", "cta", "handle"],
    posterStack: ["badge", "dishName", "price", "cta", "handle"],
    legacyEditorial: ["badge", "dishName", "price", "cta", "handle"],
    legacyBold: ["badge", "dishName", "price", "cta", "handle"],
    // Three, matching the pre-rewrite `minimal` template's three slots.
    legacyMinimal: ["dishName", "price", "handle"],
  };

  it("declares the band keys each family is documented to paint", () => {
    ASPECTS.forEach((aspect) => {
      COMPOSITION_ORDER.forEach((id) => {
        expect(COMPOSITIONS[id].bandsFor(FORMATS[aspect]).map((b) => b.key)).toEqual(
          EXPECTED_BAND_KEYS[id],
        );
      });
    });
  });

  /**
   * Realistic-but-short slot text for every slot a play can prefill. Short on
   * purpose: if the declared layout cannot pack even this without clipping or
   * shrinking to illegibility, no operator's copy will fare better.
   */
  const SAMPLE_TEXT: Record<string, string> = {
    badge: "CHEF'S PICK",
    dishName: "Grilled Octopus",
    price: "$24",
    cta: "Order tonight",
    handle: "@payverge",
  };

  const textFor = (key: string): string => {
    const text = SAMPLE_TEXT[key];
    if (!text) throw new Error(`no sample text for band key "${key}"`);
    return text;
  };

  /**
   * Deterministic stand-in for the canvas measurement the renderer injects: a
   * 0.5em average glyph advance. The solver takes `measure` as a callback
   * precisely so it can be driven headlessly; using the real solver here
   * rather than re-deriving its packing arithmetic keeps this test from
   * drifting away from solver.ts.
   */
  const measure: MeasureFn = (text, cssFont) => {
    const match = cssFont.match(/ (\d+)px /);
    if (!match) throw new Error(`not a CSS font string: ${cssFont}`);
    return text.length * Number(match[1]) * 0.5;
  };

  /**
   * The long-copy case: the longest dish name the layouts are designed to
   * typeset, at 63 characters.
   *
   * Nothing bounds this at the source. No slot input in the Marketing editor
   * carries a `maxLength` (the only one in the whole tab is 200, on an
   * unrelated settings field), and `postContent.ts` fills `dishName` from a
   * suggestion's `target_name` — a menu item name, which an operator writes as
   * long as the dish is. So the bound has to be a design budget, and this is
   * it: a preparation, an origin, the protein and two garnishes, which is about
   * as far as a menu line goes before it stops being a name. Past it the
   * design's answer is the solver's own — shrink the type scale, then clip from
   * the end of the stack — and MIN_LEGIBLE_FONT_PX is the floor that stops the
   * shrink going somewhere unreadable, which is what this test holds.
   *
   * The number is not free-chosen: both lengths at which the reviewed defect
   * reproduces sit inside it (50 characters put `splitPanel`'s handle at 15.8px
   * on 9:16, 63 did the same on 1:1), so the widened envelopes are verified
   * against known failures rather than against a guess.
   */
  const LONG_DISH_NAME =
    "Wood-Grilled Spanish Octopus, Salsa Verde and Charred Lemon Oil";

  /**
   * Every kit's type scale, read off the registry so a retuned kit reaches this
   * test instead of drifting past it.
   *
   * `buildScene` multiplies each band's `sizePct` by `kit.typePairing.scale`
   * BEFORE solving, so the scale is part of the layout budget, not a post-hoc
   * resize — and these tests used to run at 1 only, which is the one value at
   * which the six kits agree. The extremes are what matter: 0.92 shrinks the
   * type toward the legibility floor, and 1.15 inflates the stack toward
   * clipping.
   */
  const KIT_TYPE_SCALES = KIT_ORDER.map((id) => KITS[id].typePairing.scale);

  /**
   * A composition's declared bands, filled in for solving.
   *
   * The face and weight are a kit's decision, not a composition's, and this
   * file's subject is layout budgets — whether a family's declared bands fit
   * its declared bounds — so every band is measured in one face here. The
   * stand-in metric above is a function of size alone, so that choice cannot
   * change a single number below; that the SOLVER and the WALKER agree on the
   * real per-band face is `scene/fontParity.test.ts`'s subject instead.
   *
   * `typeScale` mirrors what `buildScene` folds in from the kit, at the same
   * point in the pipeline — into `sizePct`, before the solve.
   */
  const bandsWithText = (
    def: CompositionDef,
    aspect: AspectRatio,
    over: { dishName?: string; typeScale?: number } = {},
  ): Band[] =>
    def.bandsFor(FORMATS[aspect]).map((template) => ({
      ...template,
      text:
        template.key === "dishName" && over.dishName !== undefined
          ? over.dishName
          : textFor(template.key),
      sizePct: template.sizePct * (over.typeScale ?? 1),
      font: "sans",
      weight: 500,
    }));

  /**
   * Font-size floor, in device pixels on the 1080px-wide canvas every aspect
   * shares. 16px is ~1.5% of canvas width — around the point below which body
   * copy on a social post stops being readable on a phone. The solver's own
   * MIN_SCALE defends the *scale*, not the resulting size, so a family that
   * asks for type too small to survive that shrink still renders illegibly:
   * legacyMinimal's handle solved to 11.8px (1.1% of width) for exactly that
   * reason before its band list was cut to the three slots it paints.
   */
  const MIN_LEGIBLE_FONT_PX = 16;

  it("packs every family's declared bands at every aspect, kit type-scale and copy length, without clipping or illegible type", () => {
    const clipped: string[] = [];
    const illegible: string[] = [];

    [SAMPLE_TEXT.dishName, LONG_DISH_NAME].forEach((dishName) => {
      KIT_TYPE_SCALES.forEach((typeScale) => {
        ASPECTS.forEach((aspect) => {
          const { w: canvasW, h: canvasH } = ASPECT_DIMS[aspect];
          COMPOSITION_ORDER.forEach((id) => {
            const def = COMPOSITIONS[id];
            const bands = bandsWithText(def, aspect, { dishName, typeScale });
            const solved = solveStack({
              bands,
              bounds: def.boundsFor(FORMATS[aspect]),
              canvasW,
              canvasH,
              anchor: def.anchor,
              measure,
            });
            const where = `${id} @ ${aspect} x${typeScale} (${dishName.length} chars)`;

            bands.forEach((b) => {
              const out = solved[b.key];
              if (!out) {
                clipped.push(`${where}: ${b.key}`);
                return;
              }
              if (out.fontPx < MIN_LEGIBLE_FONT_PX) {
                illegible.push(
                  `${where}: ${b.key} = ${out.fontPx.toFixed(1)}px`,
                );
              }
            });
          });
        });
      });
    });

    // Asserted together so a regression that both clips and shrinks reports
    // both lists instead of stopping at whichever expectation ran first.
    expect({ clipped, illegible }).toEqual({ clipped: [], illegible: [] });
  });

  /**
   * Every inter-band gap each family produces, in DEVICE PIXELS, for all three
   * aspects.
   *
   * This is a characterization pin, not a design assertion: nothing here says a
   * number is right, only that it is what the model currently resolves to. Its
   * job is to make any change to the spacing model visible as a table rather
   * than as a diffuse "the 9:16 posts look wrong" report — which is how the
   * width/height unit mismatch survived several tuning passes.
   *
   * Device pixels, not normalized units, because normalized units are exactly
   * what hid the defect: 0.018 of height is one number on paper and three
   * different amounts of ink on the three canvases the same declaration is
   * painted on.
   *
   * Gaps are measured between CONSECUTIVE SOLVED bands rather than read off the
   * declarations, so a gap that the solver drops (because it clipped the band
   * after it) is absent here rather than asserted as though it were painted.
   */
  const solvedGapsPx = (id: CompositionId, aspect: AspectRatio): number[] => {
    const def = COMPOSITIONS[id];
    const { w: canvasW, h: canvasH } = ASPECT_DIMS[aspect];
    const bands = bandsWithText(def, aspect);
    const solved = solveStack({
      bands,
      bounds: def.boundsFor(FORMATS[aspect]),
      canvasW,
      canvasH,
      anchor: def.anchor,
      measure,
    });
    const rects = bands
      .map((b) => solved[b.key]?.rect)
      .filter((r): r is NormalizedRect => r !== undefined);
    return rects
      .slice(1)
      .map(
        (rect, i) =>
          Math.round((rect.y - (rects[i].y + rects[i].h)) * canvasH * 100) /
          100,
      );
  };

  /**
   * The pinned table, ONE ROW PER FAMILY rather than one per (family, aspect),
   * because that is now the whole claim: a declaration costs the same ink on
   * all three canvases. Held per family and not as a single shared row so that
   * `badgeHero`'s and `posterStack`'s deliberately larger gaps stay visible as
   * design decisions instead of collapsing into a default.
   *
   * What these numbers were before `gapPct` became width-relative — the
   * characterization this replaced, and the reason for the change:
   *
   *   family            1:1                    4:5                    9:16
   *   photoBottomStack  19.44 x4               24.30 x4               34.56 x4
   *   photoTopStack     19.44 x4               24.30 x4               34.56 x4
   *   splitPanel        19.44 x4               24.30 x4               34.56 x4
   *   badgeHero         32.40 19.44 19.44      40.50 24.30 24.30      57.60 34.56 34.56
   *   cornerCard        19.44 x4               24.30 x4               34.56 x4
   *   posterStack       54.00 19.44 43.20 19.44  67.50 24.30 54.00 24.30  96.00 34.56 76.80 34.56
   *   legacyEditorial   19.44 x4               24.30 x4               34.56 x4
   *   legacyBold        19.44 x4               24.30 x4               34.56 x4
   *   legacyMinimal     19.44 x2               24.30 x2               34.56 x2
   *
   * Every 1:1 column above is the row below it unchanged, which is the property
   * that mattered: the numbers were tuned on the square, so the square is what
   * must not move. 4:5 came down by 1.25x and 9:16 by 1.78x to meet it.
   */
  const GAPS_PX: Record<CompositionId, number[]> = {
    photoBottomStack: [19.44, 19.44, 19.44, 19.44],
    photoTopStack: [19.44, 19.44, 19.44, 19.44],
    splitPanel: [19.44, 19.44, 19.44, 19.44],
    badgeHero: [32.4, 19.44, 19.44],
    cornerCard: [19.44, 19.44, 19.44, 19.44],
    posterStack: [54, 19.44, 43.2, 19.44],
    legacyEditorial: [19.44, 19.44, 19.44, 19.44],
    legacyBold: [19.44, 19.44, 19.44, 19.44],
    legacyMinimal: [19.44, 19.44],
  };

  it("resolves each family's declared gaps to a pinned number of device pixels", () => {
    const table: Record<string, number[]> = {};
    const want: Record<string, number[]> = {};
    COMPOSITION_ORDER.forEach((id) => {
      ASPECTS.forEach((aspect) => {
        table[`${id} @ ${aspect}`] = solvedGapsPx(id, aspect);
        want[`${id} @ ${aspect}`] = GAPS_PX[id];
      });
    });
    expect(table).toEqual(want);
  });

  /**
   * The disc `drawLogo` actually paints, in device pixels.
   *
   * Wider than `size` on every edge: the mark is drawn inside a filled ring of
   * `size / 2 + round(width * 0.008)`, which is 9px at the 1080px width all
   * three aspects share. `size` is width-relative and `y` is height-relative,
   * so this is also where a flat anchor's three different pixel positions
   * become visible.
   */
  const logoDiscPx = (id: CompositionId, aspect: AspectRatio) => {
    const { w: canvasW, h: canvasH } = ASPECT_DIMS[aspect];
    const anchor = COMPOSITIONS[id].logoAnchorFor(FORMATS[aspect]);
    const size = Math.round(anchor.size * canvasW);
    const ring = Math.round(canvasW * 0.008);
    const left = anchor.x * canvasW;
    const top = anchor.y * canvasH;
    return {
      left: left - ring,
      top: top - ring,
      right: left + size + ring,
      bottom: top + size + ring,
    };
  };

  // The regression this replaced a flat `logoAnchor` for: at 9:16 all nine
  // families put the disc 77-106px above the safe top or 204-240px below the
  // safe bottom, and three of them overran on 1:1 and 4:5 too. Reported as a
  // list so one run names every family and edge that fails, not just the first.
  it("keeps every family's logo disc, ring included, inside the platform-safe area", () => {
    const outside: string[] = [];

    ASPECTS.forEach((aspect) => {
      const { w: canvasW, h: canvasH } = ASPECT_DIMS[aspect];
      const safe = PLATFORM_CONTENT_BOUNDS[aspect];
      COMPOSITION_ORDER.forEach((id) => {
        const disc = logoDiscPx(id, aspect);
        const overruns: Array<[string, number]> = [
          ["top", safe.y * canvasH - disc.top],
          ["bottom", disc.bottom - (safe.y + safe.h) * canvasH],
          ["left", safe.x * canvasW - disc.left],
          ["right", disc.right - (safe.x + safe.w) * canvasW],
        ];
        const past = overruns.filter(([, px]) => px > 1e-9);
        if (past.length) {
          outside.push(
            `${id} @ ${aspect}: ${past
              .map(([edge, px]) => `${px.toFixed(1)}px past ${edge}`)
              .join(", ")}`,
          );
        }
      });
    });

    expect(outside).toEqual([]);
  });

  /**
   * Fitting inside the safe area is not enough — a mark centred on the dish
   * name is inside the safe area too. This checks the disc against the SOLVED
   * band rects rather than the declared envelope, which is both stricter (the
   * envelope is a budget, the solved rects are what gets painted) and the only
   * workable check for `posterStack` at 9:16, where `safeBounds` clamps the
   * envelope down to the safe-area floor and leaves no lane outside it.
   *
   * Legacy families are exempt by design: `BOLD` at 1:1 sat its mark inside the
   * text block, and reproducing the pre-rewrite look is the entire reason those
   * three families exist.
   *
   * Swept over the same copy lengths and kit type-scales as the packing test
   * above, because both inputs move the solved rects this is checked against: a
   * taller stack at scale 1.15, or a dish name that wraps to another line,
   * reaches further into whatever lane the mark sits in.
   */
  it("keeps every choosable family's logo disc clear of its own solved text", () => {
    const collisions: string[] = [];

    [SAMPLE_TEXT.dishName, LONG_DISH_NAME].forEach((dishName) => {
      KIT_TYPE_SCALES.forEach((typeScale) => {
        ASPECTS.forEach((aspect) => {
          const { w: canvasW, h: canvasH } = ASPECT_DIMS[aspect];
          CHOOSABLE_COMPOSITIONS.forEach((id) => {
            const def = COMPOSITIONS[id];
            const bands = bandsWithText(def, aspect, { dishName, typeScale });
            const solved = solveStack({
              bands,
              bounds: def.boundsFor(FORMATS[aspect]),
              canvasW,
              canvasH,
              anchor: def.anchor,
              measure,
            });
            const disc = logoDiscPx(id, aspect);

            bands.forEach((b) => {
              const out = solved[b.key];
              if (!out) return;
              const { rect } = out;
              const overlaps =
                disc.left < (rect.x + rect.w) * canvasW &&
                disc.right > rect.x * canvasW &&
                disc.top < (rect.y + rect.h) * canvasH &&
                disc.bottom > rect.y * canvasH;
              if (overlaps) {
                collisions.push(
                  `${id} @ ${aspect} x${typeScale} (${dishName.length} chars): ${b.key}`,
                );
              }
            });
          });
        });
      });
    });

    expect(collisions).toEqual([]);
  });

  /** Is this the whole canvas, i.e. a photo that bleeds rather than a panel? */
  const isFullBleed = (rect: NormalizedRect): boolean =>
    rect.x === 0 && rect.y === 0 && rect.w === 1 && rect.h === 1;

  /**
   * Expected vertical overlap, in device pixels, between the photo a family
   * declares and the envelope it puts type in.
   *
   * A blanket "photo and text must not overlap" rule would be wrong twice over.
   * Six of the nine families bleed the photo across the whole canvas and put
   * type straight on top of it — that is what `buildScene`'s scrim is for — so
   * for them the overlap is total and meaningless. And of the two that carve a
   * discrete panel, `legacyMinimal` overlaps ON PURPOSE: the pre-rewrite MINIMAL
   * template ran its photo to 0.76 on 1:1 and 0.60 on 9:16 while starting
   * dishName at 0.75 and 0.59, and reproducing that is the family's whole job.
   *
   * So the invariant is bounded rather than absolute, and per family: a
   * discrete panel may cross into the type by exactly the number of pixels
   * declared here and no more. Swelling `splitPanel`'s panel from 0.58 to 0.95,
   * so it swallows the envelope entirely, was invisible to every other test in
   * this file.
   *
   * Only the vertical axis is measured: both panels span the full width of the
   * column their family's type sits in, so a horizontal test would report the
   * same full overlap for every entry and detect nothing.
   */
  const PHOTO_PANEL_OVERLAP_PX: Partial<
    Record<CompositionId, Record<AspectRatio, number>>
  > = {
    splitPanel: { "1:1": 0, "4:5": 0, "9:16": 0 },
    legacyMinimal: { "1:1": 10.8, "4:5": 0, "9:16": 19.2 },
  };

  it("lets a declared photo panel cross into the type only as far as it is declared to", () => {
    const wrong: string[] = [];

    COMPOSITION_ORDER.forEach((id) => {
      const declared = PHOTO_PANEL_OVERLAP_PX[id];
      ASPECTS.forEach((aspect) => {
        const { h: canvasH } = ASPECT_DIMS[aspect];
        const photo = COMPOSITIONS[id].imageAreaFor(FORMATS[aspect]);
        const bounds = COMPOSITIONS[id].boundsFor(FORMATS[aspect]);

        // A family may bleed or may carve a panel, but which one it does is
        // part of its design and must not flip silently in either direction.
        if (isFullBleed(photo) !== (declared === undefined)) {
          wrong.push(
            `${id} @ ${aspect}: ${isFullBleed(photo) ? "full bleed" : "panel"} but ${declared === undefined ? "no" : "an"} overlap budget`,
          );
          return;
        }
        if (!declared) return;

        const overlap =
          Math.max(
            0,
            Math.min(photo.y + photo.h, bounds.y + bounds.h) -
              Math.max(photo.y, bounds.y),
          ) * canvasH;
        if (Math.abs(overlap - declared[aspect]) > 1e-3) {
          wrong.push(
            `${id} @ ${aspect}: ${overlap.toFixed(1)}px != ${declared[aspect].toFixed(1)}px`,
          );
        }
      });
    });

    expect(wrong).toEqual([]);
  });

  /**
   * `requiresPhoto` has to agree with the geometry the same definition
   * declares. A family that carves a discrete photo panel out of the canvas
   * cannot render without a photo — the panel is a hole in the design, not
   * negative space — so it must say so, whether or not anything reads it today.
   *
   * `legacyMinimal` said `false` while declaring an inset panel at all three
   * aspects. That was inert rather than harmful: `usableCompositions` in
   * chooser.ts is the only reader, and it filters CHOOSABLE_COMPOSITIONS, which
   * excludes all three legacy families. The assertion below pins that
   * unreachability, so the honest flag stays a documentation fix and cannot
   * quietly become a behaviour change.
   */
  it("declares requiresPhoto in step with the photo geometry it carves out", () => {
    const dishonest: string[] = [];
    COMPOSITION_ORDER.forEach((id) => {
      const carvesPanel = ASPECTS.some(
        (aspect) => !isFullBleed(COMPOSITIONS[id].imageAreaFor(FORMATS[aspect])),
      );
      if (carvesPanel && !COMPOSITIONS[id].requiresPhoto) {
        dishonest.push(`${id}: carves a photo panel but requiresPhoto is false`);
      }
    });
    expect(dishonest).toEqual([]);

    // The reason flipping legacyMinimal's flag changes no behaviour.
    Object.keys(COMPOSITIONS)
      .filter((id) => id.startsWith("legacy"))
      .forEach((id) =>
        expect(CHOOSABLE_COMPOSITIONS).not.toContain(id as CompositionId),
      );
  });

  /**
   * The three (template, composition) pairs that exist for one reason: a
   * `creative_snapshot` saved before the scene rewrite must keep rendering the
   * way it did. Held as a list so every assertion below runs over all three.
   *
   * Pinning one family and leaving two free is exactly how this drifted:
   * `legacyMinimal` was held to `minimalSlots` while `legacyEditorial` and
   * `legacyBold` sat on the generic `standardBands` helper — centred BOLD type
   * rendered left-aligned, EDITORIAL's right-aligned CTA rendered left, and
   * four of the ten sizes were 12-24% off — and every test stayed green.
   */
  const LEGACY_FAMILIES: Array<{ id: CompositionId; template: TemplateDef }> = [
    { id: "legacyEditorial", template: EDITORIAL },
    { id: "legacyBold", template: BOLD },
    { id: "legacyMinimal", template: MINIMAL },
  ];

  it("declares one band per slot its pre-rewrite template painted, in order", () => {
    const wrong: string[] = [];
    LEGACY_FAMILIES.forEach(({ id, template }) => {
      ASPECTS.forEach((aspect) => {
        const declared = COMPOSITIONS[id].bandsFor(FORMATS[aspect]).map((b) => b.key);
        const painted = Object.keys(template.layouts[aspect].slots);
        if (declared.join() !== painted.join()) {
          wrong.push(`${id} @ ${aspect}: [${declared}] != [${painted}]`);
        }
      });
    });
    expect(wrong).toEqual([]);
  });

  it("reproduces its pre-rewrite template's photo area", () => {
    LEGACY_FAMILIES.forEach(({ id, template }) => {
      ASPECTS.forEach((aspect) => {
        expect(COMPOSITIONS[id].imageAreaFor(FORMATS[aspect])).toEqual(
          template.layouts[aspect].imageArea,
        );
      });
    });
  });

  /**
   * Every type field the pre-rewrite renderer read off a `SlotDef` and the new
   * path still has to honour — not `sizePct` alone, which is what this used to
   * assert. Mutating `legacyMinimal`'s handle to `align: "left"`, or its
   * dishName's `maxLines` 2 -> 1 and `lineHeight` 1.05 -> 1.6, left the suite
   * green. Leaving `font` out left the same hole for the face: the kit typeface
   * pin (`9f44f0d49`) moved editorial headlines onto DM Serif Display and
   * nothing in this file noticed, because the band geometry pin never looked
   * at the face.
   *
   * `align` is load-bearing rather than cosmetic: `buildScene` copies the
   * band's align onto the text node and the walker derives both `ctx.textAlign`
   * and the anchor X from it, so a wrong align moves every glyph.
   *
   * `font` is not a `BandTemplate` field — compositions deliberately omit face
   * and weight (`Omit<Band, "text" | "font" | "weight">`); `buildScene`'s
   * `bandTypeFor` fills them from the kit. On the legacy path the kit is
   * `kitForLegacyTemplate(template)`, so the face each legacy family paints is
   * a pure function of the stored template. This test derives that face the
   * same way and holds it to `templates.ts`, not to a number copied into the
   * composition definition (which would be circular and could not catch a kit
   * retune).
   *
   * Shared Band/SlotDef fields and why each is (or is not) asserted:
   *
   *   sizePct, align, maxLines, lineHeight — composition BandTemplate vs
   *     template slot, asserted below.
   *   font — legacy kit path vs template slot, asserted below; intentional
   *     deviations land in LEGACY_FONT_DEVIATIONS rather than silently.
   *   weight — shared on the types but intentionally unasserted. `bandTypeFor`
   *     reassigns every band to DISPLAY_WEIGHT (700) or BODY_WEIGHT (500) from
   *     the kit, discarding per-slot template weights. Pinning weight would
   *     force an exception on nearly every slot and would not defend a
   *     composition fidelity claim. The type-level gate below still names it
   *     so a new shared field cannot land unaccounted for.
   *
   * Band-only / SlotDef-only fields this test deliberately does not cover:
   *
   *   gapPct (Band only) — templates spaced by absolute rects; no slot field
   *     to compare. Cost is documented on the `legacy*Bands` builders.
   *   text (Band only) — runtime fill, not a composition declaration.
   *   key (Band only) — asserted separately by the band-keys-vs-slots test.
   *   rect (Slot only) — absolute placement; a packed stack cannot reproduce
   *     per-slot y. The anchor-edge pin below is the residual claim.
   *   color, uppercase, pill (Slot only) — art direction / kit / buildScene
   *     concerns, not composition geometry. `buildScene` decides muted/pill
   *     from the band key, not from a template copy.
   */
  type SharedBandSlotField = Extract<keyof SlotDef, keyof Band>;
  /**
   * Single source of truth for band↔slot fields this test compares at runtime.
   * Adding a name to AssertedSharedField without putting it here fails either
   * the compile-time gate (if omitted from AssertedSharedField entirely) or
   * leaves the field uncompared only if someone reintroduces a parallel union
   * — so the compare loop is derived from this list.
   * `font` is intentionally special-cased below via `legacyPathFont` (not a
   * BandTemplate field; kit-derived on the legacy path).
   */
  const ASSERTED_SHARED = [
    "sizePct",
    "align",
    "maxLines",
    "lineHeight",
  ] as const;
  type AssertedSharedField =
    | (typeof ASSERTED_SHARED)[number]
    | "font";
  type IntentionallyUnassertedShared = "weight";
  // Compile fails if SlotDef ∩ Band gains a field this test neither asserts
  // nor lists as intentionally unasserted.
  type _AllSharedFieldsAccountedFor = Exclude<
    SharedBandSlotField,
    AssertedSharedField | IntentionallyUnassertedShared
  > extends never
    ? true
    : never;
  const _sharedFieldGate: _AllSharedFieldsAccountedFor = true;
  void _sharedFieldGate;

  /**
   * Intentional face deviations from `templates.ts` on the legacy path.
   * Empty by default; each entry must name the painted face and why it is not
   * the template's. A silent retune of a kit's display/body face against a
   * template that still declares the old one fails the assertion below unless
   * it is recorded here.
   */
  const LEGACY_FONT_DEVIATIONS: Partial<
    Record<
      CompositionId,
      Partial<Record<SlotKey, { font: SlotFont; reason: string }>>
    >
  > = {
    // minimal kit display is sans. The pre-rewrite template declared serif on
    // dishName, but `slotFontString` forced sans under the default brand
    // font_family ("Inter"); the kit preserves that painted default rather
    // than the template declaration. See `bandTypeFor` / buildScene.test.ts.
    legacyMinimal: {
      dishName: {
        font: "sans",
        reason:
          "minimal kit display is sans; matches the pre-rewrite kill-switch default under font_family Inter, not the template's serif declaration",
      },
    },
  };

  /**
   * Face the legacy path paints for a band. Mirrors `bandTypeFor` in
   * buildScene.ts (display face on dishName, body face elsewhere) so a kit
   * retune reaches this test without re-importing a private helper.
   */
  const legacyPathFont = (
    templateStyle: TemplateStyle,
    key: string,
  ): SlotFont => {
    const kit = KITS[kitForLegacyTemplate(templateStyle)];
    return key === "dishName"
      ? kit.typePairing.display
      : kit.typePairing.body;
  };

  it("reproduces its pre-rewrite template's type, slot for slot", () => {
    const drift: string[] = [];
    LEGACY_FAMILIES.forEach(({ id, template }) => {
      ASPECTS.forEach((aspect) => {
        const slots = template.layouts[aspect].slots;
        COMPOSITIONS[id].bandsFor(FORMATS[aspect]).forEach((b) => {
          const slot = slots[b.key as SlotKey];
          if (!slot) {
            drift.push(`${id} @ ${aspect}: ${b.key} has no template slot`);
            return;
          }
          // Runtime compare is driven by ASSERTED_SHARED — listing a field in
          // AssertedSharedField without adding it here is impossible for the
          // band↔slot set (the union is derived from this const).
          for (const field of ASSERTED_SHARED) {
            if (b[field] !== slot[field]) {
              drift.push(
                `${id} @ ${aspect}: ${b.key}.${field} ${JSON.stringify(b[field])} != ${JSON.stringify(slot[field])}`,
              );
            }
          }
          // font: kit-derived on the legacy path, not a BandTemplate field.
          // Expectations come from templates.ts (or a declared deviation),
          // never from the composition definition itself.
          const fontOverride = LEGACY_FONT_DEVIATIONS[id]?.[b.key as SlotKey];
          const gotFont = legacyPathFont(template.id, b.key);
          const wantFont = fontOverride?.font ?? slot.font;
          if (gotFont !== wantFont) {
            drift.push(
              `${id} @ ${aspect}: ${b.key}.font ${JSON.stringify(gotFont)} != ${JSON.stringify(wantFont)}`,
            );
          }
        });
      });
    });
    expect(drift).toEqual([]);
  });

  /**
   * The one positional claim a packed stack can still honour.
   *
   * The old templates placed each slot at an absolute rect, so their vertical
   * distribution — a badge in one corner and the rest of the type in the
   * opposite one, for both EDITORIAL and BOLD — cannot survive a model that
   * packs one contiguous stack. What does survive is the edge the type is
   * pinned to: a bottom-anchored family's last band must end where the
   * template's lowest slot ended, and a top-anchored family's first band must
   * start where the template's highest slot started. That pins `anchor` and the
   * anchored edge of `boundsFor` together, against templates.ts rather than
   * against numbers copied into compositions.ts.
   *
   * ANCHOR_EDGE_DEVIATIONS is the one place that is knowingly not true, and it
   * is listed rather than tolerated so that any OTHER drift still fails.
   */
  const ANCHOR_EDGE_DEVIATIONS: Partial<
    Record<CompositionId, Partial<Record<AspectRatio, number>>>
  > = {
    // EDITORIAL's 4:5 scrim is declared to reach the canvas bottom, and
    // `buildScene` derives that gradient from these bounds plus a 0.08 bleed.
    // Ending the envelope on the template's 0.915 text bottom leaves a 6.75px
    // unscrimmed strip below the gradient; 0.92 closes it and sits the stack
    // 6.75px low. `templates/renderPost.test.ts` pins the scrim side.
    legacyEditorial: { "4:5": 0.92 },
  };

  it("pins each legacy stack to the canvas edge its template pinned type to", () => {
    const off: string[] = [];
    LEGACY_FAMILIES.forEach(({ id, template }) => {
      ASPECTS.forEach((aspect) => {
        const def = COMPOSITIONS[id];
        const { w: canvasW, h: canvasH } = ASPECT_DIMS[aspect];
        const bands = bandsWithText(def, aspect);
        const solved = solveStack({
          bands,
          bounds: def.boundsFor(FORMATS[aspect]),
          canvasW,
          canvasH,
          anchor: def.anchor,
          measure,
        });
        const rects = Object.values(template.layouts[aspect].slots).map(
          (s) => s.rect,
        );
        const top = def.anchor === "top";
        const want =
          ANCHOR_EDGE_DEVIATIONS[id]?.[aspect] ??
          (top
            ? Math.min(...rects.map((r) => r.y))
            : Math.max(...rects.map((r) => r.y + r.h)));

        const first = solved[bands[0].key];
        const last = solved[bands[bands.length - 1].key];
        const edge = top ? first?.rect.y : last && last.rect.y + last.rect.h;
        if (edge === undefined || Math.abs(edge - want) > 1e-9) {
          off.push(
            `${id} @ ${aspect}: ${def.anchor} edge ${edge?.toFixed(4)} != ${want.toFixed(4)}`,
          );
        }
      });
    });
    expect(off).toEqual([]);
  });

  /**
   * `boundsFor`'s free edge — the one the anchor does not pin — is a budget,
   * not a position. Its job is to be deep enough that the solver never shrinks
   * the type below what the template declared, because the type sizes ARE
   * reproducible and a shrink would throw away the only fidelity these three
   * families can offer.
   */
  it("gives every legacy family room for its template's type at full size", () => {
    const shrunk: string[] = [];
    LEGACY_FAMILIES.forEach(({ id }) => {
      ASPECTS.forEach((aspect) => {
        const def = COMPOSITIONS[id];
        const { w: canvasW, h: canvasH } = ASPECT_DIMS[aspect];
        const bands = bandsWithText(def, aspect);
        const solved = solveStack({
          bands,
          bounds: def.boundsFor(FORMATS[aspect]),
          canvasW,
          canvasH,
          anchor: def.anchor,
          measure,
        });
        bands.forEach((b) => {
          const want = b.sizePct * canvasW;
          const got = solved[b.key]?.fontPx;
          if (got === undefined || Math.abs(got - want) > 1e-9) {
            shrunk.push(
              `${id} @ ${aspect}: ${b.key} ${got?.toFixed(2)}px != ${want.toFixed(2)}px`,
            );
          }
        });
      });
    });
    expect(shrunk).toEqual([]);
  });
});

describe("compositions take a FormatDef", () => {
  const LEGACY_IDS: CompositionId[] = [
    "legacyEditorial",
    "legacyBold",
    "legacyMinimal",
  ];

  it("accepts every registry format on the choosable families", () => {
    FORMAT_ORDER.forEach((formatId) => {
      const format = FORMATS[formatId];
      CHOOSABLE_COMPOSITIONS.forEach((id) => {
        expect(COMPOSITIONS[id].supportsFormat(format)).toBe(true);
      });
    });
  });

  // The legacy families exist to reproduce a pre-rewrite template that only ever
  // had three layouts. Asking one for geometry on a format that template never
  // saw is not a layout question with an answer — it is a category error, and
  // the chooser must never route there.
  it("refuses non-legacy formats on the legacy families", () => {
    FORMAT_ORDER.forEach((formatId) => {
      const format = FORMATS[formatId];
      LEGACY_IDS.forEach((id) => {
        expect(COMPOSITIONS[id].supportsFormat(format)).toBe(
          format.legacyAspect !== null,
        );
      });
    });
  });

  it("solves inside the format's own safe area, for every family and format", () => {
    const measure: MeasureFn = (text, cssFont) => {
      const px = Number.parseFloat(cssFont.match(/([\d.]+)px/)?.[1] ?? "20");
      return text.length * px * 0.52;
    };
    FORMAT_ORDER.forEach((formatId) => {
      const format = FORMATS[formatId];
      COMPOSITION_ORDER.forEach((id) => {
        const composition = COMPOSITIONS[id];
        if (!composition.supportsFormat(format)) return;
        const bounds = composition.boundsFor(format);
        expect(bounds.x).toBeGreaterThanOrEqual(format.safe.x - 1e-9);
        expect(bounds.y).toBeGreaterThanOrEqual(format.safe.y - 1e-9);
        expect(bounds.x + bounds.w).toBeLessThanOrEqual(
          format.safe.x + format.safe.w + 1e-9,
        );
        expect(bounds.y + bounds.h).toBeLessThanOrEqual(
          format.safe.y + format.safe.h + 1e-9,
        );
        expect(bounds.w).toBeGreaterThan(0);
        expect(bounds.h).toBeGreaterThan(0);

        const bands: Band[] = composition.bandsFor(format).map((template) => ({
          ...template,
          text: "Milanesa",
          font: "sans" as const,
          weight: 500 as const,
        }));
        const solved = solveStack({
          bands,
          bounds,
          canvasW: format.px.w,
          canvasH: format.px.h,
          anchor: composition.anchor,
          measure,
        });
        Object.values(solved).forEach((band) => {
          expect(band.rect.y).toBeGreaterThanOrEqual(bounds.y - 1e-9);
          expect(band.rect.y + band.rect.h).toBeLessThanOrEqual(
            bounds.y + bounds.h + 1e-6,
          );
        });
      });
    });
  });
});

describe("logo anchors", () => {
  // The 27 values the pre-Wave-4 LOGO_ANCHORS table held, verbatim. This is the
  // contract that the format refactor changed no pixel on the three original
  // canvases. Nothing here may be "corrected" — a value that looks wrong is a
  // value some operator's stored post already renders with.
  const PINNED: Record<CompositionId, Record<string, [number, number, number]>> = {
    photoBottomStack: {
      "1:1": [0.07, 0.07, 0.11],
      "4:5": [0.07, 0.078, 0.11],
      "9:16": [0.07, 0.13, 0.11],
    },
    photoTopStack: {
      "1:1": [0.82, 0.83, 0.1],
      "4:5": [0.82, 0.84, 0.1],
      "9:16": [0.82, 0.75, 0.1],
    },
    splitPanel: {
      "1:1": [0.82, 0.07, 0.08],
      "4:5": [0.82, 0.078, 0.08],
      "9:16": [0.82, 0.13, 0.08],
    },
    badgeHero: {
      "1:1": [0.08, 0.08, 0.11],
      "4:5": [0.08, 0.08, 0.11],
      "9:16": [0.08, 0.13, 0.11],
    },
    cornerCard: {
      "1:1": [0.78, 0.08, 0.12],
      "4:5": [0.78, 0.08, 0.12],
      "9:16": [0.78, 0.13, 0.12],
    },
    posterStack: {
      "1:1": [0.46, 0.85, 0.08],
      "4:5": [0.46, 0.85, 0.08],
      "9:16": [0.46, 0.72, 0.08],
    },
    legacyEditorial: {
      "1:1": [0.83, 0.07, 0.1],
      "4:5": [0.83, 0.078, 0.1],
      "9:16": [0.84, 0.145, 0.09],
    },
    legacyBold: {
      "1:1": [0.82, 0.83, 0.1],
      "4:5": [0.82, 0.078, 0.1],
      "9:16": [0.84, 0.72, 0.09],
    },
    legacyMinimal: {
      "1:1": [0.82, 0.07, 0.1],
      "4:5": [0.82, 0.078, 0.1],
      "9:16": [0.84, 0.145, 0.09],
    },
  };

  it("reproduces all 27 pre-Wave-4 anchors exactly", () => {
    COMPOSITION_ORDER.forEach((id) => {
      (["1:1", "4:5", "9:16"] as const).forEach((formatId) => {
        const [x, y, size] = PINNED[id][formatId];
        expect(COMPOSITIONS[id].logoAnchorFor(FORMATS[formatId])).toEqual({
          x,
          y,
          size,
        });
      });
    });
  });

  // The derived path is not a fudge: measured against each format's own safe
  // area, every choosable family's HORIZONTAL inset is constant across all three
  // canvases, so the derivation reproduces x exactly. y does not derive exactly
  // because the original table mixes units (photoBottomStack is safe.y + 0.01 in
  // HEIGHT units at 1:1 and 9:16 but a width-relative 0.01 at 4:5) — which is
  // exactly why all 27 stay pinned as overrides above.
  it("derives the same x as the pinned override, for every choosable family", () => {
    CHOOSABLE_COMPOSITIONS.forEach((id) => {
      (["1:1", "4:5", "9:16"] as const).forEach((formatId) => {
        const derived = deriveLogoAnchorForTests(id, FORMATS[formatId]);
        expect(derived.x).toBeCloseTo(PINNED[id][formatId][0], 10);
      });
    });
  });

  it("keeps the whole disc, ring included, inside the safe area on every format", () => {
    // drawLogo paints a ring of size/2 + round(width * 0.008), so the disc
    // reaches RING_PCT further than `size` on every edge.
    const RING_PCT = 0.008;
    FORMAT_ORDER.forEach((formatId) => {
      const format = FORMATS[formatId];
      const vScale = format.px.w / format.px.h;
      COMPOSITION_ORDER.forEach((id) => {
        const composition = COMPOSITIONS[id];
        if (!composition.supportsFormat(format)) return;
        const anchor = composition.logoAnchorFor(format);
        const left = anchor.x - RING_PCT;
        const right = anchor.x + anchor.size + RING_PCT;
        const top = anchor.y - RING_PCT * vScale;
        const bottom = anchor.y + (anchor.size + RING_PCT) * vScale;
        expect(left).toBeGreaterThanOrEqual(format.safe.x - 1e-9);
        expect(right).toBeLessThanOrEqual(format.safe.x + format.safe.w + 1e-9);
        expect(top).toBeGreaterThanOrEqual(format.safe.y - 1e-9);
        expect(bottom).toBeLessThanOrEqual(format.safe.y + format.safe.h + 1e-9);
      });
    });
  });
});

describe("splitPanel and cornerCard derive their envelopes", () => {
  // The exact effective envelopes the pre-Wave-4 per-aspect tables produced,
  // AFTER safeBounds clamping. These are the numbers a stored post is laid out
  // with today, so a flattened declaration has to reproduce them.
  const PINNED_BOUNDS: Record<string, Record<string, NormalizedRect>> = {
    splitPanel: {
      "1:1": { x: 0.08, y: 0.59, w: 0.84, h: 0.35 },
      "4:5": { x: 0.08, y: 0.59, w: 0.84, h: 0.34 },
      "9:16": { x: 0.08, y: 0.59, w: 0.84, h: 0.23 },
    },
    cornerCard: {
      "1:1": { x: 0.08, y: 0.54, w: 0.6, h: 0.38 },
      "4:5": { x: 0.08, y: 0.54, w: 0.6, h: 0.38 },
      "9:16": { x: 0.08, y: 0.58, w: 0.6, h: 0.24 },
    },
  };

  it("reproduces splitPanel's effective envelope on all three legacy formats", () => {
    (["1:1", "4:5", "9:16"] as const).forEach((formatId) => {
      const got = COMPOSITIONS.splitPanel.boundsFor(FORMATS[formatId]);
      const want = PINNED_BOUNDS.splitPanel[formatId];
      expect(got.x).toBeCloseTo(want.x, 10);
      expect(got.y).toBeCloseTo(want.y, 10);
      expect(got.w).toBeCloseTo(want.w, 10);
      expect(got.h).toBeCloseTo(want.h, 10);
    });
  });

  // cornerCard is bottom-anchored, so only the BOTTOM edge is a position. The
  // top is a budget: raising it moves no glyph unless the stack was already
  // shrinking. Flattening 9:16's top from 0.58 to 0.54 therefore has to leave the
  // solved stack byte-identical, and that — not the declared rect — is what this
  // asserts.
  it("leaves cornerCard's solved stack unchanged at 9:16 after flattening", () => {
    const format = FORMATS["9:16"];
    const measure: MeasureFn = (text, cssFont) => {
      const px = Number.parseFloat(cssFont.match(/([\d.]+)px/)?.[1] ?? "20");
      return text.length * px * 0.52;
    };
    const bands: Band[] = COMPOSITIONS.cornerCard
      .bandsFor(format)
      .map((template) => ({
        ...template,
        text: "Milanesa napolitana con papas fritas y ensalada",
        font: "sans" as const,
        weight: 500 as const,
      }));
    const solve = (bounds: NormalizedRect) =>
      solveStack({
        bands,
        bounds,
        canvasW: format.px.w,
        canvasH: format.px.h,
        anchor: COMPOSITIONS.cornerCard.anchor,
        measure,
      });

    const before = solve(PINNED_BOUNDS.cornerCard["9:16"]);
    const after = solve(COMPOSITIONS.cornerCard.boundsFor(format));
    expect(Object.keys(after)).toEqual(Object.keys(before));
    Object.keys(before).forEach((key) => {
      expect(after[key].fontPx).toBeCloseTo(before[key].fontPx, 6);
      expect(after[key].rect.y).toBeCloseTo(before[key].rect.y, 6);
      expect(after[key].rect.h).toBeCloseTo(before[key].rect.h, 6);
    });
  });

  it("keeps cornerCard bottom-pinned to 0.92 or the safe floor, whichever is higher", () => {
    (["1:1", "4:5", "9:16"] as const).forEach((formatId) => {
      const format = FORMATS[formatId];
      const bounds = COMPOSITIONS.cornerCard.boundsFor(format);
      const expected = Math.min(0.92, format.safe.y + format.safe.h);
      expect(bounds.y + bounds.h).toBeCloseTo(expected, 10);
    });
  });
});
