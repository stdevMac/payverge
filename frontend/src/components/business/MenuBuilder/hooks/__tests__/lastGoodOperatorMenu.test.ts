import { asDollars } from "@/types/money";
import {
  type CachedOperatorMenu,
  clearLastGoodOperatorMenu,
  emptyMenuSurface,
  readLastGoodOperatorMenu,
  rememberLastGoodOperatorMenu,
} from "../lastGoodOperatorMenu";

describe("lastGoodOperatorMenu (#773)", () => {
  afterEach(() => {
    clearLastGoodOperatorMenu();
  });

  it("remembers and restores a catalog across remounts", () => {
    const cached: CachedOperatorMenu = {
      menu: [
        {
          name: "Mains",
          description: "",
          items: [
            {
              id: "1",
              name: "Steak",
              description: "",
              price: asDollars(10),
              is_available: true,
            },
          ],
        },
      ],
      version: 6,
      orderability: {},
    };
    rememberLastGoodOperatorMenu(86, cached);
    expect(readLastGoodOperatorMenu(86)).toEqual(cached);
    expect(readLastGoodOperatorMenu(1)).toBeUndefined();
  });

  it("keeps catalog when categories exist even if the refresh failed", () => {
    expect(emptyMenuSurface({ categoryCount: 6, loadFailed: true })).toBe(
      "catalog",
    );
  });

  it("shows retry — never the empty-catalog CTA — when a first fetch fails", () => {
    expect(emptyMenuSurface({ categoryCount: 0, loadFailed: true })).toBe(
      "retry",
    );
    expect(emptyMenuSurface({ categoryCount: 0, loadFailed: false })).toBe(
      "empty",
    );
  });
});
