import { resolvePrintPalette } from "../palette";
import {
  atelierFamily,
  counterFamily,
  fieldFamily,
  galleryFamily,
  maisonFamily,
  nightHouseFamily,
  osteriaFamily,
  streetFamily,
} from "../families";
import type {
  FeaturedImageSelection,
  MenuArtDirection,
  PlannedRect,
  PrintMenuSection,
  RegisteredMenuDesignFamily,
} from "../types";
import {
  SEMANTIC_MARKERS,
  estimatePlannedItemHeight,
  normalizeSemanticMarkers,
  planBalancedSection,
  planCompactSection,
  planPhotoLedSection,
  planTypeLedSection,
  resolvePlannedItemWidth,
  type SectionPlan,
} from "./strategies";

const rect: PlannedRect = { xMm: 10, yMm: 20, widthMm: 160, heightMm: 210 };
const section: PrintMenuSection = {
  id: "starters",
  name: "Starters",
  description: "Bright plates to begin.",
  items: [
    {
      id: "one",
      name: "Market vegetables",
      description: "Tender vegetables with herbs and citrus.",
      price: 12,
      imageCandidates: ["guessed.jpg"],
      dietaryTags: ["Vegan", "spicy"],
      allergens: ["tree nuts"],
    },
    {
      id: "two",
      name: "Seasonal soup",
      description: "Slow cooked produce from nearby farms.",
      price: 10,
      imageCandidates: ["also-guessed.jpg"],
      dietaryTags: ["vegetarian"],
      allergens: [],
    },
    {
      id: "three",
      name: "Fresh bread",
      description: "Warm from the oven.",
      price: 8,
      dietaryTags: [],
      allergens: ["gluten"],
    },
  ],
};

function direction(
  family: RegisteredMenuDesignFamily,
  imageSelections: FeaturedImageSelection[] = [],
): MenuArtDirection {
  return {
    familyId: family.id,
    treatment: "balanced",
    outputFormat: family.supportedFormats[0],
    paperFormat: "a4",
    palette: resolvePrintPalette({ familyId: family.id }),
    imageSelections,
    imageFocalPoints: {},
    logoTreatment: "contained",
    coverMode: "none",
    ornamentIntensity: "restrained",
    typographyPersonality: "editorial",
    contactPlacement: "footer",
  };
}

function selectedRole(
  family: RegisteredMenuDesignFamily,
): FeaturedImageSelection["role"] {
  return {
    atelier: "editorial-crop",
    maison: "half-page",
    osteria: "category-opener",
    "night-house": "editorial-crop",
    counter: "full-width",
    street: "full-width",
    field: "category-opener",
    gallery: "editorial-crop",
  }[family.id] as FeaturedImageSelection["role"];
}

function plan(
  family: RegisteredMenuDesignFamily,
  treatment: "type-led" | "balanced" | "photo-led" | "compact" = "balanced",
): SectionPlan {
  const planners = {
    "type-led": planTypeLedSection,
    balanced: planBalancedSection,
    "photo-led": planPhotoLedSection,
    compact: planCompactSection,
  };
  return planners[treatment]({
    contentRect: rect,
    section,
    direction: direction(family, [
      { itemId: "one", url: "selected.jpg", role: selectedRole(family) },
    ]),
    family,
    includeHeading: true,
  });
}

function expectContained(result: SectionPlan): void {
  expect(result.blocks.length).toBeGreaterThan(0);
  for (const block of result.blocks) {
    expect(block.rect.xMm).toBeGreaterThanOrEqual(rect.xMm);
    expect(block.rect.yMm).toBeGreaterThanOrEqual(rect.yMm);
    expect(block.rect.widthMm).toBeGreaterThanOrEqual(0);
    expect(block.rect.heightMm).toBeGreaterThanOrEqual(0);
    expect(block.rect.xMm + block.rect.widthMm).toBeLessThanOrEqual(
      rect.xMm + rect.widthMm,
    );
    expect(block.rect.yMm + block.rect.heightMm).toBeLessThanOrEqual(
      rect.yMm + rect.heightMm,
    );
  }
}

describe("section planners", () => {
  it.each([
    planTypeLedSection,
    planBalancedSection,
    planPhotoLedSection,
    planCompactSection,
  ])("returns complete atomic, in-bounds item blocks", (planner) => {
    const result = planner({
      contentRect: rect,
      section,
      direction: direction(atelierFamily),
      family: atelierFamily,
      includeHeading: true,
    });
    expectContained(result);
    expect(
      result.blocks
        .filter((block) => block.itemIds)
        .flatMap((block) => block.itemIds),
    ).toEqual(section.items.map((item) => item.id));
    expect(
      result.blocks
        .filter((block) => block.itemIds)
        .every((block) => block.itemIds?.length === 1),
    ).toBe(true);
  });

  it("uses only explicitly selected images", () => {
    const withoutSelection = planPhotoLedSection({
      contentRect: rect,
      section,
      direction: direction(galleryFamily),
      family: galleryFamily,
      includeHeading: true,
    });
    expect(
      withoutSelection.blocks.some((block) => block.kind === "image"),
    ).toBe(false);

    const selected = planPhotoLedSection({
      contentRect: rect,
      section,
      direction: direction(galleryFamily, [
        { itemId: "one", url: "selected.jpg", role: "half-page" },
      ]),
      family: galleryFamily,
      includeHeading: true,
    });
    expect(
      selected.blocks
        .filter((block) => block.image)
        .map((block) => block.image?.url),
    ).toEqual(["selected.jpg"]);
    expect(selected.blocks.find((block) => block.image)?.image?.role).toBe(
      "half-page",
    );
  });

  it("rejects an image role unsupported by the selected family", () => {
    expect(() =>
      planPhotoLedSection({
        contentRect: rect,
        section,
        direction: direction(galleryFamily, [
          { itemId: "one", url: "selected.jpg", role: "compact-tile" },
        ]),
        family: galleryFamily,
        includeHeading: true,
      }),
    ).toThrow("Menu family gallery does not support image role compact-tile");
  });

  it("exports fixed monochrome semantic marks and allergen initials", () => {
    expect(SEMANTIC_MARKERS).toEqual({
      vegetarian: "V",
      vegan: "VG",
      "gluten-free": "GF",
      spicy: "S",
    });
    const markers = normalizeSemanticMarkers(section.items[0]);
    expect(markers.map((marker) => marker.mark)).toEqual(["VG", "S", "TN"]);
    expect(markers.every(({ mark }) => /^[A-Z-]+$/.test(mark))).toBe(true);
    expect(JSON.stringify(markers)).not.toMatch(/[\u{1F300}-\u{1FAFF}]/u);
  });

  it("extracts Unicode allergen initials and skips emoji-only labels", () => {
    const markers = normalizeSemanticMarkers({
      ...section.items[0],
      dietaryTags: [],
      allergens: ["🔥", "🔥 sesame", "سمسم", "魚"],
    });
    expect(markers.map(({ mark }) => mark)).toEqual(["S", "س", "魚"]);
  });

  it("sizes atomic item blocks from measured content at the family floor", () => {
    const result = planTypeLedSection({
      contentRect: rect,
      section: {
        ...section,
        items: [
          { ...section.items[0], description: "Short." },
          {
            ...section.items[1],
            description:
              "A deliberately longer description with enough ingredients and preparation detail to wrap over several measured lines.",
          },
        ],
      },
      direction: direction(atelierFamily),
      family: atelierFamily,
      includeHeading: true,
    });
    const items = result.blocks.filter(
      (block) => block.kind === "item-list" || block.kind === "item-feature",
    );
    expect(items[1].rect.heightMm).toBeGreaterThan(items[0].rect.heightMm);
    expect(items.every((block) => block.rect.heightMm > 0)).toBe(true);
  });

  it("uses conservative script-aware glyph widths for CJK content", () => {
    const latin = estimatePlannedItemHeight({
      item: {
        ...section.items[0],
        name: "a".repeat(200),
        description: undefined,
      },
      widthMm: 80,
      family: atelierFamily,
      treatment: "type-led",
    });
    const cjk = estimatePlannedItemHeight({
      item: {
        ...section.items[0],
        name: "界".repeat(200),
        description: undefined,
      },
      widthMm: 80,
      family: atelierFamily,
      treatment: "type-led",
    });
    expect(cjk.heightMm).toBeGreaterThan(latin.heightMm);
  });

  it("reserves the rendered semantic-marker row in every measured item", () => {
    const withMarkers = estimatePlannedItemHeight({
      item: section.items[0],
      widthMm: 80,
      family: atelierFamily,
      treatment: "balanced",
    });
    const withoutMarkers = estimatePlannedItemHeight({
      item: { ...section.items[0], dietaryTags: [], allergens: [] },
      widthMm: 80,
      family: atelierFamily,
      treatment: "balanced",
    });

    expect(
      withMarkers.heightMm - withoutMarkers.heightMm,
    ).toBeGreaterThanOrEqual(2.5);
  });

  it("never silently clamps a measured item into an undersized rectangle", () => {
    expect(() =>
      planTypeLedSection({
        contentRect: { xMm: 0, yMm: 0, widthMm: 80, heightMm: 12 },
        section: {
          ...section,
          items: [{ ...section.items[0], description: "detail ".repeat(30) }],
        },
        direction: direction(atelierFamily),
        family: atelierFamily,
        includeHeading: true,
      }),
    ).toThrow("measured item block exceeds content rect");
  });
});

describe("family composition dispatch", () => {
  it("uses Atelier editorial asymmetry with a narrow masthead and 62/38 split", () => {
    const result = plan(atelierFamily);
    const heading = result.blocks.find(
      (block) => block.kind === "category-heading",
    )!;
    const image = result.blocks.find((block) => block.kind === "image")!;
    expect(heading.rect.widthMm / rect.widthMm).toBeLessThanOrEqual(0.34);
    expect(heading.rect.yMm).toBeGreaterThan(rect.yMm);
    expect(image.rect.widthMm / rect.widthMm).toBeCloseTo(0.38, 1);
    expect(
      result.blocks.find(
        (block) => block.kind === "item-list" || block.kind === "item-feature",
      )!.rect.widthMm / rect.widthMm,
    ).toBeCloseTo(0.62, 2);
    expect(image.rect.xMm).toBeCloseTo(rect.xMm + rect.widthMm * 0.62, 2);
  });

  it("reserves the full rendered height of a wrapped Osteria category", () => {
    const result = planBalancedSection({
      contentRect: { xMm: 0, yMm: 0, widthMm: 133, heightMm: 180 },
      section: { ...section, name: "From the Parrilla" },
      direction: direction(osteriaFamily),
      family: osteriaFamily,
      includeHeading: true,
    });
    const heading = result.blocks.find(
      (block) => block.kind === "category-heading",
    )!;
    const firstItem = result.blocks.find(
      (block) => block.kind === "item-list" || block.kind === "item-feature",
    )!;

    expect(heading.rect.heightMm).toBeGreaterThan(11.5);
    expect(firstItem.rect.yMm).toBeGreaterThanOrEqual(
      heading.rect.yMm + heading.rect.heightMm + 4,
    );
  });

  it("uses Maison's centered course axis, 8mm gaps, and one feature", () => {
    const result = plan(maisonFamily, "photo-led");
    const heading = result.blocks.find(
      (block) => block.kind === "category-heading",
    )!;
    const items = result.blocks.filter(
      (block) => block.kind === "item-list" || block.kind === "item-feature",
    );
    expect(heading.rect.xMm + heading.rect.widthMm / 2).toBeCloseTo(
      rect.xMm + rect.widthMm / 2,
    );
    expect(
      items[1].rect.yMm - (items[0].rect.yMm + items[0].rect.heightMm),
    ).toBeGreaterThanOrEqual(8);
    expect(
      result.blocks.filter((block) => block.kind === "image"),
    ).toHaveLength(1);
  });

  it.each(["balanced", "compact"] as const)(
    "places Maison half-page selections in %s treatment",
    (treatment) => {
      const result = plan(maisonFamily, treatment);
      const image = result.blocks.find((block) => block.kind === "image")!;
      expect(image.image).toMatchObject({
        url: "selected.jpg",
        role: "half-page",
      });
      expect(image.rect.widthMm / rect.widthMm).toBeCloseTo(0.5, 1);
    },
  );

  it("uses Osteria's unequal 54/46 columns with a print-safe gutter", () => {
    const result = plan(osteriaFamily);
    const items = result.blocks.filter(
      (block) => block.kind === "item-list" || block.kind === "item-feature",
    );
    const columns = [...new Set(items.map((block) => block.rect.xMm))]
      .sort((first, second) => first - second)
      .map((xMm) => items.find((block) => block.rect.xMm === xMm)!.rect);
    const gutter = columns[1].xMm - (columns[0].xMm + columns[0].widthMm);
    const availableWidth = rect.widthMm - gutter;

    expect(gutter).toBeGreaterThanOrEqual(5);
    expect(columns[0].widthMm / availableWidth).toBeCloseTo(0.54, 2);
    expect(columns[1].widthMm / availableWidth).toBeCloseTo(0.46, 2);
    // Headings are measured against the rendered display size so section
    // descriptions never clip; the box grows with copy but stays bounded.
    const headingHeight = result.blocks.find(
      (block) => block.kind === "category-heading",
    )!.rect.heightMm;
    expect(headingHeight).toBeGreaterThanOrEqual(8);
    expect(headingHeight).toBeLessThanOrEqual(18);
  });

  it("uses only Osteria's selected category-opener image", () => {
    const result = plan(osteriaFamily);
    expect(
      result.blocks
        .filter((block) => block.kind === "image")
        .map((block) => block.image?.url),
    ).toEqual(["selected.jpg"]);
    expect(
      result.blocks.find((block) => block.kind === "image")?.image?.role,
    ).toBe("category-opener");
  });

  it("uses Night House transitions and safe non-overlay crops", () => {
    const result = plan(nightHouseFamily, "photo-led");
    const heading = result.blocks.find(
      (block) => block.kind === "category-heading",
    )!;
    const image = result.blocks.find((block) => block.kind === "image")!;
    expect(heading.rect.heightMm).toBe(18);
    expect([3 / 2, 4 / 5]).toContain(
      Number((image.rect.widthMm / image.rect.heightMm).toFixed(2)),
    );
    expect(
      result.blocks
        .filter(
          (block) =>
            block.kind === "item-list" || block.kind === "item-feature",
        )
        .every(
          (block) => block.rect.yMm >= image.rect.yMm + image.rect.heightMm,
        ),
    ).toBe(true);
  });

  it("uses a compact cinematic photo band on a Night House drinks card", () => {
    const result = planPhotoLedSection({
      contentRect: rect,
      section,
      direction: {
        ...direction(nightHouseFamily, [
          { itemId: "one", url: "selected.jpg", role: "full-width" },
        ]),
        outputFormat: "drinks-card",
        paperFormat: "a5",
      },
      family: nightHouseFamily,
      includeHeading: true,
    });
    const image = result.blocks.find((block) => block.kind === "image")!;

    expect(image.rect.heightMm).toBe(42);
    expect(image.rect.widthMm).toBe(rect.widthMm);
  });

  it("uses Counter content-sized modules with one dominant special", () => {
    const result = planCompactSection({
      contentRect: rect,
      section: {
        ...section,
        items: [
          { ...section.items[0], description: "Short." },
          { ...section.items[1], description: "Long detail ".repeat(16) },
          {
            ...section.items[2],
            description: "Medium detail over a couple of lines.",
          },
        ],
      },
      direction: direction(counterFamily),
      family: counterFamily,
      includeHeading: true,
    });
    const items = result.blocks.filter((block) => block.itemIds);
    expect(items[0].kind).toBe("item-feature");
    expect(items[0].rect.widthMm).toBeGreaterThan(items[1].rect.widthMm);
    expect(
      new Set(items.slice(0, 3).map((block) => block.rect.widthMm)).size,
    ).toBeGreaterThan(1);
    expect(
      new Set(items.map((block) => block.rect.heightMm)).size,
    ).toBeGreaterThan(1);
    expect(items.every((block) => block.rect.heightMm > 0)).toBe(true);
  });

  it("budgets Counter tile borders and padding around measured copy", () => {
    const result = planCompactSection({
      contentRect: rect,
      section,
      direction: direction(counterFamily),
      family: counterFamily,
      includeHeading: true,
    });
    const items = result.blocks.filter((block) => block.itemIds);
    const firstCopyHeight = estimatePlannedItemHeight({
      item: section.items[0],
      widthMm: items[0].rect.widthMm,
      family: counterFamily,
      treatment: "compact",
    }).heightMm;
    const secondCopyHeight = estimatePlannedItemHeight({
      item: section.items[1],
      widthMm: items[1].rect.widthMm,
      family: counterFamily,
      treatment: "compact",
    }).heightMm;

    expect(items[0].rect.heightMm - firstCopyHeight).toBeCloseTo(7, 10);
    expect(items[1].rect.heightMm - secondCopyHeight).toBeCloseTo(3, 10);
  });

  it.each(["balanced", "photo-led", "compact"] as const)(
    "places Counter full-width selections in %s treatment",
    (treatment) => {
      const result = plan(counterFamily, treatment);
      const image = result.blocks.find((block) => block.kind === "image")!;
      expect(image.image).toMatchObject({
        url: "selected.jpg",
        role: "full-width",
      });
      expect(image.rect.widthMm).toBe(rect.widthMm);
    },
  );

  it("uses Street condensed rails without truncating long descriptions", () => {
    const verbose = {
      ...section,
      items: [{ ...section.items[0], description: "ingredient ".repeat(80) }],
    };
    const result = planCompactSection({
      contentRect: rect,
      section: verbose,
      direction: direction(streetFamily),
      family: streetFamily,
      includeHeading: true,
    });
    // The scan bar is measured, not a fixed strip, so a section description
    // can never clip inside it; it still stays a bar, not a block.
    const scanBarHeight = result.blocks.find(
      (block) => block.kind === "category-heading",
    )!.rect.heightMm;
    expect(scanBarHeight).toBeGreaterThanOrEqual(8);
    expect(scanBarHeight).toBeLessThanOrEqual(18);
    expect(result.diagnostics).toEqual([]);
    expect(
      result.blocks.find((block) => block.itemIds)!.rect.heightMm,
    ).toBeGreaterThan(30);
  });

  it("gives photo-led sections a visibly larger image composition", () => {
    const balanced = plan(galleryFamily, "balanced");
    const photoLed = plan(galleryFamily, "photo-led");
    const balancedImage = balanced.blocks.find(
      (block) => block.kind === "image",
    )!;
    const photoLedImage = photoLed.blocks.find(
      (block) => block.kind === "image",
    )!;

    expect(photoLedImage.rect.heightMm).toBeGreaterThan(
      balancedImage.rect.heightMm * 1.5,
    );
    expect(photoLed.consumedHeightMm).not.toBe(balanced.consumedHeightMm);
  });

  it("uses Street's selected full-width photo band and aligned price rail", () => {
    const result = plan(streetFamily, "balanced");
    const image = result.blocks.find((block) => block.kind === "image")!;
    const items = result.blocks.filter(
      (block) => block.kind === "item-list" || block.kind === "item-feature",
    );
    expect(image.image?.url).toBe("selected.jpg");
    expect(image.image?.role).toBe("full-width");
    expect(image.rect.widthMm).toBe(rect.widthMm);
    expect(
      new Set(items.map((block) => block.rect.xMm + block.rect.widthMm)).size,
    ).toBe(1);
  });

  it("uses Field's airy 12mm section opening and narrow description measure", () => {
    const result = plan(fieldFamily);
    const heading = result.blocks.find(
      (block) => block.kind === "category-heading",
    )!;
    const item = result.blocks.find((block) => block.itemIds)!;
    expect(
      item.rect.yMm - (heading.rect.yMm + heading.rect.heightMm),
    ).toBeGreaterThanOrEqual(12);
    expect(item.rect.widthMm).toBeLessThanOrEqual(115);
  });

  it("uses Field's selected sparse category-opener image", () => {
    const result = plan(fieldFamily, "balanced");
    expect(
      result.blocks.filter((block) => block.kind === "image"),
    ).toHaveLength(1);
    expect(
      result.blocks.find((block) => block.kind === "image")?.image?.url,
    ).toBe("selected.jpg");
    expect(
      result.blocks.find((block) => block.kind === "image")?.image?.role,
    ).toBe("category-opener");
  });

  it("uses Gallery image-led negative space without item thumbnails", () => {
    const result = plan(galleryFamily, "photo-led");
    const image = result.blocks.find((block) => block.kind === "image")!;
    expect(image.rect.widthMm * image.rect.heightMm).toBeGreaterThanOrEqual(
      rect.widthMm * rect.heightMm * 0.45,
    );
    expect(image.rect.widthMm * image.rect.heightMm).toBeLessThanOrEqual(
      rect.widthMm * rect.heightMm * 0.62,
    );
    expect(
      result.blocks.filter((block) => block.kind === "image"),
    ).toHaveLength(1);
    expect(
      result.blocks
        .filter(
          (block) =>
            block.kind === "item-list" || block.kind === "item-feature",
        )
        .every((block) => !block.image),
    ).toBe(true);
  });

  it("gives Gallery a full editorial measure whenever no photo owns the row", () => {
    const base = {
      family: galleryFamily,
      contentWidthMm: rect.widthMm,
      selectedImageRole: "half-page" as const,
      itemIndex: 0,
      ornamentIntensity: "restrained" as const,
    };
    expect(
      resolvePlannedItemWidth({
        ...base,
        treatment: "balanced",
        hasSelectedImage: true,
      }),
    ).toBe(rect.widthMm);
    expect(
      resolvePlannedItemWidth({
        ...base,
        treatment: "photo-led",
        hasSelectedImage: false,
      }),
    ).toBe(rect.widthMm);
    expect(
      resolvePlannedItemWidth({
        ...base,
        treatment: "photo-led",
        hasSelectedImage: true,
      }),
    ).toBeCloseTo(rect.widthMm * 0.39, 2);
  });

  it.each([
    [atelierFamily, "full-width", 1],
    [atelierFamily, "paired", 0.48],
    [osteriaFamily, "compact-tile", 0.32],
    [nightHouseFamily, "full-width", 1],
    [counterFamily, "category-opener", 0.55],
    [counterFamily, "compact-tile", 0.32],
    [streetFamily, "compact-tile", 0.32],
    [fieldFamily, "half-page", 0.5],
  ] as const)(
    "uses %s %s role geometry",
    (family, role, expectedWidthRatio) => {
      const result = planBalancedSection({
        contentRect: rect,
        section,
        direction: direction(family, [
          { itemId: "one", url: "selected.jpg", role },
        ]),
        family,
        includeHeading: true,
      });
      const image = result.blocks.find((block) => block.kind === "image")!;
      expect(image.image?.role).toBe(role);
      expect(image.rect.widthMm / rect.widthMm).toBeCloseTo(
        expectedWidthRatio,
        2,
      );
    },
  );
});
