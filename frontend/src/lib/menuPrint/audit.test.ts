import { photoRichRestaurantMenu } from "./__fixtures__/restaurantMenus";
import {
  MIN_TILE_PRINT_PX,
  auditMenuForPrint,
  effectivePrintDpi,
  type ImageMeasurement,
} from "./audit";
import type { PrintMenuModel } from "./types";

const emptyMenu: PrintMenuModel = {
  business: { name: "Empty Restaurant" },
  sections: [],
  currency: "USD",
  language: "en",
  warnings: [],
};

describe("auditMenuForPrint", () => {
  it("computes effective resolution from the actual printed rectangle", () => {
    expect(
      effectivePrintDpi(
        { url: "dish.jpg", status: "ready", width: 1200, height: 900 },
        { widthMm: 100, heightMm: 75 },
      ),
    ).toBeCloseTo(304.8, 1);
  });
  it("audits content and measured photography in a normalized menu", () => {
    const measurements: Record<string, ImageMeasurement> = {
      "harvest.jpg": {
        url: "harvest.jpg",
        width: 1600,
        height: 1200,
        status: "ready",
      },
      "tiny.jpg": {
        url: "tiny.jpg",
        width: 240,
        height: 180,
        status: "ready",
      },
      "broken.jpg": { url: "broken.jpg", status: "failed" },
    };

    const audit = auditMenuForPrint(photoRichRestaurantMenu, measurements);

    expect(audit.itemCount).toBeGreaterThan(0);
    expect(audit.photoCoverage).toBeGreaterThan(0);
    expect(audit.lowResolutionUrls).toContain("tiny.jpg");
    expect(audit.failedUrls).toContain("broken.jpg");
    expect(audit.averageDescriptionLength).toBeGreaterThanOrEqual(0);
  });

  it("returns zero counts and empty URL lists for an empty menu", () => {
    expect(auditMenuForPrint(emptyMenu, {})).toEqual({
      categoryCount: 0,
      itemCount: 0,
      describedItemCount: 0,
      averageDescriptionLength: 0,
      availablePhotoCount: 0,
      photoCoverage: 0,
      lowResolutionUrls: [],
      failedUrls: [],
      duplicateUrls: [],
      measurements: {},
    });
  });

  it("reports every repeated image occurrence in source order", () => {
    const menu: PrintMenuModel = {
      ...emptyMenu,
      sections: [
        {
          id: "mains",
          name: "Mains",
          items: [
            {
              id: "one",
              name: "One",
              price: 10,
              imageCandidates: ["shared.jpg", "shared.jpg"],
              dietaryTags: [],
              allergens: [],
            },
            {
              id: "two",
              name: "Two",
              price: 12,
              imageCandidates: ["other.jpg", "shared.jpg"],
              dietaryTags: [],
              allergens: [],
            },
          ],
        },
      ],
    };

    expect(auditMenuForPrint(menu, {}).duplicateUrls).toEqual([
      "shared.jpg",
      "shared.jpg",
    ]);
  });

  it("accepts the tile threshold and flags measurements below it", () => {
    const menu: PrintMenuModel = {
      ...emptyMenu,
      sections: [
        {
          id: "photos",
          name: "Photos",
          items: [
            {
              id: "threshold",
              name: "Threshold",
              price: 10,
              imageCandidates: ["threshold.jpg", "below.jpg"],
              dietaryTags: [],
              allergens: [],
            },
          ],
        },
      ],
    };

    const audit = auditMenuForPrint(menu, {
      "threshold.jpg": {
        url: "threshold.jpg",
        width: MIN_TILE_PRINT_PX,
        height: MIN_TILE_PRINT_PX,
        status: "ready",
      },
      "below.jpg": {
        url: "below.jpg",
        width: MIN_TILE_PRINT_PX - 1,
        height: MIN_TILE_PRINT_PX,
        status: "ready",
      },
    });

    expect(audit.lowResolutionUrls).toEqual(["below.jpg"]);
  });

  it("does not count a failed unique URL as an available photo", () => {
    const menu: PrintMenuModel = {
      ...emptyMenu,
      sections: [
        {
          id: "photos",
          name: "Photos",
          items: [
            {
              id: "one",
              name: "One",
              price: 10,
              imageCandidates: ["ready.jpg", "failed.jpg", "failed.jpg"],
              dietaryTags: [],
              allergens: [],
            },
          ],
        },
      ],
    };

    const audit = auditMenuForPrint(menu, {
      "ready.jpg": {
        url: "ready.jpg",
        width: 1200,
        height: 1200,
        status: "ready",
      },
      "failed.jpg": { url: "failed.jpg", status: "failed" },
    });

    expect(audit.availablePhotoCount).toBe(1);
    expect(audit.failedUrls).toEqual(["failed.jpg"]);
  });
});
