import { resolveStorefrontDisplayBusiness } from "./storefrontDisplayBusiness";
import type { PublicBusiness } from "@/api/publicBusiness";

const spanishSeed = {
  id: 1,
  name: "Demo Kitchen",
  default_language: "en",
  description: "Un restaurante modelo de Payverge.",
  welcome_message: "Bienvenidos.",
  about_story: "Un entorno de demostración realista.",
  special_features: [{ title: "Terraza", description: "Al aire libre", is_active: true }],
  gallery_images: [{ caption: "Galería de demostración 1" }],
} as unknown as PublicBusiness;

const englishLive = {
  ...spanishSeed,
  description: "A Payverge model restaurant with realistic demo data.",
  welcome_message: "Welcome. The demo team is ready.",
  about_story: "A realistic Payverge demo environment.",
  special_features: [{ title: "Patio", description: "Outdoor seating", is_active: true }],
  gallery_images: [{ caption: "Demo gallery 1" }],
} as unknown as PublicBusiness;

describe("resolveStorefrontDisplayBusiness (#400)", () => {
  it("keeps the Spanish seed while the locale still matches the seed", () => {
    expect(
      resolveStorefrontDisplayBusiness({
        seed: spanishSeed,
        localized: null,
        currentLanguage: "es",
        seedLanguage: "es",
      }).description,
    ).toBe("Un restaurante modelo de Payverge.");
  });

  it("does not reuse Spanish authored fields after a switch to English", () => {
    const display = resolveStorefrontDisplayBusiness({
      seed: spanishSeed,
      localized: null,
      currentLanguage: "en",
      seedLanguage: "es",
    });
    expect(display.description).toBe("");
    expect(display.welcome_message).toBe("");
    expect(display.about_story).toBe("");
    expect(display.special_features).toEqual([]);
    expect(display.gallery_images).toEqual([]);
    expect(display.name).toBe("Demo Kitchen");
  });

  it("uses the English refetch once it arrives", () => {
    expect(
      resolveStorefrontDisplayBusiness({
        seed: spanishSeed,
        localized: englishLive,
        currentLanguage: "en",
        seedLanguage: "es",
      }).description,
    ).toBe("A Payverge model restaurant with realistic demo data.");
  });
});
