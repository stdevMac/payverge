import { KIT_ORDER, type KitId } from "../artDirection/kits";
import { FORMATS, FORMAT_ORDER } from "../formats/formats";
import { ASPECT_DIMS, type AspectRatio } from "../templates/types";
import {
  CHOOSABLE_COMPOSITIONS,
  COMPOSITIONS,
  COMPOSITION_ORDER,
  type CompositionId,
} from "./compositions";
import {
  chooseComposition,
  contentSignalsFrom,
  isChoosableCompositionId,
  isCompositionId,
  nextComposition,
  usableCompositions,
  type ContentSignals,
  type PhotoSignals,
} from "./chooser";

const content = (over: Partial<ContentSignals> = {}): ContentSignals => ({
  play: "featured_dish",
  dishNameLength: 9,
  hasPrice: true,
  hasDiscount: false,
  hasHandle: true,
  hasPhoto: true,
  ...over,
});

const ASPECTS = Object.keys(ASPECT_DIMS) as AspectRatio[];
const DEFAULT_FORMAT = FORMATS["4:5"];
const NEGATIVE_SPACES: PhotoSignals["negativeSpace"][] = [
  "top",
  "bottom",
  "left",
  "right",
  "center",
  "none",
];

/** null (Wave 1) plus every photo-analysis result Wave 2 can produce. */
const PHOTO_CASES: Array<PhotoSignals | null> = [
  null,
  ...NEGATIVE_SPACES.flatMap((negativeSpace) =>
    [false, true].map((busy) => ({ negativeSpace, busy })),
  ),
];

const CONTENT_CASES: ContentSignals[] = [true, false].flatMap((hasPhoto) =>
  [true, false].flatMap((hasDiscount) =>
    [9, 42].map((dishNameLength) =>
      content({ hasPhoto, hasDiscount, dishNameLength }),
    ),
  ),
);

/** Compositions that render without a photo, per their own definitions. */
const PHOTO_FREE_COMPOSITIONS: CompositionId[] = COMPOSITION_ORDER.filter(
  (id) => !COMPOSITIONS[id].requiresPhoto,
);

describe("chooseComposition", () => {
  // Two space-wide invariants that no single-input test can state: every
  // point in the signal space maps to a choosable composition, and every
  // point maps there deterministically. The space is photo present/absent x
  // discount x short/long dish name x every photo analysis Wave 2 can report
  // x every kit x every aspect.
  //
  // This sweep deliberately says nothing about WHICH composition comes back -
  // a set-of-results assertion is too coarse to detect a reordered rule chain
  // (dropping one rule still leaves four distinct outputs). Rule identity and
  // rule priority are pinned by the named tests below, which fail with a
  // sentence naming the rule that lost.
  it("always returns a choosable composition, deterministically, for every combination of signals", () => {
    CONTENT_CASES.forEach((signals) => {
      PHOTO_CASES.forEach((photo) => {
        KIT_ORDER.forEach((kit: KitId) => {
          ASPECTS.forEach((aspect) => {
            const id = chooseComposition(signals, photo, kit, FORMATS[aspect]);
            expect(CHOOSABLE_COMPOSITIONS).toContain(id);
            expect(chooseComposition(signals, photo, kit, FORMATS[aspect])).toBe(id);
          });
        });
      });
    });
  });

  it("chooses the poster when there is no photo", () => {
    expect(
      chooseComposition(content({ hasPhoto: false }), null, "editorial", FORMATS["4:5"]),
    ).toBe("posterStack");
  });

  it("keeps the poster when there is no photo whatever the photo analysis says", () => {
    PHOTO_CASES.forEach((photo) => {
      expect(
        chooseComposition(
          content({ hasPhoto: false, hasDiscount: true, dishNameLength: 42 }),
          photo,
          "bold",
          FORMATS["9:16"],
        ),
      ).toBe("posterStack");
    });
  });

  it("leads with the badge when a discount is present", () => {
    expect(
      chooseComposition(content({ hasDiscount: true }), null, "bold", FORMATS["4:5"]),
    ).toBe("badgeHero");
  });

  it("splits the panel for a long dish name", () => {
    expect(
      chooseComposition(content({ dishNameLength: 42 }), null, "editorial", FORMATS["4:5"]),
    ).toBe("splitPanel");
  });

  it("moves text to the top when the photo's negative space is at the top", () => {
    expect(
      chooseComposition(content(), { negativeSpace: "top", busy: false }, "editorial", FORMATS["4:5"]),
    ).toBe("photoTopStack");
  });

  it("uses the corner card when the photo is busy", () => {
    expect(
      chooseComposition(content(), { negativeSpace: "none", busy: true }, "editorial", FORMATS["4:5"]),
    ).toBe("cornerCard");
  });

  // Priority is only observable when two rules both want to fire, and each
  // per-rule test above varies one signal against a neutral background. These
  // three present competing signals, one per adjacent pair in the rule chain
  // from the discount rule down, so a reordered or deleted rule fails a test
  // that names the loser. The fourth adjacent pair - the no-photo rule against
  // everything below it - is covered above by "keeps the poster when there is
  // no photo whatever the photo analysis says".

  it("keeps the discount ahead of the photo analysis", () => {
    expect(
      chooseComposition(
        content({ hasDiscount: true }),
        { negativeSpace: "top", busy: true },
        "bold",
        FORMATS["4:5"],
      ),
    ).toBe("badgeHero");
  });

  it("keeps a busy photo ahead of its negative space", () => {
    expect(
      chooseComposition(content(), { negativeSpace: "top", busy: true }, "editorial", FORMATS["4:5"]),
    ).toBe("cornerCard");
    expect(
      chooseComposition(content(), { negativeSpace: "bottom", busy: true }, "editorial", FORMATS["4:5"]),
    ).toBe("cornerCard");
  });

  it("keeps the photo's negative space ahead of a long dish name", () => {
    const long = content({ dishNameLength: 42 });
    expect(
      chooseComposition(long, { negativeSpace: "bottom", busy: false }, "editorial", FORMATS["4:5"]),
    ).toBe("photoBottomStack");
    expect(
      chooseComposition(long, { negativeSpace: "top", busy: false }, "editorial", FORMATS["4:5"]),
    ).toBe("photoTopStack");
  });

  // Both sides of the boundary: 28 characters is long, 27 is not. Without the
  // shorter half, a chooser whose default branch returned splitPanel would
  // still pass every other test in this file.
  it("treats a dish name as long from 28 characters up", () => {
    expect(
      chooseComposition(content({ dishNameLength: 28 }), null, "editorial", FORMATS["4:5"]),
    ).toBe("splitPanel");
    expect(
      chooseComposition(content({ dishNameLength: 27 }), null, "editorial", FORMATS["4:5"]),
    ).toBe("photoBottomStack");
  });

  // left / right / center have no composition of their own yet, so they fall
  // through to the text-length rule exactly as `none` does. Pinned so the
  // fall-through is a decision rather than an accident; Wave 2 should decide
  // whether the side-anchored cases deserve their own family.
  it("falls through to the text rules for negative space it cannot use", () => {
    (["left", "right", "center", "none"] as const).forEach((negativeSpace) => {
      expect(
        chooseComposition(content(), { negativeSpace, busy: false }, "editorial", FORMATS["4:5"]),
      ).toBe("photoBottomStack");
      expect(
        chooseComposition(
          content({ dishNameLength: 42 }),
          { negativeSpace, busy: false },
          "editorial",
          FORMATS["4:5"],
        ),
      ).toBe("splitPanel");
    });
  });
});

describe("nextComposition", () => {
  // Derived from CHOOSABLE_COMPOSITIONS rather than naming ids: the order of
  // that array is a UX decision owned by compositions.ts, and this test is
  // about the step, not the order.
  it("advances to the next candidate in the list", () => {
    const signals = content();
    expect(
      nextComposition(CHOOSABLE_COMPOSITIONS[0], signals, CHOOSABLE_COMPOSITIONS, DEFAULT_FORMAT),
    ).toBe(CHOOSABLE_COMPOSITIONS[1]);
    expect(
      nextComposition(CHOOSABLE_COMPOSITIONS[2], signals, CHOOSABLE_COMPOSITIONS, DEFAULT_FORMAT),
    ).toBe(CHOOSABLE_COMPOSITIONS[3]);
  });

  it("wraps around at the end of the candidate list", () => {
    const last = CHOOSABLE_COMPOSITIONS[CHOOSABLE_COMPOSITIONS.length - 1];
    expect(nextComposition(last, content(), CHOOSABLE_COMPOSITIONS, DEFAULT_FORMAT)).toBe(
      CHOOSABLE_COMPOSITIONS[0],
    );
  });

  it("visits every candidate exactly once per full rotation", () => {
    const signals = content();
    const visited: CompositionId[] = [];
    let current = CHOOSABLE_COMPOSITIONS[0];
    for (let i = 0; i < CHOOSABLE_COMPOSITIONS.length; i += 1) {
      visited.push(current);
      current = nextComposition(current, signals, CHOOSABLE_COMPOSITIONS, DEFAULT_FORMAT);
    }
    expect(new Set(visited).size).toBe(CHOOSABLE_COMPOSITIONS.length);
    expect(current).toBe(CHOOSABLE_COMPOSITIONS[0]);
  });

  // indexOf returns -1 for a composition that is not a candidate, so the
  // rotation lands on index 0. That is the behaviour we want: an unknown or
  // filtered-out `current` (a legacy id from an old snapshot, say) restarts
  // the rotation at the first usable candidate instead of throwing or
  // returning undefined. Pinned so a refactor cannot change it silently.
  it("restarts at the first candidate when current is not a candidate", () => {
    expect(
      nextComposition("legacyEditorial", content(), CHOOSABLE_COMPOSITIONS, DEFAULT_FORMAT),
    ).toBe(CHOOSABLE_COMPOSITIONS[0]);
  });

  it("skips photo compositions when there is no photo", () => {
    const signals = content({ hasPhoto: false });
    CHOOSABLE_COMPOSITIONS.forEach((current) => {
      const next = nextComposition(current, signals, CHOOSABLE_COMPOSITIONS, DEFAULT_FORMAT);
      expect(COMPOSITIONS[next].requiresPhoto).toBe(false);
    });
  });

  // The filter must consult CompositionDef.requiresPhoto, not a hardcoded
  // "posterStack" id. Every photo-free composition in the registry has to
  // survive the filter, otherwise a photo-free family added in a later wave
  // would drop out of Reshuffle without anyone noticing.
  it("keeps every photo-free composition in the rotation, not just posterStack", () => {
    expect(PHOTO_FREE_COMPOSITIONS.length).toBeGreaterThan(1);
    const signals = content({ hasPhoto: false });
    const visited: CompositionId[] = [];
    let current = PHOTO_FREE_COMPOSITIONS[0];
    for (let i = 0; i < PHOTO_FREE_COMPOSITIONS.length; i += 1) {
      current = nextComposition(current, signals, PHOTO_FREE_COMPOSITIONS, DEFAULT_FORMAT);
      visited.push(current);
    }
    expect(new Set(visited)).toEqual(new Set(PHOTO_FREE_COMPOSITIONS));
  });

  it("returns the current composition when no candidate is usable", () => {
    const signals = content({ hasPhoto: false });
    expect(
      nextComposition("photoBottomStack", signals, ["cornerCard", "splitPanel"], DEFAULT_FORMAT),
    ).toBe("photoBottomStack");
    expect(nextComposition("photoBottomStack", signals, [], DEFAULT_FORMAT)).toBe(
      "photoBottomStack",
    );
  });
});

// `creative_snapshot.composition` is a bare string on the wire — the backend
// bounds its shape but deliberately does not whitelist it, because the id set
// grows every wave. A value with no definition must be rejected here rather
// than reach `COMPOSITIONS[...]` and render nothing.
describe("isCompositionId", () => {
  it("recognises exactly the registered composition ids and nothing else", () => {
    COMPOSITION_ORDER.forEach((id) => expect(isCompositionId(id)).toBe(true));
    [
      "",
      " posterStack ",
      "PosterStack",
      "poster",
      "chalkboard",
      "hologram",
      "constructor",
      null,
      undefined,
      3,
      [],
    ].forEach((value) => expect(isCompositionId(value)).toBe(false));
  });

  it("narrows an unknown wire value to a composition that has a definition", () => {
    const fromWire: string = "hologram";
    const id: CompositionId = isCompositionId(fromWire)
      ? fromWire
      : chooseComposition(content(), null, "editorial", FORMATS["4:5"]);
    expect(COMPOSITIONS[id]).toBeDefined();
  });
});

// Read-back / seed path: the composer always carries a kit, so a stored
// legacy* family must not round-trip into PostCreative. isCompositionId still
// accepts the full registry (render-side lookup, unknown-build rejection);
// isChoosableCompositionId is the narrower guard for anything that will land
// on the kit path.
describe("isChoosableCompositionId", () => {
  it("recognises exactly the choosable composition ids", () => {
    CHOOSABLE_COMPOSITIONS.forEach((id) =>
      expect(isChoosableCompositionId(id)).toBe(true),
    );
  });

  it("rejects every legacy family even though isCompositionId accepts them", () => {
    (["legacyEditorial", "legacyBold", "legacyMinimal"] as const).forEach(
      (id) => {
        expect(isCompositionId(id)).toBe(true);
        expect(isChoosableCompositionId(id)).toBe(false);
      },
    );
  });

  it("rejects unknown and malformed wire values", () => {
    [
      "",
      " posterStack ",
      "PosterStack",
      "hologram",
      null,
      undefined,
      3,
    ].forEach((value) => expect(isChoosableCompositionId(value)).toBe(false));
  });
});

// Reshuffle is a no-op whenever fewer than two candidates survive the photo
// filter, so the editor has to disable the control rather than offer a button
// that does nothing. It can only do that honestly if it asks the same filter
// `nextComposition` rotates through — hence one exported predicate, not two
// copies of the same condition that can drift apart.
describe("usableCompositions", () => {
  it("is the exact candidate set nextComposition rotates through", () => {
    ([true, false] as const).forEach((hasPhoto) => {
      const signals = content({ hasPhoto });
      const usable = usableCompositions(signals, CHOOSABLE_COMPOSITIONS, DEFAULT_FORMAT);
      const visited = new Set<CompositionId>();
      let current = usable[0];
      for (let i = 0; i < usable.length; i += 1) {
        current = nextComposition(current, signals, CHOOSABLE_COMPOSITIONS, DEFAULT_FORMAT);
        visited.add(current);
      }
      expect(visited).toEqual(new Set(usable));
    });
  });

  it("drops every photo-requiring family when the post has no photo", () => {
    const usable = usableCompositions(
      content({ hasPhoto: false }),
      CHOOSABLE_COMPOSITIONS,
      DEFAULT_FORMAT,
    );
    usable.forEach((id) => expect(COMPOSITIONS[id].requiresPhoto).toBe(false));
    // Wave 1 leaves exactly one photo-free choosable family, which is why the
    // no-photo rotation is a fixed point and Reshuffle must be disabled there.
    expect(usable).toEqual(["posterStack"]);
  });

  it("keeps every candidate when the post has a photo", () => {
    expect(
      usableCompositions(content({ hasPhoto: true }), CHOOSABLE_COMPOSITIONS, DEFAULT_FORMAT),
    ).toEqual([...CHOOSABLE_COMPOSITIONS]);
  });
});

describe("contentSignalsFrom", () => {
  it("reads presence from the resolved slot values", () => {
    const signals = contentSignalsFrom({
      play: "offer",
      slots: { dishName: "Milanesa", badge: "20% OFF", cta: "Order", handle: "@casa" },
      hasPhoto: true,
      hasDiscount: true,
    });
    expect(signals).toEqual({
      play: "offer",
      dishNameLength: 8,
      hasPrice: false,
      hasDiscount: true,
      hasHandle: true,
      hasPhoto: true,
    });
  });

  it("treats whitespace-only slots as absent", () => {
    const signals = contentSignalsFrom({
      play: "",
      slots: { dishName: "  ", handle: "" },
      hasPhoto: false,
      hasDiscount: false,
    });
    expect(signals.dishNameLength).toBe(0);
    expect(signals.hasHandle).toBe(false);
  });
});

describe("chooser is format-aware", () => {
  const withPhoto: ContentSignals = {
    play: "featured_dish",
    dishNameLength: 12,
    hasPrice: true,
    hasDiscount: false,
    hasHandle: true,
    hasPhoto: true,
  };

  it("never returns a composition that refuses the format", () => {
    FORMAT_ORDER.forEach((formatId) => {
      const format = FORMATS[formatId];
      const chosen = chooseComposition(withPhoto, null, "editorial", format);
      expect(COMPOSITIONS[chosen].supportsFormat(format)).toBe(true);
    });
  });

  it("keeps its pre-Wave-4 answers on the three legacy formats", () => {
    (["1:1", "4:5", "9:16"] as const).forEach((formatId) => {
      const format = FORMATS[formatId];
      expect(chooseComposition(withPhoto, null, "editorial", format)).toBe(
        "photoBottomStack",
      );
      expect(
        chooseComposition(
          { ...withPhoto, hasPhoto: false },
          null,
          "editorial",
          format,
        ),
      ).toBe("posterStack");
      expect(
        chooseComposition(
          { ...withPhoto, hasDiscount: true },
          null,
          "editorial",
          format,
        ),
      ).toBe("badgeHero");
      expect(
        chooseComposition(
          { ...withPhoto, dishNameLength: 28 },
          null,
          "editorial",
          format,
        ),
      ).toBe("splitPanel");
    });
  });

  // Reshuffle's rotation must not offer a family that cannot render this canvas.
  it("filters the rotation by supportsFormat as well as by photo", () => {
    const format = FORMATS["4:5"];
    const usable = usableCompositions(withPhoto, COMPOSITION_ORDER, format);
    usable.forEach((id) => {
      expect(COMPOSITIONS[id].supportsFormat(format)).toBe(true);
    });
    // Legacy families support every registry format whose `legacyAspect` is set,
    // so on 4:5 they remain usable when the full order is passed. Reshuffle
    // itself only ever hands CHOOSABLE_COMPOSITIONS, which excludes them by
    // construction. Task 11's non-legacy formats make supportsFormat the
    // second gate that drops legacy ids even when the full order is passed.
    const choosable = usableCompositions(
      withPhoto,
      CHOOSABLE_COMPOSITIONS,
      format,
    );
    expect(choosable).not.toContain("legacyEditorial");
    choosable.forEach((id) => {
      expect(COMPOSITIONS[id].supportsFormat(format)).toBe(true);
    });
  });
});
