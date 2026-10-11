import type { MenuCategory, MenuItem } from "@/api/business";
import { asDollars } from "@/types/money";
import { photoRichRestaurantMenu } from "./__fixtures__/restaurantMenus";
import {
  LONG_DESCRIPTION_CHARS,
  MAX_ITEMS_SOFT_THRESHOLD,
  normalizeMenuForPrint,
  type PrintBusinessInput,
} from "./normalize";
import type { MenuPrintOptions } from "./types";

function item(
  overrides: Partial<Omit<MenuItem, "price">> & {
    name: string;
    price?: number;
  },
): MenuItem {
  const { price, ...rest } = overrides;
  return {
    description: "",
    is_available: true,
    ...rest,
    price: asDollars(price ?? 10),
  };
}

function category(
  overrides: Partial<MenuCategory> & { name: string; items: MenuItem[] },
): MenuCategory {
  return {
    description: "",
    ...overrides,
  };
}

const business: PrintBusinessInput = {
  name: "  Casa Verde  ",
  logoUrl: "https://cdn.example/logo.png",
  tagline: "  Farm to table  ",
  address: "  1 Main St  ",
  customUrl: "  casa-verde  ",
};

function options(overrides: Partial<MenuPrintOptions> = {}): MenuPrintOptions {
  return {
    paperFormat: "letter",
    language: "en",
    showDescriptions: true,
    showImages: true,
    showTags: false,
    currencySymbol: false,
    includeQr: false,
    selectedCategoryIds: "all",
    includeUnavailable: false,
    ...overrides,
  };
}

describe("normalizeMenuForPrint", () => {
  it("keeps the photo audit fixtures aligned with their measurement keys", () => {
    const items = photoRichRestaurantMenu.sections.flatMap(
      (section) => section.items,
    );
    const candidates = items.flatMap(
      (menuItem) => menuItem.imageCandidates ?? [],
    );

    expect(candidates).toEqual(
      expect.arrayContaining(["harvest.jpg", "tiny.jpg", "broken.jpg"]),
    );
    expect(candidates.length).toBeGreaterThan(new Set(candidates).size);
    expect(
      items.every(
        (menuItem) => menuItem.imageUrl === menuItem.imageCandidates?.[0],
      ),
    ).toBe(true);
  });

  it("preserves deduplicated image candidates and restaurant identity", () => {
    const model = normalizeMenuForPrint(
      [
        category({
          id: "mains",
          name: "Mains",
          items: [
            item({
              id: "dish",
              name: "Catch of the day",
              image: " hero.jpg ",
              images: ["hero.jpg", "detail.jpg", " ", "detail.jpg"],
            }),
          ],
        }),
      ],
      {
        name: "Mar",
        businessType: "fine_dining",
        primaryColor: "#123456",
        secondaryColor: "#abcdef",
      },
      options(),
    );

    expect(model.sections[0].items[0].imageCandidates).toEqual([
      "hero.jpg",
      "detail.jpg",
    ]);
    expect(model.business).toEqual({
      name: "Mar",
      logoUrl: undefined,
      tagline: undefined,
      address: undefined,
      customUrl: undefined,
      businessType: "fine_dining",
      primaryColor: "#123456",
      secondaryColor: "#abcdef",
    });
  });

  it("trims business fields and collapses blanks", () => {
    const model = normalizeMenuForPrint(
      [
        category({
          id: "c1",
          name: "Mains",
          items: [item({ id: "i1", name: "Steak", price: 24 })],
        }),
      ],
      {
        name: "  Bistro  ",
        logoUrl: "   ",
        tagline: "",
        address: "  \t  ",
        customUrl: " slug ",
        businessType: "  fine_dining  ",
        primaryColor: "   ",
        secondaryColor: "  #abcdef  ",
      },
      options(),
    );
    expect(model.business).toEqual({
      name: "Bistro",
      logoUrl: undefined,
      tagline: undefined,
      address: undefined,
      customUrl: "slug",
      businessType: "fine_dining",
      primaryColor: undefined,
      secondaryColor: "#abcdef",
    });
  });

  it("filters unavailable items by default", () => {
    const model = normalizeMenuForPrint(
      [
        category({
          id: "c1",
          name: "Mains",
          items: [
            item({ id: "a", name: "Soup", is_available: true, price: 8 }),
            item({ id: "b", name: "Sold out", is_available: false, price: 12 }),
          ],
        }),
      ],
      business,
      options(),
    );
    expect(model.sections[0].items.map((i) => i.name)).toEqual(["Soup"]);
  });

  it("includes unavailable items when includeUnavailable is true", () => {
    const model = normalizeMenuForPrint(
      [
        category({
          id: "c1",
          name: "Mains",
          items: [
            item({ id: "a", name: "Soup", is_available: true }),
            item({ id: "b", name: "Sold out", is_available: false }),
          ],
        }),
      ],
      business,
      options({ includeUnavailable: true }),
    );
    expect(model.sections[0].items.map((i) => i.name)).toEqual([
      "Soup",
      "Sold out",
    ]);
  });

  it("filters to selectedCategoryIds when not all", () => {
    const model = normalizeMenuForPrint(
      [
        category({
          id: "starters",
          name: "Starters",
          items: [item({ id: "1", name: "Bread" })],
        }),
        category({
          id: "mains",
          name: "Mains",
          items: [item({ id: "2", name: "Fish" })],
        }),
        category({
          id: "desserts",
          name: "Desserts",
          items: [item({ id: "3", name: "Cake" })],
        }),
      ],
      business,
      options({ selectedCategoryIds: ["mains", "desserts"] }),
    );
    expect(model.sections.map((s) => s.id)).toEqual(["mains", "desserts"]);
  });

  it("preserves sort_order for categories and items; never sorts by price", () => {
    const model = normalizeMenuForPrint(
      [
        category({
          id: "b",
          name: "Later cat",
          sort_order: 2,
          items: [
            item({ id: "x", name: "Expensive", price: 99, sort_order: 2 }),
            item({ id: "y", name: "Cheap", price: 5, sort_order: 1 }),
            item({ id: "z", name: "Mid", price: 20, sort_order: 3 }),
          ],
        }),
        category({
          id: "a",
          name: "First cat",
          sort_order: 1,
          items: [item({ id: "1", name: "Only", price: 1 })],
        }),
      ],
      business,
      options(),
    );
    expect(model.sections.map((s) => s.name)).toEqual([
      "First cat",
      "Later cat",
    ]);
    expect(model.sections[1].items.map((i) => i.name)).toEqual([
      "Cheap",
      "Expensive",
      "Mid",
    ]);
    // Explicit: not price-sorted
    expect(model.sections[1].items.map((i) => i.price)).toEqual([5, 99, 20]);
  });

  it("uses original order when sort_order is missing", () => {
    const model = normalizeMenuForPrint(
      [
        category({
          id: "c",
          name: "List",
          items: [
            item({ id: "1", name: "First", price: 30 }),
            item({ id: "2", name: "Second", price: 10 }),
            item({ id: "3", name: "Third", price: 20 }),
          ],
        }),
      ],
      business,
      options(),
    );
    expect(model.sections[0].items.map((i) => i.name)).toEqual([
      "First",
      "Second",
      "Third",
    ]);
  });

  it("collapses blank/whitespace descriptions to undefined and trims strings", () => {
    const model = normalizeMenuForPrint(
      [
        category({
          id: "c",
          name: "  Drinks  ",
          description: "   ",
          items: [
            item({
              id: "1",
              name: "  Latte  ",
              description: "  \n  ",
              price: 4.5,
              dietary_tags: ["  vegan  ", "", "  "],
              allergens: [" milk "],
            }),
            item({
              id: "2",
              name: "Espresso",
              description: "  Double shot  ",
              price: 3,
            }),
          ],
        }),
      ],
      business,
      options({ showDescriptions: true }),
    );
    const section = model.sections[0];
    expect(section.name).toBe("Drinks");
    expect(section.description).toBeUndefined();
    expect(section.items[0].name).toBe("Latte");
    expect(section.items[0].description).toBeUndefined();
    expect(section.items[0].dietaryTags).toEqual(["vegan"]);
    // " milk " is a hand-typed spelling of the canonical "dairy" allergen.
    expect(section.items[0].allergens).toEqual(["dairy"]);
    expect(section.items[1].description).toBe("Double shot");
  });

  it("maps hand-typed tag spellings onto the canonical menu-tag ids", () => {
    // Print markers are keyed off the ids in constants/menu-tags.ts. Operators
    // type whatever the kitchen says, and the same allergen arriving as
    // "Shellfish" on one dish and "crustaceans" on the next used to print two
    // different marks for one thing.
    const model = normalizeMenuForPrint(
      [
        category({
          id: "c",
          name: "Plates",
          items: [
            item({
              id: "1",
              name: "Ceviche",
              price: 12,
              dietary_tags: ["Veggie", "GLUTEN FREE"],
              allergens: ["Shellfish", "Sulphites", "tree nuts", "Milk"],
            }),
            item({
              id: "2",
              name: "Soba",
              price: 9,
              // Deduped to one marker even though the operator listed both.
              allergens: ["soy", "soya", "そば"],
            }),
          ],
        }),
      ],
      business,
      options(),
    );
    const [first, second] = model.sections[0].items;
    expect(first.dietaryTags).toEqual(["vegetarian", "gluten-free"]);
    expect(first.allergens).toEqual([
      "crustaceans",
      "so2",
      "treenuts",
      "dairy",
    ]);
    // An id the registry does not know still has to reach the printed sheet.
    expect(second.allergens).toEqual(["soya", "そば"]);
  });

  it("omits item descriptions when showDescriptions is false", () => {
    const model = normalizeMenuForPrint(
      [
        category({
          id: "c",
          name: "Mains",
          items: [item({ id: "1", name: "Steak", description: "Ribeye" })],
        }),
      ],
      business,
      options({ showDescriptions: false }),
    );
    expect(model.sections[0].items[0].description).toBeUndefined();
    // No missing-descriptions warning when descriptions are intentionally hidden
    expect(
      model.warnings.find((w) => w.type === "missing-descriptions"),
    ).toBeUndefined();
  });

  it("imageUrl prefers images[] then image; ignored when showImages is false", () => {
    const withBoth = normalizeMenuForPrint(
      [
        category({
          id: "c",
          name: "Mains",
          items: [
            item({
              id: "1",
              name: "A",
              image: "legacy.jpg",
              images: ["first.jpg", "second.jpg"],
            }),
            item({ id: "2", name: "B", image: "only-legacy.jpg" }),
            item({ id: "3", name: "C", images: ["  ", "  mid.jpg  "] }),
            item({ id: "4", name: "D" }),
          ],
        }),
      ],
      business,
      options({ showImages: true }),
    );
    expect(withBoth.sections[0].items.map((i) => i.imageUrl)).toEqual([
      "first.jpg",
      "only-legacy.jpg",
      "mid.jpg",
      undefined,
    ]);
    expect(withBoth.sections[0].items.map((i) => i.imageCandidates)).toEqual([
      ["first.jpg", "second.jpg", "legacy.jpg"],
      ["only-legacy.jpg"],
      ["mid.jpg"],
      [],
    ]);

    const noImages = normalizeMenuForPrint(
      [
        category({
          id: "c",
          name: "Mains",
          items: [
            item({
              id: "1",
              name: "A",
              image: "legacy.jpg",
              images: ["first.jpg"],
            }),
          ],
        }),
      ],
      business,
      options({ showImages: false }),
    );
    expect(noImages.sections[0].items[0].imageUrl).toBeUndefined();
    expect(noImages.sections[0].items[0].imageCandidates).toEqual([]);
    expect(
      noImages.warnings.find((w) => w.type === "missing-photos"),
    ).toBeUndefined();
  });

  it("drops sections that end up with zero items", () => {
    const model = normalizeMenuForPrint(
      [
        category({
          id: "empty",
          name: "Empty after filter",
          items: [item({ id: "1", name: "Gone", is_available: false })],
        }),
        category({
          id: "ok",
          name: "Still here",
          items: [item({ id: "2", name: "Dish" })],
        }),
        category({ id: "blank", name: "No items", items: [] }),
      ],
      business,
      options(),
    );
    expect(model.sections.map((s) => s.id)).toEqual(["ok"]);
  });

  it("emits too-many-items when section exceeds soft threshold", () => {
    const items = Array.from({ length: MAX_ITEMS_SOFT_THRESHOLD + 1 }, (_, i) =>
      item({ id: `i${i}`, name: `Item ${i}`, description: "ok" }),
    );
    const model = normalizeMenuForPrint(
      [category({ id: "big", name: "Mains", items })],
      business,
      options({ showDescriptions: true, showImages: false }),
    );
    expect(model.warnings).toContainEqual({
      type: "too-many-items",
      sectionName: "Mains",
      count: MAX_ITEMS_SOFT_THRESHOLD + 1,
    });
  });

  it("does not emit too-many-items at exactly the threshold", () => {
    const items = Array.from({ length: MAX_ITEMS_SOFT_THRESHOLD }, (_, i) =>
      item({ id: `i${i}`, name: `Item ${i}`, description: "ok" }),
    );
    const model = normalizeMenuForPrint(
      [category({ id: "ok", name: "Mains", items })],
      business,
      options({ showImages: false }),
    );
    expect(
      model.warnings.find((w) => w.type === "too-many-items"),
    ).toBeUndefined();
  });

  it("emits long-description count for descriptions over 140 chars", () => {
    const long = "x".repeat(LONG_DESCRIPTION_CHARS + 1);
    const model = normalizeMenuForPrint(
      [
        category({
          id: "c",
          name: "Mains",
          items: [
            item({ id: "1", name: "A", description: long }),
            item({ id: "2", name: "B", description: "short" }),
            item({ id: "3", name: "C", description: long }),
          ],
        }),
      ],
      business,
      options({ showImages: false }),
    );
    expect(model.warnings).toContainEqual({
      type: "long-description",
      sectionName: "Mains",
      count: 2,
    });
  });

  it("emits missing-descriptions and missing-photos counts when relevant", () => {
    const model = normalizeMenuForPrint(
      [
        category({
          id: "c",
          name: "Starters",
          items: [
            item({
              id: "1",
              name: "A",
              description: "has text",
              image: "a.jpg",
            }),
            item({ id: "2", name: "B", description: "", images: [] }),
            item({ id: "3", name: "C", description: "   " }),
          ],
        }),
      ],
      business,
      options({ showDescriptions: true, showImages: true }),
    );
    expect(model.warnings).toContainEqual({
      type: "missing-descriptions",
      sectionName: "Starters",
      count: 2,
    });
    expect(model.warnings).toContainEqual({
      type: "missing-photos",
      sectionName: "Starters",
      count: 2,
    });
  });

  it("sets currency from first item with currency; language from options", () => {
    const model = normalizeMenuForPrint(
      [
        category({
          id: "c",
          name: "Mains",
          items: [
            item({ id: "1", name: "A", price: 10 }),
            item({ id: "2", name: "B", price: 12, currency: "eur" }),
          ],
        }),
      ],
      business,
      options({ language: "es" }),
    );
    expect(model.currency).toBe("EUR");
    expect(model.language).toBe("es");
  });

  it("defaults currency to USD when no item currency is set", () => {
    const model = normalizeMenuForPrint(
      [
        category({
          id: "c",
          name: "Mains",
          items: [item({ id: "1", name: "A" })],
        }),
      ],
      business,
      options(),
    );
    expect(model.currency).toBe("USD");
  });

  it("keeps price as dollars (no re-division)", () => {
    const model = normalizeMenuForPrint(
      [
        category({
          id: "c",
          name: "Mains",
          items: [item({ id: "1", name: "Steak", price: 24.5 })],
        }),
      ],
      business,
      options(),
    );
    expect(model.sections[0].items[0].price).toBe(24.5);
  });

  it("is deterministic for the same inputs", () => {
    const cats = [
      category({
        id: "c",
        name: "Mains",
        sort_order: 1,
        items: [
          item({ id: "2", name: "B", sort_order: 2, price: 20 }),
          item({ id: "1", name: "A", sort_order: 1, price: 10 }),
        ],
      }),
    ];
    const a = normalizeMenuForPrint(cats, business, options());
    const b = normalizeMenuForPrint(cats, business, options());
    expect(a).toEqual(b);
  });

  it("passes through trimmed business branding from the happy path", () => {
    const model = normalizeMenuForPrint(
      [
        category({
          id: "c",
          name: "Mains",
          items: [item({ id: "1", name: "Dish" })],
        }),
      ],
      business,
      options(),
    );
    expect(model.business.name).toBe("Casa Verde");
    expect(model.business.logoUrl).toBe("https://cdn.example/logo.png");
    expect(model.business.tagline).toBe("Farm to table");
    expect(model.business.address).toBe("1 Main St");
    expect(model.business.customUrl).toBe("casa-verde");
  });
});
