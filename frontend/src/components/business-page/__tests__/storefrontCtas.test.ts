import {
  resolvePrimaryStorefrontCta,
  STOREFRONT_CTA_LABEL_KEYS,
} from "../storefrontCtas";

describe("resolvePrimaryStorefrontCta", () => {
  it("prefers reservations over delivery and menu", () => {
    expect(
      resolvePrimaryStorefrontCta({ hasReservations: true, hasDelivery: true }),
    ).toBe("reservations");
  });

  it("uses delivery when reservations are off", () => {
    expect(
      resolvePrimaryStorefrontCta({ hasReservations: false, hasDelivery: true }),
    ).toBe("delivery");
  });

  it("falls back to menu", () => {
    expect(resolvePrimaryStorefrontCta({})).toBe("menu");
    expect(STOREFRONT_CTA_LABEL_KEYS.menu).toBe("businessPage.hero.viewMenuCta");
  });
});
