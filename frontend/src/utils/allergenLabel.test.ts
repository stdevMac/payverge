import { getTranslation } from "@/i18n/getTranslation";
import { ALLERGENS } from "@/constants/menu-tags";
import {
  allergenDisplayName,
  operatorAllergenNameKey,
} from "@/utils/allergenLabel";

const TOKENS = ["dairy", "eggs", "fish", "crustaceans", "so2"] as const;
const LOCALES = ["en", "es", "es-AR"] as const;

const EXPECTED: Record<(typeof LOCALES)[number], Record<(typeof TOKENS)[number], string>> = {
  en: {
    dairy: "Dairy",
    eggs: "Eggs",
    fish: "Fish",
    crustaceans: "Crustaceans",
    so2: "Sulphur Dioxide",
  },
  es: {
    dairy: "Lácteos",
    eggs: "Huevos",
    fish: "Pescado",
    crustaceans: "Crustáceos",
    so2: "Dióxido de azufre",
  },
  "es-AR": {
    dairy: "Lácteos",
    eggs: "Huevos",
    fish: "Pescado",
    crustaceans: "Crustáceos",
    so2: "Dióxido de azufre",
  },
};

describe("allergenDisplayName (#574)", () => {
  it("maps every catalog token to a restaurant-ready label in en/es/es-AR", () => {
    for (const locale of LOCALES) {
      const translate = (key: string) => {
        const value = getTranslation(key, locale);
        return Array.isArray(value) ? value[0] ?? key : value;
      };

      for (const allergen of ALLERGENS) {
        const label = allergenDisplayName(allergen.id, translate);
        expect(label).not.toBe(allergen.id);
        expect(label).not.toMatch(/^(so2|SO2)$/);
        expect(label.length).toBeGreaterThan(2);
      }

      for (const token of TOKENS) {
        const label = allergenDisplayName(token, translate);
        expect(label).toBe(EXPECTED[locale][token]);
        expect(operatorAllergenNameKey(token)).toBe(
          `businessDashboard.dashboard.menuBuilder.items.allergenNames.${token}`,
        );
      }
    }
  });

  it("humanizes unknown extracted tokens instead of leaking the raw enum (#574 fallback)", () => {
    const translate = (key: string) => key;
    expect(allergenDisplayName("unicorn_dust", translate)).toBe("Unicorn dust");
    expect(allergenDisplayName("unicorn_dust", translate)).not.toBe("unicorn_dust");
  });

  it("never renders a raw token for any garbage/future allergen value", () => {
    const translate = (key: string) => key;
    const garbageTokens = [
      "future_allergen_2027",
      "MSG",
      "sulfite-blend",
      "unknown.nested.token",
    ];
    for (const token of garbageTokens) {
      const label = allergenDisplayName(token, translate);
      expect(label).not.toBe(token);
      expect(label.trim().length).toBeGreaterThan(0);
    }
  });

  it("accepts mixed-case extracted tokens", () => {
    const translate = (key: string) =>
      getTranslation(key, "en") as string;
    expect(allergenDisplayName("SO2", translate)).toBe("Sulphur Dioxide");
    expect(allergenDisplayName("Dairy", translate)).toBe("Dairy");
  });
});

describe("menu review field names (#575)", () => {
  it.each(["en", "es", "es-AR"] as const)(
    "interpolates field purpose and identity in %s",
    (locale) => {
      const name = getTranslation(
        "aiMenuOnboarding.reviewEditor.itemNameAria",
        locale,
        { name: "Milanesa" },
      ) as string;
      const description = getTranslation(
        "aiMenuOnboarding.reviewEditor.itemDescriptionAria",
        locale,
        { name: "Milanesa" },
      ) as string;
      const category = getTranslation(
        "aiMenuOnboarding.reviewEditor.categoryNameAria",
        locale,
        { name: "Entradas" },
      ) as string;
      const image = getTranslation(
        "aiMenuOnboarding.reviewEditor.generateAiImageFor",
        locale,
        { name: "Milanesa" },
      ) as string;

      expect(name).toContain("Milanesa");
      expect(description).toContain("Milanesa");
      expect(category).toContain("Entradas");
      expect(image).toContain("Milanesa");
      expect(name).not.toBe("Milanesa");
      expect(image.toLowerCase()).not.toBe("generate ai image");
    },
  );
});
