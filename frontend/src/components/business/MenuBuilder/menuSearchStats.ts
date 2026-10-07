/**
 * Header search/filter result stats for MenuBuilder.
 *
 * Category count includes empty categories that remain in the filtered list
 * (e.g. a newly created category with no items yet). Previously only categories
 * with items.length > 0 were counted, so creating an empty category did not
 * bump the header category stat [L3-10].
 */
export function countMenuSearchResults(
  filteredMenu: ReadonlyArray<{ items: ReadonlyArray<unknown> }>,
): { totalItems: number; totalCategories: number } {
  let totalItems = 0;
  for (const cat of filteredMenu) {
    totalItems += cat.items.length;
  }
  return {
    totalItems,
    totalCategories: filteredMenu.length,
  };
}
