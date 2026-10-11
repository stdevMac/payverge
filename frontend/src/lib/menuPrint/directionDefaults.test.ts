import { makeAudit } from "./__fixtures__/plannedDocuments";
import { photoRichRestaurantMenu } from "./__fixtures__/restaurantMenus";
import { getMenuDesignFamily } from "./families";
import { selectDefaultPrintImages } from "./directionDefaults";

function printReadyAudit() {
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
  return makeAudit(photoRichRestaurantMenu, {
    photoCoverage: 0.8,
    measurements,
  });
}

describe("selectDefaultPrintImages", () => {
  it("selects a section-diverse non-cover set for a photo-led family", () => {
    const selections = selectDefaultPrintImages({
      family: getMenuDesignFamily("gallery"),
      model: photoRichRestaurantMenu,
      audit: printReadyAudit(),
      treatment: "photo-led",
    });

    expect(selections).toHaveLength(2);
    expect(selections[0]).toMatchObject({ itemId: "harvest-bowl" });
    expect(selections[1]).toMatchObject({ itemId: "lemon-trout" });
    expect(selections[0].role).not.toBe("cover");
  });

  it("keeps balanced photography restrained and distinct from photo-led", () => {
    const family = getMenuDesignFamily("gallery");
    const audit = printReadyAudit();
    const balanced = selectDefaultPrintImages({
      family,
      model: photoRichRestaurantMenu,
      audit,
      treatment: "balanced",
    });
    const photoLed = selectDefaultPrintImages({
      family,
      model: photoRichRestaurantMenu,
      audit,
      treatment: "photo-led",
    });

    expect(balanced).toHaveLength(1);
    expect(photoLed.length).toBeGreaterThan(balanced.length);
  });

  it("does not auto-promote an image below the feature-print threshold", () => {
    const firstItem = photoRichRestaurantMenu.sections[0].items[0];
    const url = firstItem.imageCandidates![0];
    const selections = selectDefaultPrintImages({
      family: getMenuDesignFamily("gallery"),
      model: photoRichRestaurantMenu,
      audit: {
        ...printReadyAudit(),
        measurements: {
          ...printReadyAudit().measurements,
          [url]: { url, status: "ready", width: 1100, height: 900 },
        },
      },
      treatment: "photo-led",
    });

    expect(selections.some((selection) => selection.url === url)).toBe(false);
  });

  it("fails closed while image dimensions are still unknown", () => {
    expect(
      selectDefaultPrintImages({
        family: getMenuDesignFamily("gallery"),
        model: photoRichRestaurantMenu,
        audit: makeAudit(photoRichRestaurantMenu),
        treatment: "photo-led",
      }),
    ).toEqual([]);
  });

  it("keeps type-led and compact directions intentionally image-free", () => {
    for (const treatment of ["type-led", "compact"] as const) {
      expect(
        selectDefaultPrintImages({
          family: getMenuDesignFamily("atelier"),
          model: photoRichRestaurantMenu,
          audit: makeAudit(photoRichRestaurantMenu, { photoCoverage: 0.8 }),
          treatment,
        }),
      ).toEqual([]);
    }
  });

  it("never selects failed or low-resolution photography", () => {
    const firstItem = photoRichRestaurantMenu.sections[0].items[0];
    const selections = selectDefaultPrintImages({
      family: getMenuDesignFamily("atelier"),
      model: photoRichRestaurantMenu,
      audit: makeAudit(photoRichRestaurantMenu, {
        ...printReadyAudit(),
        failedUrls: firstItem.imageCandidates!,
        lowResolutionUrls: [],
      }),
      treatment: "balanced",
    });

    expect(selections[0]?.itemId).not.toBe(firstItem.id);
  });
});
