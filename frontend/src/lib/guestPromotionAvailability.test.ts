import {
  computeGuestFulfillmentMinimumDelta,
  filterGuestMerchBundles,
  filterGuestMerchOffers,
  filterGuestSellableBundles,
  filterGuestSellableOffers,
  isGuestBundleSellable,
  isGuestOfferSellable,
  parseGuestBundleItems,
  resolveGuestBundleChildName,
  shouldHideGuestPromoMerchForSearch,
} from "./guestPromotionAvailability";

const steak = {
  id: "demo-steak",
  name: "Steak Plate",
  is_available: true,
};
const cocktail = {
  id: "demo-cocktail",
  name: "Demo Spritz",
  is_available: true,
};
const dessert = {
  id: "demo-dessert",
  name: "Chocolate Tart",
  is_available: true,
};

const dateNight = {
  id: 9,
  name: "Date Night for Two",
  is_active: true,
  items: [
    { menu_item_id: "demo-steak", name: "Steak Plate", quantity: 1 },
    { menu_item_id: "demo-cocktail", name: "Demo Spritz", quantity: 2 },
    { menu_item_id: "demo-dessert", name: "Chocolate Tart", quantity: 1 },
  ],
};

const steakOffer = {
  name: "$5 Off the Steak Plate",
  is_active: true,
  applicable_to: "item",
  target_id: "demo-steak",
};

const lunchOffer = {
  name: "Weekday Lunch 15% Off",
  is_active: true,
  applicable_to: "all",
};

const dateNightOffer = {
  name: "$10 Off Date Night",
  is_active: true,
  applicable_to: "bundle",
  target_id: "9",
};

const openCatalog = {
  categories: [{ items: [steak, cocktail, dessert] }],
  orderability: {
    "demo-steak": { state: "available" as const, orderable: true },
    "demo-cocktail": { state: "available" as const, orderable: true },
    "demo-dessert": { state: "available" as const, orderable: true },
  },
  bundles: [dateNight],
};

describe("parseGuestBundleItems", () => {
  it("reads object refs, string ids, and JSON payloads", () => {
    expect(parseGuestBundleItems(dateNight)).toHaveLength(3);
    expect(parseGuestBundleItems({ items: ["demo-steak"] })).toEqual([
      { menu_item_id: "demo-steak", quantity: 1 },
    ]);
    expect(
      parseGuestBundleItems({
        items: JSON.stringify([{ menu_item_id: "demo-steak", quantity: 2 }]),
      }),
    ).toEqual([{ menu_item_id: "demo-steak", quantity: 2 }]);
    expect(parseGuestBundleItems({ items: "not-json" })).toEqual([]);
  });
});

describe("guest promotion availability (issue 345)", () => {
  it("keeps the steak offer and Date Night while those targets are orderable", () => {
    expect(isGuestOfferSellable(steakOffer, openCatalog)).toBe(true);
    expect(isGuestBundleSellable(dateNight, openCatalog)).toBe(true);
    expect(
      filterGuestSellableOffers([steakOffer, lunchOffer], openCatalog).map(
        (offer) => offer.name,
      ),
    ).toEqual(["$5 Off the Steak Plate", "Weekday Lunch 15% Off"]);
  });

  it("hides $5 Off the Steak Plate when the steak is 86'd", () => {
    const catalog = {
      ...openCatalog,
      categories: [
        {
          items: [
            { ...steak, is_available: false },
            cocktail,
            dessert,
          ],
        },
      ],
    };
    expect(isGuestOfferSellable(steakOffer, catalog)).toBe(false);
    expect(
      filterGuestSellableOffers([steakOffer, lunchOffer], catalog).map(
        (offer) => offer.name,
      ),
    ).toEqual(["Weekday Lunch 15% Off"]);
  });

  it("hides the steak offer when orderability marks demo-steak unorderable", () => {
    const catalog = {
      ...openCatalog,
      orderability: {
        ...openCatalog.orderability,
        "demo-steak": { state: "inventory_out" as const, orderable: false },
      },
    };
    expect(isGuestOfferSellable(steakOffer, catalog)).toBe(false);
  });

  it("hides Date Night when the steak child is unavailable", () => {
    const catalog = {
      ...openCatalog,
      categories: [
        {
          items: [
            { ...steak, is_available: false },
            cocktail,
            dessert,
          ],
        },
      ],
    };
    expect(isGuestBundleSellable(dateNight, catalog)).toBe(false);
    expect(filterGuestSellableBundles([dateNight], catalog)).toEqual([]);
  });

  it("hides a bundle-targeted offer when Date Night cannot be fulfilled", () => {
    const catalog = {
      ...openCatalog,
      orderability: {
        ...openCatalog.orderability,
        "demo-steak": { state: "manual_disabled" as const, orderable: false },
      },
    };
    expect(isGuestOfferSellable(dateNightOffer, catalog)).toBe(false);
  });

  it("does not treat venue-closed orderability as an item 86", () => {
    const catalog = {
      ...openCatalog,
      orderability: {
        "demo-steak": { state: "business_closed" as const, orderable: false },
        "demo-cocktail": { state: "business_closed" as const, orderable: false },
        "demo-dessert": { state: "business_closed" as const, orderable: false },
      },
    };
    expect(isGuestOfferSellable(steakOffer, catalog)).toBe(true);
    expect(isGuestBundleSellable(dateNight, catalog)).toBe(true);
  });

  it("keeps broad / category offers even when the steak is 86'd", () => {
    const catalog = {
      ...openCatalog,
      categories: [{ items: [{ ...steak, is_available: false }, cocktail] }],
    };
    expect(
      isGuestOfferSellable(
        { ...lunchOffer, applicable_to: "category", target_id: "Mains" },
        catalog,
      ),
    ).toBe(true);
  });

  it("hides an item-targeted offer whose menu row is gone once the catalog loaded", () => {
    const catalog = {
      ...openCatalog,
      categories: [{ items: [cocktail, dessert] }],
    };
    expect(isGuestOfferSellable(steakOffer, catalog)).toBe(false);
  });

  it("keeps offers while the menu catalog has not arrived", () => {
    expect(
      isGuestOfferSellable(steakOffer, { categories: [], orderability: {} }),
    ).toBe(true);
    expect(
      isGuestBundleSellable(dateNight, { categories: [], orderability: {} }),
    ).toBe(true);
  });
});

describe("resolveGuestBundleChildName (issue 398)", () => {
  const spanishCatalog = {
    categories: [
      {
        items: [
          { id: "demo-steak", name: "Plato de bistec" },
          { id: "demo-cocktail", name: "Spritz demo" },
          { id: "demo-dessert", name: "Tarta de chocolate" },
        ],
      },
    ],
  };

  it("prefers the localized catalog name over the English snapshot", () => {
    expect(
      resolveGuestBundleChildName(
        { menu_item_id: "demo-steak", name: "Steak Plate" },
        spanishCatalog,
      ),
    ).toBe("Plato de bistec");
    expect(
      resolveGuestBundleChildName(
        { menu_item_id: "demo-cocktail", name: "Demo Spritz" },
        new Map([["demo-cocktail", { name: "Spritz demo" }]]),
      ),
    ).toBe("Spritz demo");
  });

  it("falls back to the snapshot name when the catalog has no row", () => {
    expect(
      resolveGuestBundleChildName(
        { menu_item_id: "missing-item", name: "Steak Plate" },
        spanishCatalog,
      ),
    ).toBe("Steak Plate");
  });

  it("keeps English catalog names unchanged", () => {
    expect(
      resolveGuestBundleChildName(
        { menu_item_id: "demo-steak", name: "Steak Plate" },
        openCatalog,
      ),
    ).toBe("Steak Plate");
  });

  it("uses the same catalog-first rule for regional (es-AR) names", () => {
    expect(
      resolveGuestBundleChildName(
        { menu_item_id: "demo-steak", name: "Steak Plate" },
        { categories: [{ items: [{ id: "demo-steak", name: "Plato de bife" }] }] },
      ),
    ).toBe("Plato de bife");
  });
});

describe("guest merch search (issue 417)", () => {
  it("matches item-adjacent offer, bundle, and child queries", () => {
    expect(
      filterGuestMerchOffers([steakOffer, lunchOffer], "steak plate").map(
        (offer) => offer.name,
      ),
    ).toEqual(["$5 Off the Steak Plate"]);
    expect(filterGuestMerchOffers([steakOffer, lunchOffer], "weekday")).toEqual([
      lunchOffer,
    ]);
    expect(
      filterGuestMerchBundles([dateNight], "date night", openCatalog).map(
        (bundle) => bundle.name,
      ),
    ).toEqual(["Date Night for Two"]);
    expect(
      filterGuestMerchBundles([dateNight], "chocolate tart", openCatalog),
    ).toEqual([dateNight]);
    expect(
      filterGuestMerchBundles([dateNight], "zzzz-no-match", openCatalog),
    ).toEqual([]);
  });

  it("hides leftover merch only when search is active and no items remain", () => {
    expect(shouldHideGuestPromoMerchForSearch(false, 0)).toBe(false);
    expect(shouldHideGuestPromoMerchForSearch(true, 3)).toBe(false);
    expect(shouldHideGuestPromoMerchForSearch(true, 0)).toBe(true);
  });
});

describe("computeGuestFulfillmentMinimumDelta (issue 406)", () => {
  it("uses the live cart subtotal, not the address-time quote of zero", () => {
    expect(computeGuestFulfillmentMinimumDelta(0, 15)).toBe(15);
    expect(computeGuestFulfillmentMinimumDelta(10, 15)).toBe(5);
    expect(computeGuestFulfillmentMinimumDelta(15, 15)).toBe(0);
    expect(computeGuestFulfillmentMinimumDelta(18.5, 15)).toBe(0);
    expect(computeGuestFulfillmentMinimumDelta(50, 15)).toBe(0);
  });
});
