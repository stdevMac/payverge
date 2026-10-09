/** @jest-environment node */

import {
  formatStorefrontSeoPrice,
  normalizeStorefrontMenuPayload,
} from "./menuPayload";

describe("normalizeStorefrontMenuPayload", () => {
  it("prefers parsed_categories over the raw categories field", () => {
    const result = normalizeStorefrontMenuPayload({
      parsed_categories: [
        { name: "Pizzas", description: "", items: [{ name: "Margherita" }] },
      ],
      categories: JSON.stringify([{ name: "Stale", items: [] }]),
    });
    expect(result.categories).toHaveLength(1);
    expect(result.categories[0].name).toBe("Pizzas");
  });

  it("parses a JSON-string categories field", () => {
    const result = normalizeStorefrontMenuPayload({
      categories: JSON.stringify([
        { name: "Mains", description: "", items: [] },
      ]),
    });
    expect(result.categories[0].name).toBe("Mains");
  });

  it("reads item_orderability from the top level, then from menu, then {}", () => {
    const direct = normalizeStorefrontMenuPayload({
      parsed_categories: [],
      item_orderability: { steak: { state: "available", orderable: true } },
      menu: { item_orderability: { steak: { state: "inventory_out" } } },
    });
    expect(direct.itemOrderability.steak).toEqual({
      state: "available",
      orderable: true,
    });

    const nested = normalizeStorefrontMenuPayload({
      parsed_categories: [],
      menu: { item_orderability: { steak: { state: "inventory_out" } } },
    });
    expect(nested.itemOrderability.steak).toEqual({ state: "inventory_out" });

    const none = normalizeStorefrontMenuPayload({ parsed_categories: [] });
    expect(none.itemOrderability).toEqual({});
  });

  it("defaults offers/bundles to [] and tolerates garbage input", () => {
    const empty = normalizeStorefrontMenuPayload(null);
    expect(empty).toEqual({
      categories: [],
      offers: [],
      bundles: [],
      itemOrderability: {},
    });

    const garbage = normalizeStorefrontMenuPayload({
      parsed_categories: "not-an-array",
      offers: "nope",
      bundles: 42,
    });
    expect(garbage.categories).toEqual([]);
    expect(garbage.offers).toEqual([]);
    expect(garbage.bundles).toEqual([]);
  });

  it("throws on a malformed categories JSON string (mirrors client behavior)", () => {
    expect(() =>
      normalizeStorefrontMenuPayload({ categories: "{not json" }),
    ).toThrow();
  });
});

describe("formatStorefrontSeoPrice", () => {
  it("formats a numeric price with a valid currency", () => {
    const formatted = formatStorefrontSeoPrice(12.5, "usd");
    expect(formatted).toContain("12.50");
  });

  it("falls back to the bare number for an invalid currency code", () => {
    expect(formatStorefrontSeoPrice(12.5, "not-a-code")).toBe("12.5");
  });

  it("renders the bare number when no currency is known", () => {
    expect(formatStorefrontSeoPrice(8, undefined)).toBe("8");
  });

  it("returns null for non-numeric or non-finite prices", () => {
    expect(formatStorefrontSeoPrice("12.5", "USD")).toBeNull();
    expect(formatStorefrontSeoPrice(Number.NaN, "USD")).toBeNull();
    expect(formatStorefrontSeoPrice(undefined, "USD")).toBeNull();
  });
});
