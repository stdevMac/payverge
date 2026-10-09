/**
 * #829 — delivery partners are couriers. OpenTable and Resy are reservation
 * platforms and must not appear in the delivery partner picker, while the
 * full catalog keeps them for reservation booking links.
 */
import {
  DELIVERY_PROVIDER_CATALOG,
  PROVIDER_CATALOG,
  PROVIDER_CATALOG_BY_KEY,
  RESERVATION_PROVIDER_CATALOG,
} from "./providerCatalog";

describe("#829 provider catalog kinds", () => {
  it("excludes reservation platforms from the delivery catalog", () => {
    const deliveryKeys = DELIVERY_PROVIDER_CATALOG.map((p) => p.key);
    expect(deliveryKeys).not.toContain("opentable");
    expect(deliveryKeys).not.toContain("resy");
    expect(deliveryKeys.sort()).toEqual(
      ["careem", "deliveroo", "doordash", "talabat", "ubereats", "zomato"].sort(),
    );
  });

  it("keeps OpenTable/Resy in the full and reservation catalogs", () => {
    expect(PROVIDER_CATALOG_BY_KEY.opentable).toBeDefined();
    expect(PROVIDER_CATALOG_BY_KEY.resy).toBeDefined();
    expect(RESERVATION_PROVIDER_CATALOG.map((p) => p.key).sort()).toEqual([
      "opentable",
      "resy",
    ]);
  });

  it("tags every catalog entry with a kind", () => {
    for (const provider of PROVIDER_CATALOG) {
      expect(["delivery", "reservation"]).toContain(provider.kind);
    }
  });
});
