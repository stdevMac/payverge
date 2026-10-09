import { countMenuSearchResults } from "../menuSearchStats";

describe("countMenuSearchResults [L3-10]", () => {
  it("counts empty categories toward totalCategories", () => {
    const stats = countMenuSearchResults([
      { items: [{ id: 1 }, { id: 2 }] },
      { items: [] }, // newly created empty category
      { items: [{ id: 3 }] },
    ]);
    expect(stats.totalCategories).toBe(3);
    expect(stats.totalItems).toBe(3);
  });

  it("returns zeros for an empty menu", () => {
    expect(countMenuSearchResults([])).toEqual({
      totalItems: 0,
      totalCategories: 0,
    });
  });

  it("counts a menu of only empty categories", () => {
    expect(
      countMenuSearchResults([{ items: [] }, { items: [] }]),
    ).toEqual({ totalItems: 0, totalCategories: 2 });
  });
});
