import {
  deliveryGuestPath,
  resolveDeliveryInitialLanguage,
} from "./deliveryLocale";

describe("deliveryGuestPath", () => {
  it("omits English and empty lang from recovery links", () => {
    expect(deliveryGuestPath("QA-1", "track")).toBe("/delivery/QA-1/track");
    expect(deliveryGuestPath("QA-1", "pay", "en")).toBe("/delivery/QA-1/pay");
  });

  it("preserves an explicit guest locale on track and pay", () => {
    expect(deliveryGuestPath("QA-1", "track", "es-AR")).toBe(
      "/delivery/QA-1/track?lang=es-AR",
    );
    expect(deliveryGuestPath("QA-1", "pay", "es")).toBe(
      "/delivery/QA-1/pay?lang=es",
    );
  });
});

describe("resolveDeliveryInitialLanguage", () => {
  it("honors an explicit supported lang query", () => {
    expect(resolveDeliveryInitialLanguage("es-AR")).toBe("es-AR");
    expect(resolveDeliveryInitialLanguage("es")).toBe("es");
  });

  it("falls back to English for missing or unsupported queries", () => {
    expect(resolveDeliveryInitialLanguage(null)).toBe("en");
    expect(resolveDeliveryInitialLanguage("xx")).toBe("en");
  });
});
