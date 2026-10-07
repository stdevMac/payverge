/* eslint-disable no-restricted-syntax -- deterministic print fixture palettes, not Tailwind UI */
import type { PrintMenuItem, PrintMenuModel, PrintMenuSection } from "../types";

interface FixtureItemOptions {
  description?: string;
  imageCandidates?: string[];
  dietaryTags?: string[];
  allergens?: string[];
}

function fixtureItem(
  id: string,
  name: string,
  price: number,
  options: FixtureItemOptions = {},
): PrintMenuItem {
  const imageCandidates = [...(options.imageCandidates ?? [])];
  return {
    id,
    name,
    description: options.description,
    price,
    imageUrl: imageCandidates[0],
    imageCandidates,
    dietaryTags: [...(options.dietaryTags ?? [])],
    allergens: [...(options.allergens ?? [])],
  };
}

function denseSection(
  id: string,
  name: string,
  entries: ReadonlyArray<readonly [string, string, number, string]>,
): PrintMenuSection {
  return {
    id,
    name,
    items: entries.map(([itemId, itemName, price, description]) =>
      fixtureItem(itemId, itemName, price, { description }),
    ),
  };
}

export const sparseRestaurantMenu: PrintMenuModel = {
  business: {
    name: "Sol y Olivo",
    tagline: "Seasonal plates from the coast",
    address: "18 Paseo del Mar, Valencia",
    customUrl: "sol-y-olivo",
    businessType: "restaurant",
    primaryColor: "#315c4c",
    secondaryColor: "#d8a64a",
  },
  sections: [
    {
      id: "small-plates",
      name: "To Begin",
      description: "A few bright plates for the table.",
      items: [
        fixtureItem("marinated-olives", "Citrus-Marinated Olives", 7, {
          description: "Gordal olives, orange peel, rosemary and warm spices.",
          dietaryTags: ["vegan", "gluten-free"],
        }),
        fixtureItem("tomato-toast", "Tomato & Saffron Toast", 12, {
          description: "Charred country bread, grated tomato and saffron oil.",
          dietaryTags: ["vegetarian"],
          allergens: ["gluten"],
        }),
      ],
    },
    {
      id: "mains",
      name: "From the Kitchen",
      items: [
        fixtureItem("sea-bass", "Roasted Sea Bass", 29, {
          description: "Fennel, preserved lemon and almond picada.",
          allergens: ["fish", "treenuts"],
        }),
      ],
    },
  ],
  currency: "EUR",
  language: "en",
  warnings: [],
};

export const denseRestaurantMenu: PrintMenuModel = {
  business: {
    name: "Mercado del Puerto",
    tagline: "Buenos Aires cooking, all day",
    address: "246 Defensa, San Telmo",
    customUrl: "mercado-del-puerto",
    businessType: "restaurant",
    primaryColor: "#6f2f26",
    secondaryColor: "#d6aa62",
  },
  sections: [
    denseSection("small-plates", "Small Plates", [
      [
        "provoleta",
        "Wood-Fired Provoleta",
        11,
        "Oregano, chilli and grilled sourdough.",
      ],
      [
        "empanada-beef",
        "Braised Beef Empanada",
        6,
        "Olive, egg and smoked paprika.",
      ],
      [
        "empanada-corn",
        "Sweet Corn Empanada",
        6,
        "Basil, roasted pepper and mozzarella.",
      ],
      [
        "beet-salad",
        "Ember-Roasted Beets",
        10,
        "Goat cheese, walnuts and sherry dressing.",
      ],
      [
        "calamari",
        "Crisp Calamari",
        14,
        "Parsley, lemon and roasted garlic aioli.",
      ],
      ["chorizo", "House Chorizo", 9, "Salsa criolla and warm bread."],
      [
        "mushrooms",
        "Garlic Field Mushrooms",
        10,
        "Thyme, vermouth and soft polenta.",
      ],
      [
        "croquettes",
        "Smoked Ham Croquettes",
        9,
        "Mustard leaf salad and pickled shallot.",
      ],
    ]),
    denseSection("grill", "From the Parrilla", [
      [
        "skirt-steak",
        "Skirt Steak",
        27,
        "Chimichurri, charred onions and fries.",
      ],
      ["ribeye", "Dry-Aged Ribeye", 39, "Roasted bone marrow and watercress."],
      [
        "short-rib",
        "Glazed Short Rib",
        32,
        "Malbec jus and smoked potato purée.",
      ],
      [
        "half-chicken",
        "Herb-Grilled Chicken",
        24,
        "Lemon, ají amarillo and market greens.",
      ],
      [
        "pork-collar",
        "Pork Collar",
        26,
        "Quince glaze, cabbage and toasted seeds.",
      ],
      [
        "trout",
        "Patagonian Trout",
        28,
        "Brown butter, capers and crushed potatoes.",
      ],
      [
        "cauliflower",
        "Whole Roasted Cauliflower",
        22,
        "Almond cream, raisins and salsa verde.",
      ],
      [
        "mixed-grill",
        "Parrillada for Two",
        58,
        "Steak, sausage, chicken and seasonal vegetables.",
      ],
    ]),
    denseSection("desserts", "Desserts & Cheese", [
      [
        "flan",
        "Dulce de Leche Flan",
        9,
        "Vanilla custard and softly whipped cream.",
      ],
      [
        "chocolate-torte",
        "Dark Chocolate Torte",
        10,
        "Sea salt, olive oil and cocoa nibs.",
      ],
      ["pear", "Malbec-Poached Pear", 9, "Mascarpone and hazelnut praline."],
      ["pavlova", "Citrus Pavlova", 10, "Passion fruit, orange and mint."],
      [
        "pancakes",
        "Dulce de Leche Crêpes",
        11,
        "Caramelized banana and vanilla ice cream.",
      ],
      [
        "cheese",
        "Argentine Cheese Board",
        15,
        "Quince paste, walnuts and seeded crackers.",
      ],
      ["sorbet", "Seasonal Sorbet", 7, "Three scoops made with market fruit."],
      [
        "affogato",
        "Espresso Affogato",
        8,
        "Vanilla bean ice cream and cacao crumble.",
      ],
    ]),
  ],
  currency: "USD",
  language: "en",
  warnings: [
    { type: "too-many-items", sectionName: "Small Plates", count: 8 },
    { type: "too-many-items", sectionName: "From the Parrilla", count: 8 },
    { type: "too-many-items", sectionName: "Desserts & Cheese", count: 8 },
  ],
};

const photoUrl = (fileName: string): string =>
  `https://images.payverge.test/${fileName}`;

export const photoRichRestaurantMenu: PrintMenuModel = {
  business: {
    name: "Juniper & Tide",
    logoUrl: photoUrl("juniper-and-tide-wordmark.svg"),
    tagline: "Garden-led cooking by the water",
    address: "7 Harbour Lane, Portland",
    customUrl: "juniper-and-tide",
    businessType: "fine_dining",
    primaryColor: "#214f47",
    secondaryColor: "#c78b4a",
  },
  sections: [
    {
      id: "garden",
      name: "Garden",
      items: [
        fixtureItem("harvest-bowl", "Harvest Bowl", 18, {
          description:
            "Farro, roasted squash, bitter greens, apple and cider vinaigrette.",
          imageCandidates: [
            "harvest.jpg",
            photoUrl("harvest-bowl-1600x1200.jpg"),
            photoUrl("harvest-bowl-detail-1000x1250.jpg"),
          ],
          dietaryTags: ["vegetarian"],
          allergens: ["gluten"],
        }),
        fixtureItem("charred-carrots", "Charred Carrots", 15, {
          description: "Sunflower tahini, preserved citrus and toasted seeds.",
          imageCandidates: [
            "tiny.jpg",
            photoUrl("charred-carrots-1400x1050.jpg"),
            photoUrl("shared-table-1800x1200.jpg"),
          ],
          dietaryTags: ["vegan", "gluten-free"],
        }),
        fixtureItem("spring-risotto", "Spring Pea Risotto", 24, {
          description: "English peas, mint, pecorino and lemon.",
          imageCandidates: [
            "broken.jpg",
            photoUrl("spring-pea-risotto-1600x1200.jpg"),
            photoUrl("harvest-bowl-detail-1000x1250.jpg"),
          ],
          dietaryTags: ["vegetarian", "gluten-free"],
          allergens: ["dairy"],
        }),
      ],
    },
    {
      id: "coast",
      name: "Coast",
      items: [
        fixtureItem("lemon-trout", "Lemon-Roasted Trout", 31, {
          description: "New potatoes, sea herbs and smoked mussel butter.",
          imageCandidates: [
            photoUrl("lemon-trout-1800x1200.jpg"),
            photoUrl("shared-table-1800x1200.jpg"),
          ],
          allergens: ["fish", "mollusc", "dairy"],
        }),
        fixtureItem("crab-toast", "Brown Crab Toast", 19, {
          description: "Cultured cream, celery leaf and rye toast.",
          imageCandidates: [
            photoUrl("brown-crab-toast-1500x1000.jpg"),
            photoUrl("brown-crab-toast-detail-900x1200.jpg"),
          ],
          allergens: ["crustaceans", "dairy", "gluten"],
        }),
        fixtureItem("olive-oil-cake", "Olive Oil Cake", 12, {
          description: "Meyer lemon curd, crème fraîche and bay leaf sugar.",
          imageCandidates: [
            photoUrl("olive-oil-cake-1400x1050.jpg"),
            photoUrl("brown-crab-toast-detail-900x1200.jpg"),
          ],
          dietaryTags: ["vegetarian"],
          allergens: ["eggs", "dairy", "gluten"],
        }),
      ],
    },
  ],
  currency: "USD",
  language: "en",
  warnings: [],
};

export const rtlRestaurantMenu: PrintMenuModel = {
  business: {
    name: "دار الياسمين",
    tagline: "مطبخ شامي موسمي",
    address: "١٢ شارع الزيتون، عمّان",
    customUrl: "dar-al-yasmin",
    businessType: "restaurant",
    primaryColor: "#244b3b",
    secondaryColor: "#c49a58",
  },
  sections: [
    {
      id: "mezze",
      name: "المقبلات",
      items: [
        // Names and descriptions are localized; dietary/allergen tags stay on
        // the canonical ids the product stores, exactly as production data
        // reaches the planner after normalizeMenuForPrint.
        fixtureItem("hummus", "حمص بالطحينة", 6, {
          description: "حمص ناعم، طحينة، ليمون وزيت زيتون.",
          dietaryTags: ["vegetarian"],
          allergens: ["sesame"],
        }),
        fixtureItem("fattoush", "فتوش موسمي", 8, {
          description: "خس، بندورة، سماق وخبز محمص.",
          dietaryTags: ["vegetarian"],
          allergens: ["gluten"],
        }),
      ],
    },
    {
      id: "mains",
      name: "الأطباق الرئيسية",
      items: [
        fixtureItem("lamb", "كتف غنم مطهو ببطء", 24, {
          description: "فريكة، لوز محمص وصلصة الرمان.",
          allergens: ["treenuts", "gluten"],
        }),
        fixtureItem("maqluba", "مقلوبة الباذنجان", 18, {
          description: "أرز بسمتي، باذنجان، لبن بالنعناع.",
          dietaryTags: ["vegetarian"],
          allergens: ["dairy"],
        }),
      ],
    },
  ],
  currency: "JOD",
  language: "ar",
  warnings: [],
};

export const hebrewRestaurantMenu: PrintMenuModel = {
  business: {
    name: "בית הזית",
    tagline: "מטבח עונתי מקומי",
    address: "רחוב הגן 12, תל אביב",
    customUrl: "beit-hazayit",
    businessType: "restaurant",
    primaryColor: "#29493f",
    secondaryColor: "#b98552",
  },
  sections: [
    {
      id: "starters",
      name: "מנות ראשונות",
      items: [
        fixtureItem("eggplant", "חציל קלוי", 42, {
          description: "טחינה גולמית, עגבניות צלויות ושמן זית.",
          dietaryTags: ["vegan"],
          allergens: ["sesame"],
        }),
        fixtureItem("beets", "סלקים מהשדה", 46, {
          description: "לבנה, פיסטוק, עשבי תיבול והדרים.",
          dietaryTags: ["vegetarian"],
          allergens: ["dairy", "treenuts"],
        }),
      ],
    },
    {
      id: "mains",
      name: "עיקריות",
      items: [
        fixtureItem("fish", "דג ים צלוי", 118, {
          description: "תפוחי אדמה, עלים ירוקים וחמאת לימון.",
          allergens: ["fish", "dairy"],
        }),
        fixtureItem("cauliflower", "כרובית שלמה", 76, {
          description: "קרם שקדים, צימוקים, צלפים ועשבי תיבול.",
          dietaryTags: ["vegan"],
          allergens: ["treenuts"],
        }),
      ],
    },
  ],
  currency: "ILS",
  language: "he",
  warnings: [],
};

export const cjkRestaurantMenu: PrintMenuModel = {
  business: {
    name: "木漏れ日食堂",
    tagline: "旬の素材を、ていねいに",
    address: "東京都渋谷区青葉町 4-12",
    customUrl: "komorebi-shokudo",
    businessType: "restaurant",
    primaryColor: "#33483b",
    secondaryColor: "#b06d3b",
  },
  sections: [
    {
      id: "small-dishes",
      name: "小さな料理",
      items: [
        fixtureItem("tofu", "胡麻豆腐", 780, {
          description: "白胡麻、山葵、出汁醤油。",
          dietaryTags: ["vegetarian"],
          allergens: ["sesame", "soya"],
        }),
        fixtureItem("eggplant", "焼き茄子", 920, {
          description: "生姜、茗荷、鰹節。",
          allergens: ["fish"],
        }),
        fixtureItem("sashimi", "季節のお造り", 1680, {
          description: "本日の鮮魚三種、土佐醤油。",
          allergens: ["fish", "soya"],
        }),
      ],
    },
    {
      id: "rice-noodles",
      name: "ご飯と麺",
      items: [
        fixtureItem("salmon-rice", "鮭といくらの土鍋ご飯", 2480, {
          description: "北海道産鮭、いくら、三つ葉。",
          allergens: ["fish"],
        }),
        // "そば" (buckwheat) has no canonical id in menu-tags.ts, so it stays
        // verbatim: unknown tags must still reach print as a marker.
        fixtureItem("duck-soba", "鴨南蛮そば", 1580, {
          description: "炙り鴨、焼き葱、柚子。",
          allergens: ["そば", "gluten", "soya"],
        }),
      ],
    },
  ],
  currency: "JPY",
  language: "ja",
  warnings: [],
};
