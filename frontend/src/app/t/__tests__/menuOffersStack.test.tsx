import fs from "fs";
import path from "path";

/**
 * Task 1.11 — Consolidate offers/bundles stack on /t/{code}/menu.
 *
 * The guest menu page used to render the same bundles twice:
 *   1. As cards in the top "Active Offers + Bundles" strip
 *      (the `availableBundles.map` JSX block inside the section that
 *      gates on `availableOffers.length > 0 || availableBundles.length > 0`).
 *   2. As a synthetic top-level "Bundles" category that `menuDisplayCategories`
 *      prepended to the categories grid rendered by `GuestMenuViews`.
 *
 * We keep (1) — the curated, top-of-page strip — and remove (2).
 *
 * The source-level assertions below are intentionally co-located with
 * the existing `page.ordering.test.ts` static-analysis style, which is
 * how this page is tested today (the component itself is a client tree
 * with dynamic imports + i18n providers + Next 15 params Promises, so a
 * full render test buys little for a deletion fix).
 */
describe("/t/[tableCode]/menu offers stack is not duplicated", () => {
  const source = fs.readFileSync(
    path.resolve(__dirname, "../[tableCode]/menu/page.tsx"),
    "utf8",
  );

  it("still renders the top curated strip with offers and bundles", () => {
    // The single source of truth for the offers/bundles overview lives at
    // the top of the menu content and is gated on either list having items.
    // The strip now renders the search/filter-aware `filteredOffers` /
    // `filteredBundles` (derived from `availableOffers` / `availableBundles`)
    // so the curated overview respects the active search query and filters.
    expect(source).toMatch(
      /\(filteredOffers\.length > 0 \|\| filteredBundles\.length > 0\)/,
    );
    // It iterates `filteredOffers.map(...)` exactly once for the curated list.
    const offersMapMatches = source.match(/filteredOffers\.map\(/g) || [];
    expect(offersMapMatches).toHaveLength(1);
  });

  it("filters Active Offers and bundles through sellable-target helpers (issue 345)", () => {
    expect(source).toContain("filterGuestSellableOffers");
    expect(source).toContain("filterGuestSellableBundles");
    expect(source).toContain("sellableOffers");
    expect(source).toContain("sellableBundles");
    expect(source).toMatch(
      /filteredOffers[\s\S]*?return sellableOffers\.filter/,
    );
  });

  it("does not synthesize a top-level Bundles category in the menu grid", () => {
    // The duplicate path was a synthetic category injected at the head of
    // `menuDisplayCategories` whose `items` came from mapping
    // `availableBundles.filter(...).map(...)` into menu-item-shaped objects.
    // After the fix, bundles are surfaced ONLY in the top curated strip and
    // via `promotion_offers` chips on individual menu items — not as their
    // own category in the categories grid.
    expect(source).not.toMatch(/t\("menu\.bundles\.categoryName"\)/);
    expect(source).not.toMatch(/t\("menu\.bundles\.categoryDescription"\)/);
    // No spread that prepends a bundles category onto the categories list.
    expect(source).not.toMatch(
      /items: bundleItems,[\s\S]{0,40}\},[\s\S]{0,40}\.\.\.categoriesWithOffers/,
    );
  });
});
