// Normalize live menu + business data into a deterministic PrintMenuModel.
// Pure: no DOM, no Date.now. Ordering is stable (sort_order); never sort by price.

import type { MenuCategory, MenuItem } from "@/api/business";
import { ALLERGENS, DIETARY_TAGS } from "@/constants/menu-tags";
import type {
  MenuPrintOptions,
  PrintMenuItem,
  PrintMenuModel,
  PrintMenuSection,
  PrintWarning,
} from "./types";

/** Soft-warning threshold: sections with more items than this emit too-many-items. */
export const MAX_ITEMS_SOFT_THRESHOLD = 7;

/** Soft-warning threshold: descriptions longer than this emit long-description. */
export const LONG_DESCRIPTION_CHARS = 140;

export interface PrintBusinessInput {
  name: string;
  logoUrl?: string;
  tagline?: string;
  address?: string;
  customUrl?: string;
  businessType?: string;
  primaryColor?: string;
  secondaryColor?: string;
}

/**
 * Build a print-ready menu model from operator menu data and studio options.
 */
export function normalizeMenuForPrint(
  categories: MenuCategory[],
  business: PrintBusinessInput,
  options: MenuPrintOptions,
): PrintMenuModel {
  const includeUnavailable = options.includeUnavailable === true;
  const selected = options.selectedCategoryIds;

  // Stable category order: sort_order ascending, original index as tie-break.
  // Never sort by price.
  const orderedCategories = categories
    .map((cat, index) => ({ cat, index }))
    .sort((a, b) => {
      const ao = a.cat.sort_order ?? Number.MAX_SAFE_INTEGER;
      const bo = b.cat.sort_order ?? Number.MAX_SAFE_INTEGER;
      if (ao !== bo) return ao - bo;
      return a.index - b.index;
    })
    .map(({ cat }) => cat)
    .filter((cat) => {
      if (selected === "all") return true;
      const id = cat.id;
      if (id === undefined || id === null || id === "") return false;
      return selected.includes(String(id));
    });

  const warnings: PrintWarning[] = [];
  const sections: PrintMenuSection[] = [];
  let currency = "USD";
  let currencyFound = false;

  for (const cat of orderedCategories) {
    const sectionName = trimOrEmpty(cat.name) || "Untitled";
    const sectionId =
      cat.id !== undefined && cat.id !== null && String(cat.id) !== ""
        ? String(cat.id)
        : sectionName;

    const orderedItems = (cat.items ?? [])
      .map((item, index) => ({ item, index }))
      .sort((a, b) => {
        const ao = a.item.sort_order ?? Number.MAX_SAFE_INTEGER;
        const bo = b.item.sort_order ?? Number.MAX_SAFE_INTEGER;
        if (ao !== bo) return ao - bo;
        return a.index - b.index;
      })
      .map(({ item }) => item)
      .filter((item) => includeUnavailable || item.is_available !== false);

    const printItems: PrintMenuItem[] = [];
    let longDescCount = 0;
    let missingDescCount = 0;
    let missingPhotoCount = 0;

    for (const item of orderedItems) {
      const name = trimOrEmpty(item.name);
      if (!name) continue;

      if (!currencyFound && item.currency) {
        const c = trimOrEmpty(item.currency);
        if (c) {
          currency = c.toUpperCase();
          currencyFound = true;
        }
      }

      const rawDescription = collapseBlank(item.description);
      if (rawDescription && rawDescription.length > LONG_DESCRIPTION_CHARS) {
        longDescCount += 1;
      }
      if (!rawDescription) {
        missingDescCount += 1;
      }

      const imageCandidates = options.showImages
        ? resolveImageCandidates(item)
        : [];
      const imageUrl = imageCandidates[0];
      if (options.showImages && !imageUrl) {
        missingPhotoCount += 1;
      }

      printItems.push({
        id:
          item.id !== undefined && item.id !== null && String(item.id) !== ""
            ? String(item.id)
            : name,
        name,
        description: options.showDescriptions ? rawDescription : undefined,
        price: typeof item.price === "number" && Number.isFinite(item.price) ? item.price : 0,
        imageUrl,
        imageCandidates,
        dietaryTags: canonicalizeTagList(item.dietary_tags, DIETARY_TAG_IDS),
        allergens: canonicalizeTagList(item.allergens, ALLERGEN_IDS),
      });
    }

    // Drop empty sections.
    if (printItems.length === 0) continue;

    if (printItems.length > MAX_ITEMS_SOFT_THRESHOLD) {
      warnings.push({
        type: "too-many-items",
        sectionName,
        count: printItems.length,
      });
    }
    if (longDescCount > 0) {
      warnings.push({
        type: "long-description",
        sectionName,
        count: longDescCount,
      });
    }
    if (options.showDescriptions && missingDescCount > 0) {
      warnings.push({
        type: "missing-descriptions",
        sectionName,
        count: missingDescCount,
      });
    }
    if (options.showImages && missingPhotoCount > 0) {
      warnings.push({
        type: "missing-photos",
        sectionName,
        count: missingPhotoCount,
      });
    }

    const sectionDescription = collapseBlank(cat.description);

    sections.push({
      id: sectionId,
      name: sectionName,
      description: sectionDescription,
      items: printItems,
    });
  }

  return {
    business: {
      name: trimOrEmpty(business.name) || "Menu",
      logoUrl: collapseBlank(business.logoUrl),
      tagline: collapseBlank(business.tagline),
      address: collapseBlank(business.address),
      customUrl: collapseBlank(business.customUrl),
      businessType: collapseBlank(business.businessType),
      primaryColor: collapseBlank(business.primaryColor),
      secondaryColor: collapseBlank(business.secondaryColor),
    },
    sections,
    currency,
    language: options.language,
    warnings,
  };
}

function resolveImageCandidates(item: MenuItem): string[] {
  const candidates = [...(item.images ?? []), item.image ?? ""];
  const seen = new Set<string>();
  return candidates.flatMap((value) => {
    const url = collapseBlank(value);
    if (!url || seen.has(url)) return [];
    seen.add(url);
    return [url];
  });
}

/**
 * Fold a free-text tag into a lookup key: case-insensitive, and blind to the
 * separator an operator happened to type ("Gluten Free", "gluten_free",
 * "GLUTEN-FREE" all collapse to the same key).
 */
function tagLookupKey(value: string): string {
  return value
    .toLowerCase()
    .replace(/[\s_-]+/g, " ")
    .trim();
}

/**
 * Common spellings and near-synonyms that operators type by hand, mapped onto
 * the canonical ids the rest of the product stores. Only entries whose meaning
 * is unambiguous belong here — anything unrecognised is passed through so a
 * localized or bespoke tag still prints a marker instead of vanishing.
 */
const TAG_SYNONYMS: Readonly<Record<string, string>> = {
  // Dietary
  "veggie": "vegetarian",
  "veg": "vegetarian",
  "plant based": "vegan",
  "glutenfree": "gluten-free",
  "no gluten": "gluten-free",
  "dairyfree": "dairy-free",
  "no dairy": "dairy-free",
  "lactose free": "dairy-free",
  "nutfree": "nut-free",
  "no nuts": "nut-free",
  // Allergens
  "milk": "dairy",
  "milk products": "dairy",
  "lactose": "dairy",
  "egg": "eggs",
  "wheat": "gluten",
  "tree nut": "treenuts",
  "tree nuts": "treenuts",
  "treenut": "treenuts",
  "nut": "treenuts",
  "nuts": "treenuts",
  "peanuts": "peanut",
  "groundnut": "peanut",
  "groundnuts": "peanut",
  "shellfish": "crustaceans",
  "crustacean": "crustaceans",
  "shrimp": "crustaceans",
  "prawns": "crustaceans",
  "mollusks": "mollusc",
  "mollusk": "mollusc",
  "molluscs": "mollusc",
  "shellfish mollusc": "mollusc",
  "soy": "soya",
  "soya beans": "soya",
  "soybean": "soya",
  "soybeans": "soya",
  "sesame seed": "sesame",
  "sesame seeds": "sesame",
  "sulphites": "so2",
  "sulfites": "so2",
  "sulphur dioxide": "so2",
  "sulfur dioxide": "so2",
  "celeriac": "celery",
  "lupine": "lupin",
  "lupins": "lupin",
  "shell fish": "crustaceans",
};

const buildCanonicalIndex = (
  ids: readonly string[],
): ReadonlyMap<string, string> => {
  const index = new Map<string, string>();
  for (const id of ids) index.set(tagLookupKey(id), id);
  for (const [alias, id] of Object.entries(TAG_SYNONYMS)) {
    if (ids.includes(id)) index.set(tagLookupKey(alias), id);
  }
  return index;
};

// "spicy" predates the current dietary vocabulary but is still stored by some
// menus and still carries a print marker, so it stays canonical here.
const DIETARY_TAG_IDS = buildCanonicalIndex([
  ...DIETARY_TAGS.map(({ id }) => id),
  "spicy",
]);
const ALLERGEN_IDS = buildCanonicalIndex(ALLERGENS.map(({ id }) => id));

/**
 * Trim, canonicalize and de-duplicate a dietary/allergen list. Markers are
 * derived from these strings downstream, so an operator who typed "Milk" and a
 * menu that stores "dairy" must reach the printer as the same tag; unknown
 * values (localized labels, house-specific tags) survive untouched.
 */
function canonicalizeTagList(
  values: string[] | undefined,
  index: ReadonlyMap<string, string>,
): string[] {
  if (!values || values.length === 0) return [];
  const out: string[] = [];
  const seen = new Set<string>();
  for (const value of values) {
    const trimmed = trimOrEmpty(value);
    if (!trimmed) continue;
    const canonical = index.get(tagLookupKey(trimmed)) ?? trimmed;
    if (seen.has(canonical)) continue;
    seen.add(canonical);
    out.push(canonical);
  }
  return out;
}

function trimOrEmpty(value: string | undefined | null): string {
  if (value === undefined || value === null) return "";
  return String(value).trim();
}

/** Blank / whitespace-only strings collapse to undefined. */
function collapseBlank(value: string | undefined | null): string | undefined {
  const t = trimOrEmpty(value);
  return t === "" ? undefined : t;
}
