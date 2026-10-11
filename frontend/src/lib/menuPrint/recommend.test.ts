import type { MenuPrintAudit } from "./audit";
import { planFixtureDocument } from "./__fixtures__/plannedDocuments";
import { getMenuDesignFamily } from "./families";
import { recommendMenuDirections } from "./recommend";
import {
  MENU_DESIGN_FAMILY_IDS,
  MENU_OUTPUT_FORMATS,
  MENU_TREATMENTS,
} from "./types";

const makeAudit = (
  overrides: Partial<MenuPrintAudit> = {},
): MenuPrintAudit => ({
  categoryCount: 4,
  itemCount: 12,
  describedItemCount: 12,
  averageDescriptionLength: 48,
  availablePhotoCount: 0,
  photoCoverage: 0,
  lowResolutionUrls: [],
  failedUrls: [],
  duplicateUrls: [],
  ...overrides,
});

describe("recommendMenuDirections", () => {
  it.each([
    "cafe",
    "fine_dining",
    "bar",
    "quick_service",
    "bakery",
    "food_truck",
    "restaurant",
    "other",
    "unknown",
  ])("never recommends an unsupported direction for %s", (businessType) => {
    for (const audit of [
      makeAudit({ categoryCount: 1, itemCount: 3 }),
      makeAudit({
        categoryCount: 8,
        itemCount: 48,
        averageDescriptionLength: 120,
        photoCoverage: 0.8,
      }),
    ]) {
      for (const recommendation of recommendMenuDirections({
        audit,
        businessType,
      })) {
        const family = getMenuDesignFamily(recommendation.familyId);
        expect(family.supportedFormats).toContain(recommendation.outputFormat);
        expect(family.supportedTreatments).toContain(recommendation.treatment);
      }
    }
  });

  it.each([
    ["cafe", 0.9, 18, "counter", "photo-led", "takeaway-trifold"],
    ["fine_dining", 0.1, 12, "maison", "type-led", "two-page-spread"],
    ["bar", 0.4, 42, "night-house", "compact", "two-page-spread"],
    ["quick_service", 0.7, 58, "atelier", "compact", "two-page-spread"],
  ] as const)(
    "recommends a stable direction for %s",
    (
      businessType,
      photoCoverage,
      itemCount,
      familyId,
      treatment,
      outputFormat,
    ) => {
      expect(
        recommendMenuDirections({
          audit: makeAudit({ photoCoverage, itemCount }),
          businessType,
        })[0],
      ).toMatchObject({ familyId, treatment, outputFormat });
    },
  );

  it("returns at least three distinct, valid recommendations", () => {
    const recommendations = recommendMenuDirections({
      audit: makeAudit(),
      businessType: "restaurant",
    });

    expect(recommendations.length).toBeGreaterThanOrEqual(3);
    expect(new Set(recommendations.map(({ familyId }) => familyId)).size).toBe(
      recommendations.length,
    );
    for (const recommendation of recommendations) {
      expect(MENU_DESIGN_FAMILY_IDS).toContain(recommendation.familyId);
      expect(MENU_TREATMENTS).toContain(recommendation.treatment);
      expect(MENU_OUTPUT_FORMATS).toContain(recommendation.outputFormat);
      expect(recommendation.reasonKey).toMatch(
        /^print\.recommendation\.[a-zA-Z]+$/,
      );
      expect(Number.isFinite(recommendation.score)).toBe(true);
    }
  });

  it("keeps the street takeaway direction first for focused food trucks", () => {
    expect(
      recommendMenuDirections({
        audit: makeAudit({ itemCount: 5, photoCoverage: 0.4 }),
        businessType: "food_truck",
      })[0],
    ).toMatchObject({
      familyId: "street",
      treatment: "balanced",
      outputFormat: "takeaway-trifold",
    });
  });

  it("keeps a small photo-rich restaurant menu on one confident sheet", () => {
    expect(
      recommendMenuDirections({
        audit: makeAudit({
          categoryCount: 3,
          itemCount: 6,
          photoCoverage: 1,
        }),
        businessType: "restaurant",
      })[0],
    ).toMatchObject({
      familyId: "atelier",
      treatment: "photo-led",
      outputFormat: "single-sheet",
    });
  });

  it.each([
    [
      { itemCount: 36, averageDescriptionLength: 110, photoCoverage: 0.6 },
      "photo-led",
    ],
    [
      { itemCount: 37, averageDescriptionLength: 0, photoCoverage: 0.9 },
      "compact",
    ],
    [
      { itemCount: 1, averageDescriptionLength: 111, photoCoverage: 0.9 },
      "compact",
    ],
    [
      { itemCount: 1, averageDescriptionLength: 110, photoCoverage: 0.2 },
      "balanced",
    ],
    [
      { itemCount: 1, averageDescriptionLength: 110, photoCoverage: 0.19 },
      "type-led",
    ],
  ] as const)(
    "applies treatment thresholds for audit %j",
    (audit, treatment) => {
      const recommendations = recommendMenuDirections({
        audit: makeAudit(audit),
        businessType: "other",
      });

      for (const recommendation of recommendations) {
        const family = getMenuDesignFamily(recommendation.familyId);
        expect(recommendation.treatment).toBe(
          family.supportedTreatments.includes(treatment)
            ? treatment
            : "balanced",
        );
      }
    },
  );

  it.each([
    ["cafe", 11, 24, "single-sheet"],
    ["restaurant", 13, 0, "single-sheet"],
    ["bar", 13, 0, "folded-booklet"],
    ["quick_service", 13, 0, "takeaway-trifold"],
    ["cafe", 18, 0, "takeaway-trifold"],
    ["bar", 18, 0, "folded-booklet"],
    ["quick_service", 18, 0, "takeaway-trifold"],
    ["bar", 42, 0, "two-page-spread"],
    ["quick_service", 58, 0, "two-page-spread"],
  ] as const)(
    "opens the top %s recommendation for %i items in a printable direction",
    (businessType, itemCount, descriptionLength, expectedFormat) => {
      const sectionCount = 3;
      const sections = Array.from(
        { length: sectionCount },
        (_, sectionIndex) => ({
          id: `section-${sectionIndex}`,
          name: `Section ${sectionIndex + 1}`,
          items: Array.from(
            {
              length:
                Math.floor(itemCount / sectionCount) +
                (sectionIndex < itemCount % sectionCount ? 1 : 0),
            },
            (_, itemIndex) => ({
              id: `item-${sectionIndex}-${itemIndex}`,
              name: `Dish ${sectionIndex + 1}.${itemIndex + 1}`,
              description: descriptionLength
                ? "A".repeat(descriptionLength)
                : undefined,
              price: 12 + itemIndex,
              imageCandidates: [],
              dietaryTags: [],
              allergens: [],
            }),
          ),
        }),
      );
      const audit = makeAudit({
        categoryCount: sectionCount,
        itemCount,
        describedItemCount: descriptionLength ? itemCount : 0,
        averageDescriptionLength: descriptionLength,
      });
      const recommendation = recommendMenuDirections({
        audit,
        businessType,
      })[0];
      const document = planFixtureDocument({
        model: {
          business: { name: "Test Restaurant", businessType },
          sections,
          currency: "USD",
          language: "en",
          warnings: [],
        },
        audit,
        direction: recommendation,
      });

      expect(recommendation.outputFormat).toBe(expectedFormat);
      expect(document.readiness).not.toBe("blocked");
    },
  );

  it.each([
    ["cafe", 4, 110, 0, "type-led", "single-sheet"],
    ["bakery", 4, 110, 0, "type-led", "single-sheet"],
    ["cafe", 7, 90, 0, "type-led", "single-sheet"],
    ["bakery", 7, 90, 0, "type-led", "single-sheet"],
    ["cafe", 6, 24, 1, "photo-led", "single-sheet"],
    ["cafe", 6, 130, 0, "compact", "single-sheet"],
    ["cafe", 17, 24, 1, "photo-led", "takeaway-trifold"],
  ] as const)(
    "keeps a %s recommendation printable with %i items, %i-character copy, and %i photo coverage",
    (
      businessType,
      itemCount,
      averageDescriptionLength,
      photoCoverage,
      treatment,
      outputFormat,
    ) => {
      const sections = [
        {
          id: "menu",
          name: "Menu",
          items: Array.from({ length: itemCount }, (_, index) => ({
            id: `item-${index}`,
            name: `Dish ${index + 1}`,
            description: averageDescriptionLength
              ? "A".repeat(averageDescriptionLength)
              : undefined,
            price: 10 + index,
            imageCandidates: photoCoverage ? [`dish-${index}.jpg`] : [],
            dietaryTags: [],
            allergens: [],
          })),
        },
      ];
      const audit = makeAudit({
        categoryCount: 1,
        itemCount,
        describedItemCount: averageDescriptionLength ? itemCount : 0,
        averageDescriptionLength,
        photoCoverage,
        availablePhotoCount: photoCoverage ? itemCount : 0,
      });
      const recommendation = recommendMenuDirections({
        audit,
        businessType,
      })[0];
      const document = planFixtureDocument({
        model: {
          business: { name: "Test Restaurant", businessType },
          sections,
          currency: "USD",
          language: "en",
          warnings: [],
        },
        audit,
        direction: recommendation,
      });

      expect(recommendation).toMatchObject({
        familyId: "counter",
        outputFormat,
        treatment,
      });
      expect(document.readiness).not.toBe("blocked");
    },
  );

  it("uses a deterministic fallback and sorts score ties by family id", () => {
    const audit = makeAudit();
    const first = recommendMenuDirections({ audit, businessType: "unknown" });
    const second = recommendMenuDirections({ audit, businessType: "unknown" });
    const missingType = recommendMenuDirections({
      audit,
      businessType: undefined,
    });

    expect(first).toEqual(second);
    expect(first).toEqual(missingType);
    expect(first[0].familyId).toBe("atelier");
    for (let index = 1; index < first.length; index += 1) {
      const previous = first[index - 1];
      const current = first[index];
      expect(previous.score).toBeGreaterThanOrEqual(current.score);
      if (previous.score === current.score) {
        expect(previous.familyId.localeCompare(current.familyId)).toBeLessThan(
          0,
        );
      }
    }
  });

  it.each(["__proto__", "constructor", "toString"])(
    "uses the general fallback for inherited object key %s",
    (businessType) => {
      const audit = makeAudit();
      const expected = recommendMenuDirections({
        audit,
        businessType: "unknown",
      });

      expect(recommendMenuDirections({ audit, businessType })).toEqual(
        expected,
      );
    },
  );

  it("does not mutate the audit or its nested arrays", () => {
    const audit = makeAudit({
      lowResolutionUrls: ["small.jpg"],
      failedUrls: ["failed.jpg"],
    });
    const snapshot = JSON.parse(JSON.stringify(audit)) as MenuPrintAudit;

    recommendMenuDirections({ audit, businessType: "cafe" });

    expect(audit).toEqual(snapshot);
  });
});
