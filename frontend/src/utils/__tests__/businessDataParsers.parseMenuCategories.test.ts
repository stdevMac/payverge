import { parseMenuCategories } from "../businessDataParsers";
import type { MenuCategory } from "@/api/business";

const cat = (name: string): MenuCategory => ({
  name,
  description: "",
  items: [{ name: "x", description: "", price: 1 as never, is_available: true }],
});

describe("parseMenuCategories", () => {
  it("prefers parsed_categories (translated copy) over categories", () => {
    const translated = [cat("Bebidas")];
    const result = parseMenuCategories({
      categories: [cat("Drinks")],
      parsed_categories: translated,
    });
    expect(result).toBe(translated);
    expect(result[0].name).toBe("Bebidas");
  });

  it("returns an array when categories is already an array and no parsed_categories", () => {
    const arr = [cat("Drinks")];
    expect(parseMenuCategories({ categories: arr })).toBe(arr);
  });

  it("parses a JSON-string categories field into an array", () => {
    const json = JSON.stringify([cat("Drinks")]);
    const result = parseMenuCategories({ categories: json });
    expect(Array.isArray(result)).toBe(true);
    expect(result[0].name).toBe("Drinks");
  });

  it("returns [] for malformed JSON string instead of throwing", () => {
    expect(parseMenuCategories({ categories: "{not json" })).toEqual([]);
  });

  it("returns [] when categories is a JSON string that is not an array", () => {
    expect(parseMenuCategories({ categories: JSON.stringify({ a: 1 }) })).toEqual([]);
  });

  it("returns [] when categories is missing/null/undefined", () => {
    expect(parseMenuCategories({ categories: undefined as never })).toEqual([]);
    expect(parseMenuCategories({ categories: null as never })).toEqual([]);
  });

  it("ignores a non-array parsed_categories and falls back to categories", () => {
    const arr = [cat("Drinks")];
    expect(
      parseMenuCategories({ categories: arr, parsed_categories: {} as never }),
    ).toBe(arr);
  });
});
