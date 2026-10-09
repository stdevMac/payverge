import {
  cjkRestaurantMenu,
  denseRestaurantMenu,
  hebrewRestaurantMenu,
  photoRichRestaurantMenu,
  rtlRestaurantMenu,
  sparseRestaurantMenu,
} from "../__fixtures__/restaurantMenus";
import {
  makeAudit,
  makeDirection,
  planFixtureDocument,
} from "../__fixtures__/plannedDocuments";
import { selectDefaultPrintImages } from "../directionDefaults";
import { galleryFamily } from "../families";
import type { PlannedMenuDocument, PrintMenuModel } from "../types";
import { planMenuDocument } from "./planDocument";
import { estimatePlannedItemHeight } from "./strategies";

function expectPhysicalBounds(document: PlannedMenuDocument): void {
  const { geometry } = document;
  for (const page of document.pages) {
    for (const block of page.blocks) {
      expect(block.rect.xMm).toBeGreaterThanOrEqual(geometry.safeArea.left);
      expect(block.rect.yMm).toBeGreaterThanOrEqual(geometry.safeArea.top);
      expect(block.rect.widthMm).toBeGreaterThanOrEqual(0);
      expect(block.rect.heightMm).toBeGreaterThanOrEqual(0);
      expect(block.rect.xMm + block.rect.widthMm).toBeLessThanOrEqual(
        geometry.widthMm - geometry.safeArea.right,
      );
      expect(block.rect.yMm + block.rect.heightMm).toBeLessThanOrEqual(
        geometry.heightMm - geometry.safeArea.bottom,
      );
    }
  }
}

function assertNoOrphans(document: PlannedMenuDocument): void {
  for (const page of document.pages) {
    page.blocks.forEach((block, index) => {
      if (block.kind === "category-heading") {
        expect(
          page.blocks
            .slice(index + 1)
            .some(
              (candidate) =>
                candidate.sectionId === block.sectionId &&
                candidate.itemIds?.length,
            ),
        ).toBe(true);
      }
    });
  }
}

function expectTrifoldPanels(document: PlannedMenuDocument): void {
  expect(document.pages).toHaveLength(2);
  const [firstFold, secondFold] = document.geometry.foldsMm;
  const panels = [
    [document.geometry.safeArea.left, firstFold - 3],
    [firstFold + 3, secondFold - 3],
    [
      secondFold + 3,
      document.geometry.widthMm - document.geometry.safeArea.right,
    ],
  ];
  for (const page of document.pages) {
    for (const block of page.blocks) {
      expect(
        panels.some(
          ([left, right]) =>
            block.rect.xMm >= left &&
            block.rect.xMm + block.rect.widthMm <= right,
        ),
      ).toBe(true);
    }
  }
}

describe("planMenuDocument", () => {
  it("blocks a document with no printable items", () => {
    const document = planFixtureDocument({
      model: {
        business: { name: "Empty Room" },
        sections: [],
        currency: "USD",
        language: "en",
        warnings: [],
      },
    });

    expect(document.readiness).toBe("blocked");
    expect(document.diagnostics).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "menu:empty",
          messageKey: "print.emptyMenu",
          severity: "blocked",
        }),
      ]),
    );
  });

  it.each([
    ["sparse", sparseRestaurantMenu],
    ["dense", denseRestaurantMenu],
    ["photo-rich", photoRichRestaurantMenu],
    ["RTL", rtlRestaurantMenu],
    ["Hebrew RTL", hebrewRestaurantMenu],
    ["CJK", cjkRestaurantMenu],
  ])(
    "plans %s menus deterministically within physical bounds",
    (_name, model) => {
      const input = {
        model,
        audit: makeAudit(model),
        direction: makeDirection(),
      };
      const first = planMenuDocument(input);
      const second = planMenuDocument(input);
      expect(first).toEqual(second);
      expect(first).not.toBe(second);
      expectPhysicalBounds(first);
      assertNoOrphans(first);
      if (first.readiness !== "blocked") {
        expect(
          first.pages
            .flatMap(({ blocks }) => blocks)
            .filter(
              (block) =>
                block.kind === "item-list" || block.kind === "item-feature",
            )
            .map((block) => block.itemIds?.[0])
            .sort(),
        ).toEqual(
          model.sections
            .flatMap(({ items }) => items.map(({ id }) => id))
            .sort(),
        );
      }
    },
  );

  it("paginates before an atomic heading-and-first-item pair or item overflow", () => {
    const document = planFixtureDocument(denseRestaurantMenu, {
      direction: { outputFormat: "folded-booklet" },
    });
    assertNoOrphans(document);
    expectPhysicalBounds(document);
    expect(
      document.pages
        .flatMap((page) => page.blocks)
        .filter((block) => block.itemIds)
        .every((block) => block.itemIds?.length === 1),
    ).toBe(true);
  });

  it.each([
    ["field", "half-letter"],
    ["gallery", "a4"],
    ["night-house", "a5"],
    ["atelier", "a4"],
  ] as const)(
    "centers the %s masthead geometry on the physical %s page",
    (familyId, paperFormat) => {
      const document = planFixtureDocument({
        model:
          familyId === "night-house"
            ? photoRichRestaurantMenu
            : sparseRestaurantMenu,
        familyId,
        treatment: "balanced",
        outputFormat: "single-sheet",
        paperFormat,
      });
      const page = document.pages[0];
      const masthead = page.blocks.find((block) => block.kind === "masthead")!;
      const firstContentTop = Math.min(
        ...page.blocks
          .filter(
            (block) =>
              block.kind === "category-heading" || block.kind === "image",
          )
          .map((block) => block.rect.yMm),
      );

      expect(masthead.rect.widthMm).toBeGreaterThan(
        document.geometry.widthMm * 0.4,
      );
      expect(masthead.rect.heightMm).toBeGreaterThan(20);
      const physicalCenter = document.geometry.widthMm / 2;
      const mastheadCenter = masthead.rect.xMm + masthead.rect.widthMm / 2;
      expect(mastheadCenter).toBeCloseTo(physicalCenter, 1);
      expect(firstContentTop).toBeGreaterThanOrEqual(
        masthead.rect.yMm + masthead.rect.heightMm + 5,
      );
    },
  );

  it("reserves masthead room for a long name that wraps at rendered size", () => {
    // "Payverge AI Pro Demo Lounge" wraps to two rendered lines at atelier's
    // 22pt × 1.45 display size; the planner must reserve both lines so the
    // second is never clipped by the masthead block.
    const document = planFixtureDocument({
      model: {
        ...sparseRestaurantMenu,
        business: {
          ...sparseRestaurantMenu.business,
          name: "Payverge AI Pro Demo Lounge",
          tagline: undefined,
          logoUrl: undefined,
        },
      },
      familyId: "atelier",
      treatment: "type-led",
      outputFormat: "single-sheet",
      paperFormat: "a4",
    });
    const masthead = document.pages[0].blocks.find(
      (block) => block.kind === "masthead",
    )!;
    expect(masthead.rect.heightMm).toBeGreaterThanOrEqual(27.5);
    expect(document.readiness).not.toBe("blocked");
  });

  it.each([
    ["atelier", "a4"],
    ["field", "a4"],
    ["counter", "letter"],
  ] as const)(
    "seats the whole %s cluster, masthead included, in the middle of a sparse sheet",
    (familyId, paperFormat) => {
      // A short menu used to hang off the top edge with every millimetre of
      // leftover depth dumped underneath it. The masthead is part of the
      // cluster, not a fixed header, so it travels with the courses.
      const document = planFixtureDocument({
        model: sparseRestaurantMenu,
        familyId,
        treatment: "balanced",
        outputFormat: "single-sheet",
        paperFormat,
      });
      const page = document.pages[0];
      const masthead = page.blocks.find((block) => block.kind === "masthead")!;
      const footer = page.blocks.find((block) => block.kind === "footer")!;
      const content = page.blocks.filter(
        (block) => block.kind !== "footer" && block.kind !== "folio",
      );
      const top = Math.min(...content.map((block) => block.rect.yMm));
      const bottom = Math.max(
        ...content.map((block) => block.rect.yMm + block.rect.heightMm),
      );

      expect(top).toBe(masthead.rect.yMm);
      expect(masthead.rect.yMm).toBeGreaterThan(
        document.geometry.safeArea.top + 15,
      );

      // Optically centred rather than mathematically: a mass reads as sunken
      // when the air above and below it is exactly equal.
      const above = top - document.geometry.safeArea.top;
      const below = footer.rect.yMm - bottom;
      expect(above / (above + below)).toBeGreaterThan(0.35);
      expect(above / (above + below)).toBeLessThan(0.5);
    },
  );

  it("spends leftover depth on the gaps between courses, not on the masthead lockup", () => {
    // The masthead-to-first-course distance is a designed relationship. Only
    // the course-to-course gaps absorb slack, and only up to 9mm each.
    const document = planFixtureDocument({
      model: sparseRestaurantMenu,
      familyId: "atelier",
      treatment: "balanced",
      outputFormat: "single-sheet",
      paperFormat: "a4",
    });
    const page = document.pages[0];
    const masthead = page.blocks.find((block) => block.kind === "masthead")!;
    const headings = page.blocks
      .filter((block) => block.kind === "category-heading")
      .sort((first, second) => first.rect.yMm - second.rect.yMm);
    expect(headings.length).toBeGreaterThanOrEqual(2);

    const lockup =
      headings[0].rect.yMm - (masthead.rect.yMm + masthead.rect.heightMm);
    const firstCourseBottom = Math.max(
      ...page.blocks
        .filter((block) => block.sectionId === headings[0].sectionId)
        .map((block) => block.rect.yMm + block.rect.heightMm),
    );
    const courseGap = headings[1].rect.yMm - firstCourseBottom;

    expect(lockup).toBeGreaterThan(0);
    expect(lockup).toBeLessThanOrEqual(10);
    expect(courseGap).toBeGreaterThan(lockup);
    expect(courseGap - lockup).toBeLessThanOrEqual(9.5);
  });

  it.each([
    ["maison", "letter"],
    ["atelier", "a4"],
  ] as const)(
    "never strands a one or two dish tail when a %s course continues onto the next page",
    (familyId, paperFormat) => {
      // A course that spills by one or two dishes should hand the whole tail
      // forward, or keep a longer group back — a lone dish under a repeated
      // heading reads as a printing error.
      const document = planFixtureDocument({
        model: denseRestaurantMenu,
        familyId,
        treatment: "balanced",
        outputFormat: "two-page-spread",
        paperFormat,
      });
      const chunks = new Map<string, number[]>();
      for (const page of document.pages) {
        const counts = new Map<string, number>();
        for (const block of page.blocks) {
          if (!block.itemIds?.length || !block.sectionId) continue;
          counts.set(
            block.sectionId,
            (counts.get(block.sectionId) ?? 0) + block.itemIds.length,
          );
        }
        counts.forEach((count, sectionId) => {
          chunks.set(sectionId, [...(chunks.get(sectionId) ?? []), count]);
        });
      }
      for (const [, counts] of chunks) {
        counts.slice(1).forEach((count) => expect(count).toBeGreaterThan(2));
      }
      const placed = new Set(
        document.pages.flatMap((page) =>
          page.blocks.flatMap((block) => block.itemIds ?? []),
        ),
      );
      expect(placed.size).toBe(
        denseRestaurantMenu.sections.reduce(
          (total, section) => total + section.items.length,
          0,
        ),
      );
      expect(document.readiness).not.toBe("blocked");
    },
  );

  it("opens a sparse continuation page high on the sheet, not floating mid-air", () => {
    // One dish alone on a second page is already awkward; centring it made it
    // an island between two equal voids. An upper bias reads as an opening.
    const document = planFixtureDocument({
      model: sparseRestaurantMenu,
      familyId: "atelier",
      treatment: "balanced",
      outputFormat: "two-page-spread",
      paperFormat: "a4",
    });
    expect(document.pages).toHaveLength(2);
    const page = document.pages[1];
    const footer = page.blocks.find((block) => block.kind === "footer")!;
    const content = page.blocks.filter(
      (block) => block.kind !== "footer" && block.kind !== "folio",
    );
    const items = content.reduce(
      (total, block) => total + (block.itemIds?.length ?? 0),
      0,
    );
    expect(items).toBeLessThanOrEqual(2);

    const regionTop = document.geometry.safeArea.top;
    const regionHeight = footer.rect.yMm - regionTop;
    const top = Math.min(...content.map((block) => block.rect.yMm));
    const share = (top - regionTop) / regionHeight;
    expect(share).toBeGreaterThan(0.08);
    expect(share).toBeLessThan(0.25);
  });

  it("places every feasible item on a compact A5 drinks card", () => {
    const compactModel: PrintMenuModel = {
      ...photoRichRestaurantMenu,
      sections: [
        {
          ...photoRichRestaurantMenu.sections[0],
          items: photoRichRestaurantMenu.sections[0].items.slice(0, 2),
        },
      ],
    };
    const document = planFixtureDocument({
      model: compactModel,
      familyId: "night-house",
      treatment: "photo-led",
      outputFormat: "drinks-card",
      paperFormat: "a5",
      imageSelections: [
        {
          itemId: "harvest-bowl",
          url: "https://images.payverge.test/harvest-bowl-1600x1200.jpg",
          role: "cover",
        },
        {
          itemId: "charred-carrots",
          url: "https://images.payverge.test/charred-carrots-1400x1050.jpg",
          role: "full-width",
        },
      ],
      coverMode: "photographic",
    });

    expect(document.readiness).not.toBe("blocked");
    expect(
      document.pages.flatMap(({ blocks }) =>
        blocks.flatMap((block) => block.itemIds ?? []),
      ),
    ).toEqual(expect.arrayContaining(["harvest-bowl", "charred-carrots"]));
  });

  it("blocks the original six-item photographic A5 card with exact omission counts", () => {
    const document = planFixtureDocument({
      model: photoRichRestaurantMenu,
      familyId: "night-house",
      treatment: "photo-led",
      outputFormat: "drinks-card",
      paperFormat: "a5",
      imageSelections: [
        {
          itemId: "harvest-bowl",
          url: "https://images.payverge.test/harvest-bowl-1600x1200.jpg",
          role: "cover",
        },
        {
          itemId: "charred-carrots",
          url: "https://images.payverge.test/charred-carrots-1400x1050.jpg",
          role: "full-width",
        },
      ],
      coverMode: "photographic",
    });
    const densityDiagnostic = document.diagnostics.find(
      ({ id }) => id === "format:drinks-card:too-dense",
    );

    expect(document.readiness).toBe("blocked");
    expect(densityDiagnostic).toEqual(
      expect.objectContaining({
        severity: "blocked",
        messageKey: "print.diagnostics.chooseLargerFormat",
        values: {
          omittedItems: expect.any(Number),
          placedItems: expect.any(Number),
          totalItems: 6,
        },
      }),
    );
    expect(densityDiagnostic?.values?.omittedItems).toBeGreaterThan(0);
    expect(
      Number(densityDiagnostic?.values?.placedItems) +
        Number(densityDiagnostic?.values?.omittedItems),
    ).toBe(6);
  });

  it.each([
    ["folded-booklet", "osteria", denseRestaurantMenu],
    ["takeaway-trifold", "street", photoRichRestaurantMenu],
  ] as const)(
    "intentionally composes every %s surface",
    (outputFormat, familyId, model) => {
      const document = planFixtureDocument({
        model,
        familyId,
        treatment: "balanced",
        outputFormat,
        paperFormat: "a4",
      });

      for (const page of document.pages) {
        expect(
          page.blocks.some(
            (block) => block.kind !== "footer" && block.kind !== "folio",
          ),
        ).toBe(true);
      }
    },
  );

  it("blocks dense fixed single sheets and recommends a larger format", () => {
    const document = planFixtureDocument(denseRestaurantMenu);
    expect(document.pages).toHaveLength(1);
    expect(document.readiness).toBe("blocked");
    expect(document.diagnostics).toContainEqual(
      expect.objectContaining({
        id: "format:single-sheet:too-dense",
        severity: "blocked",
        messageKey: "print.diagnostics.chooseLargerFormat",
        values: {
          omittedItems: expect.any(Number),
          placedItems: expect.any(Number),
          totalItems: 24,
        },
      }),
    );
    expect(
      document.diagnostics.find(
        ({ id }) => id === "format:single-sheet:too-dense",
      )?.values?.omittedItems,
    ).toBeGreaterThan(0);
  });

  it("terminates when one atomic item cannot fit a fresh multipage surface", () => {
    const impossible: PrintMenuModel = {
      ...sparseRestaurantMenu,
      sections: [
        {
          id: "impossible",
          name: "Impossible course",
          items: [
            {
              ...sparseRestaurantMenu.sections[0].items[0],
              id: "impossible-item",
              description: "unbounded detail ".repeat(4000),
            },
          ],
        },
      ],
    };
    const document = planFixtureDocument({
      model: impossible,
      familyId: "atelier",
      treatment: "type-led",
      outputFormat: "two-page-spread",
    });
    expect(document.pages.length).toBeLessThanOrEqual(2);
    expect(document.readiness).toBe("blocked");
    expect(document.diagnostics).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "section:impossible:overflow",
          severity: "blocked",
        }),
        expect.objectContaining({
          id: "format:two-page-spread:too-dense",
          severity: "blocked",
          messageKey: "print.diagnostics.chooseLargerFormat",
        }),
      ]),
    );
  });

  it("drops photos instead of blocking when a later course cannot fit", () => {
    const url = "https://images.test/harvest-hero.jpg";
    const model: PrintMenuModel = {
      ...sparseRestaurantMenu,
      sections: [
        {
          ...sparseRestaurantMenu.sections[0],
          items: [
            { ...sparseRestaurantMenu.sections[0].items[0], imageUrl: url },
            sparseRestaurantMenu.sections[0].items[1],
          ],
        },
        {
          ...sparseRestaurantMenu.sections[1],
          items: Array.from({ length: 7 }, (_, index) => ({
            ...sparseRestaurantMenu.sections[1].items[0],
            id: `mains-${index}`,
            name: `Course ${index + 1}`,
          })),
        },
      ],
    };
    const document = planFixtureDocument({
      model,
      familyId: "atelier",
      treatment: "balanced",
      outputFormat: "single-sheet",
      imageSelections: [
        { itemId: model.sections[0].items[0].id, url, role: "full-width" },
      ],
    });
    // The hero belongs to the first course, so the per-chunk retry cannot
    // reclaim its depth for the overflowing second course — only the
    // whole-document fallback can.
    expect(document.readiness).toBe("warning");
    expect(document.direction.imageSelections).toHaveLength(0);
    expect(
      document.pages.flatMap(({ blocks }) => blocks).some(({ image }) => image),
    ).toBe(false);
    expect(document.diagnostics).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: `image:${url}:unplaced`,
          severity: "warning",
          messageKey: "print.diagnostics.imageUnplaced",
        }),
      ]),
    );
    expect(
      document.diagnostics.some(({ severity }) => severity === "blocked"),
    ).toBe(false);
  });

  it.each([
    ["two-page-spread", 2],
    ["folded-booklet", 2],
    ["takeaway-trifold", 2],
    ["single-sheet", 1],
    ["drinks-card", 1],
    ["counter-menu", 1],
  ] as const)("enforces %s physical page rules", (outputFormat, expected) => {
    const familyId =
      outputFormat === "takeaway-trifold" || outputFormat === "counter-menu"
        ? "counter"
        : "atelier";
    const document = planFixtureDocument(sparseRestaurantMenu, {
      direction: { familyId, outputFormat },
    });
    expect(document.pages).toHaveLength(expected);
    if (outputFormat === "folded-booklet") {
      expect(document.geometry.orientation).toBe("landscape");
      expect(document.pages.every((page) => page.blocks.length > 0)).toBe(true);
    }
  });

  it("imposes a one-sheet booklet with a front cover and two inside menu panels", () => {
    const document = planFixtureDocument({
      model: sparseRestaurantMenu,
      familyId: "maison",
      treatment: "type-led",
      outputFormat: "folded-booklet",
      paperFormat: "a4",
    });
    const fold = document.geometry.foldsMm[0];
    const outside = document.pages[0];
    const inside = document.pages[1];
    const identity = outside.blocks.find(
      (block) => block.kind === "masthead" || block.kind === "cover",
    );

    expect(document.pages).toHaveLength(2);
    expect(identity?.rect.xMm).toBeGreaterThan(fold);
    expect(identity!.rect.yMm + identity!.rect.heightMm / 2).toBeCloseTo(
      document.geometry.heightMm / 2,
      1,
    );
    expect(
      inside.blocks.filter(
        (block) => block.kind === "item-list" || block.kind === "item-feature",
      ),
    ).not.toHaveLength(0);
    expect(
      outside.blocks.filter((block) => block.kind === "footer"),
    ).toHaveLength(1);
    expect(
      inside.blocks.filter((block) => block.kind === "footer"),
    ).toHaveLength(0);
  });

  it("keeps the gallery photo-led side column inside a booklet panel", () => {
    // Regression: rounding the copy column's x offset (0.61x) and width
    // (0.39x) independently overshot the panel rect by 0.01mm, tripping the
    // measured-item invariant for every photo-led gallery booklet.
    const measurements = Object.fromEntries(
      photoRichRestaurantMenu.sections.flatMap((section) =>
        section.items.flatMap((item) =>
          (item.imageCandidates ?? []).map((url) => [
            url,
            { url, status: "ready" as const, width: 1800, height: 1400 },
          ]),
        ),
      ),
    );
    const audit = makeAudit(photoRichRestaurantMenu, {
      photoCoverage: 0.8,
      measurements,
    });
    const family = galleryFamily;
    const imageSelections = selectDefaultPrintImages({
      family,
      model: photoRichRestaurantMenu,
      audit,
      treatment: "photo-led",
    });
    const document = planMenuDocument({
      model: photoRichRestaurantMenu,
      audit,
      direction: makeDirection({
        familyId: "gallery",
        treatment: "photo-led",
        outputFormat: "folded-booklet",
        paperFormat: "a4",
        imageSelections,
      }),
    });

    expect(document.readiness).not.toBe("blocked");
    expectPhysicalBounds(document);
  });

  it("uses one identity treatment when an explicit cover is selected", () => {
    const coverUrl =
      photoRichRestaurantMenu.sections[0].items[0].imageCandidates![0];
    const document = planFixtureDocument({
      model: photoRichRestaurantMenu,
      familyId: "gallery",
      treatment: "photo-led",
      outputFormat: "two-page-spread",
      coverMode: "photographic",
      imageSelections: [
        { itemId: "harvest-bowl", url: coverUrl, role: "cover" },
      ],
    });
    const firstPageIdentity = document.pages[0].blocks.filter(
      (block) => block.kind === "masthead" || block.kind === "cover",
    );

    expect(firstPageIdentity).toHaveLength(1);
    expect(firstPageIdentity[0].kind).toBe("cover");
    expect(firstPageIdentity[0].rect.heightMm).toBeGreaterThanOrEqual(60);
    expect(firstPageIdentity[0].rect.heightMm).toBeLessThan(
      document.geometry.heightMm / 2,
    );
    expect(
      document.pages
        .flatMap((page) => page.blocks)
        .filter(
          (block) =>
            block.kind === "item-list" || block.kind === "item-feature",
        )
        .flatMap((block) => block.itemIds ?? [])
        .sort(),
    ).toEqual(
      photoRichRestaurantMenu.sections
        .flatMap((section) => section.items.map((item) => item.id))
        .sort(),
    );
  });

  it("promotes a selected dish photo when photographic cover mode is chosen", () => {
    const url =
      photoRichRestaurantMenu.sections[0].items[0].imageCandidates![0];
    const document = planFixtureDocument({
      model: photoRichRestaurantMenu,
      familyId: "gallery",
      treatment: "photo-led",
      outputFormat: "two-page-spread",
      coverMode: "photographic",
      imageSelections: [
        { itemId: "harvest-bowl", url, role: "editorial-crop" },
      ],
    });

    expect(
      document.pages[0].blocks.find((block) => block.kind === "cover")?.image,
    ).toMatchObject({ itemId: "harvest-bowl", url });
  });

  it("blocks a photographic cover until a usable photo is selected", () => {
    const document = planFixtureDocument({
      model: sparseRestaurantMenu,
      familyId: "gallery",
      treatment: "photo-led",
      outputFormat: "two-page-spread",
      coverMode: "photographic",
      imageSelections: [],
    });

    expect(document.diagnostics).toContainEqual(
      expect.objectContaining({
        id: "setting:cover:photograph-required",
        severity: "blocked",
      }),
    );
  });

  it("turns unused folded panels into deliberate restaurant story panels", () => {
    const document = planFixtureDocument({
      model: sparseRestaurantMenu,
      familyId: "street",
      treatment: "balanced",
      outputFormat: "takeaway-trifold",
      paperFormat: "a4",
    });

    expect(
      document.pages.flatMap((page) =>
        page.blocks.filter((block) => block.kind === "panel-note"),
      ).length,
    ).toBeGreaterThan(0);
  });

  it("warns when a forced spread leaves a nearly empty trailing page", () => {
    const document = planFixtureDocument({
      model: sparseRestaurantMenu,
      familyId: "maison",
      treatment: "type-led",
      outputFormat: "two-page-spread",
      paperFormat: "letter",
    });

    expect(document.diagnostics).toContainEqual(
      expect.objectContaining({
        id: "page:1:underfilled",
        severity: "warning",
      }),
    );
    // Spread balancing still moves the trailing category onto the second
    // page so the pair prints as a composed spread, not page + blank.
    expect(
      document.pages[1].blocks.some(
        (block) => block.sectionId === "mains" && block.kind === "item-list",
      ),
    ).toBe(true);
  });

  it("keeps Osteria headings and menu items in separate physical rectangles", () => {
    const document = planFixtureDocument({
      model: denseRestaurantMenu,
      familyId: "osteria",
      treatment: "balanced",
      outputFormat: "folded-booklet",
      paperFormat: "a4",
    });
    const overlaps: string[] = [];
    for (const page of document.pages) {
      const semantic = page.blocks.filter((block) =>
        ["category-heading", "item-list", "item-feature"].includes(block.kind),
      );
      for (let leftIndex = 0; leftIndex < semantic.length; leftIndex += 1) {
        for (
          let rightIndex = leftIndex + 1;
          rightIndex < semantic.length;
          rightIndex += 1
        ) {
          const left = semantic[leftIndex];
          const right = semantic[rightIndex];
          const intersectionWidth =
            Math.min(
              left.rect.xMm + left.rect.widthMm,
              right.rect.xMm + right.rect.widthMm,
            ) - Math.max(left.rect.xMm, right.rect.xMm);
          const intersectionHeight =
            Math.min(
              left.rect.yMm + left.rect.heightMm,
              right.rect.yMm + right.rect.heightMm,
            ) - Math.max(left.rect.yMm, right.rect.yMm);
          if (intersectionWidth > 0.01 && intersectionHeight > 0.01) {
            overlaps.push(`${page.index}:${left.id}:${right.id}`);
          }
        }
      }
    }
    expect(overlaps).toEqual([]);
  });

  it("lines Osteria's two columns up row by row instead of letting one sink", () => {
    // The columns are deliberately unequal, so the same description wraps
    // taller on the right. Advancing each column by its own content left the
    // stacks 20mm+ apart with nothing aligned across the gutter.
    const document = planFixtureDocument({
      model: denseRestaurantMenu,
      familyId: "osteria",
      treatment: "balanced",
      outputFormat: "folded-booklet",
      paperFormat: "a4",
    });
    const groups = new Map<string, Map<number, number[]>>();
    for (const page of document.pages) {
      for (const block of page.blocks) {
        if (!block.itemIds?.length || !block.sectionId) continue;
        const key = `${page.index}:${block.sectionId}`;
        if (!groups.has(key)) groups.set(key, new Map());
        const columns = groups.get(key)!;
        const column = Math.round(block.rect.xMm * 10) / 10;
        columns.set(column, [
          ...(columns.get(column) ?? []),
          block.rect.yMm + block.rect.heightMm,
        ]);
      }
    }
    const twoColumn = [...groups.values()].filter(
      (columns) => columns.size === 2,
    );
    expect(twoColumn.length).toBeGreaterThanOrEqual(3);
    for (const columns of twoColumn) {
      const bottoms = [...columns.values()].map((values) =>
        Math.max(...values),
      );
      expect(Math.max(...bottoms) - Math.min(...bottoms)).toBeLessThan(10);
    }
  });

  it("measures a Field course heading across a readable line, not a 42mm sliver", () => {
    // The heading box was capped at 42mm while the rendered heading spans the
    // full column, so two-word courses were measured as two lines and clipped.
    const document = planFixtureDocument({
      model: sparseRestaurantMenu,
      familyId: "field",
      treatment: "balanced",
      outputFormat: "single-sheet",
      paperFormat: "a4",
    });
    const headings = document.pages[0].blocks.filter(
      (block) => block.kind === "category-heading",
    );
    expect(headings.length).toBeGreaterThanOrEqual(2);
    for (const heading of headings) {
      expect(heading.rect.widthMm).toBeCloseTo(78, 1);
      expect(heading.rect.heightMm).toBeLessThan(13);
    }
  });

  it.each([
    ["single-sheet", "atelier"],
    ["two-page-spread", "atelier"],
    ["folded-booklet", "atelier"],
    ["takeaway-trifold", "counter"],
    ["drinks-card", "atelier"],
    ["counter-menu", "counter"],
  ] as const)(
    "reserves QR-capable footer geometry on the contact surface for %s",
    (outputFormat, familyId) => {
      const document = planFixtureDocument({
        model: sparseRestaurantMenu,
        familyId,
        outputFormat,
      });
      const pageBottom =
        document.geometry.heightMm - document.geometry.safeArea.bottom;

      for (const page of document.pages) {
        const footers = page.blocks.filter((block) => block.kind === "footer");
        expect(footers).toHaveLength(
          outputFormat === "folded-booklet" ||
            outputFormat === "takeaway-trifold"
            ? page.index === 0
              ? 1
              : 0
            : 1,
        );
        for (const footer of footers) {
          expect(footer.rect.heightMm).toBeGreaterThanOrEqual(12);
          expect(footer.rect.yMm + footer.rect.heightMm).toBeLessThanOrEqual(
            pageBottom,
          );
          for (const block of page.blocks.filter(
            (candidate) => candidate.kind !== "footer",
          )) {
            const overlapsFooterInline =
              block.rect.xMm < footer.rect.xMm + footer.rect.widthMm &&
              block.rect.xMm + block.rect.widthMm > footer.rect.xMm;
            if (overlapsFooterInline) {
              expect(block.rect.yMm + block.rect.heightMm).toBeLessThanOrEqual(
                footer.rect.yMm,
              );
            }
          }
        }
      }
    },
  );

  it("treats failed and low-resolution audit findings as deduped warnings", () => {
    const document = planFixtureDocument(photoRichRestaurantMenu, {
      audit: {
        failedUrls: ["broken.jpg", "broken.jpg"],
        lowResolutionUrls: ["tiny.jpg", "tiny.jpg"],
        duplicateUrls: ["shared.jpg", "shared.jpg"],
      },
    });
    expect(document.diagnostics.map(({ id }) => id)).toEqual(
      expect.arrayContaining([
        "image:broken.jpg:failed",
        "image:tiny.jpg:low-resolution",
        "image:shared.jpg:duplicate",
      ]),
    );
    expect(new Set(document.diagnostics.map(({ id }) => id)).size).toBe(
      document.diagnostics.length,
    );
    expect(document.readiness).toBe("warning");
    expect(
      document.diagnostics.find(({ id }) => id === "image:broken.jpg:failed")
        ?.severity,
    ).toBe("warning");
  });

  it("does not flag Night House as a problem for simply being a dark family", () => {
    // Choosing Night House is the operator stating an intent, not a mistake:
    // an unconditional warning fired on every plan, could never be resolved,
    // and trained operators to ignore the diagnostics list.
    const document = planFixtureDocument({
      model: sparseRestaurantMenu,
      familyId: "night-house",
      treatment: "type-led",
      outputFormat: "single-sheet",
    });

    expect(
      document.diagnostics.some(({ id }) => id === "setting:palette:ink-heavy"),
    ).toBe(false);
    expect(document.readiness).toBe("ready");
  });

  it("evaluates selected photography at its planned print size", () => {
    const url = "harvest.jpg";
    const document = planFixtureDocument({
      model: photoRichRestaurantMenu,
      familyId: "gallery",
      treatment: "photo-led",
      outputFormat: "two-page-spread",
      imageSelections: [{ itemId: "harvest-bowl", url, role: "half-page" }],
      audit: {
        measurements: {
          [url]: { url, status: "ready", width: 600, height: 450 },
        },
      },
    });

    expect(document.diagnostics).toContainEqual(
      expect.objectContaining({
        id: `image:${url}:effective-dpi`,
        severity: "warning",
        messageKey: "print.diagnostics.imageEffectiveDpi",
        values: expect.objectContaining({ effectiveDpi: expect.any(Number) }),
      }),
    );
  });

  it("gives fixed-format overflow blocked precedence over image warnings", () => {
    const document = planFixtureDocument(denseRestaurantMenu, {
      audit: {
        failedUrls: ["failed.jpg"],
        lowResolutionUrls: ["small.jpg"],
      },
    });
    expect(document.readiness).toBe("blocked");
    expect(document.diagnostics.map(({ id }) => id)).toEqual(
      expect.arrayContaining([
        "image:failed.jpg:failed",
        "format:single-sheet:too-dense",
      ]),
    );
  });

  it("diagnoses failed selected images without planning an image slot", () => {
    const document = planFixtureDocument({
      model: photoRichRestaurantMenu,
      familyId: "gallery",
      treatment: "photo-led",
      outputFormat: "single-sheet",
      imageSelections: [
        {
          itemId: "spring-risotto",
          url: "broken.jpg",
          role: "editorial-crop",
        },
      ],
      failedUrls: ["broken.jpg"],
    });
    expect(
      document.pages
        .flatMap(({ blocks }) => blocks)
        .some((block) => block.image?.url === "broken.jpg"),
    ).toBe(false);
    expect(document.diagnostics.map(({ id }) => id)).toContain(
      "image:broken.jpg:failed",
    );
    expect(document.readiness).toBe("warning");
  });

  it("enforces the family feature-image cap across section calls", () => {
    const model: PrintMenuModel = {
      ...sparseRestaurantMenu,
      sections: ["one", "two", "three"].map((id) => ({
        id,
        name: `Course ${id}`,
        items: [
          {
            ...sparseRestaurantMenu.sections[0].items[0],
            id: `item-${id}`,
            name: `Dish ${id}`,
            description: "Brief description.",
            imageUrl: `${id}.jpg`,
            imageCandidates: [`${id}.jpg`],
          },
        ],
      })),
    };
    const document = planFixtureDocument({
      model,
      familyId: "maison",
      treatment: "photo-led",
      outputFormat: "two-page-spread",
      imageSelections: ["one", "two", "three"].map((id) => ({
        itemId: `item-${id}`,
        url: `${id}.jpg`,
        role: "half-page" as const,
      })),
    });
    for (const page of document.pages) {
      expect(
        page.blocks.filter((block) => block.kind === "image").length,
      ).toBeLessThanOrEqual(1);
    }
  });

  it("budgets Maison's 8mm course gaps in compact document pagination", () => {
    const source = sparseRestaurantMenu.sections[0].items[0];
    const model: PrintMenuModel = {
      ...sparseRestaurantMenu,
      sections: [
        {
          id: "courses",
          name: "Courses",
          items: Array.from({ length: 18 }, (_, index) => ({
            ...source,
            id: `course-${index}`,
            name: `Course ${index}`,
            description: "A measured compact course description.",
          })),
        },
      ],
    };
    const document = planFixtureDocument({
      model,
      familyId: "maison",
      treatment: "compact",
      outputFormat: "folded-booklet",
      paperFormat: "a5",
    });
    for (const page of document.pages) {
      const items = page.blocks.filter(
        (block) => block.kind === "item-list" || block.kind === "item-feature",
      );
      expect(items.every((block) => block.rect.heightMm > 0)).toBe(true);
      const panels = new Map<number, typeof items>();
      for (const item of items) {
        panels.set(item.rect.xMm, [...(panels.get(item.rect.xMm) ?? []), item]);
      }
      for (const panelItems of panels.values()) {
        const ordered = [...panelItems].sort(
          (first, second) => first.rect.yMm - second.rect.yMm,
        );
        for (let index = 1; index < ordered.length; index += 1) {
          expect(
            ordered[index].rect.yMm -
              (ordered[index - 1].rect.yMm + ordered[index - 1].rect.heightMm),
          ).toBeCloseTo(8, 2);
        }
      }
    }
  });

  it("budgets Field half-page image height before measuring item blocks", () => {
    const item = {
      ...sparseRestaurantMenu.sections[0].items[0],
      id: "field-half-page-item",
      imageUrl: "field-half-page.jpg",
      imageCandidates: ["field-half-page.jpg"],
    };
    expect(() =>
      planFixtureDocument({
        model: {
          ...sparseRestaurantMenu,
          sections: [
            { id: "field-half-page", name: "Field course", items: [item] },
          ],
        },
        familyId: "field",
        treatment: "balanced",
        outputFormat: "two-page-spread",
        imageSelections: [
          {
            itemId: item.id,
            url: "field-half-page.jpg",
            role: "half-page",
          },
        ],
      }),
    ).not.toThrow();
  });

  it.each([
    ["maison", 8],
    ["field", 12],
  ] as const)(
    "applies %s inter-section whitespace at document level",
    (familyId, gapMm) => {
      // Three sections so spread balancing keeps "one" and "two" together on
      // page 1 (the break lands before "three") and their gap is measurable.
      const model: PrintMenuModel = {
        ...sparseRestaurantMenu,
        sections: ["one", "two", "three"].map((id) => ({
          id,
          name: `Section ${id}`,
          items: [
            {
              ...sparseRestaurantMenu.sections[0].items[0],
              id: `item-${id}`,
              description: "Short.",
            },
          ],
        })),
      };
      const document = planFixtureDocument({
        model,
        familyId,
        treatment: "type-led",
        outputFormat: "two-page-spread",
      });
      const page = document.pages.find(
        ({ blocks }) =>
          blocks.some((block) => block.sectionId === "one") &&
          blocks.some((block) => block.sectionId === "two"),
      )!;
      const firstBottom = Math.max(
        ...page.blocks
          .filter((block) => block.sectionId === "one")
          .map((block) => block.rect.yMm + block.rect.heightMm),
      );
      const secondTop = Math.min(
        ...page.blocks
          .filter((block) => block.sectionId === "two")
          .map((block) => block.rect.yMm),
      );
      expect(secondTop - firstBottom).toBeGreaterThanOrEqual(gapMm);
    },
  );

  it("reserves Gallery image area against the usable photo-led page", () => {
    const model: PrintMenuModel = {
      ...sparseRestaurantMenu,
      sections: ["one", "two"].map((id) => ({
        id,
        name: `Gallery ${id}`,
        items: [
          {
            ...sparseRestaurantMenu.sections[0].items[0],
            id: `item-${id}`,
            description: "Short.",
            imageUrl: `${id}.jpg`,
            imageCandidates: [`${id}.jpg`],
          },
        ],
      })),
    };
    const document = planFixtureDocument({
      model,
      familyId: "gallery",
      treatment: "photo-led",
      outputFormat: "two-page-spread",
      imageSelections: ["one", "two"].map((id) => ({
        itemId: `item-${id}`,
        url: `${id}.jpg`,
        role: "editorial-crop" as const,
      })),
    });
    const firstPage = document.pages[0];
    const image = firstPage.blocks.find((block) => block.kind === "image")!;
    const mastheadBottom = Math.max(
      ...firstPage.blocks
        .filter((block) => block.kind === "masthead")
        .map((block) => block.rect.yMm + block.rect.heightMm + 5),
    );
    const usableHeight =
      document.geometry.heightMm -
      document.geometry.safeArea.bottom -
      6 -
      mastheadBottom;
    const usableWidth =
      document.geometry.widthMm -
      document.geometry.safeArea.left -
      document.geometry.safeArea.right;
    const ratio =
      (image.rect.widthMm * image.rect.heightMm) / (usableWidth * usableHeight);
    // Photo-led gallery runs the photograph as a side column beside the copy:
    // real presence without the old full-page allocation that pushed a
    // handful of dishes across four pages.
    expect(ratio).toBeGreaterThanOrEqual(0.2);
    expect(ratio).toBeLessThanOrEqual(0.5);
    const heading = firstPage.blocks.find(
      (block) => block.kind === "category-heading" && block.sectionId === "one",
    )!;
    expect(heading.rect.yMm).toBe(image.rect.yMm);
    expect(heading.rect.widthMm + image.rect.widthMm).toBeLessThanOrEqual(
      usableWidth,
    );
    expect(firstPage.blocks.some((block) => block.sectionId === "two")).toBe(
      false,
    );
    expect(
      document.pages[1].blocks.some((block) => block.sectionId === "two"),
    ).toBe(true);
  });

  it.each(["photo-led", "balanced"] as const)(
    "paginates Gallery %s items without clipping accepted measured blocks",
    (treatment) => {
      const source = sparseRestaurantMenu.sections[0].items[0];
      const items = Array.from({ length: 6 }, (_, index) => ({
        ...source,
        id: `gallery-item-${index}`,
        name: `Gallery item ${index}`,
        description:
          "Garden vegetables, toasted grains, preserved citrus, herbs and a carefully measured seasonal dressing.",
        imageUrl: index === 0 ? "gallery.jpg" : undefined,
        imageCandidates: index === 0 ? ["gallery.jpg"] : [],
      }));
      const model: PrintMenuModel = {
        ...sparseRestaurantMenu,
        sections: [{ id: "gallery-course", name: "Gallery course", items }],
      };
      const document = planFixtureDocument({
        model,
        familyId: "gallery",
        treatment,
        outputFormat: "two-page-spread",
        imageSelections: [
          {
            itemId: items[0].id,
            url: "gallery.jpg",
            role: "editorial-crop",
          },
        ],
      });
      const itemBlocks = document.pages
        .flatMap(({ blocks }) => blocks)
        .filter(
          (block) =>
            block.kind === "item-list" || block.kind === "item-feature",
        );
      expect(itemBlocks.map((block) => block.itemIds?.[0]).sort()).toEqual(
        items.map(({ id }) => id).sort(),
      );
      for (const block of itemBlocks) {
        const item = items.find(({ id }) => id === block.itemIds?.[0])!;
        expect(block.rect.heightMm).toBe(
          estimatePlannedItemHeight({
            item,
            widthMm: block.rect.widthMm,
            family: galleryFamily,
            treatment,
          }).heightMm,
        );
      }
      expect(document.pages.length).toBeGreaterThan(1);
      expect(
        document.diagnostics.some(
          ({ id }) => id === "section:gallery-course:overflow",
        ),
      ).toBe(false);
    },
  );

  it("blocks a fixed A5 sheet for a 1000-code-point CJK description", () => {
    const model: PrintMenuModel = {
      ...cjkRestaurantMenu,
      sections: [
        {
          id: "cjk-long",
          name: "長い説明",
          items: [
            {
              ...cjkRestaurantMenu.sections[0].items[0],
              id: "cjk-1000",
              description: "界".repeat(1000),
            },
          ],
        },
      ],
    };
    const document = planFixtureDocument({
      model,
      familyId: "field",
      treatment: "type-led",
      outputFormat: "single-sheet",
      paperFormat: "a5",
    });
    expect(document.readiness).toBe("blocked");
    expect(document.diagnostics.map(({ id }) => id)).toContain(
      "format:single-sheet:too-dense",
    );
  });

  it.each(["a4", "letter"] as const)(
    "keeps every %s trifold block inside one fold-safe panel",
    (paperFormat) => {
      const document = planFixtureDocument({
        model: sparseRestaurantMenu,
        familyId: "counter",
        treatment: "compact",
        outputFormat: "takeaway-trifold",
        paperFormat,
      });
      expectTrifoldPanels(document);
    },
  );

  it("keeps block ids unique when one section continues across trifold panels", () => {
    const source = denseRestaurantMenu.sections[0].items[0];
    const model: PrintMenuModel = {
      ...denseRestaurantMenu,
      sections: [
        {
          id: "panel-continuation",
          name: "Panel continuation",
          items: Array.from({ length: 30 }, (_, index) => ({
            ...source,
            id: `continuation-item-${index}`,
            description: "Fold-safe description with measured ingredients.",
          })),
        },
      ],
    };
    const document = planFixtureDocument({
      model,
      familyId: "counter",
      treatment: "compact",
      outputFormat: "takeaway-trifold",
      paperFormat: "letter",
    });
    expect(
      document.pages[1].blocks.filter(
        (block) =>
          block.kind === "category-heading" &&
          block.sectionId === "panel-continuation",
      ).length,
    ).toBeGreaterThan(1);
    const ids = document.pages.flatMap(({ blocks }) =>
      blocks.map(({ id }) => id),
    );
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("allows four compact categories to share three inside trifold panels", () => {
    const source = denseRestaurantMenu.sections[0].items[0];
    const model: PrintMenuModel = {
      ...denseRestaurantMenu,
      sections: Array.from({ length: 4 }, (_, index) => ({
        id: `compact-category-${index}`,
        name: `Compact category ${index + 1}`,
        items: [
          {
            ...source,
            id: `compact-item-${index}`,
            description: "A concise fold-safe description.",
          },
        ],
      })),
    };
    const document = planFixtureDocument({
      model,
      familyId: "counter",
      treatment: "compact",
      outputFormat: "takeaway-trifold",
      paperFormat: "letter",
    });

    expect(
      document.diagnostics.some(
        (diagnostic) => diagnostic.severity === "blocked",
      ),
    ).toBe(false);
    expect(
      document.pages.flatMap(({ blocks }) =>
        blocks.flatMap((block) => block.itemIds ?? []),
      ),
    ).toEqual([
      "compact-item-0",
      "compact-item-1",
      "compact-item-2",
      "compact-item-3",
    ]);
  });

  it("blocks dense trifold overflow after the three inside panels", () => {
    const source = denseRestaurantMenu.sections[0].items[0];
    const model: PrintMenuModel = {
      ...denseRestaurantMenu,
      sections: [
        {
          id: "dense-panels",
          name: "Dense panels",
          items: Array.from({ length: 80 }, (_, index) => ({
            ...source,
            id: `panel-item-${index}`,
            description:
              "Long folded menu description with ingredients and preparation.",
          })),
        },
      ],
    };
    const document = planFixtureDocument({
      model,
      familyId: "counter",
      treatment: "compact",
      outputFormat: "takeaway-trifold",
      paperFormat: "letter",
    });
    expect(document.pages).toHaveLength(2);
    expect(document.readiness).toBe("blocked");
    expect(document.diagnostics.map(({ id }) => id)).toContain(
      "format:takeaway-trifold:too-dense",
    );
    expectTrifoldPanels(document);
  });

  it("binds an explicit cover selection to the first cover block", () => {
    const coverUrl =
      photoRichRestaurantMenu.sections[0].items[0].imageCandidates![1];
    const document = planFixtureDocument({
      model: photoRichRestaurantMenu,
      familyId: "gallery",
      treatment: "photo-led",
      outputFormat: "two-page-spread",
      imageSelections: [
        { itemId: "harvest-bowl", url: coverUrl, role: "cover" },
      ],
    });
    const cover = document.pages[0].blocks.find(
      (block) => block.kind === "cover",
    )!;
    expect(cover.image).toMatchObject({ url: coverUrl, role: "cover" });
    expect(
      document.diagnostics.some(
        ({ id }) => id === `image:${coverUrl}:unplaced`,
      ),
    ).toBe(false);
  });

  it("promotes one photographic cover without repeating it inside the menu", () => {
    const coverUrl =
      photoRichRestaurantMenu.sections[0].items[0].imageCandidates![1];
    const document = planFixtureDocument({
      model: photoRichRestaurantMenu,
      familyId: "gallery",
      treatment: "photo-led",
      outputFormat: "two-page-spread",
      coverMode: "photographic",
      imageSelections: [
        { itemId: "harvest-bowl", url: coverUrl, role: "editorial-crop" },
      ],
    });
    const placements = document.pages
      .flatMap((page) => page.blocks)
      .filter((block) => block.image?.url === coverUrl);

    expect(placements).toHaveLength(1);
    expect(placements[0]).toMatchObject({ kind: "cover" });
  });

  it("warns for valid nonfailed selections intentionally unplaced by type-led", () => {
    const url =
      photoRichRestaurantMenu.sections[0].items[0].imageCandidates![1];
    const document = planFixtureDocument({
      model: photoRichRestaurantMenu,
      familyId: "atelier",
      treatment: "type-led",
      outputFormat: "two-page-spread",
      imageSelections: [
        { itemId: "harvest-bowl", url, role: "editorial-crop" },
      ],
    });
    expect(document.diagnostics).toContainEqual(
      expect.objectContaining({
        id: `image:${url}:unplaced`,
        severity: "warning",
        messageKey: "print.diagnostics.imageUnplaced",
      }),
    );
  });

  it("rejects missing, duplicate, and foreign image selections deterministically", () => {
    const harvest = photoRichRestaurantMenu.sections[0].items[0];
    expect(() =>
      planFixtureDocument({
        model: photoRichRestaurantMenu,
        familyId: "gallery",
        treatment: "photo-led",
        imageSelections: [
          { itemId: "missing", url: "missing.jpg", role: "editorial-crop" },
        ],
      }),
    ).toThrow("Image selection item not found: missing");
    expect(() =>
      planFixtureDocument({
        model: photoRichRestaurantMenu,
        familyId: "gallery",
        treatment: "photo-led",
        imageSelections: harvest.imageCandidates!.slice(0, 2).map((url) => ({
          itemId: harvest.id,
          url,
          role: "editorial-crop" as const,
        })),
      }),
    ).toThrow(`Duplicate image selection itemId: ${harvest.id}`);
    expect(() =>
      planFixtureDocument({
        model: photoRichRestaurantMenu,
        familyId: "gallery",
        treatment: "photo-led",
        imageSelections: [
          { itemId: harvest.id, url: "foreign.jpg", role: "editorial-crop" },
        ],
      }),
    ).toThrow(
      `Image selection URL does not belong to item ${harvest.id}: foreign.jpg`,
    );
    const sharedUrl = "https://images.payverge.test/shared-table-1800x1200.jpg";
    expect(() =>
      planFixtureDocument({
        model: photoRichRestaurantMenu,
        familyId: "gallery",
        treatment: "photo-led",
        imageSelections: [
          { itemId: "charred-carrots", url: sharedUrl, role: "editorial-crop" },
          { itemId: "lemon-trout", url: sharedUrl, role: "editorial-crop" },
        ],
      }),
    ).toThrow(`Duplicate image selection URL: ${sharedUrl}`);
  });

  it("rejects duplicate section and globally duplicate item IDs", () => {
    expect(() =>
      planFixtureDocument({
        model: {
          ...sparseRestaurantMenu,
          sections: [
            sparseRestaurantMenu.sections[0],
            {
              ...sparseRestaurantMenu.sections[1],
              id: sparseRestaurantMenu.sections[0].id,
            },
          ],
        },
      }),
    ).toThrow(
      `Duplicate menu section id: ${sparseRestaurantMenu.sections[0].id}`,
    );
    expect(() =>
      planFixtureDocument({
        model: {
          ...sparseRestaurantMenu,
          sections: [
            sparseRestaurantMenu.sections[0],
            {
              ...sparseRestaurantMenu.sections[1],
              items: [
                {
                  ...sparseRestaurantMenu.sections[1].items[0],
                  id: sparseRestaurantMenu.sections[0].items[0].id,
                },
              ],
            },
          ],
        },
      }),
    ).toThrow(
      `Duplicate menu item id: ${sparseRestaurantMenu.sections[0].items[0].id}`,
    );
  });

  it("rejects unsupported family treatment or format combinations clearly", () => {
    expect(() =>
      planFixtureDocument(sparseRestaurantMenu, {
        direction: { familyId: "gallery", treatment: "compact" },
      }),
    ).toThrow("does not support treatment compact");
    expect(() =>
      planFixtureDocument(sparseRestaurantMenu, {
        direction: { familyId: "gallery", outputFormat: "counter-menu" },
      }),
    ).toThrow("does not support format counter-menu");
    expect(() =>
      planFixtureDocument({
        model: sparseRestaurantMenu,
        familyId: "gallery",
        treatment: "photo-led",
        outputFormat: "two-page-spread",
        imageSelections: [
          {
            itemId: "marinated-olives",
            url: "unsupported.jpg",
            role: "compact-tile",
          },
        ],
      }),
    ).toThrow("Menu family gallery does not support image role compact-tile");
  });

  it("never mutates model, audit, or direction inputs", () => {
    const model = JSON.parse(
      JSON.stringify(sparseRestaurantMenu),
    ) as PrintMenuModel;
    const audit = makeAudit(model, { failedUrls: ["failed.jpg"] });
    const direction = makeDirection({ imageSelections: [] });
    const snapshot = JSON.stringify({ model, audit, direction });
    planMenuDocument({ model, audit, direction });
    expect(JSON.stringify({ model, audit, direction })).toBe(snapshot);
  });

  it("provides forward-compatible typed fixture override builders", () => {
    expect(makeAudit({ photoCoverage: 0.8 }).photoCoverage).toBe(0.8);
    const document = planFixtureDocument({
      model: sparseRestaurantMenu,
      familyId: "maison",
      treatment: "type-led",
      outputFormat: "folded-booklet",
      paperFormat: "a4",
      failedUrls: ["failed.jpg"],
      lowResolutionUrls: ["small.jpg"],
    });
    expect(document.direction.familyId).toBe("maison");
    expect(document.pages).toHaveLength(2);
    expect(document.diagnostics.map(({ id }) => id)).toEqual(
      expect.arrayContaining([
        "image:failed.jpg:failed",
        "image:small.jpg:low-resolution",
      ]),
    );
  });
});
